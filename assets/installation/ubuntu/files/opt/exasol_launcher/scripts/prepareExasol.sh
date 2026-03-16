#!/usr/bin/env bash
# prepareExasol.sh - Prepare compute instance for Exasol deployment
#
# This script performs the following tasks:
# 1. Downloads the c4 binary (Exasol installer tool)
# 2. Configures udev rules for Exasol data disk permissions
# 3. Runs c4 preplay to prepare the system environment
# 4. Installs the c4 binary to the ubuntu user's home directory
#
# This script runs during instance initialization and must be executed as root.

set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"
# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

c4_version='4.29.0'

log_step_info "Preparing system..."

log_substep_info "Downloading c4 ...${c4_version}"

if ! curl -fsSL -O "https://x-up.s3.amazonaws.com/releases/c4/linux/x86_64/${c4_version}/c4"; then
  log_error "Failed to download c4 binary from remote server"
  exit 1
fi

chmod +x c4

log_substep_info "Setting up disk permissions"

final_disk_path="$(node_jq -er '.hostDatadisk')"
final_disk_name="$(basename "${final_disk_path}")"

# The infrastructure preset injects a hostDatadiskMatch object with one of two shapes:
#   1. { "udevKey": "...", "udevValue": "..." }  — deterministic match known at plan time
#   2. { "discoveryPaths": ["/dev/..."] }        — runtime probe required
match_key="$(node_jq -r '.hostDatadiskMatch.udevKey // empty')"
match_value="$(node_jq -r '.hostDatadiskMatch.udevValue // empty')"

if [[ -n "${match_key}" && -n "${match_value}" ]]; then
  log_substep_info "Using pre-resolved udev match: ${match_key}=${match_value}"
else
  # Runtime discovery: probe candidate paths until the disk appears.
  readarray -t discovery_paths < <(node_jq -er '.hostDatadiskMatch.discoveryPaths[]')

  if [[ ${#discovery_paths[@]} -eq 0 ]]; then
    log_error "No udev match and no discovery paths configured in node metadata"
    exit 1
  fi

  log_substep_info "Probing disk discovery paths: ${discovery_paths[*]}"

  disk_link=""
  for attempt in {30..0}; do
    for candidate in "${discovery_paths[@]}"; do
      if [[ -e "${candidate}" ]]; then
        disk_link="${candidate}"
        break 2
      fi
    done

    if [[ "${attempt}" -eq 0 ]]; then
      log_error "Data disk did not appear at any of: ${discovery_paths[*]}"
      exit 1
    fi
    sleep 1
  done

  # Resolve symlink to the real block device.
  data_disk_device="$(readlink -f "${disk_link}")"
  if [[ -z "${data_disk_device}" || ! -b "${data_disk_device}" ]]; then
    log_error "Resolved disk device ${data_disk_device:-<empty>} is not a valid block device"
    exit 1
  fi

  log_substep_info "Resolved ${disk_link} -> ${data_disk_device}"

  # Find a stable udev property for a persistent rule.
  udev_props="$(udevadm info --query=property --name="${data_disk_device}")"

  match_key=""
  match_value=""
  for key in ID_SERIAL_SHORT ID_PATH ID_SERIAL; do
    value="$(awk -F= -v k="${key}" '$1 == k { print $2 }' <<<"${udev_props}")"
    if [[ -n "${value}" ]]; then
      match_key="${key}"
      match_value="${value}"
      break
    fi
  done

  if [[ -z "${match_key}" ]]; then
    log_error "Could not determine stable udev property for disk ${data_disk_device}"
    exit 1
  fi

  log_substep_info "Detected udev match: ${match_key}=${match_value}"
fi

cat <<EOF | tee /etc/udev/rules.d/90-exasol.rules
SUBSYSTEM=="block", ENV{${match_key}}=="${match_value}", OWNER="ubuntu", MODE="0660"
SUBSYSTEM=="block", ENV{${match_key}}=="${match_value}", SYMLINK+="${final_disk_name}", MODE="0660"
EOF

udevadm control --reload-rules
udevadm trigger

log_substep_info "Running c4 preplay"

# Sometimes fails with
#   user@1000.service: Failed to attach to cgroup /user.slice/user-1000.slice/user@1000.service: Device or resource busy
#   user@1000.service: Failed at step CGROUP spawning /lib/systemd/systemd: Device or resource busy
#   user@1000.service: Main process exited, code=exited, status=219/CGROUP
# Maybe https://github.com/systemd/systemd/issues/23164
for attempt in {10..0}; do
  ./c4 _ preplay ubuntu && break
  if [[ "${attempt}" -eq 0 ]]; then
    log_error "Failed to prepare system"
    exit 1
  fi
done

log_substep_info "Moving c4 binary to user home"

mv ./c4 ~ubuntu/c4
chown ubuntu: ~ubuntu/c4

log_step_info "System preparation completed"

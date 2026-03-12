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
disk_source_type="$(node_jq -er '.hostDatadiskSourceType')"
disk_source_value="$(node_jq -er '.hostDatadiskSource')"

case "${disk_source_type}" in
  aws-ebs-volume-id)
    data_volume_id="${disk_source_value//-/}"
    cat <<EOF | tee /etc/udev/rules.d/90-exasol.rules
SUBSYSTEM=="block", ENV{ID_SERIAL_SHORT}=="${data_volume_id}", OWNER="ubuntu", MODE="0660"
SUBSYSTEM=="block", ENV{ID_SERIAL_SHORT}=="${data_volume_id}", SYMLINK+="${final_disk_name}", MODE="0660"
EOF
    ;;

  azure-lun)
    azure_lun="${disk_source_value}"
    azure_disk_link=""

    # Azure exposes attached data disks via LUN-based symlinks, but the exact
    # directory can differ across images/udev setups. Probe the known Azure
    # LUN paths and wait briefly for the device link to appear before resolving
    # it to the backing block device.
    for attempt in {30..0}; do
      for candidate in \
        "/dev/disk/azure/scsi1/lun${azure_lun}" \
        "/dev/disk/azure/data/by-lun/${azure_lun}"
      do
        if [[ -e "${candidate}" ]]; then
          azure_disk_link="${candidate}"
          break 2
        fi
      done

      if [[ "${attempt}" -eq 0 ]]; then
        log_error "Azure data disk for LUN ${azure_lun} did not appear"
        exit 1
      fi
      sleep 1
    done

    data_disk_device="$(readlink -f "${azure_disk_link}")"
    if [[ -z "${data_disk_device}" || ! -b "${data_disk_device}" ]]; then
      log_error "Resolved Azure data disk device ${data_disk_device:-<empty>} is invalid"
      exit 1
    fi

    udev_props="$(udevadm info --query=property --name="${data_disk_device}")"

    azure_match_key=""
    azure_match_value=""

    # Prefer a persistent udev attribute over transient kernel names such as sdc.
    # We try the most useful identifiers in order and use the first one available
    # to build a stable rule for the Azure data disk.
    for key in ID_PATH ID_SERIAL_SHORT ID_SERIAL; do
      value="$(awk -F= -v k="${key}" '$1 == k { print $2 }' <<<"${udev_props}")"
      if [[ -n "${value}" ]]; then
        azure_match_key="${key}"
        azure_match_value="${value}"
        break
      fi
    done

    if [[ -z "${azure_match_key}" || -z "${azure_match_value}" ]]; then
      log_error "Could not determine stable udev property for Azure disk ${data_disk_device}"
      exit 1
    fi

    cat <<EOF | tee /etc/udev/rules.d/90-exasol.rules
SUBSYSTEM=="block", ENV{DEVTYPE}=="disk", ENV{${azure_match_key}}=="${azure_match_value}", OWNER="ubuntu", MODE="0660"
SUBSYSTEM=="block", ENV{DEVTYPE}=="disk", ENV{${azure_match_key}}=="${azure_match_value}", SYMLINK+="${final_disk_name}", MODE="0660"
EOF
    ;;

  *)
    log_error "Unsupported hostDatadiskSourceType: ${disk_source_type}"
    exit 1
    ;;
esac

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

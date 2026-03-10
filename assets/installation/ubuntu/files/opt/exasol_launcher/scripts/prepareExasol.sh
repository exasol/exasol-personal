#!/usr/bin/env bash
# prepareExasol.sh - Prepare compute instance for Exasol deployment
#
# This script performs the following tasks:
# 1. Downloads the c4 binary (Exasol installer tool)
# 2. Runs c4 preplay to prepare the system environment
# 3. Installs the c4 binary to the ubuntu user's home directory
#
# Note: Data disk permissions are configured by infrastructure-specific
# preInstall hooks (see infrastructure preset files).
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
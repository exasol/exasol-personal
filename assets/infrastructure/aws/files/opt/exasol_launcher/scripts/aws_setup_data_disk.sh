#!/usr/bin/env bash
# AWS-specific data disk setup
# Configures udev rules for EBS volume access

set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"
# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

log_substep_info "Setting up AWS EBS data disk permissions"

data_volume_id="$(node_jq -er '.hostDatadisk')"
cat <<EOF > /etc/udev/rules.d/90-exasol.rules
# AWS EBS volumes: match on ID_SERIAL_SHORT with hyphen stripped (vol-xxxxx -> volxxxxx)
SUBSYSTEM=="block", ENV{ID_SERIAL_SHORT}=="${data_volume_id/-/}", OWNER="ubuntu", MODE="0660", SYMLINK+="exasol_data_01"
EOF
udevadm control --reload-rules
udevadm trigger

log_substep_info "AWS EBS data disk configured"

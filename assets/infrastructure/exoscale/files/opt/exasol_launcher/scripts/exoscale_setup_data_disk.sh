#!/usr/bin/env bash
# Exoscale-specific data disk setup
# Configures udev rules for block storage volume access

set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"
# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

log_substep_info "Setting up Exoscale block storage data disk permissions"

data_volume_id="$(node_jq -er '.hostDatadisk')"
# Truncate to first 20 characters (virtio truncates UUIDs in serial to 20 chars)
data_volume_id_short="${data_volume_id:0:20}"
cat <<EOF > /etc/udev/rules.d/90-exasol.rules
# Exoscale block storage: match on ID_SERIAL (virtio truncates UUIDs to 20 chars)
SUBSYSTEM=="block", ENV{ID_SERIAL}=="${data_volume_id_short}", OWNER="ubuntu", MODE="0660", SYMLINK+="exasol_data_01"
EOF
udevadm control --reload-rules
udevadm trigger

log_substep_info "Exoscale block storage data disk configured"

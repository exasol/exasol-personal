#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

log_step_info "Installing Exasol"

log_substep_info "Setting up c4 config"

exasol_version='@exasol-2025.2.0'

if [[ ! -x ./c4 ]]; then
  log_error "c4 binary not found. Please run /opt/exasol_launcher/scripts/prepare.sh first."
  exit 1
fi

quote_b64() {
  # decode base64-encoded input string and quote it in `'`,
  # except `'` which are escaped as `\'`
  local decoded="$(base64 -d <<<"${1}")"
  local quoted=${decoded//\'/\'\\\'\'}  
  printf "'%s'" "${quoted}"
}

host_addrs="$(infra_jq -er '.hostAddrs')"
host_external_addrs="$(infra_jq -er '.hostExternalAddrs')"
db_password_b64="$(infra_jq -er '.dbPasswordB64')"
adminui_password_b64="$(infra_jq -er '.adminUiPasswordB64')"

cat << CONFEOF | tee ./config > /dev/null
CCC_HOST_ADDRS="${host_addrs}"
CCC_HOST_EXTERNAL_ADDRS="${host_external_addrs}"
CCC_HOST_KEY_PAIR_FILE="id_rsa"
CCC_HOST_IMAGE_USER=ubuntu
CCC_HOST_DATADISK="/dev/exasol_data_01"
CCC_PLAY_WORKING_COPY=${exasol_version}
CCC_PLAY_ROOTLESS=true
CCC_PLAY_DB_PASSWORD=$(quote_b64 "${db_password_b64}")
CCC_ADMINUI_START_SERVER=true
CCC_ADMINUI_ADMIN_PASSWORD=$(quote_b64 "${adminui_password_b64}")
CCC_AWS_PROFILE=none
CCC_PLAY_LICENSE=@license:personal
CONFEOF

log_substep_info "Starting Exasol installation using c4"

./c4 host play -i ./config

log_step_info "Exasol installation completed"

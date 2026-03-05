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

# Optional installation-preset variables (launcher-generated).
db_version_check_enabled="true"
cluster_identity=""
if [[ -f "${INSTALL_JSON}" ]]; then
  no_db_version_check="$(install_jq -er '.no_db_version_check // false' 2>/dev/null || echo false)"
  if [[ "${no_db_version_check}" == "true" ]]; then
    db_version_check_enabled="false"
  fi
  cluster_identity="$(install_jq -er '.cluster_identity // ""' 2>/dev/null || echo "")"
  log_substep_info "cluster identity: ${cluster_identity}"
else
  log_substep_info "installation variables file not found at ${INSTALL_JSON}; using defaults"
fi

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
CCC_PLAY_VERSION_UPDATE_CHECK=${db_version_check_enabled}
CONFEOF

# Append optional values only when set.
if [[ -n "${cluster_identity}" ]]; then
  echo "CCC_PLAY_CLUSTER_IDENTITY=\"${cluster_identity}\"" | tee -a ./config > /dev/null
fi

log_substep_info "Starting Exasol installation using c4"

./c4 host play -i ./config

log_step_info "Exasol installation completed"

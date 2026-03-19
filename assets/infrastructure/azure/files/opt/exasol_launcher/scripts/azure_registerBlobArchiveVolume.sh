#!/usr/bin/env bash
set -Eeuo pipefail
# This script registers an Azure Blob container as a remote archive volume using confd_client.
# Note that this is only run on one node (n11) via the post-install systemd unit.

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

source "${SCRIPT_DIR}/shared_post_install.sh"

log_substep_info "Looking up cluster PLAY_ID"
PLAY_ID="$("$C4_PATH" config -e .play.id)"

ARCHIVE_ENABLED="$(infra_jq -er '.azure.archive.enabled')"
if [[ "${ARCHIVE_ENABLED}" != "true" ]]; then
  log_substep_info "Archive setup disabled; skipping registration"
  exit 0
fi

archive_url="$(infra_jq -er '.azure.archive.url')"
archive_username="$(infra_jq -er '.azure.archive.username')"
archive_password="$(infra_jq -er '.azure.archive.password')"
archive_volume_name="$(infra_jq -er '.azure.archive.volumeName')"

log_substep_info "Registering Azure Blob container ${archive_url} as archive volume"

{
  printf 'ARCHIVE_URL=%q\n' "${archive_url}"
  printf 'ARCHIVE_USERNAME=%q\n' "${archive_username}"
  printf 'ARCHIVE_PASSWORD=%q\n' "${archive_password}"
  printf 'ARCHIVE_VOLUME_NAME=%q\n' "${archive_volume_name}"
  cat <<'CONNECTEOF'
set -Eeuo pipefail

DB_NAME=$(confd_client db_list --json | jq '.[0]')
if [[ -z "${DB_NAME}" ]]; then
  exit 1
fi

OWNER_TUPLE=$(confd_client db_info db_name: "${DB_NAME}" --json | jq -c '.config.owner')
if [[ -z "${OWNER_TUPLE}" ]]; then
  exit 1
fi

if ! confd_client remote_volume_add \
  vol_type: azure \
  url: "${ARCHIVE_URL}" \
  username: "${ARCHIVE_USERNAME}" \
  password: "${ARCHIVE_PASSWORD}" \
  owner: "${OWNER_TUPLE}" \
  remote_volume_name: "${ARCHIVE_VOLUME_NAME}"; then
  exit 1
fi

CONNECTEOF
} | "$C4_PATH" connect -s cos -i "$PLAY_ID"

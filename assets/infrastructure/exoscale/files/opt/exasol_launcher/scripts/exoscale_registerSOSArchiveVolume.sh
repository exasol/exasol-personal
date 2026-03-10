#!/usr/bin/env bash
set -Eeuo pipefail
# This script registers an Exoscale SOS bucket as a remote archive volume using confd_client.
# Exoscale SOS is S3-compatible, so we use vol_type: s3 with explicit credentials.
# Note that this is only run on one node (n11) via the post-install systemd unit.

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

source "${SCRIPT_DIR}/shared_post_install.sh"

log_substep_info "Looking up cluster PLAY_ID"
PLAY_ID="$("$C4_PATH" config -e .play.id)"

# Skip archive registration when disabled in deployment variables
ARCHIVE_ENABLED="$(infra_jq -er '.exoscale.archive.enabled')"
if [[ "${ARCHIVE_ENABLED}" != "true" ]]; then
  log_substep_info "Archive setup disabled; skipping registration"
  exit 0
fi

bucket_id="$(infra_jq -er '.exoscale.archive.bucketId')"
sos_endpoint="$(infra_jq -er '.exoscale.archive.sosEndpoint')"
archive_volume_name="$(infra_jq -er '.exoscale.archive.volumeName')"
access_key="$(infra_jq -er '.exoscale.archive.accessKey')"
secret_key="$(infra_jq -er '.exoscale.archive.secretKey')"

log_substep_info "Registering SOS bucket ${bucket_id} as archive volume"
# Doing this via stdin avoids a lot of quoting issues compared to passing commands to `c4 connect`
# (imagine quoting the `confd_client` call correctly for bash->ssh->bash...)
#
# It's also necessary because `c4 connect` unconditionally consumes stdin. If didn't supply it with
# something on stdin, it would read the rest of our script as the deployment scripts are essentially
# being run via `cat deployment_script.sh | ssh nXY`

{
  printf 'BUCKET_ID=%q\n' "${bucket_id}"
  printf 'SOS_ENDPOINT=%q\n' "${sos_endpoint}"
  printf 'ARCHIVE_VOLUME_NAME=%q\n' "${archive_volume_name}"
  printf 'ACCESS_KEY=%q\n' "${access_key}"
  printf 'SECRET_KEY=%q\n' "${secret_key}"
  cat <<'CONNECTEOF'
set -Eeuo pipefail

S3_URL="${SOS_ENDPOINT}/${BUCKET_ID}"

VOLUME_NAME="${ARCHIVE_VOLUME_NAME}"

# Get the database name from the cluster
DB_NAME=$(confd_client db_list --json | jq ".[0]")
if [ -z "$DB_NAME" ]; then
  exit 1
fi

# Get the owner tuple for the database
OWNER_TUPLE=$(confd_client db_info db_name: "$DB_NAME" --json | jq -c ".config.owner")
if [ -z "$OWNER_TUPLE" ]; then
  exit 1
fi

# Register the SOS bucket as a remote archive volume
# Exoscale SOS is S3-compatible; we pass explicit credentials since there is no instance profile.
if ! confd_client remote_volume_add vol_type: s3 url: "$S3_URL" owner: "$OWNER_TUPLE" remote_volume_name: "$VOLUME_NAME" s3_access_key: "$ACCESS_KEY" s3_secret_key: "$SECRET_KEY"; then
  exit 1
fi

CONNECTEOF
} | "$C4_PATH" connect -s cos -i "$PLAY_ID"

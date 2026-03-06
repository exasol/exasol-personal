#!/usr/bin/env bash
set -Eeuo pipefail
# This script registers an s3 bucket as a remote archive volume using confd_client
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
ARCHIVE_ENABLED="$(infra_jq -er '.aws.archive.enabled')"
if [[ "${ARCHIVE_ENABLED}" != "true" ]]; then
  log_substep_info "Archive setup disabled; skipping registration"
  exit 0
fi

bucket_id="$(infra_jq -er '.aws.archive.bucketId')"
aws_region="$(infra_jq -er '.aws.region')"
archive_volume_name="$(infra_jq -er '.aws.archive.volumeName')"

log_substep_info "Registering s3 bucket ${bucket_id} as archive volume"
# Doing this via stdin avoids a lot of quoting issues compared to passing commands to `c4 connect`
# (imagine quoting the `confd_client` call correctly for bash->ssh->bash...)
#
# It's also necessary because `c4 connect` unconditionally consumes stdin. If didn't supply it with
# something on stdin, it would read the rest of our script as the deployment scripts are essentially
# being run via `cat deployment_script.sh | ssh nXY`

{
  printf 'BUCKET_ID=%q\n' "${bucket_id}"
  printf 'AWS_REGION=%q\n' "${aws_region}"
  printf 'ARCHIVE_VOLUME_NAME=%q\n' "${archive_volume_name}"
  cat <<'CONNECTEOF'
set -Eeuo pipefail

S3_URL="https://${BUCKET_ID}.s3.${AWS_REGION}.amazonaws.com"

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

# Register the S3 bucket as a remote archive volume
if ! confd_client remote_volume_add vol_type: s3 url: "$S3_URL" owner: "$OWNER_TUPLE" remote_volume_name: "$VOLUME_NAME"; then
  exit 1
fi

CONNECTEOF
} | "$C4_PATH" connect -s cos -i "$PLAY_ID"

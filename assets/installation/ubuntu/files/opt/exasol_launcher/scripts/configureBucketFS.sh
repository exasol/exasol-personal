#!/usr/bin/env bash
set +x
set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
source "${SCRIPT_DIR}/logging.sh"
source "${SCRIPT_DIR}/config.sh"
source "${SCRIPT_DIR}/shared_post_install.sh"

log_step_info "Configuring BucketFS credentials for bfsdefault/default..."

configure_bucketfs() {
    local play_id read_b64 write_b64
    play_id=$("$C4_PATH" config -e .play.id) || return 1
    read_b64=$(infra_jq -er '.bucketfsReadPasswordB64') || return 1
    write_b64=$(infra_jq -er '.bucketfsWritePasswordB64') || return 1
    [[ "$read_b64" =~ ^[A-Za-z0-9+/]+=*$ && "$write_b64" =~ ^[A-Za-z0-9+/]+=*$ ]] || return 1

    # C4 and ConfD can echo requests and responses, so keep their entire streams private.
    {
        printf '%s\n' "set +x" "set -Eeuo pipefail"
        printf 'read_b64="%s"\nwrite_b64="%s"\n' "$read_b64" "$write_b64"
        cat <<'BUCKETFS_REMOTE'
info=$(printf '%s\n' '{"bucketfs_name":"bfsdefault"}' | confd_client --json bucketfs_info -a -) || exit 1
public=$(printf '%s\n' "$info" | jq -er '.buckets.default.public | if type == "boolean" then tostring else error("Invalid bucket policy") end') || exit 1
[[ "$public" == true || "$public" == false ]] || exit 1
params=$(printf '%s\n' "$read_b64" "$write_b64" | jq -Ren --argjson public "$public" '
    (input | @base64d) as $read | (input | @base64d) as $write |
    {bucketfs_name: "bfsdefault", bucket_name: "default", public: $public,
     read_password: $read, write_password: $write}
') || exit 1
printf '%s\n' "$params" | confd_client --json bucket_modify -a - || exit 1
BUCKETFS_REMOTE
    } | "$C4_PATH" connect -s cos -i "$play_id"
}

if configure_bucketfs >/dev/null 2>&1; then
    log_step_info "BucketFS credentials configured for bfsdefault/default"
else
    log_error "Failed to configure BucketFS credentials for bfsdefault/default"
    exit 1
fi

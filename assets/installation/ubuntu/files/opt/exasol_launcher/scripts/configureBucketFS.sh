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
        printf '%s\n' "set +x" "set -Eeuo pipefail" "python3 - <<'BUCKETFS_PY'"
        printf 'read_b64 = "%s"\nwrite_b64 = "%s"\n' "$read_b64" "$write_b64"
        cat <<'BUCKETFS_REMOTE'
import base64
import json
import subprocess

def confd(command, params):
    result = subprocess.run(
        ["confd_client", "--json", command, "-a", "-"],
        input=json.dumps(params), text=True, capture_output=True, check=True,
    )
    return json.loads(result.stdout) if result.stdout.strip() else None

params = {"bucketfs_name": "bfsdefault"}
info = confd("bucketfs_info", params)
public = info["buckets"]["default"]["public"]
if not isinstance(public, bool):
    raise ValueError("Invalid bucket access policy")
read_password = base64.b64decode(read_b64, validate=True).decode("ascii")
write_password = base64.b64decode(write_b64, validate=True).decode("ascii")
if (len(read_password) < 32 or len(write_password) < 32
        or not read_password.isalnum() or not write_password.isalnum()
        or read_password == write_password):
    raise ValueError("Invalid BucketFS credentials")
confd("bucket_modify", dict(params, bucket_name="default", public=public,
      read_password=read_password, write_password=write_password))
BUCKETFS_PY
BUCKETFS_REMOTE
    } | "$C4_PATH" connect -s cos -i "$play_id"
}

if configure_bucketfs >/dev/null 2>&1; then
    log_step_info "BucketFS credentials configured for bfsdefault/default"
else
    log_error "Failed to configure BucketFS credentials for bfsdefault/default"
    exit 1
fi

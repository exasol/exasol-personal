#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

# This script waits for confd to become available after deployment.
# It wraps `c4 wait --stage d` with a shell-level timeout because the `c4 wait`
# command itself does currently not provide distinct exit codes for success vs timeout.
# The script:
#   - Uses TIMEOUT_SECONDS as the maximum wait duration for the `timeout`.
#   - Effectively disables the timeout of `c4 wait` by making CCC_CLOUD_ACTION_WAIT_TIMEOUT very large
#   - Exits non-zero on timeout or any other failure.

log_substep_info "Waiting for confd to be ready ..."

# Timeout for the waiting operation
# 5 minutes should be enough typically
TIMEOUT_SECONDS=300

source "${SCRIPT_DIR}/shared_post_install.sh"

CONFD_PORT=$($C4_PATH config -F -e .play.confd_port)

if timeout "$TIMEOUT_SECONDS" curl --retry-all-errors --retry-delay 5 --retry 120 --fail --silent --insecure "https://localhost:$CONFD_PORT/is_master" >/dev/null; then
    :
else
    rc=$?
    # 124 = The command was terminated by timeout because it didn’t finish within the given time.
    if [[ $rc -eq 124 ]]; then
        log_error "Timed out waiting for confd to be ready"
    else
        log_error "curl failed with exit code $rc"
    fi
    exit $rc
fi

log_substep_info "confd is ready"

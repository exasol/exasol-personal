#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

# This script waits for the database to become available after deployment.
# It wraps `c4 wait --stage d` with a shell-level timeout because the `c4 wait`
# command itself does currently not provide distinct exit codes for success vs timeout.
# The script:
#   - Uses TIMEOUT_SECONDS as the maximum wait duration for the `timeout`.
#   - Effectively disables the timeout of `c4 wait` by making CCC_CLOUD_ACTION_WAIT_TIMEOUT very large
#   - Exits non-zero on timeout or any other failure.

log_substep_info "Waiting for Exasol to boot up"

# Timeout for the waiting operation
# 5 minutes should be enough typically
TIMEOUT_SECONDS=300

# Path to c4 executable for the DB
# Needed to hardcode this because we need to use exactly this c4 from this path
# It looks for its config in ../etc/c4.yaml or rather $HOME/.ccc/ccc/etc/c4.yaml
# We use the synlink in $HOME/.local/bin here though instead to abstract from these internals.
# Unfortunetedly, it appears the installation process doesn't update PATH in .bashrc
# until some point later during the init (we should probly fix that)
# Otherwise we could have simply sourced $HOME/.bashrc
C4_PATH=$HOME/.local/bin/c4

# Play ID (or deployment ID)
# Needed for subsequent c4 operations as most ops need to target a specific deployment
# We have only one deployment here, but it is better to make this explicit
PLAY_ID=$($C4_PATH config -e .play.id)
log_substep_info "Cluster PLAY_ID=$PLAY_ID"

if CCC_CLOUD_ACTION_WAIT_TIMEOUT=3600 timeout "$TIMEOUT_SECONDS" $C4_PATH wait $PLAY_ID --stage d; then
    :
else
    rc=$?
    # 124 = The command was terminated by timeout because it didn’t finish within the given time.
    if [[ $rc -eq 124 ]]; then
        log_error "Timed out waiting for Exasol to boot up"
    else
        log_error "c4 wait failed with exit code $rc"
    fi
    exit $rc
fi

log_substep_info "Exasol booted up"
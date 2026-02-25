#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

log_substep_info "Waiting for local c4 to be installed"

# Path to c4 executable
# Needed to hardcode this because we need to use exactly this c4 from this path
# It looks for its config in ../etc/c4.yaml or rather $HOME/.ccc/ccc/etc/c4.yaml
# We use the synlink in $HOME/.local/bin here though instead to abstract from these internals.
C4_PATH=$HOME/.local/bin/c4

FOUND=0
# Wait up to ~10 minutes, downloading packages and getting them to all nodes
# can take a while
for _i in {1..120}; do
  if [[ -f "${C4_PATH}" ]]; then
    FOUND=1
    break
  fi
  sleep 5
done
if [[ ${FOUND} -ne 1 ]]; then
  log_error "c4 not found after timeout"
  exit 1
fi

log_substep_info "local c4 was installed"
#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

log_substep_info "Waiting for cloud-init to complete..."

# Run cloud-init status if available, but do not fail on non-zero exit codes
# (cloud-init can return 2 for warnings). We'll rely on a marker file instead.
if command -v cloud-init >/dev/null 2>&1; then
  log_substep_info "Detected cloud-init; waiting (non-zero exit tolerated) ..."
  cloud-init status --wait --long || true
fi

MARKER="/var/lib/exasol_launcher/state/cloud-init.complete"
log_substep_info "Waiting for marker: ${MARKER}"
FOUND=0
for i in {1..120}; do # up to ~10 minutes
  if [[ -f "${MARKER}" ]]; then
    FOUND=1
    break
  fi
  sleep 5
done
if [[ ${FOUND} -ne 1 ]]; then
  log_error "Marker ${MARKER} not found after timeout"
  if command -v cloud-init >/dev/null 2>&1; then
    cloud-init status --long || true
  fi
  exit 1
fi
log_substep_info "Marker found; cloud-init preparation complete."

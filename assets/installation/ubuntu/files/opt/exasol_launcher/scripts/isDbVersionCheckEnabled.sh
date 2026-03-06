#!/usr/bin/env bash
# isDbVersionCheckEnabled.sh - Returns success (exit 0) iff DB version checks are enabled.
#
# Intended usage:
#   - systemd ExecCondition= for optional host-side fallback behavior
#
# Contract:
#   - exit 0 => DB version checks enabled
#   - exit 1 => DB version checks disabled
#   - exit >1 => unexpected error (missing config, jq failure)

set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

if [[ ! -f "${INSTALL_JSON}" ]]; then
  echo "installation config not found: ${INSTALL_JSON}" >&2
  exit 2
fi

no_db_version_check="$(install_jq -er '.no_db_version_check // false')"
if [[ "${no_db_version_check}" == "true" ]]; then
  exit 1
fi

exit 0

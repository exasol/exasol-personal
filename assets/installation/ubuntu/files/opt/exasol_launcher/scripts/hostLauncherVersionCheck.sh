#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

# Local scheduler-level guard in case timer cadence is changed later.
readonly CHECK_INTERVAL_SECONDS=$((24 * 60 * 60))
readonly LAST_ATTEMPT_FILE="/var/lib/exasol_launcher/state/launcher_version_check.last_attempt_epoch"
readonly DEFAULT_VERSION_CHECK_URL="https://metrics-test.exasol.com/v1/version-check"
readonly VERSION_CHECK_CATEGORY="Exasol 8"
readonly VERSION_CHECK_OS="Linux"
readonly VERSION_CHECK_ARCH="x86_64"

read_install_var() {
  local key="$1"
  install_jq -er ".${key} // \"\"" 2>/dev/null || echo ""
}

db_check_enabled="false"
if "${SCRIPT_DIR}/isDbVersionCheckEnabled.sh"; then
  db_check_enabled="true"
fi
if [[ "${db_check_enabled}" != "true" ]]; then
  echo "DB version checks disabled; skipping host launcher version check"
  exit 0
fi

cluster_identity="$(read_install_var "cluster_identity")"
exasol_version="$(read_install_var "exasol_version")"
version_check_url="$(read_install_var "version_check_url")"
if [[ -z "${version_check_url}" ]]; then
  version_check_url="${DEFAULT_VERSION_CHECK_URL}"
fi
if [[ -z "${exasol_version}" ]]; then
  exasol_version="2025.2.0"
fi

# Missing launcher-governed values means init/deploy artifacts are incomplete.
# Treat as a soft skip so this auxiliary feature cannot affect deployment health.
if [[ -z "${cluster_identity}" ]]; then
  echo "missing cluster_identity; skipping host launcher version check"
  exit 0
fi

now_epoch="$(date +%s)"
last_attempt_epoch=0
if [[ -f "${LAST_ATTEMPT_FILE}" ]]; then
  last_attempt_epoch="$(cat "${LAST_ATTEMPT_FILE}" 2>/dev/null || echo 0)"
fi

if [[ $((now_epoch - last_attempt_epoch)) -lt ${CHECK_INTERVAL_SECONDS} ]]; then
  echo "host launcher version check skipped due to local rate limit"
  exit 0
fi

mkdir -p "$(dirname "${LAST_ATTEMPT_FILE}")"
# Record attempts before the network call so repeated failures/timeouts do not loop rapidly.
echo "${now_epoch}" > "${LAST_ATTEMPT_FILE}"

if curl \
  --silent \
  --show-error \
  --fail \
  --get \
  --connect-timeout 3 \
  --max-time 3 \
  "${version_check_url}" \
  --data-urlencode "category=${VERSION_CHECK_CATEGORY}" \
  --data-urlencode "operatingSystem=${VERSION_CHECK_OS}" \
  --data-urlencode "architecture=${VERSION_CHECK_ARCH}" \
  --data-urlencode "version=${exasol_version}" \
  --data-urlencode "identity=${cluster_identity}" \
  >/dev/null; then
  echo "host launcher version check completed"
else
  # Best-effort semantics: failures remain visible in logs but never fail the unit.
  echo "host launcher version check failed"
fi

exit 0

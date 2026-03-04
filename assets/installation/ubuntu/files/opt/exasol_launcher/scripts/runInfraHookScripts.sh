#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

usage() {
  cat <<'USAGE'
Usage:
  runInfraHookScripts.sh <pre-install-root|pre-install-user|post-install>

Contract:
  Reads /etc/exasol_launcher/infrastructure.json and executes scripts declared by the infrastructure preset.

Mappings:
  pre-install-root -> .preInstall.root.scripts
  pre-install-user -> .preInstall.user.scripts
  post-install     -> .postInstall.scripts
USAGE
}

phase="${1:-}"
if [[ -z "${phase}" || "${phase}" == "-h" || "${phase}" == "--help" ]]; then
  usage
  exit 0
fi

if (( $# != 1 )); then
  echo "Expected exactly one argument (phase), got $#" >&2
  usage
  exit 2
fi

label=""
jq_expr=""

case "${phase}" in
  pre-install-root)
    label="pre-install (root)"
    jq_expr=".preInstall.root.scripts"
    ;;
  pre-install-user)
    label="pre-install (user)"
    jq_expr=".preInstall.user.scripts"
    ;;
  post-install)
    label="post-install"
    jq_expr=".postInstall.scripts"
    ;;
  *)
    echo "Unknown phase: ${phase}" >&2
    usage
    exit 2
    ;;
esac

log_step_info "Running infrastructure hook scripts: ${label}"

# Expect jq_expr to evaluate to an array of strings (script paths). Missing or null is treated as empty.
mapfile -t scripts < <(infra_jq -r "(${jq_expr} // [])[]")

if (( ${#scripts[@]} == 0 )); then
  log_substep_info "No infrastructure hook scripts configured for '${label}'; skipping"
  exit 0
fi

for script_path in "${scripts[@]}"; do
  if [[ -z "${script_path}" ]]; then
    continue
  fi

  log_substep_info "Executing: ${script_path}"

  if [[ ! -e "${script_path}" ]]; then
    log_error "Configured hook script does not exist: ${script_path}"
    exit 1
  fi

  if [[ -x "${script_path}" ]]; then
    "${script_path}"
  else
    # Be forgiving if permissions were not set to executable by cloud-init.
    /usr/bin/env bash "${script_path}"
  fi
done

log_step_info "Infrastructure hook scripts completed: ${label}"

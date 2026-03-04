#!/usr/bin/env bash
# Common logging helpers for installation scripts. Must be sourced.

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  echo "logging.sh must be sourced, not executed" >&2
  exit 1
fi

log_step_info() {
  printf 'EXASOL-INSTALL-STEP: %s\n' "$*"
}

log_substep_info() {
  printf 'EXASOL-INSTALL-SUBSTEP: %s\n' "$*"
}

log_error() {
  printf 'EXASOL-INSTALL-ERROR: %s\n' "$*" >&2
}

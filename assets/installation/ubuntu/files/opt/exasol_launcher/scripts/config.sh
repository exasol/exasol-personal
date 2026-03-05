#!/usr/bin/env bash
# Common JSON access helpers for installation scripts. Must be sourced.
#
# Provides:
#   - INFRA_JSON, NODE_JSON, INSTALL_JSON paths
#   - infra_jq / node_jq / install_jq wrappers around jq

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  echo "config.sh must be sourced, not executed" >&2
  exit 1
fi

INFRA_JSON='/etc/exasol_launcher/infrastructure.json'
NODE_JSON='/etc/exasol_launcher/node.json'
INSTALL_JSON='/etc/exasol_launcher/installation.json'

infra_jq() {
  jq "$@" "${INFRA_JSON}"
}

node_jq() {
  jq "$@" "${NODE_JSON}"
}

install_jq() {
  jq "$@" "${INSTALL_JSON}"
}

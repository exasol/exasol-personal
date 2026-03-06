#!/usr/bin/env bash
# isAccessNode.sh - Returns success (exit 0) iff this node is the access/primary node (n11).
#
# Intended usage:
#   - systemd ExecCondition= for services that must only run on the access node
#   - exit 0 => this node is n11
#   - exit 1 => this node is NOT n11
#   - exit >1 => unexpected error (missing config, jq failure)

set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

# node.json is written by cloud-init; if it is missing, that is an error.
if [[ ! -f "${NODE_JSON}" ]]; then
  echo "node config not found: ${NODE_JSON}" >&2
  exit 2
fi

node_id="$(node_jq -er '.myId // .name')"
if [[ "${node_id}" == 'n11' ]]; then
  exit 0
fi

exit 1

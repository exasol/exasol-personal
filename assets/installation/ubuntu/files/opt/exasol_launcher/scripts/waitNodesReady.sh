#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"
# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

shopt -s nullglob

node_state_dir='/var/lib/exasol_launcher/state/nodes'

server() {
  log_step_info "Waiting for all cluster nodes to be ready"

  local num_nodes="${1}"
  local -a nodes

  while true; do
    nodes=()
    # This uses nullglob to run the loop 0 times if there are no files
    local f
    for f in "${node_state_dir}"/*; do
      nodes+=( "${f##*/}")
    done
    
    log_substep_info "Node barrier markers present: ${nodes[*]}"

    if [[ "${#nodes[@]}" -eq "${num_nodes}" ]]; then
      return
    fi
    sleep 5
  done

  log_step_info "All cluster nodes ready"
}

client() {
  log_step_info "Synchronizing this node with the cluster"

  local server="${1}"
  local my_id="${2}"

  while ! ssh "ubuntu@${server}" -- touch "${node_state_dir}/${my_id}" ; do
    log_substep_info "Waiting for barrier server at ${server}"
    sleep 5
  done

  log_step_info "This node is synchronized with the cluster"
}

if [[ "${1}" == 'server' ]]; then
  server "$(infra_jq -er '.numNodes')"
else
  client "$(infra_jq -er '.n11Ip')" "$(node_jq -er '.myId')"
fi
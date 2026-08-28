#!/usr/bin/env bash

set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"
# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

shopt -s nullglob

node_state_dir='/var/lib/exasol_launcher/state/nodes'

# Bounded retries so an unreachable node surfaces as a failed installation
# instead of blocking the systemd unit indefinitely.
readonly BARRIER_SERVER_ATTEMPTS=120
readonly BARRIER_CLIENT_ATTEMPTS=30
readonly BARRIER_INTERVAL_SECONDS=5
readonly BARRIER_SSH_TIMEOUT_SECONDS=15

server() {
  log_step_info "Waiting for all cluster nodes to be ready"

  local num_nodes="${1}"
  local -a nodes

  local -i attempt
  for (( attempt = 1; attempt <= BARRIER_SERVER_ATTEMPTS; attempt++ )); do
    nodes=()
    # This uses nullglob to run the loop 0 times if there are no files
    local f
    for f in "${node_state_dir}"/*; do
      nodes+=( "${f##*/}")
    done

    log_substep_info "Node barrier markers present: ${nodes[*]}"

    if [[ "${#nodes[@]}" -eq "${num_nodes}" ]]; then
      log_step_info "All cluster nodes ready"
      return
    fi
    sleep "${BARRIER_INTERVAL_SECONDS}"
  done

  log_error "Only ${#nodes[@]} of ${num_nodes} cluster nodes reached the barrier"
  exit 1
}

client() {
  log_step_info "Synchronizing this node with the cluster"

  local server="${1}"
  local my_id="${2}"

  # Bound each SSH invocation so authentication or a stalled remote command
  # cannot consume the full barrier budget.
  local -i attempt
  for (( attempt = 1; attempt <= BARRIER_CLIENT_ATTEMPTS; attempt++ )); do
    if timeout "${BARRIER_SSH_TIMEOUT_SECONDS}s" ssh \
      -o BatchMode=yes \
      -o ConnectTimeout=10 \
      -o ServerAliveInterval=5 \
      -o ServerAliveCountMax=1 \
      "ubuntu@${server}" -- touch "${node_state_dir}/${my_id}"; then
      log_step_info "This node is synchronized with the cluster"
      return
    fi
    log_substep_info "Waiting for barrier server at ${server}"
    sleep "${BARRIER_INTERVAL_SECONDS}"
  done

  log_error "Could not register with the barrier server at ${server}"
  exit 1
}

if [[ "${1}" == 'server' ]]; then
  server "$(infra_jq -er '.numNodes')"
else
  client "$(infra_jq -er '.n11Ip')" "$(node_jq -er '.myId')"
fi

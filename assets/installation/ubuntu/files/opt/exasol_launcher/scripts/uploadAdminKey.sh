#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

# shellcheck source=./config.sh
source "${SCRIPT_DIR}/config.sh"

log_step_info "Configuring SSH access..."

log_substep_info "Installing admin SSH key into ~/.ssh/id_rsa"
umask 077
mkdir -p ~/.ssh
infra_jq -r '.adminPrivateKey' > ~/.ssh/id_rsa
chmod 600 ~/.ssh/id_rsa

log_substep_info "Pre-seeding known_hosts for cluster nodes"
HOSTS="$(infra_jq -er '.hostAddrs')"
touch ~/.ssh/known_hosts
chmod 644 ~/.ssh/known_hosts

# A missing entry makes every later SSH to that node fail host key verification,
# which stalls the node barrier for the whole installation. Nodes may still be
# booting or their network may not be up yet, so retry before giving up.
readonly KEYSCAN_ATTEMPTS=30
# HOSTS is a space-separated list and must word-split.
# shellcheck disable=SC2086
for host in ${HOSTS}; do
  keys=''
  for _attempt in $(seq 1 "${KEYSCAN_ATTEMPTS}"); do
    keys="$(ssh-keyscan -T 5 -H "${host}" 2>/dev/null || true)"
    if [[ -n "${keys}" ]]; then
      break
    fi
    log_substep_info "Waiting for SSH host key of ${host}"
    sleep 5
  done
  if [[ -z "${keys}" ]]; then
    log_error "No SSH host key from ${host} after ${KEYSCAN_ATTEMPTS} attempts"
    exit 1
  fi
  printf '%s\n' "${keys}" >> ~/.ssh/known_hosts
done

log_step_info "SSH access configured"
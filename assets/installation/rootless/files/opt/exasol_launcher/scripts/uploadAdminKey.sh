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
if [[ -n "$HOSTS" ]]; then
  ssh-keyscan -T 5 -H $HOSTS >> ~/.ssh/known_hosts || true
fi

log_step_info "SSH access configured"
#!/usr/bin/env bash
set -euo pipefail

SSH_KEY="vm-key"

if [ ! -f "$SSH_KEY" ]; then
    echo "==> Generating SSH key pair..."
    ssh-keygen -t ed25519 -f "$SSH_KEY" -N "" -C "exasol-vm-key"
    echo "==> SSH key generated: $SSH_KEY and $SSH_KEY.pub"
else
    echo "==> SSH key already exists: $SSH_KEY"
fi

echo ""
echo "==> Use this command to SSH into the VM:"
echo "    ssh -i $SSH_KEY -p 2222 exasol@localhost"

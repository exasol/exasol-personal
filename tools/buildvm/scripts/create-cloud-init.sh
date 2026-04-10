#!/usr/bin/env bash
set -euo pipefail

CLOUD_INIT_ISO="cloud-init.iso"
SSH_KEY="vm-key"

if [ ! -f "$SSH_KEY.pub" ]; then
    echo "Error: SSH key not found. Run 'task generate-ssh-key' first."
    exit 1
fi

echo "==> Reading SSH public key..."
SSH_PUB_KEY=$(cat "$SSH_KEY.pub")

echo "==> Copying scripts to shared directory..."
mkdir -p shared/scripts
cp cloud-init/scripts/* shared/scripts/
chmod +x shared/scripts/*.sh shared/scripts/*.initd

echo "==> Generating user-data with SSH key..."
sed "s|SSH_PUBLIC_KEY_PLACEHOLDER|$SSH_PUB_KEY|g" cloud-init/user-data > user-data

echo "==> Copying meta-data..."
cp cloud-init/meta-data meta-data

echo "==> Creating cloud-init configuration ISO..."
genisoimage -output "$CLOUD_INIT_ISO" \
    -volid cidata \
    -rational-rock \
    -joliet \
    -joliet-long \
    user-data \
    meta-data

echo "==> Cleaning up temporary files..."
rm user-data meta-data

echo "==> Cloud-init ISO created: $CLOUD_INIT_ISO"
echo "==> Scripts copied to shared/scripts/"

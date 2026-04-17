#!/bin/sh
set -e
echo "==> Setting up SSH service..."
rc-update add sshd default
service sshd start

echo "==> Setting up SSH key import service..."
cp /mnt/host/scripts/import-shared-keys.initd /etc/init.d/import-shared-keys
chmod +x /etc/init.d/import-shared-keys
rc-update add import-shared-keys default

echo "==> Setting up container load service..."
cp /mnt/host/scripts/load-shared-container.initd /etc/init.d/load-shared-container
chmod +x /etc/init.d/load-shared-container
rc-update add load-shared-container default

echo "==> Services configured"

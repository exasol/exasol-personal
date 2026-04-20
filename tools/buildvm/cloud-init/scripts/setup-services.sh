#!/bin/sh
set -e
echo "==> Setting up SSH service..."
rc-update add sshd default
service sshd start

echo "==> Setting up SSH key import service..."
cp /mnt/host/scripts/import-shared-keys.initd /etc/init.d/import-shared-keys
chmod +x /etc/init.d/import-shared-keys
rc-update add import-shared-keys default

echo "==> Setting up DB load service..."
cp /mnt/host/scripts/load-db.initd /etc/init.d/load-db
chmod +x /etc/init.d/load-db
rc-update add load-db default

echo "==> Services configured"

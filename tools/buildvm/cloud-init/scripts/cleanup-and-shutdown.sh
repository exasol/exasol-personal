#!/bin/sh
set -e
echo "==> Signaling completion..."
touch /mnt/host/cloud-init-complete

echo "==> Disabling cloud-init services (one-time setup complete)..."
rc-update del cloud-init-local boot 2>/dev/null || true
rc-update del cloud-init default 2>/dev/null || true
rc-update del cloud-config default 2>/dev/null || true
rc-update del cloud-final default 2>/dev/null || true
touch /etc/cloud/cloud-init.disabled

echo "==> Cleaning up system..."
apk cache clean
rm -rf /var/cache/apk/*
rm -rf /tmp/* /var/tmp/* || true
find /var/log -type f -exec truncate -s 0 {} \;

echo "==> Zeroing free space..."
dd if=/dev/zero of=/zerofill bs=1M || true
sync
rm -f /zerofill
sync

echo "==> Shutting down..."
poweroff

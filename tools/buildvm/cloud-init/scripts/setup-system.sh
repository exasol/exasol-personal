#!/bin/sh
set -e
echo "==> Growing partition and filesystem..."
growpart /dev/vda 2
resize2fs /dev/vda2

echo "==> Mounting shared folder..."
mkdir -p /mnt/host
mount -a || true

echo "==> System setup complete"

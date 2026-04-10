#!/bin/sh
set -e
echo "==> Growing partition and filesystem..."
growpart /dev/vda 2
resize2fs /dev/vda2

echo "==> Mounting shared folder..."
mkdir -p /mnt/host
mount -a || true

echo "==> Configuring rootless podman (subuid/subgid)..."
# Add subuid/subgid ranges for alpine user for rootless containers
if ! grep -q "^alpine:" /etc/subuid; then
  echo "alpine:100000:65536" >> /etc/subuid
fi
if ! grep -q "^alpine:" /etc/subgid; then
  echo "alpine:100000:65536" >> /etc/subgid
fi

# Set environment variable to suppress cgroups-v1 warning
if ! grep -q "PODMAN_IGNORE_CGROUPSV1_WARNING" /home/alpine/.profile 2>/dev/null; then
  echo "export PODMAN_IGNORE_CGROUPSV1_WARNING=1" >> /home/alpine/.profile
fi

echo "==> System setup complete"

#!/bin/sh
set -e
echo "==> Growing partition and filesystem..."
growpart /dev/vda 2
resize2fs /dev/vda2

echo "==> Mounting shared folder..."
mkdir -p /mnt/host
mount -a || true

echo "==> Mounting cgroup2 for container support..."
if [ ! -f /sys/fs/cgroup/cgroup.controllers ]; then
  mount -t cgroup2 none /sys/fs/cgroup
  echo "cgroup2 mounted"
fi

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

echo "==> Configuring GRUB to skip boot menu..."
# Set GRUB timeout to 0 to boot immediately
if [ -f /etc/default/grub ]; then
  sed -i 's/^GRUB_TIMEOUT=.*/GRUB_TIMEOUT=0/' /etc/default/grub
  # Also set hidden timeout
  if ! grep -q "^GRUB_HIDDEN_TIMEOUT=" /etc/default/grub; then
    echo "GRUB_HIDDEN_TIMEOUT=0" >> /etc/default/grub
  else
    sed -i 's/^GRUB_HIDDEN_TIMEOUT=.*/GRUB_HIDDEN_TIMEOUT=0/' /etc/default/grub
  fi
  # Regenerate GRUB configuration
  if command -v grub-mkconfig >/dev/null 2>&1; then
    grub-mkconfig -o /boot/grub/grub.cfg
    echo "GRUB configured for immediate boot"
  elif command -v update-grub >/dev/null 2>&1; then
    update-grub
    echo "GRUB configured for immediate boot"
  fi
fi

echo "==> System setup complete"

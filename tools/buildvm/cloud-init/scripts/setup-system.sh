#!/bin/sh
set -e
echo "==> Growing partition and filesystem..."
growpart /dev/vda 2
resize2fs /dev/vda2

echo "==> Setting up shared storage at /mnt/host..."
mkdir -p /mnt/host

# Try to mount from fstab (virtiofs for Mac/QEMU/vfkit)
mount -a || true

# Check if /mnt/host is mounted
if mountpoint -q /mnt/host; then
  echo "==> Using virtiofs for /mnt/host (Mac/QEMU/vfkit)"
else
  # Check for secondary block device (Hyper-V data disk)
  # Look for first virtio disk that isn't vda (system disk)
  DATA_DISK=""
  for disk in /dev/vdb /dev/sdb /dev/xvdb; do
    if [ -b "$disk" ]; then
      DATA_DISK="$disk"
      break
    fi
  done
  
  if [ -n "$DATA_DISK" ]; then
    echo "==> Detected block device $DATA_DISK for /mnt/host (Hyper-V)"
    
    # Check if disk is already formatted
    if ! blkid "$DATA_DISK" > /dev/null 2>&1; then
      echo "==> Formatting $DATA_DISK as ext4..."
      mkfs.ext4 -L exasol-data "$DATA_DISK"
    else
      echo "==> Using existing filesystem on $DATA_DISK"
    fi
    
    # Mount the disk
    echo "==> Mounting $DATA_DISK at /mnt/host..."
    mount "$DATA_DISK" /mnt/host
    
    # Add to fstab for persistence (if not already there)
    if ! grep -q "^LABEL=exasol-data" /etc/fstab; then
      echo "LABEL=exasol-data /mnt/host ext4 defaults,nofail 0 2" >> /etc/fstab
      echo "==> Added $DATA_DISK to /etc/fstab for auto-mount on boot"
    fi
  else
    echo "==> Warning: No shared storage available (no virtiofs or block device found)"
    echo "==> /mnt/host will be empty. Container features requiring shared storage will not work."
  fi
fi

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

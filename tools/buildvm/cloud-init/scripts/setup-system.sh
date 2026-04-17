#!/bin/sh
set -e
echo "==> Growing partition and filesystem..."
# growpart returns non-zero when partition can't grow (already at max size)
# Handle gracefully to allow script to continue
if growpart /dev/vda 2 2>&1 | grep -q "NOCHANGE"; then
  echo "Partition already at maximum size"
else
  resize2fs /dev/vda2
  echo "Partition and filesystem grown successfully"
fi

echo "==> Adding hostname to /etc/hosts..."
HOSTNAME=$(hostname)
if ! grep -q "$HOSTNAME" /etc/hosts; then
  echo "127.0.0.1 $HOSTNAME" >> /etc/hosts
  echo "Added $HOSTNAME to /etc/hosts"
fi

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
# Add subuid/subgid ranges for exasol user for rootless containers
if ! grep -q "^exasol:" /etc/subuid; then
  echo "exasol:100000:65536" >> /etc/subuid
fi
if ! grep -q "^exasol:" /etc/subgid; then
  echo "exasol:100000:65536" >> /etc/subgid
fi

# Set environment variable to suppress cgroups-v1 warning
if ! grep -q "PODMAN_IGNORE_CGROUPSV1_WARNING" /home/exasol/.profile 2>/dev/null; then
  echo "export PODMAN_IGNORE_CGROUPSV1_WARNING=1" >> /home/exasol/.profile
fi

echo "==> Configuring GRUB to skip boot menu..."
# Edit GRUB config directly (Alpine cloud images don't ship with grub-mkconfig tooling)
if [ -f /boot/grub/grub.cfg ]; then
  # Replace ALL timeout settings in the file (including those in conditional blocks)
  sed -i 's/^[[:space:]]*set timeout=.*/  set timeout=0/' /boot/grub/grub.cfg
  sed -i 's/^[[:space:]]*set timeout_style=.*/  set timeout_style=hidden/' /boot/grub/grub.cfg
  
  # Verify changes took effect and check for any remaining non-zero timeouts
  REMAINING_TIMEOUTS=$(grep -E '^[[:space:]]*set timeout=' /boot/grub/grub.cfg | grep -v 'timeout=0' || true)
  REMAINING_STYLES=$(grep -E '^[[:space:]]*set timeout_style=' /boot/grub/grub.cfg | grep -v 'timeout_style=hidden' || true)
  
  if [ -z "$REMAINING_TIMEOUTS" ] && [ -z "$REMAINING_STYLES" ]; then
    echo "GRUB configured for immediate boot (timeout=0, style=hidden)"
  else
    echo "WARNING: GRUB configuration may not have applied correctly"
    echo "Remaining timeout settings:"
    grep -E '^[[:space:]]*set timeout' /boot/grub/grub.cfg || true
  fi
else
  echo "WARNING: /boot/grub/grub.cfg not found - GRUB timeout not changed"
fi

echo "==> System setup complete"

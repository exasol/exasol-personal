#!/usr/bin/env bash
set -euo pipefail

IMAGE_URL="$1"
TEMP_QCOW2="alpine-cloud.qcow2"
DISK_IMG="disk.img"

echo "==> Downloading NoCloud image from Alpine servers..."
wget -O "$TEMP_QCOW2" "$IMAGE_URL"

echo "==> Converting to raw disk image..."
qemu-img convert -f qcow2 -O raw "$TEMP_QCOW2" "$DISK_IMG"

echo "==> Cleaning up qcow2 file..."
rm "$TEMP_QCOW2"

echo "==> Detecting disk architecture..."
MOUNT_DIR=$(mktemp -d)
LOOP_DEVICE=$(sudo losetup -f --show -P "$DISK_IMG")

# Wait for partition device to appear
sleep 1

# Try to find and mount the partition with the kernel
KERNEL_FILE=""
for PART in "${LOOP_DEVICE}p1" "${LOOP_DEVICE}p2"; do
    if [ -e "$PART" ] && sudo mount "$PART" "$MOUNT_DIR" 2>/dev/null; then
        # Look for kernel in common locations
        KERNEL_FILE=$(sudo find "$MOUNT_DIR" -name "vmlinuz-*" -o -name "vmlinuz" 2>/dev/null | head -n 1)
        if [ -n "$KERNEL_FILE" ]; then
            echo "==> Found kernel: $KERNEL_FILE"
            break
        fi
        sudo umount "$MOUNT_DIR" 2>/dev/null
    fi
done

if [ -z "$KERNEL_FILE" ]; then
    echo "Error: Could not find kernel image in disk partitions"
    sudo losetup -d "$LOOP_DEVICE"
    rmdir "$MOUNT_DIR"
    exit 1
fi

ARCH_INFO=$(sudo file "$KERNEL_FILE")
echo "==> Kernel info: $ARCH_INFO"

# Parse architecture
if echo "$ARCH_INFO" | grep -iq "x86-64\|x86_64"; then
    ARCH="x86_64"
    echo "==> Detected architecture: x86_64"
elif echo "$ARCH_INFO" | grep -iq "aarch64\|ARM aarch64\|ARM64"; then
    ARCH="aarch64"
    echo "==> Detected architecture: aarch64"
else
    echo "Error: Could not detect architecture from kernel: $ARCH_INFO"
    sudo umount "$MOUNT_DIR" 2>/dev/null
    sudo losetup -d "$LOOP_DEVICE"
    rmdir "$MOUNT_DIR"
    exit 1
fi

# Write architecture to file
echo "$ARCH" > disk-arch.txt
echo "==> Architecture saved to disk-arch.txt"

# Cleanup
sudo umount "$MOUNT_DIR"
sudo losetup -d "$LOOP_DEVICE"
rmdir "$MOUNT_DIR"

echo "==> Done! disk.img is ready."
touch image-downloaded.flag

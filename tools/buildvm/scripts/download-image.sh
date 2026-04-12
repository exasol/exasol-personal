#!/usr/bin/env bash
set -euo pipefail

IMAGE_URL="$1"
TEMP_QCOW2="alpine-cloud.qcow2"
CACHED_IMAGE="alpine-pristine.img"
DISK_IMG="disk.img"

# Check if cached pristine image exists
if [ -f "$CACHED_IMAGE" ]; then
    echo "==> Using cached image: $CACHED_IMAGE"
    
    # Check if architecture file exists, detect if not
    if [ ! -f "disk-arch.txt" ]; then
        echo "==> Detecting architecture from cached image..."
        MOUNT_DIR=$(mktemp -d)
        LOOP_DEVICE=$(sudo losetup -f --show -P "$CACHED_IMAGE")
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
        
        if echo "$ARCH_INFO" | grep -iq "x86-64\|x86_64"; then
            ARCH="x86_64"
        elif echo "$ARCH_INFO" | grep -iq "aarch64\|ARM aarch64"; then
            ARCH="aarch64"
        else
            echo "Error: Could not detect architecture: $ARCH_INFO"
            sudo umount "$MOUNT_DIR" 2>/dev/null
            sudo losetup -d "$LOOP_DEVICE"
            rmdir "$MOUNT_DIR"
            exit 1
        fi
        
        echo "$ARCH" > disk-arch.txt
        echo "==> Detected architecture: $ARCH"
        
        sudo umount "$MOUNT_DIR"
        sudo losetup -d "$LOOP_DEVICE"
        rmdir "$MOUNT_DIR"
    else
        echo "==> Using existing disk-arch.txt: $(cat disk-arch.txt)"
    fi
    
    echo "==> Copying to $DISK_IMG..."
    cp "$CACHED_IMAGE" "$DISK_IMG"
    echo "==> Done! disk.img is ready."
    exit 0
fi

# Download and convert if cache doesn't exist
echo "==> Downloading NoCloud image from Alpine servers..."
wget -O "$TEMP_QCOW2" "$IMAGE_URL"

echo "==> Converting to raw disk image..."
qemu-img convert -f qcow2 -O raw "$TEMP_QCOW2" "$CACHED_IMAGE"

echo "==> Cleaning up qcow2 file..."
rm "$TEMP_QCOW2"

echo "==> Copying pristine image to $DISK_IMG..."
cp "$CACHED_IMAGE" "$DISK_IMG"

echo "==> Detecting disk architecture..."
MOUNT_DIR=$(mktemp -d)
LOOP_DEVICE=$(sudo losetup -f --show -P "$CACHED_IMAGE")

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
elif echo "$ARCH_INFO" | grep -iq "aarch64\|ARM aarch64"; then
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

echo "==> Done! disk.img is ready (cached copy saved as $CACHED_IMAGE)."

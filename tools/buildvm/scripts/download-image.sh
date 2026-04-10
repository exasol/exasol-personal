#!/usr/bin/env bash
set -euo pipefail

IMAGE_URL="$1"
TEMP_QCOW2="alpine-cloud.qcow2"
CACHED_IMAGE="alpine-pristine.img"
DISK_IMG="disk.img"

# Check if cached pristine image exists
if [ -f "$CACHED_IMAGE" ]; then
    echo "==> Using cached image: $CACHED_IMAGE"
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

echo "==> Done! disk.img is ready (cached copy saved as $CACHED_IMAGE)."

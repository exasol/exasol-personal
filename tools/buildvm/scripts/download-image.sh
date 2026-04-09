#!/usr/bin/env bash
set -euo pipefail

IMAGE_URL="$1"
IMAGE_FILE="alpine-cloud.qcow2"

echo "==> Downloading NoCloud image..."
wget -O "$IMAGE_FILE" "$IMAGE_URL"

echo "==> Converting to raw disk image..."
qemu-img convert -f qcow2 -O raw "$IMAGE_FILE" disk.img

echo "==> Cleaning up qcow2 file..."
rm "$IMAGE_FILE"

echo "==> Done! disk.img is ready."

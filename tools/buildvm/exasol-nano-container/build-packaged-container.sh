#!/bin/bash
set -euo pipefail

# Pull exasol/nano image and package it as a distributable tarball
# for loading into the buildvm Alpine VM.

IMAGE="exasol/nano:latest"
OUTPUT_DIR="dist"
ARCHIVE_NAME="exasol-nano.tar.gz"

echo "==> Pulling $IMAGE..."
podman pull "$IMAGE"

echo "==> Saving image to $OUTPUT_DIR/$ARCHIVE_NAME..."
mkdir -p "$OUTPUT_DIR"
podman save "$IMAGE" | gzip > "$OUTPUT_DIR/$ARCHIVE_NAME"

echo "==> Copying manifest..."
cp container-manifest.json "$OUTPUT_DIR/container-manifest.json"

SIZE=$(du -h "$OUTPUT_DIR/$ARCHIVE_NAME" | cut -f1)
echo "==> Done. Package: $OUTPUT_DIR/$ARCHIVE_NAME ($SIZE)"

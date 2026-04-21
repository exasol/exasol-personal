#!/bin/bash
set -euo pipefail

# Pull exasol/nano image and package it as a distributable tarball
# for loading into the buildvm Alpine VM.

IMAGE="docker.io/exasol/nano:latest"
OUTPUT_DIR="dist"
ARCHIVE_NAME="exasol-nano.tar.gz"
ARCH_FILE="../disk-arch.txt"
DEFAULT_PLATFORM="linux/arm64"

# Resolve target container platform. Priority: $PLATFORM env > disk-arch.txt > default.
if [ -z "${PLATFORM:-}" ]; then
    if [ -f "$ARCH_FILE" ]; then
        case "$(cat "$ARCH_FILE")" in
            aarch64) PLATFORM="linux/arm64" ;;
            x86_64)  PLATFORM="linux/amd64" ;;
            *)
                echo "Error: unsupported architecture in $ARCH_FILE: $(cat "$ARCH_FILE")"
                exit 1
                ;;
        esac
    else
        PLATFORM="$DEFAULT_PLATFORM"
    fi
fi

echo "==> Pulling $IMAGE ($PLATFORM)..."
podman pull --platform "$PLATFORM" "$IMAGE"

echo "==> Saving image to $OUTPUT_DIR/$ARCHIVE_NAME..."
mkdir -p "$OUTPUT_DIR"
podman save "$IMAGE" | gzip > "$OUTPUT_DIR/$ARCHIVE_NAME"

echo "==> Copying manifest..."
cp container-manifest.json "$OUTPUT_DIR/container-manifest.json"

SIZE=$(du -h "$OUTPUT_DIR/$ARCHIVE_NAME" | cut -f1)
echo "==> Done. Package: $OUTPUT_DIR/$ARCHIVE_NAME ($SIZE)"

#!/usr/bin/env bash
set -euo pipefail

# Fetch the Exasol Nano linux.run artifact and entrypoint script
# from exasol-labs/exasol-nano GitHub releases.

REPO="exasol-labs/exasol-nano"
OUTPUT_DIR="dist"

# Determine architecture — prefer argument, fall back to disk-arch.txt, then host
ARCH="${1:-}"
if [ -z "$ARCH" ]; then
  if [ -f "../disk-arch.txt" ]; then
    ARCH=$(cat "../disk-arch.txt")
  else
    HOST_ARCH=$(uname -m)
    case "$HOST_ARCH" in
      x86_64|amd64) ARCH="x86_64" ;;
      aarch64|arm64) ARCH="aarch64" ;;
      *) echo "Unsupported architecture: $HOST_ARCH"; exit 1 ;;
    esac
  fi
fi

echo "==> Target architecture: $ARCH"

# Get latest release tag
echo "==> Fetching latest release info..."
TAG=$(gh release view --repo "$REPO" --json tagName --jq '.tagName')
echo "==> Latest release: $TAG"

# Find the matching .run asset for this arch
ASSET_NAME=$(gh release view --repo "$REPO" --json assets --jq ".assets[] | select(.name | endswith(\"${ARCH}.run\")) | .name")
if [ -z "$ASSET_NAME" ]; then
  echo "Error: no .run asset found for arch $ARCH in release $TAG"
  exit 1
fi
echo "==> Downloading $ASSET_NAME..."

mkdir -p "$OUTPUT_DIR"
gh release download "$TAG" --repo "$REPO" --pattern "$ASSET_NAME" --dir "$OUTPUT_DIR" --clobber

# Rename to a stable filename
mv "$OUTPUT_DIR/$ASSET_NAME" "$OUTPUT_DIR/db.run"
chmod +x "$OUTPUT_DIR/db.run"
echo "==> Saved: $OUTPUT_DIR/db.run"

# Fetch entrypoint.sh from the repo (private repo — use gh api)
echo "==> Fetching entrypoint.sh from $REPO@$TAG..."
if gh api "repos/${REPO}/contents/artifacts/common/entrypoint.sh?ref=${TAG}" --jq '.content' 2>/dev/null | base64 -d > "$OUTPUT_DIR/entrypoint.sh" && [ -s "$OUTPUT_DIR/entrypoint.sh" ]; then
  chmod +x "$OUTPUT_DIR/entrypoint.sh"
  echo "==> Saved: $OUTPUT_DIR/entrypoint.sh"
else
  echo "Warning: Could not fetch entrypoint.sh from release tag. Using repo's main branch..."
  gh api "repos/${REPO}/contents/artifacts/common/entrypoint.sh?ref=main" --jq '.content' | base64 -d > "$OUTPUT_DIR/entrypoint.sh"
  chmod +x "$OUTPUT_DIR/entrypoint.sh"
  echo "==> Saved: $OUTPUT_DIR/entrypoint.sh (from main)"
fi

SIZE=$(du -h "$OUTPUT_DIR/db.run" | cut -f1)
echo "==> Done. Package: $OUTPUT_DIR/db.run ($SIZE)"

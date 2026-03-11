#!/usr/bin/env bash
set -euo pipefail

S3_URL="x-up.s3.eu-west-1.amazonaws.com"
BASE_URL="https://$S3_URL/releases/exasol-personal"

# Detect OS and architecture
UNAME_S=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Normalize architecture
case "$ARCH" in
    x86_64|amd64)
        ARCH="x86_64"
        ;;
    arm64|aarch64)
        ARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

DOWNLOAD_FILENAME="exasol"

case "$UNAME_S" in
    darwin)
        OS="macos"
        ;;
    linux)
        OS="linux"
        ;;
    *)
        echo "Unsupported OS: $UNAME_S"
        echo "This installer supports macos and Linux only."
        exit 1
        ;;
esac

DOWNLOAD_DIR="$(pwd)"
DOWNLOAD_PATH="$DOWNLOAD_DIR/$DOWNLOAD_FILENAME"
PACKAGE_DOWNLOAD_URL="$BASE_URL/$OS/$ARCH/latest/$DOWNLOAD_FILENAME"

echo "Detected OS: $OS"
echo "Detected Architecture: $ARCH"
echo "Downloading Exasol Personal binary..."

if ! curl -fSL --progress-bar "$PACKAGE_DOWNLOAD_URL" -o "$DOWNLOAD_PATH"; then
    echo "Error: Failed to download from $PACKAGE_DOWNLOAD_URL"
    exit 1
fi

chmod +x "$DOWNLOAD_PATH"

echo
echo "Download complete!"
echo

cat <<EOF
Next steps:
  1. Create a new deployment directory:
       mkdir -p deployment && cd deployment

  2. Setup AWS Profile - Refer https://docs.exasol.com/db/latest/get_started/exasol_personal_aws_setup.htm

  3. Run the installer from the deployment directory:
       ../exasol install <infra preset name-or-path> [install preset name-or-path]

Where:
  <infra preset name-or-path>   Infrastructure preset to use (e.g., aws)
  [install preset name-or-path] Optional installation preset (e.g., rootless)

Examples:
  ../exasol install aws
  ../exasol install aws rootless
EOF

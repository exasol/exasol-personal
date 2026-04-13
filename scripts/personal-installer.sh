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

INSTALL_DIR="$HOME/.local/bin"
DOWNLOAD_PATH="$INSTALL_DIR/$DOWNLOAD_FILENAME"
PACKAGE_DOWNLOAD_URL="$BASE_URL/$OS/$ARCH/latest/$DOWNLOAD_FILENAME"

echo "Detected OS: $OS"
echo "Detected Architecture: $ARCH"
echo "Installing Exasol Personal binary to $INSTALL_DIR..."

mkdir -p "$INSTALL_DIR"

if ! curl -fSL --progress-bar "$PACKAGE_DOWNLOAD_URL" -o "$DOWNLOAD_PATH"; then
    echo "Error: Failed to download from $PACKAGE_DOWNLOAD_URL"
    exit 1
fi

chmod +x "$DOWNLOAD_PATH"

echo
echo "Installation complete!"
echo

case ":$PATH:" in
    *":$INSTALL_DIR:"*)
        ;;
    *)
        echo "  $INSTALL_DIR is not in your PATH. Add this to your shell config:"
        echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
        echo
        echo "  For zsh:  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc"
        echo "  For bash: echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"
        echo
        ;;
esac

cat <<EOF
Next steps:
  1. Create a new deployment directory:
       mkdir -p deployment && cd deployment

  2. Setup AWS Profile - Refer https://docs.exasol.com/db/latest/get_started/exasol_personal_aws_setup.htm

  3. Run the installer from the deployment directory:
       exasol install <infra preset name-or-path> [install preset name-or-path]

Where:
  <infra preset name-or-path>   Infrastructure preset to use (e.g., aws)
  [install preset name-or-path] Optional installation preset (e.g., ubuntu)

Examples:
  exasol install aws
  exasol install aws ubuntu
EOF

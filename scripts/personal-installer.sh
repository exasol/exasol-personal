#!/bin/sh
set -eu

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
INSTALL_PATH="$INSTALL_DIR/$DOWNLOAD_FILENAME"
PACKAGE_DOWNLOAD_URL="$BASE_URL/$OS/$ARCH/latest/$DOWNLOAD_FILENAME"

echo "Detected OS: $OS"
echo "Detected Architecture: $ARCH"
echo "Installing the Exasol Launcher (the 'exasol' command) to $INSTALL_DIR..."

mkdir -p "$INSTALL_DIR"

if ! curl -fSL --proto '=https' --progress-bar "$PACKAGE_DOWNLOAD_URL" -o "$INSTALL_PATH"; then
    echo "Error: Failed to download from $PACKAGE_DOWNLOAD_URL" >&2
    exit 1
fi

chmod +x "$INSTALL_PATH"

echo
echo "Installation complete!"
echo
echo "The Exasol Launcher ('exasol') is the command-line tool that deploys and"
echo "manages your Exasol databases. It was installed to:"
echo "    $INSTALL_PATH"
echo

# Make sure the install dir is on PATH so 'exasol' can be run from anywhere.
case ":$PATH:" in
    *":$INSTALL_DIR:"*)
        echo "$INSTALL_DIR is on your PATH — you're ready to go."
        ;;
    *)
        echo "$INSTALL_DIR is NOT on your PATH yet. Add it so you can run 'exasol' from anywhere:"
        echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
        echo
        echo "  For zsh:  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc"
        echo "  For bash: echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"
        echo "  Then restart your shell."
        ;;
esac
echo

if [ "$OS" = "macos" ]; then
cat <<EOF
Getting started:

  Run Exasol locally on your Mac — ready in seconds:
      exasol install local

  Or deploy to your own cloud (AWS, Azure, Exoscale, STACKIT), e.g.:
      exasol install aws

  Then see how to connect, and open a SQL shell:
      exasol info
      exasol connect

Full documentation and all options:
  https://github.com/exasol/exasol-personal
EOF
else
cat <<EOF
Getting started:

  Deploy Exasol to your own cloud (AWS, Azure, Exoscale, STACKIT), e.g.:
      exasol install aws

  Then see how to connect, and open a SQL shell:
      exasol info
      exasol connect

  (Local deployment is currently macOS only; Windows and Linux support is coming soon.)

Full documentation and all options:
  https://github.com/exasol/exasol-personal
EOF
fi

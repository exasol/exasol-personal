#!/usr/bin/env bash
set -euo pipefail

SHARED_DIR="shared"

echo "==> Cleaning shared folder..."

# Remove everything - start-vm recreates what it needs
if [ -d "$SHARED_DIR" ]; then
    rm -rf "$SHARED_DIR"/*
    echo "==> Shared folder cleaned (empty)"
else
    echo "==> Shared folder doesn't exist, nothing to clean"
fi

echo "==> Shared folder ready for start-vm"
echo "    - start-vm will recreate authorized_keys"
echo "    - load-db will recreate logs/ as needed"

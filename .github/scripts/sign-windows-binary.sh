#!/bin/bash
set -euo pipefail

# Script to sign Windows binaries using SSL.com CodeSignTool
# Called from GoReleaser post-build hook

BINARY_PATH="$1"

if [[ -z "$BINARY_PATH" ]]; then
    echo "Usage: $0 <binary_path>"
    exit 1
fi

# Check if this is a Windows binary
if [[ "$BINARY_PATH" != *.exe ]]; then
    echo "Not a Windows binary, skipping signing: $BINARY_PATH"
    exit 0
fi

echo "Signing Windows binary: $BINARY_PATH"

# Verify required environment variables
if [[ -z "$SSLCOM_USERNAME" ]] || [[ -z "$SSLCOM_PASSWORD" ]] || [[ -z "$SSLCOM_TOTP_SECRET" ]]; then
    echo "ERROR: SSL.com credentials not set in environment"
    echo "Required: SSLCOM_USERNAME, SSLCOM_PASSWORD, SSLCOM_TOTP_SECRET"
    exit 1
fi

# Convert to absolute path since we'll change directory
BINARY_PATH=$(realpath "$BINARY_PATH")

# Change to CodeSignTool directory (required for relative jar paths to work)
cd /tmp/codesigntool

# Build the command using a bash array to properly handle all special characters
# This is the safest way - bash arrays preserve each argument exactly as-is,
# preventing any shell interpretation of special characters in the password
CMD=(
    ./CodeSignTool.sh
    sign
    "-username=$SSLCOM_USERNAME"
    "-password=$SSLCOM_PASSWORD"
    "-totp_secret=$SSLCOM_TOTP_SECRET"
    "-input_file_path=$BINARY_PATH"
    "-override=true"
)

# Add credential_id only if it's set
if [[ -n "$SSLCOM_CREDENTIAL_ID" ]]; then
    CMD+=("-credential_id=$SSLCOM_CREDENTIAL_ID")
fi

# Execute the command - the array expansion ensures each element is a separate argument
# No matter what special characters are in the password, they won't be interpreted
"${CMD[@]}"

echo "Successfully signed: $BINARY_PATH"

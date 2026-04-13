#!/bin/sh
# Import SSH keys from shared folder
# Security: REPLACES all existing keys with keys from shared folder
SHARED_KEYS="/mnt/host/authorized_keys"
USER_KEYS="/home/exasol/.ssh/authorized_keys"

# Exit if shared folder not mounted or no keys file
[ ! -f "$SHARED_KEYS" ] && exit 0

# Create .ssh directory if it doesn't exist
mkdir -p /home/exasol/.ssh
chmod 700 /home/exasol/.ssh

# SECURITY: Clear existing keys - only keys in shared folder will have access
> "$USER_KEYS"

# Import all keys from shared folder
while IFS= read -r key; do
  # Skip empty lines and comments
  [ -z "$key" ] && continue
  echo "$key" | grep -q "^#" && continue
  
  # Add key
  echo "$key" >> "$USER_KEYS"
  logger -t import-shared-keys "Added SSH key: ${key%% *}..."
done < "$SHARED_KEYS"

# Set correct permissions
chmod 600 "$USER_KEYS"
chown exasol:exasol "$USER_KEYS"
chown exasol:exasol /home/exasol/.ssh

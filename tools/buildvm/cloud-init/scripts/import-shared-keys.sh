#!/bin/sh
# Import SSH keys from shared folder
SHARED_KEYS="/mnt/host/authorized_keys"
USER_KEYS="/home/alpine/.ssh/authorized_keys"

# Exit if shared folder not mounted or no keys file
[ ! -f "$SHARED_KEYS" ] && exit 0

# Create .ssh directory if it doesn't exist
mkdir -p /home/alpine/.ssh
chmod 700 /home/alpine/.ssh

# Import keys that aren't already present
while IFS= read -r key; do
  # Skip empty lines and comments
  [ -z "$key" ] && continue
  echo "$key" | grep -q "^#" && continue
  
  # Add key if not already present
  if ! grep -qF "$key" "$USER_KEYS" 2>/dev/null; then
    echo "$key" >> "$USER_KEYS"
    logger -t import-shared-keys "Added SSH key: ${key%% *}..."
  fi
done < "$SHARED_KEYS"

# Set correct permissions
chmod 600 "$USER_KEYS"
chown alpine:alpine "$USER_KEYS"
chown alpine:alpine /home/alpine/.ssh

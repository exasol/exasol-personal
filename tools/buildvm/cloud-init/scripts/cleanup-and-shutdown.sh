#!/bin/sh
set -e

CONTAINER_NAME="container"

SQL_PORT=8563

echo "==> Waiting for database to accept connections on port $SQL_PORT..."
MAX_WAIT=600
ELAPSED=0
DB_READY=false
while [ $ELAPSED -lt $MAX_WAIT ]; do
  if nc -z -w 1 127.0.0.1 $SQL_PORT 2>/dev/null; then
    DB_READY=true
    break
  fi
  sleep 5
  ELAPSED=$((ELAPSED + 5))
  if [ $((ELAPSED % 30)) -eq 0 ]; then
    echo "==> Still waiting for database... (${ELAPSED}s / ${MAX_WAIT}s)"
  fi
done

if [ "$DB_READY" = "true" ]; then
  echo "==> Database is fully started. Stopping container cleanly..."
  podman stop -t 30 "$CONTAINER_NAME"
  echo "==> Container stopped. Filesystem state preserved for fast restart."
else
  echo "==> Warning: Database did not report ready within ${MAX_WAIT}s"
  echo "==> Stopping container anyway..."
  podman stop -t 10 "$CONTAINER_NAME" 2>/dev/null || true
fi

echo "==> Signaling completion..."
touch /mnt/host/cloud-init-complete

echo "==> Disabling cloud-init services (one-time setup complete)..."
rc-update del cloud-init-local boot 2>/dev/null || true
rc-update del cloud-init default 2>/dev/null || true
rc-update del cloud-config default 2>/dev/null || true
rc-update del cloud-final default 2>/dev/null || true
touch /etc/cloud/cloud-init.disabled

echo "==> Cleaning up system..."
apk cache clean
rm -rf /var/cache/apk/*
find /var/log -type f -exec truncate -s 0 {} \;

echo "==> Zeroing free space..."
dd if=/dev/zero of=/zerofill bs=1M || true
sync
rm -f /zerofill
sync

echo "==> Shutting down..."
poweroff

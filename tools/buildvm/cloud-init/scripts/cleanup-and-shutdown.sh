#!/bin/sh
set -e

SQL_PORT=8563
DB_PID_FILE=/var/run/exasol-db.pid

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
  echo "==> Database is fully started. Stopping it cleanly..."
  # entrypoint.sh handles graceful shutdown via SIGTERM
  if [ -f "$DB_PID_FILE" ]; then
    DB_PID=$(cat "$DB_PID_FILE")
    kill -TERM "$DB_PID" 2>/dev/null || true
    # Wait up to 60s for graceful shutdown
    for i in $(seq 1 60); do
      kill -0 "$DB_PID" 2>/dev/null || break
      sleep 1
    done
    kill -KILL "$DB_PID" 2>/dev/null || true
    rm -f "$DB_PID_FILE"
  fi
  echo "==> Database stopped. DB files preserved on disk for fast restart."
else
  echo "==> Warning: Database did not report ready within ${MAX_WAIT}s"
  echo "==> Stopping DB anyway..."
  [ -f "$DB_PID_FILE" ] && kill -KILL "$(cat "$DB_PID_FILE")" 2>/dev/null || true
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

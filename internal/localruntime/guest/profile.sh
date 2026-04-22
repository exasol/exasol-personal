#!/bin/sh
set -eu

ENTRYPOINT_SOURCE="/.exanano/provision/exasol-localruntime-entrypoint.sh"
ENTRYPOINT_TARGET="/entrypoint.sh"
PAYLOAD_SOURCE="/.exanano/payload/db.run"
PAYLOAD_TARGET="/db.run"

if [ ! -f "$ENTRYPOINT_SOURCE" ]; then
  echo "Guest bootstrap entrypoint missing: $ENTRYPOINT_SOURCE" >&2
  exit 1
fi

if [ ! -f "$PAYLOAD_SOURCE" ]; then
  echo "Expected Linux .run payload not found: $PAYLOAD_SOURCE" >&2
  exit 1
fi

mkdir -p /.exanano/control /.exanano/logs
cp "$ENTRYPOINT_SOURCE" "$ENTRYPOINT_TARGET"
chmod 0755 "$ENTRYPOINT_TARGET"
chmod 0755 "$PAYLOAD_SOURCE" 2>/dev/null || true
ln -sfn "$PAYLOAD_SOURCE" "$PAYLOAD_TARGET"
rm -f /entrypoint.exasol-localruntime-original.sh 2>/dev/null || true

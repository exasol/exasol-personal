#!/bin/sh
# Launcher-authored boot hook executed by the guest's exasol-bootstrap OpenRC
# service from /mnt/host/start.sh.
#
# Responsibilities:
#   - Run the launcher-staged Exasol installer at /mnt/host/db.run.
#   - Detach the database process so the OpenRC service exits cleanly while
#     the database keeps running.

set -e

LOG_DIR=/var/log
PID_FILE=/var/run/exasol-db.pid
RUN=/mnt/host/db.run

export HOME="${HOME:-/root}"

if [ ! -x "$RUN" ]; then
  echo "start.sh: $RUN not found or not executable" >&2
  exit 1
fi

mkdir -p "$LOG_DIR"

# Detach so OpenRC's load script returns immediately; the database keeps
# running. The DB's controller reads from stdin and treats EOF as a
# clean-shutdown signal (Ctrl+D). Redirecting from /dev/null still produces
# EOF on read. Pipe `tail -f /dev/null` instead — its write end of the pipe
# stays open forever, so the DB blocks on read indefinitely without seeing
# EOF. The subshell groups tail+db.run so the recorded PID covers the pair.
( tail -f /dev/null | "$RUN" >> "$LOG_DIR/exasol-db.log" 2>&1 ) &
echo $! > "$PID_FILE"

#!/bin/sh
# Load and run Exasol Nano linux.run directly in the VM.
# Replaces the podman-based load-shared-container.sh.

# Ensure we run as root (OpenRC services run as root; cloud-init runcmd too)
if [ "$(id -u)" -ne 0 ]; then
  echo "Error: load-db.sh must be run as root"
  exit 1
fi

SHARED_DIR="/mnt/host"
INSTALL_DIR="/opt/exasol"
EXA_DIR="/exa"
LOG_DIR="$SHARED_DIR/logs"
LOG_FILE="$LOG_DIR/db-load-$(date +%Y%m%d-%H%M%S).log"
STATE_FILE="/var/lib/db-state.sha256"

SHARED_RUN="$SHARED_DIR/db.run"
SHARED_ENTRYPOINT="$SHARED_DIR/entrypoint.sh"
INSTALLED_RUN="$INSTALL_DIR/db.run"
INSTALLED_ENTRYPOINT="$INSTALL_DIR/entrypoint.sh"

mkdir -p "$LOG_DIR" "$INSTALL_DIR" "$EXA_DIR"

log_msg() {
  local msg="$1"
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $msg" | tee -a "$LOG_FILE"
  logger -t load-db "$msg"
}

exec 1> >(tee -a "$LOG_FILE")
exec 2>&1

log_msg "=== DB Load Script Started ==="

# Check for new db.run in shared folder (update path)
RELOAD_NEEDED=false
if [ -f "$SHARED_RUN" ]; then
  CURRENT_SHA=$(sha256sum "$SHARED_RUN" | cut -d' ' -f1)
  PREVIOUS_SHA=""
  [ -f "$STATE_FILE" ] && PREVIOUS_SHA=$(cat "$STATE_FILE")

  if [ "$CURRENT_SHA" != "$PREVIOUS_SHA" ]; then
    log_msg "New db.run detected in shared folder (checksum changed)"
    RELOAD_NEEDED=true
  else
    log_msg "db.run in shared folder unchanged, using installed copy"
  fi
fi

# Install/update db.run and entrypoint.sh from shared folder
if [ "$RELOAD_NEEDED" = "true" ]; then
  log_msg "Installing db.run to $INSTALLED_RUN..."
  cp "$SHARED_RUN" "$INSTALLED_RUN"
  chmod +x "$INSTALLED_RUN"
  echo "$CURRENT_SHA" > "$STATE_FILE"

  if [ -f "$SHARED_ENTRYPOINT" ]; then
    cp "$SHARED_ENTRYPOINT" "$INSTALLED_ENTRYPOINT"
    chmod +x "$INSTALLED_ENTRYPOINT"
    log_msg "Installed entrypoint.sh"
  fi
fi

if [ ! -x "$INSTALLED_RUN" ]; then
  log_msg "Error: db.run not installed at $INSTALLED_RUN"
  exit 1
fi

if [ ! -x "$INSTALLED_ENTRYPOINT" ]; then
  log_msg "Error: entrypoint.sh not installed at $INSTALLED_ENTRYPOINT"
  exit 1
fi

# Check if DB is already running
if pgrep -f "exasol-nano-db|db.run|controller" > /dev/null; then
  log_msg "Database already running, nothing to do"
  log_msg "=== DB Load Script Completed Successfully ==="
  exit 0
fi

# Start the DB via entrypoint.sh in the background.
# entrypoint.sh expects DB_RUN at /db.run — create a symlink.
ln -sf "$INSTALLED_RUN" /db.run

log_msg "Starting database via entrypoint.sh..."
RUNTIME_LOG="$LOG_DIR/db-runtime-$(date +%Y%m%d-%H%M%S).log"

# Ensure HOME is set — the DB .run script requires it
export HOME="${HOME:-/root}"

# Launch DB via entrypoint.sh (runs detached, manages restart policy)
EXANANO_EXADIR="$EXA_DIR" HOME="$HOME" nohup "$INSTALLED_ENTRYPOINT" > "$RUNTIME_LOG" 2>&1 &
DB_PID=$!
echo "$DB_PID" > /var/run/exasol-db.pid

log_msg "Database started (PID: $DB_PID)"
log_msg "Runtime log: $RUNTIME_LOG"
log_msg "=== DB Load Script Completed Successfully ==="

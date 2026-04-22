#!/bin/sh
set -eu

CONTROL_DIR="/.exanano/control"
RUNTIME_STATE_PATH="$CONTROL_DIR/runtime.state"
ENTRYPOINT_PID_PATH="$CONTROL_DIR/exanano.pid"
DB_RUN="/db.run"

RESTART_POLICY="always"
SHUTDOWN_REQUESTED=0
DB_PID=""
DB_STDIN_FWD_PID=""
DB_STDIN_FIFO=""
DB_STDIN_EOF_WATCH_PID=""
SHUTDOWN_ESCALATOR_PID=""
DB_INITIAL_SCRIPT_LANGUAGES="${EXANANO_INITIAL_SCRIPT_LANGUAGES:-R=builtin_r:JAVA=builtin_java:PYTHON3=builtin_python3:RUST=builtin_rust}"
DB_SANDBOX_PATH=""
ENTRYPOINT_SELF_PID="$$"
exec 8<&0

sql_port="8563"
ui_port="8443"
jupyter_enabled="0"
jupyter_port="8888"
voila_port="8866"

cleanup_stdin_forwarder() {
  cleanup_stdin_eof_watcher
  if [ -n "$DB_STDIN_FWD_PID" ]; then
    kill "$DB_STDIN_FWD_PID" 2>/dev/null || true
    wait "$DB_STDIN_FWD_PID" 2>/dev/null || true
    DB_STDIN_FWD_PID=""
  fi
  if [ -n "$DB_STDIN_FIFO" ]; then
    rm -f "$DB_STDIN_FIFO" 2>/dev/null || true
    DB_STDIN_FIFO=""
  fi
}

cleanup_stdin_eof_watcher() {
  if [ -n "$DB_STDIN_EOF_WATCH_PID" ]; then
    kill "$DB_STDIN_EOF_WATCH_PID" 2>/dev/null || true
    wait "$DB_STDIN_EOF_WATCH_PID" 2>/dev/null || true
    DB_STDIN_EOF_WATCH_PID=""
  fi
}

cleanup_shutdown_escalator() {
  if [ -n "$SHUTDOWN_ESCALATOR_PID" ]; then
    kill "$SHUTDOWN_ESCALATOR_PID" 2>/dev/null || true
    wait "$SHUTDOWN_ESCALATOR_PID" 2>/dev/null || true
    SHUTDOWN_ESCALATOR_PID=""
  fi
}

cleanup_shm() {
  if [ ! -d /dev/shm ]; then
    return 0
  fi

  find /dev/shm -mindepth 1 -maxdepth 1 -exec rm -rf {} + 2>/dev/null || true
}

db_controller_running() {
  for cmdline in /proc/[0-9]*/cmdline; do
    [ -r "$cmdline" ] || continue
    cmd="$(tr '\0' ' ' < "$cmdline" 2>/dev/null || true)"
    case "$cmd" in
      *pddserver*|*objectserver*|*exasqllog*|*loaderd*|*exacs*|*EXASolution_DB*)
        return 0
        ;;
    esac
  done
  return 1
}

wait_for_db_controller_exit() {
  timeout_s="${1:-60}"
  waited=0

  while [ "$waited" -lt "$timeout_s" ]; do
    if ! db_controller_running; then
      echo "Database controller shutdown completed."
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done

  echo "Warning: database controller still appears active after ${timeout_s}s, continuing shutdown..."
  return 1
}

start_shutdown_escalator() {
  if [ -n "$SHUTDOWN_ESCALATOR_PID" ] || [ -z "$DB_PID" ]; then
    return 0
  fi

  (
    sleep 60
    if [ -n "$DB_PID" ] && kill -0 "$DB_PID" 2>/dev/null; then
      echo "Graceful shutdown timed out after 60s; sending SIGTERM to database (PID: $DB_PID)..."
      kill -TERM "$DB_PID" 2>/dev/null || true
      sleep 10
      if [ -n "$DB_PID" ] && kill -0 "$DB_PID" 2>/dev/null; then
        echo "Database did not exit after SIGTERM grace period; sending SIGKILL (PID: $DB_PID)..."
        kill -KILL "$DB_PID" 2>/dev/null || true
      fi
    fi
  ) &
  SHUTDOWN_ESCALATOR_PID=$!
}

start_stdin_forwarder() {
  DB_STDIN_FIFO="$(mktemp -u /tmp/exanano-db-stdin.XXXXXX)"
  rm -f "$DB_STDIN_FIFO"
  mkfifo "$DB_STDIN_FIFO"

  stdin_path="$(readlink /proc/self/fd/8 2>/dev/null || true)"
  if [ "$stdin_path" = "/dev/null" ]; then
    tail -f /dev/null > "$DB_STDIN_FIFO" &
    DB_STDIN_FWD_PID=$!
  else
    cat <&8 > "$DB_STDIN_FIFO" &
    DB_STDIN_FWD_PID=$!
  fi
}

start_stdin_eof_watcher() {
  if [ -z "$DB_STDIN_FWD_PID" ] || [ -z "$DB_PID" ]; then
    return 0
  fi

  stdin_fwd_pid="$DB_STDIN_FWD_PID"
  db_pid="$DB_PID"
  (
    while kill -0 "$db_pid" 2>/dev/null; do
      if ! kill -0 "$stdin_fwd_pid" 2>/dev/null; then
        echo "Database stdin closed; requesting graceful shutdown..."
        kill -TERM "$ENTRYPOINT_SELF_PID" 2>/dev/null || true
        exit 0
      fi
      sleep 1
    done
  ) &
  DB_STDIN_EOF_WATCH_PID=$!
}

request_shutdown() {
  SHUTDOWN_REQUESTED=1
  start_shutdown_escalator
  if [ -n "$DB_STDIN_FWD_PID" ]; then
    echo "Shutdown requested; closing database stdin (graceful EOF)..."
    cleanup_stdin_forwarder
  else
    echo "Shutdown requested; waiting for database shutdown..."
  fi
}

detect_exasol_conf_path() {
  if [ -f "/.exanano/exasol.conf" ]; then
    echo "/.exanano/exasol.conf"
    return 0
  fi
  if [ -f "/exa/exasol.conf" ]; then
    echo "/exa/exasol.conf"
    return 0
  fi
  return 1
}

build_db_run_controller_args() {
  if [ -n "${EXANANO_SANDBOX_PATH:-}" ]; then
    DB_SANDBOX_PATH="${EXANANO_SANDBOX_PATH}"
    return 0
  fi

  EXASOL_CONF_PATH="$(detect_exasol_conf_path || true)"
  if [ -z "$EXASOL_CONF_PATH" ] || ! grep -Eq '^[[:space:]]*sandboxPath=' "$EXASOL_CONF_PATH"; then
    DB_SANDBOX_PATH="/usr/opt/mountjail"
  fi
}

ensure_sandbox_path() {
  if [ -z "$DB_SANDBOX_PATH" ]; then
    return 0
  fi
  if ! mkdir -p "$DB_SANDBOX_PATH"; then
    echo "ERROR: Failed to create sandbox path: $DB_SANDBOX_PATH"
    exit 1
  fi
}

configure_udf_ccache() {
  if [ -d /overlay-storage ]; then
    export EXANANO_UDF_CCACHE="/overlay-storage/.exanano/.ccache"
    export UDF_CCACHE="$EXANANO_UDF_CCACHE"
    mkdir -p "$EXANANO_UDF_CCACHE" 2>/dev/null || true
  fi
}

start_db_run() {
  set -- -- -concurrentConnections=20 "-initialScriptLanguages=${DB_INITIAL_SCRIPT_LANGUAGES}"
  if [ -n "$DB_SANDBOX_PATH" ]; then
    set -- "$@" "-sandboxPath=${DB_SANDBOX_PATH}"
  fi

  "$DB_RUN" "$@" < "$DB_STDIN_FIFO" &
  DB_PID=$!
}

write_runtime_state() {
  mkdir -p "$CONTROL_DIR"
  {
    echo "sql_port=$sql_port"
    echo "ui_port=$ui_port"
    echo "jupyter_enabled=$jupyter_enabled"
    echo "jupyter_port=$jupyter_port"
    echo "voila_port=$voila_port"
  } > "$RUNTIME_STATE_PATH"
  echo "$ENTRYPOINT_SELF_PID" > "$ENTRYPOINT_PID_PATH"
}

trap request_shutdown TERM INT

for token in $(cat /proc/cmdline 2>/dev/null); do
  case "$token" in
    exa_restart=*)
      RESTART_POLICY="${token#exa_restart=}"
      ;;
    exa_sql_port=*)
      sql_port="${token#exa_sql_port=}"
      ;;
    exa_ui_port=*)
      ui_port="${token#exa_ui_port=}"
      ;;
    exa_jupyter_enabled=*)
      jupyter_enabled="${token#exa_jupyter_enabled=}"
      ;;
    exa_jupyter_port=*)
      jupyter_port="${token#exa_jupyter_port=}"
      ;;
    exa_voila_port=*)
      voila_port="${token#exa_voila_port=}"
      ;;
  esac
done

if [ ! -x "$DB_RUN" ]; then
  echo "ERROR: Linux .run payload not found or not executable at $DB_RUN"
  exit 1
fi

build_db_run_controller_args
ensure_sandbox_path
configure_udf_ccache
write_runtime_state

RESTART_COUNT=0
MAX_RESTARTS=10
BACKOFF_DELAY=2

while true; do
  cleanup_shm
  echo "Starting database (attempt $((RESTART_COUNT + 1)))..."

  start_stdin_forwarder
  start_db_run
  start_stdin_eof_watcher

  set +e
  wait "$DB_PID"
  EXIT_CODE=$?
  set -e

  DB_PID=""
  cleanup_shutdown_escalator
  cleanup_stdin_forwarder

  if [ "$SHUTDOWN_REQUESTED" -eq 1 ]; then
    wait_for_db_controller_exit 60 || true
    echo "Database stopped after shutdown request, exiting entrypoint..."
    exit 0
  fi

  echo "Database exited with code $EXIT_CODE"

  case "$RESTART_POLICY" in
    never)
      echo "Restart policy is 'never', exiting..."
      exit "$EXIT_CODE"
      ;;
    on-failure)
      if [ "$EXIT_CODE" -eq 0 ]; then
        echo "Database exited successfully, restart policy is 'on-failure', exiting..."
        exit 0
      fi
      echo "Database failed (exit code $EXIT_CODE), restarting..."
      ;;
    always)
      echo "Restart policy is 'always', restarting..."
      ;;
    *)
      echo "Unknown restart policy: $RESTART_POLICY, defaulting to 'always'"
      ;;
  esac

  RESTART_COUNT=$((RESTART_COUNT + 1))
  if [ "$RESTART_COUNT" -ge "$MAX_RESTARTS" ]; then
    echo "ERROR: Maximum restart count ($MAX_RESTARTS) reached, giving up"
    exit 1
  fi

  echo "Waiting ${BACKOFF_DELAY}s before restart..."
  sleep "$BACKOFF_DELAY"
done

#!/usr/bin/env bash
# Streams progress for the unattended installation so the launcher can surface
# actionable feedback while the nodes work autonomously.
#
# Responsibilities:
# - Tail /var/log/cloud-init-output.log while blocking on cloud-init status so
#   EXASOL-* lines reach the launcher in real time.
# - Follow exasol_launcher*.service units via journalctl using a persisted
#   cursor, allowing resumable monitoring across SSH reconnects.
# - Poll critical systemd units and completion markers, emitting
#   EXASOL-* messages for failures, milestones, and heartbeat updates.
# - Ensure background tails and journalctl streams are cleaned up when the
#   script exits or the SSH session is terminated.

set -euo pipefail

# ============================================================================
# Setup and Dependencies
# ============================================================================

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./logging.sh
source "${SCRIPT_DIR}/logging.sh"

# ============================================================================
# Cloud-Init Monitoring
# ============================================================================

readonly CLOUD_INIT_LOG="/var/log/cloud-init-output.log"
readonly CLOUD_INIT_DONE_MARKER="/var/lib/exasol_launcher/state/cloud-init.complete"
CLOUD_INIT_TAIL_PID=""

start_cloud_init_tail() {
  if [[ -n "${CLOUD_INIT_TAIL_PID:-}" ]]; then
    return
  fi

  local wait_seconds=30
  while (( wait_seconds > 0 )) && [[ ! -f "$CLOUD_INIT_LOG" ]]; do
    sleep 1
    wait_seconds=$((wait_seconds - 1))
  done

  if [[ ! -f "$CLOUD_INIT_LOG" ]]; then
    log_substep_info "Cloud-init output log not found, skipping live tail"
    return
  fi

  # Tail from beginning since the log is typically short and contains valuable context
  tail -n +1 -F "$CLOUD_INIT_LOG" &
  CLOUD_INIT_TAIL_PID=$!
}

stop_cloud_init_tail() {
  if [[ -n "${CLOUD_INIT_TAIL_PID:-}" ]]; then
    if kill "$CLOUD_INIT_TAIL_PID" 2>/dev/null; then
      wait "$CLOUD_INIT_TAIL_PID" 2>/dev/null || true
    fi
    CLOUD_INIT_TAIL_PID=""
  fi
}

wait_for_cloud_init() {
  # Skip tailing if cloud-init already completed in a previous run
  if [[ -f "$CLOUD_INIT_DONE_MARKER" ]]; then
    log_step_info "Cloud-init - already completed"
    return
  fi
  
  log_step_info "Cloud-init - bootstrap started"
  
  if ! command -v cloud-init >/dev/null 2>&1; then
    log_substep_info "Cloud-init - client binary not present, skipping live log tail"
    log_step_info "Cloud-init - bootstrap completed"
    mkdir -p "$(dirname "$CLOUD_INIT_DONE_MARKER")"
    touch "$CLOUD_INIT_DONE_MARKER"
    return
  fi

  start_cloud_init_tail
  cloud-init status --wait || true
  stop_cloud_init_tail
  
  log_step_info "Cloud-init - bootstrap completed"    
}

# ============================================================================
# Journal Monitoring
# ============================================================================

CURSOR_FILE="/var/lib/exasol_launcher/state/journal.cursor"
JOURNAL_PID=""

# Systemd services tracked in dependency order
readonly LAUNCHER_SERVICES=(
  "exasol_launcher_prepare.service"
  "exasol_launcher_prepare_user.service"
  "exasol_launcher_node_barrier_server.service"
  "exasol_launcher_node_barrier_client.service"
  "exasol_launcher_install.service"
  "exasol_launcher_ready.service"
  "exasol_launcher_post_install.service"
)

stop_journalctl_tail() {
  if [[ -n "${JOURNAL_PID:-}" ]]; then
    if kill "$JOURNAL_PID" 2>/dev/null; then
      wait "$JOURNAL_PID" 2>/dev/null || true
    fi
    JOURNAL_PID=""
  fi
}

start_journalctl_tail() {
  mkdir -p "$(dirname "$CURSOR_FILE")"
  touch "$CURSOR_FILE"

  local cursor=""
  if [[ -s "$CURSOR_FILE" ]]; then
    cursor=$(<"$CURSOR_FILE")
  fi

  # Build resumable journal arguments with cursor persistence
  local journal_args=(
    "--no-pager"
    "--follow"
    "--utc"
    "--output=short-iso"
    "--cursor-file=$CURSOR_FILE"
  )

  if [[ -n "$cursor" ]]; then
    journal_args+=("--after-cursor=$cursor")
  else
    journal_args+=("--lines=0")
  fi

  # Track the aggregate target plus all services in the dependency chain
  journal_args+=("--unit" "exasol_launcher.target")
  for service in "${LAUNCHER_SERVICES[@]}"; do
    journal_args+=("--unit" "$service")
  done

  journalctl "${journal_args[@]}" &
  JOURNAL_PID=$!
}

# ============================================================================
# Installation Status Monitoring
# ============================================================================

readonly SUCCESS_MARKER="/var/lib/exasol_launcher/state/post_install.complete"

check_service_failure() {
  local service=$1
  local active_state
  active_state=$(systemctl show --value --property=ActiveState "$service" 2>/dev/null || echo "")
  
  if [[ "$active_state" != "failed" ]]; then
    return 0
  fi

  # Print detailed status before error summary
  # Note: systemctl status returns non-zero for failed units, but we handle this
  # explicitly to avoid triggering 'set -e' before logging the error summary
  systemctl status "$service" --no-pager 2>&1 | sed -n '1,20p' | while read -r line; do
    log_substep_info "$line"
  done || true

  log_error "Exasol installation failed!"
  log_error "------------------------------------"
  exit 1
}

check_target_failure() {
  local target_state
  target_state=$(systemctl show --value --property=ActiveState exasol_launcher.target 2>/dev/null || echo "")
  
  if [[ "$target_state" != "failed" ]]; then
    return 0
  fi

  # Print detailed status before error summary
  systemctl status exasol_launcher.target --no-pager 2>&1 | sed -n '1,25p' | while read -r line; do
    log_substep_info "$line"
  done || true

  log_error "Exasol installation failed!"
  log_error "------------------------------------"
  exit 1
}

check_installation_complete() {
  if [[ -f "$SUCCESS_MARKER" ]]; then
    log_step_info "Exasol installation completed successfully!"
    log_step_info "----------------------------------------------------"
    exit 0
  fi
}

monitor_installation_status() {
  log_step_info "Begin tracking installation status"

  while true; do
    # Check each service in the launcher dependency chain
    for service in "${LAUNCHER_SERVICES[@]}"; do
      check_service_failure "$service"
    done

    check_target_failure
    check_installation_complete

    log_substep_info "Continuing, please wait..."
    sleep 10
  done
}

# ============================================================================
# Cleanup and Signal Handling
# ============================================================================

cleanup() {
  stop_cloud_init_tail
  stop_journalctl_tail
}

trap cleanup EXIT

# ============================================================================
# Main Execution
# ============================================================================

main() {
  wait_for_cloud_init
  
  log_step_info "Begin tailing installation logs"
  start_journalctl_tail
  
  monitor_installation_status
}

main

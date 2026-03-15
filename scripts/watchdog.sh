#!/usr/bin/env bash
# watchdog.sh — ensures the goated daemon is always running.
# Add to crontab:  */2 * * * * /path/to/goated/scripts/watchdog.sh
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PID_FILE="$REPO_DIR/logs/goated_daemon.pid"
GOATED_BIN="$REPO_DIR/goated"
LOG_FILE="$REPO_DIR/logs/goated_daemon.log"
WATCHDOG_LOG="$REPO_DIR/logs/watchdog.log"

log() {
    echo "[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*" >> "$WATCHDOG_LOG"
}

# Check if goated binary exists
if [ ! -x "$GOATED_BIN" ]; then
    log "ERROR: goated binary not found at $GOATED_BIN"
    exit 1
fi

# Check if daemon is running
daemon_running=false
if [ -f "$PID_FILE" ]; then
    pid=$(cat "$PID_FILE" 2>/dev/null || true)
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        daemon_running=true
    fi
fi

if $daemon_running; then
    exit 0
fi

# Daemon is not running — start it
log "Daemon not running, starting it"
cd "$REPO_DIR"
output=$($GOATED_BIN daemon run 2>&1) || true
log "Started daemon: $output"

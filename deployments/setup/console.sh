#!/bin/bash
# Runs the console Vite dev server natively on http://localhost:3000.
# Runtime config comes from console/apps/web-ui/.env.development (see vite.config.ts).
# Usage: console.sh [--background]
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/utils.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOG="$REPO_ROOT/.make/console.log"

BACKGROUND=false
for arg in "$@"; do
    case "$arg" in
        --background) BACKGROUND=true ;;
    esac
done

# Stop any console started from this checkout before starting fresh
"$SCRIPT_DIR/stop-console.sh"

# Login only works on :3000 (Thunder redirect URIs and CORS), so never fall back to another port
if is_port_in_use 3000; then
    holders=$(containers_publishing_port 3000)
    echo "❌ Port 3000 is already in use${holders:+ — held by container(s): $holders}"
    echo "   Find the process with: $(port_owner_command)"
    exit 1
fi

check_command pnpm

cd "$REPO_ROOT/console"
if $BACKGROUND; then
    mkdir -p "$(dirname "$LOG")"
    # stdin must not be the terminal: a background Vite reading it gets SIGTTIN and stops
    pnpm dev </dev/null >"$LOG" 2>&1 &
    echo "✅ Console starting at http://localhost:3000"
    echo "   Logs: $LOG"
    echo ""
    echo "💡 Running in background — use 'make stop-console' to stop"
else
    exec pnpm dev
fi

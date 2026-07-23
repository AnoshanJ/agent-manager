#!/usr/bin/env bash
# ----------------------------------------------------------------------------
# Attach the AMP Console sign-in layout to the console application.
#
# The layout's actual CSS/structure is created declaratively, not here — see
# doc 52-amp-console-layout.yaml in
# deployments/helm-charts/wso2-amp-thunder-extension/templates/amp-thunder-bootstrap.yaml
# (and its comment for why that document has no fixed id). Because Thunder
# assigns the layout's id at creation time, it can't be hardcoded into the
# application's layoutId the way themeId is — there's no handle-based alias
# resolution for layoutId, unlike authFlowHandle. This script does that one
# remaining step: look the layout up by handle and attach it to the app, after
# the main bootstrap has already created both.
#
# Runs after the platform Thunder deployment is ready (see setup-openchoreo.sh)
# and is idempotent: re-running it is a no-op if the application already
# references the current layout id.
#
# Overridable via environment:
#   THUNDER_PUBLIC_URL     (default: http://thunder.amp.localhost:8080)
#   SYSTEM_CLIENT_ID       (default: amp-system-client)
#   SYSTEM_CLIENT_SECRET   (default: amp-system-client-secret)
#   CONSOLE_APP_ID         (default: amp-console-client)
#   LAYOUT_HANDLE          (default: wso2-agent-manager-console)
# ----------------------------------------------------------------------------
set -euo pipefail

THUNDER_PUBLIC_URL="${THUNDER_PUBLIC_URL:-http://thunder.amp.localhost:8080}"
THUNDER_PUBLIC_URL="${THUNDER_PUBLIC_URL%/}"
SYSTEM_CLIENT_ID="${SYSTEM_CLIENT_ID:-amp-system-client}"
SYSTEM_CLIENT_SECRET="${SYSTEM_CLIENT_SECRET:-amp-system-client-secret}"
CONSOLE_APP_ID="${CONSOLE_APP_ID:-amp-console-client}"
LAYOUT_HANDLE="${LAYOUT_HANDLE:-wso2-agent-manager-console}"

# The System resource server's identifier is "<publicUrl>/mcp" (set by the bootstrap
# doc 70-fix-thunder-system-rs-identifier.yaml). Scoping the client_credentials token
# to it is what actually grants the "system" permission the admin APIs require; without
# the resource indicator the token resolves against amp-resource-server and is rejected.
SYSTEM_RESOURCE="${THUNDER_PUBLIC_URL}/mcp"

log()  { echo "   $*"; }
warn() { echo "⚠️  $*" >&2; }
fail() { echo "❌ $*" >&2; exit 1; }

command -v jq >/dev/null 2>&1 || fail "jq is required but not installed"

echo "🎨 Attaching console sign-in layout (handle: $LAYOUT_HANDLE) to ${CONSOLE_APP_ID}..."

# --- 1. Obtain a system-scoped admin token -------------------------------------------
fetch_token() {
  curl -sf -u "${SYSTEM_CLIENT_ID}:${SYSTEM_CLIENT_SECRET}" \
    -X POST "${THUNDER_PUBLIC_URL}/oauth2/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode "grant_type=client_credentials" \
    --data-urlencode "scope=system" \
    --data-urlencode "resource=${SYSTEM_RESOURCE}" \
    | jq -r '.access_token // empty'
}

TOKEN=""
for attempt in 1 2 3 4 5; do
  if TOKEN="$(fetch_token)" && [ -n "$TOKEN" ]; then
    break
  fi
  log "Thunder token endpoint not ready yet (attempt ${attempt}/5); retrying..."
  sleep 5
done
[ -n "$TOKEN" ] || fail "could not obtain a system token from ${THUNDER_PUBLIC_URL}"

auth=(-H "Authorization: Bearer ${TOKEN}")

# --- 2. Find the layout the declarative bootstrap already created, by handle --------
layout_id="$(curl -sf "${auth[@]}" "${THUNDER_PUBLIC_URL}/design/layouts" \
  | jq -r --arg h "$LAYOUT_HANDLE" '.layouts[]? | select(.handle == $h) | .id' | head -1)"

[ -n "$layout_id" ] && [ "$layout_id" != "null" ] || \
  fail "no layout found with handle '${LAYOUT_HANDLE}' — did the bootstrap job (52-amp-console-layout.yaml) run and succeed?"

log "Found layout (id: $layout_id)"

# --- 3. Attach the layout to the console application ---------------------------------
app_json="$(curl -sf "${auth[@]}" "${THUNDER_PUBLIC_URL}/applications/${CONSOLE_APP_ID}")" \
  || fail "failed to fetch application ${CONSOLE_APP_ID}"

current_layout="$(echo "$app_json" | jq -r '.layoutId // empty')"
if [ "$current_layout" = "$layout_id" ]; then
  log "Console app already references layout ${layout_id} — nothing to do"
else
  updated_app="$(echo "$app_json" | jq --arg lid "$layout_id" '.layoutId = $lid')"
  if ! curl -sf "${auth[@]}" -X PUT "${THUNDER_PUBLIC_URL}/applications/${CONSOLE_APP_ID}" \
      -H "Content-Type: application/json" --data "$updated_app" >/dev/null; then
    fail "failed to attach layout to application ${CONSOLE_APP_ID}"
  fi
  log "Attached layout ${layout_id} to application ${CONSOLE_APP_ID}"
fi

# --- 4. Verify the layout is served in the flow metadata -----------------------------
if curl -sf "${THUNDER_PUBLIC_URL}/flow/meta?id=${CONSOLE_APP_ID}&type=APP&language=en-US" \
    | jq -e '.design.layout.head.stylesheets | length > 0' >/dev/null 2>&1; then
  echo "✅ Console sign-in layout attached and served"
else
  warn "Layout attached, but flow metadata did not report a stylesheet yet (may need a moment to propagate)"
fi

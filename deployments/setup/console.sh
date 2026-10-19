#!/bin/bash
# Runs the console Vite dev server natively on http://localhost:3000.
# Usage: console.sh [--background]
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/utils.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONSOLE_DIR="$REPO_ROOT/console"
WEB_UI_PUBLIC="$CONSOLE_DIR/apps/web-ui/public"
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

check_command envsubst
check_command pnpm

# Runtime config — every value can be overridden from the environment,
# e.g. DISABLE_AUTH=true make console
export AUTH_CLIENT_ID="${AUTH_CLIENT_ID:-amp-console-client}"
export AUTH_BASE_URL="${AUTH_BASE_URL:-http://thunder.amp.localhost:8080}"
export SIGN_IN_REDIRECT_URL="${SIGN_IN_REDIRECT_URL:-http://localhost:3000/login}"
export SIGN_OUT_REDIRECT_URL="${SIGN_OUT_REDIRECT_URL:-http://localhost:3000/login}"
export API_BASE_URL="${API_BASE_URL:-http://localhost:9000}"
export DISABLE_AUTH="${DISABLE_AUTH:-false}"
export AUTH_SCOPES="${AUTH_SCOPES:-openid profile email amp:org:view amp:org:modify-settings amp:org:invite-member amp:org:remove-member amp:org:assign-role amp:org:manage-idp amp:org:manage-service-account amp:project:create amp:project:read amp:project:update amp:project:delete amp:environment:create amp:environment:read amp:environment:update amp:environment:delete amp:gateway:create amp:gateway:read amp:gateway:update amp:gateway:delete amp:gateway:token-manage amp:data-plane:read amp:deployment-pipeline:create amp:deployment-pipeline:read amp:deployment-pipeline:update amp:deployment-pipeline:delete amp:git-secret:create amp:git-secret:read amp:git-secret:delete amp:llm-provider-template:create amp:llm-provider-template:read amp:llm-provider-template:update amp:llm-provider-template:delete amp:llm-provider:create amp:llm-provider:read amp:llm-provider:update amp:llm-provider:delete amp:llm-provider:configure-guardrail amp:llm-provider:connect amp:llm-provider:deploy amp:llm-provider:api-key-manage amp:mcp-server:create amp:mcp-server:read amp:mcp-server:update amp:mcp-server:delete amp:mcp-server:configure-guardrail amp:mcp-server:connect amp:mcp-server:api-key-manage amp:llm-proxy:create amp:llm-proxy:read amp:llm-proxy:update amp:llm-proxy:delete amp:llm-proxy:deploy amp:llm-proxy:api-key-manage amp:evaluator:create amp:evaluator:read amp:evaluator:update amp:evaluator:delete amp:agent:create amp:agent:read amp:agent:update amp:agent:delete amp:agent:build amp:agent:env-non-production amp:agent:env-production amp:agent:rollback amp:agent:suspend amp:agent:token-manage amp:agent:api-key-manage amp:monitor:create amp:monitor:read amp:monitor:update amp:monitor:delete amp:monitor:execute amp:monitor:score-read amp:monitor:score-publish amp:observability:trace-read amp:observability:log-read amp:observability:build-log-read amp:observability:metric-read amp:role:create amp:role:read amp:role:update amp:role:delete amp:group:create amp:group:read amp:group:update amp:group:delete amp:catalog:read amp:repository:read amp:agent-kind:read amp:agent-kind:create amp:agent-kind:update amp:agent-kind:delete amp:profile:read amp:profile:update-attributes amp:scope:create amp:scope:read amp:scope:update amp:scope:delete amp:agent-identity:read amp:agent-identity:create amp:agent-identity:update amp:agent-identity:delete}"
export GATEWAY_CONTROL_PLANE_URL="${GATEWAY_CONTROL_PLANE_URL:-http://localhost:9243}"
export GATEWAY_VERSION="${GATEWAY_VERSION:-v2026.09.24}"
# Agent Manager release version — pins the deployment-script git tag (amp/vX.Y.Z).
# Keep in sync with the agent-manager-service AMP_VERSION in deployments/docker-compose.yml.
export AMP_VERSION="${AMP_VERSION:-v0.16.0}"
# kgateway-routed vhost — no port-forward to the gateway runtime needed.
export INSTRUMENTATION_URL="${INSTRUMENTATION_URL:-http://default-default.gateway.localhost:19080/otel}"
# Cluster-internal URLs the gateway uses to reach Agent Manager — piped into the
# add-environment.sh command rendered by the Create Environment drawer (display
# text only, the browser never fetches them). AMS runs in docker-compose on the
# host, so the gateway (which runs in k3d) reaches it via host.docker.internal.
export AGENT_MANAGER_INTERNAL_BASE_URL="${AGENT_MANAGER_INTERNAL_BASE_URL:-http://host.docker.internal:9000}"
export AGENT_MANAGER_INTERNAL_CP_HOST="${AGENT_MANAGER_INTERNAL_CP_HOST:-host.docker.internal:9243}"
# Base domain env-Thunder instances are hosted under, and whether this deployment
# serves them over TLS — piped into the same add-environment.sh command. Matches
# agent-manager-service's own IDP_HOST_BASE_DOMAIN/TLS_ENABLED defaults for local dev.
export IDP_HOST_BASE_DOMAIN="${IDP_HOST_BASE_DOMAIN:-amp.localhost}"
export TLS_ENABLED="${TLS_ENABLED:-false}"
export GUARDRAILS_CATALOG_URL="${GUARDRAILS_CATALOG_URL:-https://db720294-98fd-40f4-85a1-cc6a3b65bc9a-prod.e1-us-east-azure.choreoapis.dev/api-platform/policy-hub-api/policy-hub-public/v1.0/policies?categories=Guardrails,AI&limit=100}"
export GUARDRAILS_DEFINITION_BASE_URL="${GUARDRAILS_DEFINITION_BASE_URL:-https://db720294-98fd-40f4-85a1-cc6a3b65bc9a-prod.e1-us-east-azure.choreoapis.dev/api-platform/policy-hub-api/policy-hub-public/v1.0/policies}"
export DOCS_URL="${DOCS_URL:-https://wso2.github.io/agent-manager/docs/latest}"
# Largest request body the console sends; bigger saves are refused with a
# readable error instead of a WAF's opaque 403. Unset means no limit.
# export MAX_REQUEST_BODY_BYTES="${MAX_REQUEST_BODY_BYTES:-57344}"
# Per-file cap on file-mount content. Matches agent-manager-service's
# FILE_MOUNT_MAX_FILE_BYTES in deployments/docker-compose.yml. When MAX_REQUEST_BODY_BYTES
# is set, the console also holds it under that value less 8 KB.
export FILE_MOUNT_MAX_FILE_BYTES="${FILE_MOUNT_MAX_FILE_BYTES:-1000000}"
export FEATURE_FLAG_ENABLE_PRIVATE_REPO_SUPPORT="${FEATURE_FLAG_ENABLE_PRIVATE_REPO_SUPPORT:-true}"
export FEATURE_FLAG_ENABLE_PROFILE_MANAGEMENT="${FEATURE_FLAG_ENABLE_PROFILE_MANAGEMENT:-true}"
export FEATURE_FLAG_ENABLE_USER_MANAGEMENT="${FEATURE_FLAG_ENABLE_USER_MANAGEMENT:-true}"

# Re-rendered on every start, so don't edit config.js by hand
envsubst < "$WEB_UI_PUBLIC/config.template.js" > "$WEB_UI_PUBLIC/config.js"

cd "$CONSOLE_DIR"
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

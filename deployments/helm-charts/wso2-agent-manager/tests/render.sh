#!/usr/bin/env bash
# Render assertions for wso2-agent-manager.
# Run: bash deployments/helm-charts/wso2-agent-manager/tests/render.sh
#
# Covers the derived auth values that plain `helm template` with default values
# cannot distinguish from hardcoded ones: OAUTH_AUTHORIZATION_SERVERS falling
# back to keyManager.issuer, and serverPublicURL being appended to
# keyManager.audience. Both are invisible at install time and surface only as a
# broken MCP login — `invalid_target` on authorize if the advertised
# authorization server is wrong, or 401 on every tool call if the audience is
# (see issues #1414 and #1424).
set -uo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FAILURES=0

# cm_value <key> [helm --set args...] -> the rendered value of ConfigMap data key <key>
# A render failure is reported rather than silently becoming an empty value, so a
# crash cannot look like a wrong value.
cm_value() {
  local key="$1" rendered
  shift
  if ! rendered="$(helm template test-release "$CHART_DIR" \
    --show-only templates/agent-manager-service/configmap.yaml "$@" 2>&1)"; then
    printf 'helm template failed: %s\n' "$rendered" >&2
    return 1
  fi
  awk -v k="$key" '
    $1 == k":" {
      sub(/^[[:space:]]*[^:]+:[[:space:]]*/, "")
      sub(/[[:space:]]+$/, "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  ' <<<"$rendered"
}

assert_cm() {
  local label="$1" key="$2" expected="$3"
  shift 3
  local actual
  actual="$(cm_value "$key" "$@")"
  if [[ "$expected" == "$actual" ]]; then
    printf 'ok   - %s\n' "$label"
  else
    printf 'FAIL - %s\n      expected %s: %q\n      actual   %s: %q\n' \
      "$label" "$key" "$expected" "$key" "$actual"
    FAILURES=$((FAILURES + 1))
  fi
}

CLIENTS="urn:wso2:amp,amp-console-client,amp-api-client,amp-publisher-*,amctl,am-mcp"

# Defaults must stay byte-identical to the pre-derivation literals, so existing
# installs and quick-start (which passes no overrides) are unaffected.
assert_cm "default audience keeps the gateway resource entry" \
  KEY_MANAGER_AUDIENCE "${CLIENTS},http://api.amp.localhost:8080/"
assert_cm "default authorization servers fall back to the default issuer" \
  OAUTH_AUTHORIZATION_SERVERS "http://thunder.amp.localhost:8080"

# Issue #1424: one issuer override has to move the advertised authorization
# server too, or MCP clients discover an authorization server whose tokens this
# service then rejects.
assert_cm "issuer override moves the advertised authorization server" \
  OAUTH_AUTHORIZATION_SERVERS "https://thunder.example.com" \
  --set agentManagerService.config.keyManager.issuer=https://thunder.example.com
assert_cm "explicit authorization servers win over the issuer" \
  OAUTH_AUTHORIZATION_SERVERS "https://as.example.com" \
  --set agentManagerService.config.keyManager.issuer=https://thunder.example.com \
  --set agentManagerService.config.oauthAuthorizationServers=https://as.example.com

# Issue #1414: serverPublicURL is the RFC 8707 resource identifier MCP tokens are
# minted with, so it must reach the audience list with exactly one trailing slash.
assert_cm "serverPublicURL override is appended to the audience" \
  KEY_MANAGER_AUDIENCE "${CLIENTS},https://api.example.com/" \
  --set agentManagerService.config.serverPublicURL=https://api.example.com
assert_cm "a serverPublicURL that already ends in a slash is not doubled" \
  KEY_MANAGER_AUDIENCE "${CLIENTS},https://api.example.com/" \
  --set agentManagerService.config.serverPublicURL=https://api.example.com/
assert_cm "an audience that already lists the resource gains no duplicate" \
  KEY_MANAGER_AUDIENCE "amp,https://api.example.com/" \
  --set agentManagerService.config.serverPublicURL=https://api.example.com \
  --set 'agentManagerService.config.keyManager.audience=amp\,https://api.example.com/'
assert_cm "whitespace in the audience does not defeat the duplicate check" \
  KEY_MANAGER_AUDIENCE "amp,https://api.example.com/" \
  --set agentManagerService.config.serverPublicURL=https://api.example.com \
  --set 'agentManagerService.config.keyManager.audience=amp\, https://api.example.com/'
assert_cm "an empty serverPublicURL appends nothing and leaves no trailing comma" \
  KEY_MANAGER_AUDIENCE "$CLIENTS" \
  --set-string agentManagerService.config.serverPublicURL=
assert_cm "a stray comma in the audience does not produce an empty entry" \
  KEY_MANAGER_AUDIENCE "amp,https://api.example.com/" \
  --set agentManagerService.config.serverPublicURL=https://api.example.com \
  --set 'agentManagerService.config.keyManager.audience=amp\,'

if ((FAILURES > 0)); then
  printf '\n%d assertion(s) failed\n' "$FAILURES"
  exit 1
fi
printf '\nAll render assertions passed\n'

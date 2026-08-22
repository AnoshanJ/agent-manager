#!/usr/bin/env bash
# Render assertions for wso2-amp-platform-resources-extension.
# Run: bash deployments/helm-charts/wso2-amp-platform-resources-extension/tests/render.sh
#
# Covers the shell quoting of the amp-generate-workload build step. Argo escapes
# a substituted parameter for JSON, never for the shell, so a parameter carrying
# user text that is interpolated into the step's script becomes shell source: one
# apostrophe in a file mount closes the string early and the rest of the script
# is reparsed as code. The build then dies on a bare `(` far from the real cause
# and Argo reports only a missing output artifact (see issue #1639). The values
# have to reach the container as env vars, which the shell never parses.
set -uo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKLOAD_TMPL=templates/cluster-workflow-templates/generate-workload.yaml
FAILURES=0

if ! RENDERED="$(helm template test-release "$CHART_DIR" --show-only "$WORKLOAD_TMPL" 2>&1)"; then
  printf 'FAIL - helm template failed\n%s\n' "$RENDERED"
  exit 1
fi

# script_body — prints the container args block scalar, dedented. Everything in
# it is shell source, so a parameter reaching this text is a parameter the shell
# parses.
script_body() {
  awk '
    function ind(s) { match(s, /^ */); return RLENGTH }
    !found && $0 ~ /^ *- \|-?$/ { found = 1; bi = ind($0); next }
    found {
      if ($0 ~ /^[ ]*$/) { print ""; next }
      if (ind($0) <= bi) { exit }
      if (ci == 0) ci = ind($0)
      print substr($0, ci + 1)
    }
  ' <<<"$RENDERED"
}

SCRIPT="$(script_body)"

if [[ -z "$SCRIPT" ]]; then
  printf 'FAIL - could not extract the generate-workload script from %s\n' "$WORKLOAD_TMPL"
  exit 1
fi

# resolve — reads the script on stdin and substitutes the Argo placeholders the
# way the controller would, taking each parameter value from V_* in the
# environment. Placeholders with no V_* entry become an inert token, so a check
# reports the quoting defect and not an unrelated empty expansion.
resolve() {
  awk '
    function replace(s, from, to,   out, p) {
      while ((p = index(s, from)) > 0) {
        out = out substr(s, 1, p - 1) to
        s = substr(s, p + length(from))
      }
      return out s
    }
    BEGIN {
      n = 0
      keys[++n] = "{{workflow.parameters.file-mounts}}";           vals[n] = ENVIRON["V_FILE_MOUNTS"]
      keys[++n] = "{{workflow.parameters.environment-variables}}"; vals[n] = ENVIRON["V_ENV_VARS"]
      keys[++n] = "{{workflow.parameters.endpoints}}";             vals[n] = ENVIRON["V_ENDPOINTS"]
      keys[++n] = "{{workflow.parameters.app-path}}";              vals[n] = ENVIRON["V_APP_PATH"]
    }
    {
      line = $0
      for (i = 1; i <= n; i++) line = replace(line, keys[i], vals[i])
      while ((a = index(line, "{{")) > 0) {
        rest = substr(line, a + 2)
        b = index(rest, "}}")
        if (b == 0) break
        line = substr(line, 1, a - 1) "placeholder" substr(rest, b + 2)
      }
      print line
    }
  '
}

# Benign values for the parameters a case is not exercising. Exactly one hostile
# value per case: two stray apostrophes re-balance the quoting and the script
# parses again while silently carrying mangled data, which would report a defect
# as fixed.
BENIGN_ENDPOINTS='[]'
BENIGN_ENV_VARS='[{"name":"GREETING","value":"hello"}]'
BENIGN_FILE_MOUNTS='[{"key":"policy","mountPath":"/etc/policy.txt","value":"plain"}]'
BENIGN_APP_PATH='.'

# assert_parses <label> <endpoints> <env-vars> <file-mounts> <app-path>
# Checked with sh, which is what the step's container runs the script under, so a
# bashism that a bash-backed image happens to tolerate cannot pass unnoticed.
assert_parses() {
  local label="$1" err
  err="$(V_ENDPOINTS="$2" V_ENV_VARS="$3" V_FILE_MOUNTS="$4" V_APP_PATH="$5" \
    resolve <<<"$SCRIPT" | sh -n 2>&1)"
  if [[ -z "$err" ]]; then
    printf 'ok   - generate-workload parses with %s\n' "$label"
  else
    printf 'FAIL - %s breaks generate-workload\n      %s\n' "$label" "$err"
    FAILURES=$((FAILURES + 1))
  fi
}

# The literal case that broke a real build was a cancellation policy reading
# "charged one night's rate". The other two carry the same user text through
# different parameters, and a build path is exercised with the double quote that
# would close the one string it is interpolated into.
assert_parses "an apostrophe in a file mount" \
  "$BENIGN_ENDPOINTS" "$BENIGN_ENV_VARS" \
  '[{"key":"policy","mountPath":"/etc/policy.txt","value":"charged one night'\''s rate"}]' \
  "$BENIGN_APP_PATH"
assert_parses "an apostrophe in an environment variable" \
  "$BENIGN_ENDPOINTS" '[{"name":"GREETING","value":"it'\''s here"}]' \
  "$BENIGN_FILE_MOUNTS" "$BENIGN_APP_PATH"
assert_parses "an apostrophe in an endpoint schema" \
  '[{"name":"api","port":8080,"basePath":"/","visibility":["Public"],"schemaContent":"summary: the user'\''s profile"}]' \
  "$BENIGN_ENV_VARS" "$BENIGN_FILE_MOUNTS" "$BENIGN_APP_PATH"
assert_parses "a double quote in the build path" \
  "$BENIGN_ENDPOINTS" "$BENIGN_ENV_VARS" "$BENIGN_FILE_MOUNTS" 'app"dir'

# env_value <name> — the container env value for that variable, empty when the
# variable is not rendered at all.
env_value() {
  awk -v n="$1" '
    $1 == "-" && $2 == "name:" { in_var = ($3 == n); next }
    in_var && $1 == "value:" {
      sub(/^[[:space:]]*value:[[:space:]]*/, "")
      sub(/[[:space:]]+$/, "")
      gsub(/^'\''|'\''$/, "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  ' <<<"$RENDERED"
}

# The parameters that carry arbitrary user text — OpenAPI schemas, environment
# variable values, file mount contents, and a build path — must be delivered
# through the pod spec rather than pasted into the script. A syntax check alone
# would pass on an even number of stray quotes, so this is what keeps the values
# out of the shell's reach in the first place.
assert_param_via_env() {
  local var="$1" param="$2"
  if grep -qF "{{$param}}" <<<"$SCRIPT"; then
    printf 'FAIL - %s is interpolated into the script instead of reaching it as $%s\n' "$param" "$var"
    FAILURES=$((FAILURES + 1))
    return
  fi
  local actual
  actual="$(env_value "$var")"
  if [[ "$actual" == "{{$param}}" ]]; then
    printf 'ok   - %s reaches the container as $%s\n' "$param" "$var"
  else
    printf 'FAIL - $%s does not carry %s\n      actual: %q\n' "$var" "$param" "$actual"
    FAILURES=$((FAILURES + 1))
  fi
}

assert_param_via_env ENDPOINTS workflow.parameters.endpoints
assert_param_via_env ENV_VARS workflow.parameters.environment-variables
assert_param_via_env FILE_MOUNTS workflow.parameters.file-mounts
assert_param_via_env APP_PATH workflow.parameters.app-path

# The defect was one quoting style in one step; this is the pattern itself, so a
# parameter added later in the same shape is caught before it ships.
QUOTED_ASSIGNMENT="$(grep -nE "^[A-Z_]+='\{\{" <<<"$SCRIPT")"
if [[ -z "$QUOTED_ASSIGNMENT" ]]; then
  printf 'ok   - no Argo parameter is assigned inside a single-quoted string\n'
else
  printf 'FAIL - an Argo parameter is assigned inside a single-quoted string\n%s\n' "$QUOTED_ASSIGNMENT"
  FAILURES=$((FAILURES + 1))
fi

if ((FAILURES > 0)); then
  printf '\n%d assertion(s) failed\n' "$FAILURES"
  exit 1
fi
printf '\nAll render assertions passed\n'

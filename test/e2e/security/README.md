# AMP Security Tests

Negative-path tests: specs that assert a principal **cannot** do something.
They run against a deployed platform, same as the e2e suites, and share the
same `framework/` and `operations/` packages.

```bash
make security-test                       # static invariants + live suites
make security-test-static                # route-table invariants only, no cluster
make security-test SUITE=authz           # one live suite
make security-test FOCUS="scope matrix"  # one spec
```

## Why this lives outside `tests/`

`make e2e-test` and the e2e CI job both glob `./tests/...`. Security suites sit
in `./security/...` so they are never swept into an e2e run — these suites abort
hard on a misconfigured deployment (see below), and that must fail a security
run, not the e2e run.

Everything else is shared: same Go module, same `framework.AMPClient`, same
`operations/` call wrappers, same cluster.

## The two layers

| Layer | Where | Needs a cluster | What it proves |
|---|---|---|---|
| Static | `agent-manager-service/api/route_authz_invariant_test.go` | no | Route inventory, permission enforcement, exact service↔Thunder predefined-role policy, scope-catalog consistency, and token-derived org handling |
| Live | `test/e2e/security/` | yes | The guards actually behave at runtime |

Static invariants are named `TestSecurityInvariant*` and selected by prefix, so
a correctly-named new one runs automatically. A self-check enforces the naming
from both ends — a `-run` regex matching nothing exits 0, which would report
success while checking nothing.

The static layer inventories both `Handle` and `HandleFunc*` registrations in
the API and MCP routing packages. Unguarded exceptions are keyed by exact
function, registrar, and pattern, so a replacement route cannot inherit an old
count-based exemption. It also counts a permission as enforced only when it is
an argument to an authorization-bearing registrar. The predefined-role
invariant compares every permission in `rbac/predefined_roles.go` with every
permission configured for `admin`, `developer`, `ai-lead`, and
`platform-engineer` by the Thunder Helm bootstrap; missing and extra grants both
fail.

The live layer proves the wiring is real, which static analysis cannot. Its
matrix remains deliberately risk-based rather than exhaustive.

## Preconditions the suite enforces on itself

A security suite that passes because it was misconfigured is worse than no suite.
`BeforeSuite` in `authz/` therefore refuses to run unless it first proves:

1. **The IDP honours scope reduction.** The whole harness rests on Thunder
   returning `requested ∩ allowed` for a `client_credentials` token, so asking
   for a subset yields a genuinely under-privileged token. If that ever stops
   holding, every negative spec would pass against a full-privilege token.

2. **RBAC enforcement is on.** `RBAC_ENABLED` defaults to `false` in the Go
   config loader (`true` in the Helm chart and in `deployments/docker-compose.yml`).
   With it off, `RequirePermission` short-circuits and every route is reachable
   by any authenticated caller. The suite sends an unscoped token at a guarded
   endpoint and aborts unless it gets a 403.

Both aborts name the fix in the failure message.

## Writing a spec

Same layering as the e2e suites: raw API calls go in `operations/`, specs
compose them. Two extra rules:

- **Assert the denial exactly.** `framework.ExpectForbidden` requires 403. A 404
  means the request reached the handler — authorization did not stop it, and the
  resource merely happened not to exist.
- **Pair every denial with a positive control.** `framework.ExpectNotForbidden`
  with a token that *does* carry the scope proves the 403 came from the scope
  check and not from an unrelated guard. Without it, a route guarded by the
  wrong permission constant looks correctly protected. A downstream 5xx still
  proves static scope middleware passed, so it is recorded in Ginkgo/JUnit
  evidence without failing this authorization suite. Dynamic-permission routes
  require stronger route-specific controls because their resolver can itself 5xx.

Requests in the scope matrix are side-effect free by construction: GET/DELETE
target deliberately absent resources, and POST/PUT send no body so the handler
fails at JSON decode before touching state. Keep that property — the allow-case
request reaches the handler on purpose.

## Scope-reduced tokens

```go
deny  := framework.FetchTokenWithScopes(Cfg, framework.ScopesExcept("amp:agent:delete"))
allow := framework.FetchTokenWithScopes(Cfg, []string{"amp:agent:delete"})
client := framework.NewAMPClientWithToken(Cfg, deny)
```

`framework.TokenScopes(token)` decodes what the IDP actually issued — use it
whenever a spec's correctness depends on a scope being absent.

This models the *scope* layer only. It does not test Thunder's role→scope
bindings. The separate `roles/` suite creates disposable OAuth applications,
assigns each real deployed role, verifies the issued token contains exactly the
role's configured AMP permissions, exercises representative allowed and denied
API boundaries, and deletes the applications after the run. Applications are
used as non-interactive personas because browser-login automation would test the
login UI more than the authorization policy.

The local quick-start defaults are sufficient. For shared and cloud
environments, inject the administrative Thunder endpoint and system-client
credentials as secrets:

```bash
THUNDER_ADMIN_URL=https://thunder.example.com \
THUNDER_SYSTEM_CLIENT_ID=... \
THUNDER_SYSTEM_CLIENT_SECRET=... \
make security-test SUITE=roles
```

Run this suite only against an environment where creating short-lived test
applications is allowed. Names begin with `e2e-test-sec-persona-`, and cleanup
is registered before provisioning so a partial setup failure is also reaped.

`SEC-ROLE-002` also creates one disposable custom role through Agent Manager and
grants two harmless read permissions. After assigning a persona, it removes
only `amp:agent-kind:read` while retaining `amp:project:read`. A newly issued
token must omit the first scope, retain the second, receive exactly 403 from the
agent-kind endpoint, and still reach the project endpoint. This proves partial
revocation at the authorization layer without losing the AMP audience.

The scenario then removes the final permission and checks the zero-permission
boundary separately. Thunder may omit the AMP audience entirely (401), or issue
an AMP token without the scope (403); both are fail-closed. The test deliberately
uses **new** tokens after each mutation: an existing signed JWT remains valid
until its expiry unless the platform adds a separate token-revocation mechanism.

## Current coverage

| Suite | Specs | What it covers |
|---|---|---|
| `authz/` | 72 | SEC-AUTHZ-001 scope matrix over 36 agent-manager-service routes, both directions per route |
| `tokens/` | 23 | SEC-AUTH-001 forged, tampered, and malformed credentials must all be refused |
| `observability/` | 31 | SEC-OBS-001 full scope cross-product over the observer's 6 data routes |
| `roles/` | 18 | SEC-ROLE-001 predefined role bindings plus SEC-ROLE-002 custom-role grant/revocation lifecycle |
| `publisher/` | 12 | SEC-PUB-001 score-ingestion audience plus SEC-PUB-002 Observer trace-only confinement |
| `agentid/` | 7 | SEC-AGENTID-001 external-agent credential non-disclosure, authorization, rotation, and revocation |
| `runtime/` | 10 | SEC-RUNTIME-001 real deployed-agent sandbox posture, Kubernetes API egress denial, AgentID scope enforcement through MCP, workload credential rotation, and revocation |

The AgentID suite creates an externally hosted agent, so it requires no image
build or deployment. It does require the environment-specific Thunder token
endpoint in order to prove the returned credentials work and that rotation and
revocation invalidate them for future token requests. The suite discovers the
registered external URL from Agent Manager because environment Thunder hosts
use opaque handles and cannot be derived from org/environment names. Shared and
cloud targets may override that discovery when necessary:

```bash
AGENT_IDP_TOKEN_URL=https://<environment-thunder>/oauth2/token \
make security-test-live SUITE=agentid
```

Role and publisher personas request Thunder's `system` scope against the
System resource indicator (`<THUNDER_ADMIN_URL>/mcp`). If a deployment uses a
different identifier, set `THUNDER_SYSTEM_RESOURCE` explicitly.

The suite never prints a client secret or access token. It also deliberately
does not require an already-issued access token to stop working after rotation
or revocation: Thunder invalidates the client credential immediately, while
previously signed access tokens remain valid until their expiry.

The `runtime/` suite complements that cheap external-agent lifecycle with one
real internal deployment. It builds the deterministic fixture under
`test/e2e/fixtures/security-probe-agent`, invokes it only through a short-lived
agent API key, and verifies the security boundary from inside the workload. The
fixture has no LLM and accepts no arbitrary command, URL, token, or credential.
Its responses contain only booleans, status codes, granted scope names, and
fixed error categories.

The runtime sequence proves:

1. the process is non-root, the root filesystem is read-only, `/tmp` is the
   bounded writable area, Linux capabilities are dropped, privilege escalation
   is disabled, RuntimeDefault seccomp is active, and no Kubernetes service
   account token is mounted;
2. the pod cannot reach the in-cluster Kubernetes API network path (a 401 or
   403 from that API is a failure because it still proves network reachability);
3. an unassigned AgentID cannot call either fixed MCP tool;
4. a one-scope role can call only its matching tool, adding a second scope
   permits both, and removing only the first scope immediately restores the
   partial denial for newly minted tokens; and
5. rotation refreshes the running workload onto the new secret, while
   revocation removes its injected AgentID configuration and prevents further
   MCP access.

The build source must be remotely cloneable by the cluster. `main` is the
default after this fixture is merged. While testing a branch locally, push the
branch and point the suite at that repository and ref:

```bash
SECURITY_PROBE_REPOSITORY_URL=https://github.com/<owner>/agent-manager \
SECURITY_PROBE_REPOSITORY_BRANCH=<branch> \
make security-test-live SUITE=runtime
```

CI passes its checked-out repository and ref explicitly, so the fixture tested
there matches the security test code. The suite also reuses the controlled MCP
everything-server endpoint already used by the MCP E2E tests.

Publisher lookalikes are rejected with 401 when Observer runs normal JWKS
validation. The local quick-start deliberately sets Observer's
`isLocalDevEnv=true`; in that mode the same genuine lookalike token is parsed as
an ordinary zero-scope client and receives 403 instead. The publisher suite
accepts those two fail-closed outcomes but never a handler response. Agent
Manager runs strict JWT validation locally, so its equivalent lookalike check
still requires exactly 401.

Each suite has its own `BeforeSuite` vacuity guard. `observability/` checks the
observer's RBAC flag separately — it is a different service with its own
`amObserver.auth.rbacEnabled`, so the `authz/` suite passing says nothing about it.

The deployed conformance suites run as a distinct step in the E2E, nightly, and
release workflows. They publish `security-report.xml` independently from the
ordinary E2E report so failures remain attributable to the security gate.

### Not yet covered

- **Build secret leakage regression** — the product fix is now on `main`, but
  the strong end-to-end assertion still needs build-workspace or pod access;
  see the git-credential note below.
- **Cross-agent network isolation** — the runtime suite proves Kubernetes API
  egress denial, but it does not yet deploy a second private agent endpoint and
  prove that one workload cannot reach the other directly.
- **Post-agent-delete credential invalidation** — the runtime suite covers
  explicit AgentID revocation on a running internal agent, not deletion of the
  whole agent followed by direct reuse of its previously captured credential.

### The pod-access decision

The runtime sandbox assertions deliberately do **not** exec into the pod and do
not add `client-go` to the E2E module. They are exposed by a fixed-purpose agent
endpoint and exercised through the same public gateway boundary as a real
agent. Pod/build-workspace inspection is still appropriate for the future build
secret regression because image layers and checkout state cannot be proven
from the running application alone.

## Fixed finding awaiting regression coverage: git credentials in the build workspace

Found while scoping the build-secret suite and fixed on `main`.

The vulnerable implementation embedded the git secret's username and password
in the clone URL, causing `git clone` to persist them in
`/mnt/vol/source/.git/config`. The build step mounts that same workspace, so a
user-supplied Dockerfile could read them across the `git-secret:read` boundary.
The fixed checkout workflow supplies the credential from a checkout-container
file under `/tmp` and leaves the recorded remote URL credential-free.

Add a permanent regression test proving the credential is absent from the
build workspace, build logs, image layers, and final image.

## On multi-tenancy

There are deliberately **no cross-org tests**. AMP on-prem is single-org today,
so there is no second tenant to construct a cross-tenant request from, and
assertions written now would likely be wrong by the time multi-org ships.

What is covered instead is one necessary—but not sufficient—tenant-safety
invariant: `TestSecurityInvariantHandlersNeverReadOrgFromPath` asserts no
handler reads `{orgName}` from the URL, because `middleware.RequireOrgMatch`
takes the org solely from the token's `ouId`/`ouHandle` claims. It does not
prove that every operation checks the caller against the loaded resource, or
that collection and search queries filter by tenant. Those guarantees remain
outside the current suite until a real second organization can be exercised.

Two known items to revisit when multi-org work starts:

- `agent-manager-observer` takes `organization` from the query string and never
  compares it to the token. Inert today (`NamespaceFor` ignores the argument and
  returns a single namespace), a cross-tenant read the moment it does not.
- Collection and search endpoints need tenant-filtering coverage; per-resource
  GETs are the easy half.

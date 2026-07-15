# Plan: Re-model MCP Proxy Environments as Endpoints

**Service:** `agent-manager-service` (Go control plane)
**Author context:** menaka@wso2.com
**Status:** Code-complete — all 8 stages implemented. `make codegen`, `make fmt`,
`make lint` (0 issues), and the full unit suite pass. Only outstanding gate: `make
dev-migrate` (needs a running local Postgres; not run in this environment).
**Date:** 2026-07-09

---

## 0. Implementation status (living section)

**Last updated:** 2026-07-09. Sequencing follows §5 (1 migration → 8 tests). **All 8 stages done.**

### Done (stages 1–8) — full service + test tree compiles; codegen/fmt/lint/unit-tests green

| Stage | Files | Notes |
|---|---|---|
| 1. Migration | `db_migrations/032_mcp_proxy_endpoints.go`, `migration_list.go` | `mcp_proxy_endpoints` + `mcp_proxy_endpoint_environments` with `uq_endpoint_env`, **`uq_proxy_env_single`**, all FKs (endpoint→proxy CASCADE, join→artifacts CASCADE) and all §3.4 indexes. `latestVersion` → 32; registered. Endpoint→artifacts FK **dropped** per §7.1 default (artifacts rows belong to the join-table deployments). |
| 2. Models | `models/mcp_proxy.go` | Dropped `Environments` from `MCPProxyConfig`. `MCPEnvironmentConfig` → **`MCPEndpointConfig`** (removed `ArtifactUUID`). Added GORM `MCPProxyEndpoint`, `MCPProxyEndpointEnvironment`; `Endpoints` preload on `MCPProxy`. DTO reshaped: `MCPProxyDTO.Endpoints []MCPProxyEndpointDTO`, each with flat config fields + `Environments []MCPEndpointEnvironmentDTO{EnvironmentUUID, DeploymentStatus}`. |
| 3. Repo + DI + mocks | `repositories/mcp_proxy_endpoint_repository.go`, `mcp_proxy_repository.go`, `wiring/wire.go`, `wiring/wire_gen.go`, `repositories/repomocks/mcp_proxy_endpoint_repository_mock.go` | New repo interface + GORM impl incl. `GetEndpointEnvByProxyAndEnv` (binding resolver) and `ListEndpointEnvironmentsByProxy` (delete teardown). Preloads added to `GetByHandle`/`GetByHandleForUpdate` (table-scoped `FOR UPDATE`)/`GetByUUID`. **Also `List` now preloads `Endpoints`/`Endpoints.Environments`** (needed by scope-binding counts + env-cleanup sweep). Wired into DI both blocks; moq mock generated. |
| 4. Service C/U/D + Get | `services/mcp_proxy_service.go`, `utils/errors.go` | `endpointRepo` field + ctor param. `validateMCPEndpoints` (handle uniqueness, SSRF, env-uuid validity, cross-endpoint env-uniqueness), `validateMCPEndpointSecurity`, `buildMCPEndpointsForStorage`, `persistMCPEndpoints`, `mapMCPProxyWriteError` (23505 → constraint-specific sentinels). `Create` / `Update` (replace-set) / `Delete` (teardown via join rows) / `Get` (per-endpoint, per-env status). New sentinel `utils.ErrMCPEnvAlreadyBound`. **Added `RemoveEnvironmentFromEndpoints` (env-deletion teardown; see stage 6).** |
| 5. Deployment | `services/mcp_proxy_deployment.go` | `deployMCPProxyEnvironments` → **`deployMCPProxyEndpoints`** (endpoints × env join rows). `buildMCPProxyEnvArtifact(source, endpoint, ee)`. Handle `{proxy}-{endpoint}-{envSuffix}`. YAML builders / policy injection / deletion broadcast unchanged. |
| 6. Agent binding | `services/agent_configuration_service.go`, `services/scope_service.go`, `services/mcp_proxy_service.go`, `repositories/env_agent_mcp_mapping_repository.go`, `repositories/mcp_proxy_repository.go` | See "Stage 6 as built" below. Resolved **from the preloaded endpoint graph** (no repo/ctx threaded into the builder or the agent-config/scope services). |
| 7. OpenAPI + codegen | `docs/api_v1_openapi.yaml`, regenerated `spec/*` | `environments` map → `endpoints[]` in `MCPProxyRequest`/`MCPProxyResponse`; new `MCPProxyEndpoint` + `MCPEndpointEnvironment` schemas; `MCPEnvironmentConfig` schema retired. `make spec` (openapi-generator) + `make codegen` (wire/moq) both run. Controller unchanged — confirmed it decodes straight into `models.MCPProxyDTO`; the generated `spec.*` MCP types are doc-only (not referenced in the request path). |
| 8. Tests + gates | `services/mcp_proxy_service_unit_test.go`, `mcp_proxy_deployment_test.go`, `scope_service_unit_test.go`, `agent_configuration_system_managed_test.go`, **new** `services/mcp_endpoint_resolver_unit_test.go` | Old-shape tests rewritten to endpoints. New resolver coverage (bound endpoint, no-match, nil/empty, artifact-UUID + security helpers, `buildAgentMCPConfigProxy` flatten + empty-when-unbound), plus endpoint-validation dup-env rejection (`ErrMCPEnvAlreadyBound`) and a per-endpoint artifact-handle distinctness test. `make codegen` / `make fmt` / `make lint` (**0 issues**) / unit suite all green. |

### Decisions made during implementation

- **§7.1 Endpoint→artifacts FK:** DROPPED. Artifacts rows are created lazily per `(endpoint,env)` deployment (`ensureMCPProxyEnvArtifactRow`, `Kind=KindMCPMapping`); the join row's `artifact_uuid` FKs to `artifacts` with CASCADE, matching current deployments/deployment_status cascade.
- **§7.2 DTO env-status shape:** array of `[]{environmentUuid, deploymentStatus}` (confirmed with user).
- **§7.3 Artifact handle:** `{proxy}-{endpoint}-{envSuffix}` (confirmed; endpoint handles are only per-proxy unique).
- **§7.4 Sentinel:** `utils.ErrMCPEnvAlreadyBound` added; raised by the service pre-check AND mapped from DB `23505` on `uq_proxy_env_single`/`uq_endpoint_env`.
- **Request payload shape (confirmed with user):** endpoints reuse the old per-env field names (flat `upstream/policies/capabilities/security/toolScopeBindings`) plus `environments: [{environmentUuid}]` — smallest console diff. The controller decodes straight into `models.MCPProxyDTO` (no separate spec/convert layer), so the DTO IS the wire contract.
- **Update strategy:** endpoint config is fully specified on each PUT, so Update **replaces the endpoint set** (delete all endpoints → cascade join rows → re-insert). Artifact UUIDs are **preserved per environment** (keyed by env UUID, not endpoint), so an env that stays bound keeps its artifact identity and agent `(proxy,env)→artifact_uuid` bindings survive even if the env moves to a different endpoint.
- **Get response:** `endpoints[]`, each with config + per-env `{environmentUuid, deploymentStatus}` (confirmed).

### Stage 6 as built — key deviation from §4.6 (simpler than planned)

The plan (§4.6) worried about threading `endpointRepo` (with `ctx`/errors) through
`buildAgentMCPConfigProxy` and ~10 call sites. In practice **every call site already had a
preloaded `*models.MCPProxy` (`mapping.MCPProxy` or `sourceProxy`) in scope**, so the whole
refactor collapsed to reading the endpoint layer off the preloaded graph — **no repo or
`ctx` threaded into the builder, the agent-config service, or the scope service, and no
call-site signatures changed.** What was actually done:

- **New pure resolver** `resolveMCPEndpointForEnv(proxy, envID) (*MCPProxyEndpoint, *MCPProxyEndpointEnvironment)`
  in `agent_configuration_service.go` — walks `proxy.Endpoints[].Environments`; `uq_proxy_env_single`
  guarantees ≤1 match. **Replaces the deleted `findMCPEnvironmentConfig`.**
- `mcpProxySecurityForEnv` / `mcpProxyEnvArtifactUUID` rewritten to delegate to the resolver
  (security from `endpoint.Configuration.Security`, artifact UUID from the join row). The other
  helpers (`mcpProxyAPIKeySecurityEnabled`, `mcpProxyAPIKeyHeaderName`) sit on top of these and
  needed no change → all their call sites (`:1357, :2317, :4660, :4945, :5079`, etc.) are untouched.
- `buildAgentMCPConfigProxy` now flattens the resolved endpoint's config (upstream/policies/
  caps/security **+ `ToolScopeBindings`**, which the old blueprint path dropped). Its 6-arg
  signature is unchanged (`envName` remains an accepted-but-unused param, as before).
- The two `configured := findMCPEnvironmentConfig(...) != nil` checks (`:1299, :2231`) → `resolveMCPEndpointForEnv(...) != nil`.
- **Preloads added** so `mapping.MCPProxy` carries its endpoint graph:
  `env_agent_mcp_mapping_repository.go` `ListByConfig`/`ListByMCPProxy`/`ListByEnvironment` now
  `Preload("MCPProxy.Endpoints")` + `Preload("MCPProxy.Endpoints.Environments")`.
- **`CleanupEnvironmentMCPArtifacts` (b)** no longer strips a JSONB env block. It now calls the
  new `MCPProxyService.RemoveEnvironmentFromEndpoints(proxy, envUUID, org)` which, per endpoint
  bound to the vanished env, tears down the `(endpoint,env)` gateway artifact and deletes the
  join row (endpoint rows stay — they may still serve other envs). Keeps endpoint-repo access
  inside the proxy service (which owns it); the agent-config service gained **no** new dependency.
- `scope_service.go` `BindingCounts` iterates `proxy.Endpoints[].Configuration.ToolScopeBindings`
  instead of `Configuration.Environments`. **`ScopeService` ctor/struct unchanged** (it already
  had `proxyRepo`; the `List` preload change feeds it the endpoints).

### Remaining gate (environment-blocked, not code)

- **`make dev-migrate`** — not run here: no local Postgres reachable (`context deadline
  exceeded`). Start a DB (`cd agent-manager && make dev-up`, or a local Postgres matching
  `.env`) then run `make dev-migrate` to confirm `032_mcp_proxy_endpoints` applies cleanly.
- **Console (§4.9)** — still out of scope; the `environments`→`endpoints` DTO change is breaking
  and needs a matching console PR.
- Optional: integration-tier tests for the DB `23505`→`ErrMCPEnvAlreadyBound` mapping and the
  Update add/remove-env teardown paths (the unit tier covers the service pre-check and resolver;
  the DB-constraint mapping needs a real DB, so it belongs in the `-tags=integration` tier).

---

## 1. Goal

Today an MCP proxy stores its per-environment configuration inside a single JSONB blob
(`mcp_proxies.configuration.environments`, keyed by environment UUID). Each environment
carries its own upstream URL, auth, policies, capabilities, security, and a stable
`ArtifactUUID`. Governance is therefore effectively *per environment*.

We are changing the model so that **each endpoint is treated as an MCP proxy** and
**governance happens at the endpoint level, not the environment level**:

- A parent `mcp_proxies` record becomes a logical grouping.
- Each **endpoint** is the actual deployable proxy definition (upstream URL, auth,
  policies, capabilities, security), stored in a new `mcp_proxy_endpoints` table.
- **One endpoint can be deployed to 1–N environments.** That endpoint→environment mapping
  is stored in a new join table `mcp_proxy_endpoint_environments`, which also holds the
  stable per-deployment gateway artifact identity and status.
- **Hard rule:** within one parent proxy, an environment maps to **at most one endpoint**
  (enforced by a DB unique constraint). This keeps agent binding unambiguous.

### Design decisions locked in (from clarifying Q&A)

| # | Decision | Choice |
|---|---|---|
| 1 | Endpoint config storage | **JSONB `configuration` per endpoint row** (mirrors current `MCPProxyConfig` shape, minimal serializer churn) |
| 2 | Endpoint↔environment mapping | **New join table** `mcp_proxy_endpoint_environments` with `artifact_uuid` + `status` |
| 3 | Existing data migration | **Fresh schema only** — pre-GA/dev; the old `configuration.environments` blob is abandoned, not migrated |
| 4 | Agent→proxy binding | **Proxy-level binding retained.** `env_agent_mcp_mapping` is unchanged; resolution `(mcp_proxy_uuid, environment_uuid) → artifact_uuid` |
| 5 | Multiple endpoints per env within one proxy | **Not allowed.** `UNIQUE(mcp_proxy_uuid, environment_uuid)` on the join table |
| 6 | `mcp_proxy_mappings` (agent-scoped deployable) | **Untouched** — still works under proxy-level binding |

Because of decision #5, the `(proxy, env)` → endpoint resolution always returns exactly
one row, so agent binding stays clean and proxy-level, with no `endpoint_uuid` needed on
`env_agent_mcp_mapping` and no "error on multiple" fallback.

---

## 2. Current state (what we are replacing)

### 2.1 Tables (migration `db_migrations/024_create_mcp_proxies.go`)

- `mcp_proxies(uuid PK→artifacts, description, created_by, status, configuration JSONB)`
- `mcp_proxy_mappings(uuid PK→artifacts, source_mcp_proxy_uuid→mcp_proxies, description, status, configuration JSONB)`
- `env_agent_mcp_mapping(id, config_uuid→agent_configurations, environment_uuid, mcp_proxy_uuid→mcp_proxies, artifact_uuid→mcp_proxy_mappings, created_at, UNIQUE(config_uuid, environment_uuid))`

### 2.2 Models (`models/mcp_proxy.go`)

- `MCPProxy` — GORM row; `Configuration MCPProxyConfig` serialized as JSONB.
- `MCPProxyConfig` — carries two shapes:
  - **Source proxy (blueprint):** populates `Environments map[string]MCPEnvironmentConfig`
    (keyed by env UUID), leaves flat root `Upstream/Policies/Capabilities/Security` empty.
  - **Agent-scoped mapping (deployable):** populates the flat root fields, leaves
    `Environments` empty. Deployment YAML builder reads only the flat root fields.
- `MCPEnvironmentConfig` — per-env blueprint block: `ArtifactUUID *uuid.UUID`, `Upstream`,
  `Policies`, `Capabilities`, `Security`, `DeploymentStatus` (computed on read, never
  persisted).
- `MCPProxyDTO` — request/response body; carries `Environments map[string]MCPEnvironmentConfig`.

### 2.3 Key current code paths

- **Routes** `api/mcp_proxy_routes.go` — `POST/GET/PUT/DELETE /orgs/{orgName}/mcp-proxies...`
  (RBAC: `mcp-server:create/read/update/delete/connect`). Route surface does **not** change.
- **Create** `services/mcp_proxy_service.go:105` — validates handle/name/version, SSRF-validates
  environments (`validateMCPEnvironments`), builds storage blocks
  (`buildMCPEnvironmentsForStorage`, allocates per-env `ArtifactUUID` + encrypts auth),
  persists in a transaction, then best-effort `deployMCPProxyEnvironments`.
- **Credential encryption** `services/mcp_proxy_service.go:538` (`prepareMCPUpstreamAuthForStorage`)
  — AES-256-GCM encrypt plaintext `Value` → base64 → `SecretRef`, clear `Value`. Preserves
  existing `SecretRef` when the client omits credentials on update
  (`preserveUpstreamAuthCredential`). `Value`/`SecretRef` mutually exclusive.
- **Deploy** `services/mcp_proxy_deployment.go:221` `deployMCPProxyEnvironments` — loops
  `for envID, envCfg := range proxy.Configuration.Environments`; per env: parse UUID, require
  `ArtifactUUID`, `resolveGatewayForEnvironment` (AI-type first, else any active; skip on
  `errNoActiveGatewayForEnvironment`), `buildMCPProxyEnvArtifact` (flatten block → flat proxy),
  `ensureMCPProxyEnvArtifactRow` (create `artifacts` row `Kind=KindMCPMapping`),
  `deployMCPProxyToGateway` (build YAML, record `Deployment`, broadcast
  `MCPProxyDeploymentEvent`). Best-effort; errors aggregated via `errors.Join`.
- **Artifact handle** `services/mcp_proxy_deployment.go:130` `mcpProxyEnvArtifactHandle` =
  `{proxyHandle}-{envUUIDWithoutHyphens}` (satisfies `artifacts UNIQUE(handle, org)`).
- **Delete** `services/mcp_proxy_service.go:432` — blocks if `env_agent_mcp_mapping` rows exist
  (`ErrMCPProxyHasMappings`); otherwise collects artifact UUIDs from
  `proxy.Configuration.Environments` and tears down gateway artifacts.
- **Agent binding** `services/agent_configuration_service.go`:
  - `EnvAgentMCPMapping` created at `:1332`, `:2299`; `buildAgentMCPConfigProxy` called at
    ~10 sites (`:1338, :2305, :2499, :2556, :2610, :3016, :4950, :5096, :5345`).
  - `buildAgentMCPConfigProxy` (`:4275`) resolves the env block via
    `findMCPEnvironmentConfig(source.Configuration.Environments, mapping.EnvironmentUUID)`
    (`:4304, :4342`) and flattens it into a deployable `MCPProxy` using
    `mapping.ArtifactUUID` as the artifact identity.
  - `buildMCPProxyMapping` (`:4355`) wraps the flattened proxy into an `MCPProxyMapping` row.
- **OpenAPI** `docs/api_v1_openapi.yaml` — MCP proxy request/response schemas include the
  `environments` map. Generated types via `oapi-codegen` (`make codegen`).

---

## 3. Target data model

### 3.1 `mcp_proxies` (parent — keep, slim the config blob)

```
uuid           UUID PRIMARY KEY, FK -> artifacts(uuid) ON DELETE CASCADE
description    TEXT
created_by     VARCHAR(255)
status         VARCHAR(20) NOT NULL DEFAULT 'pending'
configuration  JSONB NOT NULL   -- shared metadata ONLY: name, version, context, vhost, specVersion
                                 -- NO 'environments' key anymore
```

The parent's `configuration` becomes shared metadata only. Per-endpoint deployable config
lives on the endpoint rows.

### 3.2 `mcp_proxy_endpoints` (NEW — the deployable proxy definition)

```
uuid           UUID PRIMARY KEY, FK -> artifacts(uuid) ON DELETE CASCADE
mcp_proxy_uuid UUID NOT NULL, FK -> mcp_proxies(uuid) ON DELETE CASCADE
handle         VARCHAR(...) NOT NULL   -- endpoint identity, unique within the parent proxy
name           VARCHAR(...)            -- display name (optional; may fold into configuration)
status         VARCHAR(20) NOT NULL DEFAULT 'pending'
configuration  JSONB NOT NULL          -- upstream (URL + auth.secretRef), policies,
                                        -- capabilities, security
created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP
updated_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP

CONSTRAINT uq_mcp_endpoint_handle UNIQUE(mcp_proxy_uuid, handle)
```

Notes:
- `uuid` is BOTH the endpoint PK and (for the gateway) an artifact-backed identity, mirroring
  the existing pattern where `mcp_proxies.uuid` FKs to `artifacts.uuid`. **Decision to confirm
  at implementation:** whether the endpoint row itself needs an `artifacts` row, or only the
  per-(endpoint,env) deployment artifacts do. Current model gives the *deployment* artifact the
  `artifacts` row (`Kind=KindMCPMapping`), not the source proxy's config. We keep that: the
  endpoint's own `uuid`→`artifacts` FK is optional and may be dropped if the endpoint is purely
  a config holder. **Default: drop the endpoint→artifacts FK; artifacts rows belong to the
  join-table deployments (§3.3).** (Left explicit so the migration author decides deliberately.)

### 3.3 `mcp_proxy_endpoint_environments` (NEW — endpoint→env deployment join)

```
id               SERIAL PRIMARY KEY
mcp_proxy_uuid   UUID NOT NULL   -- denormalized from the endpoint, required for the
                                 -- cross-endpoint uniqueness constraint below
endpoint_uuid    UUID NOT NULL, FK -> mcp_proxy_endpoints(uuid) ON DELETE CASCADE
environment_uuid UUID NOT NULL
artifact_uuid    UUID NOT NULL   -- stable gateway artifact identity for this (endpoint, env);
                                 -- replaces the old per-env ArtifactUUID that lived in JSONB.
                                 -- FK -> artifacts(uuid) ON DELETE ... (see note)
status           VARCHAR(20) NOT NULL DEFAULT 'pending'
created_at       TIMESTAMP DEFAULT CURRENT_TIMESTAMP

CONSTRAINT uq_endpoint_env       UNIQUE(endpoint_uuid, environment_uuid)
CONSTRAINT uq_proxy_env_single   UNIQUE(mcp_proxy_uuid, environment_uuid)  -- << the hard rule
CONSTRAINT fk_endpoint_env_proxy FOREIGN KEY (mcp_proxy_uuid)
    REFERENCES mcp_proxies(uuid) ON DELETE CASCADE
```

- `uq_proxy_env_single` is the enforcement of decision #5: **within one proxy, at most one
  endpoint per environment.** Postgres cannot enforce this by reaching through the endpoint FK,
  which is why `mcp_proxy_uuid` is denormalized onto this row.
- `artifact_uuid` → `artifacts(uuid)`: one gateway artifact per `(endpoint, env)`. The
  `artifacts` row is created lazily at deploy time (see `ensureMCPProxyEnvArtifactRow`
  replacement, §4.5). The FK / cascade direction must match how deployments/deployment_status
  cascade today (they FK to `artifacts` and cascade on artifact delete). **Keep parity with the
  current cascade behavior.**

### 3.4 Indexes

```
idx_mcp_endpoint_proxy            ON mcp_proxy_endpoints(mcp_proxy_uuid)
idx_endpoint_env_endpoint         ON mcp_proxy_endpoint_environments(endpoint_uuid)
idx_endpoint_env_environment      ON mcp_proxy_endpoint_environments(environment_uuid)
idx_endpoint_env_proxy            ON mcp_proxy_endpoint_environments(mcp_proxy_uuid)
idx_endpoint_env_artifact         ON mcp_proxy_endpoint_environments(artifact_uuid)
idx_endpoint_env_proxy_env        ON mcp_proxy_endpoint_environments(mcp_proxy_uuid, environment_uuid)
```

### 3.5 Unchanged tables

- `mcp_proxy_mappings` — unchanged (decision #6).
- `env_agent_mcp_mapping` — unchanged (decision #4). Still keyed by
  `(config_uuid, environment_uuid)` and referencing `mcp_proxy_uuid` + `artifact_uuid`.

---

## 4. Implementation steps (in dependency order)

### 4.1 Migration — NEW file `db_migrations/0NN_mcp_proxy_endpoints.go`

- Use the next sequential migration ID (inspect `db_migrations/` for the current highest; do
  NOT hardcode — the tree already has ≥024). Register it in the migrations slice/list wherever
  migrations are collected (same place `migration024` is registered).
- `Migrate`: `CREATE TABLE mcp_proxy_endpoints`, `CREATE TABLE mcp_proxy_endpoint_environments`,
  all indexes, all constraints from §3. Wrap in `db.Transaction(func(tx) { runSQL(tx, sql) })`
  exactly like `migration024`.
- Fresh schema (decision #3): do **not** read/transform existing `mcp_proxies.configuration`.
  Optionally, in the same migration, `ALTER TABLE`/document that the `environments` key inside
  `mcp_proxies.configuration` is deprecated — but since it is JSONB, no DDL is required; the app
  simply stops reading/writing it.
- **Do not** add a `down`/rollback unless the migration framework requires one — match existing
  migration style (migration024 has only `Migrate`).

### 4.2 Models `models/mcp_proxy.go`

- **`MCPProxyConfig`**: remove the `Environments map[string]MCPEnvironmentConfig` field. Keep
  `Name, Version, Context, Vhost, SpecVersion`. The flat root fields (`Upstream, Policies,
  Capabilities, Security`) remain for the agent-scoped mapping shape (still used by
  `MCPProxyMapping` + `buildAgentMCPConfigProxy`).
- **`MCPEnvironmentConfig`**: repurpose as the **endpoint configuration** shape OR introduce a
  dedicated `MCPEndpointConfig`. Fields needed on an endpoint's `configuration` JSONB:
  `Upstream *UpstreamEndpoint` (or `UpstreamConfig`), `Policies []MCPPolicy`,
  `Capabilities *MCPProxyCapabilities`, `Security *SecurityConfig`. Keep
  `DeploymentStatus string` as a **per-(endpoint,env) response-only** field (computed on read
  from the join row's `status`, never persisted).
  - Recommendation: rename to `MCPEndpointConfig` for clarity and delete the now-orphaned
    `ArtifactUUID` field from it (artifact identity now lives on the join row, not the config).
- **New GORM models:**
  ```go
  type MCPProxyEndpoint struct {
      UUID          uuid.UUID           `gorm:"column:uuid;primaryKey"`
      MCPProxyUUID  uuid.UUID           `gorm:"column:mcp_proxy_uuid"`
      Handle        string              `gorm:"column:handle"`
      Name          string              `gorm:"column:name"`
      Status        string              `gorm:"column:status"`
      Configuration MCPEndpointConfig   `gorm:"column:configuration;type:jsonb;serializer:json"`
      CreatedAt     time.Time           `gorm:"column:created_at"`
      UpdatedAt     time.Time           `gorm:"column:updated_at"`
      Environments  []MCPProxyEndpointEnvironment `gorm:"foreignKey:EndpointUUID;references:UUID;constraint:OnDelete:CASCADE"`
  }
  func (MCPProxyEndpoint) TableName() string { return "mcp_proxy_endpoints" }

  type MCPProxyEndpointEnvironment struct {
      ID              uint      `gorm:"column:id;primaryKey;autoIncrement"`
      MCPProxyUUID    uuid.UUID `gorm:"column:mcp_proxy_uuid"`
      EndpointUUID    uuid.UUID `gorm:"column:endpoint_uuid"`
      EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid"`
      ArtifactUUID    uuid.UUID `gorm:"column:artifact_uuid"`
      Status          string    `gorm:"column:status"`
      CreatedAt       time.Time `gorm:"column:created_at"`
  }
  func (MCPProxyEndpointEnvironment) TableName() string { return "mcp_proxy_endpoint_environments" }
  ```
- **`MCPProxy`**: add a preload relation `Endpoints []MCPProxyEndpoint` (foreignKey
  `MCPProxyUUID`, references `UUID`, `OnDelete:CASCADE`).
- **`MCPProxyDTO`**: replace `Environments map[string]MCPEnvironmentConfig` with
  `Endpoints []MCPProxyEndpointDTO`. Each `MCPProxyEndpointDTO` carries:
  `Id/Handle string`, `Name string`, `Upstream`, `Policies`, `Capabilities`, `Security`, and
  `Environments []string` (target env UUIDs) plus a read-only
  `EnvironmentStatuses map[string]string` (env UUID → `Deployed`/`Undeployed`) OR a
  `[]{environmentUuid, deploymentStatus}` list — pick one and mirror it in OpenAPI (§4.7).

### 4.3 Repositories

- **NEW `repositories/mcp_proxy_endpoint_repository.go`** (interface + GORM impl):
  - `CreateEndpoint(ctx, tx, *MCPProxyEndpoint) error`
  - `UpdateEndpoint(ctx, tx, *MCPProxyEndpoint) error`
  - `DeleteEndpoint(ctx, tx, endpointUUID) error`
  - `GetEndpoint(ctx, endpointUUID) (*MCPProxyEndpoint, error)`
  - `ListEndpointsByProxy(ctx, proxyUUID) ([]MCPProxyEndpoint, error)` (preload `Environments`)
  - `AddEndpointEnvironment(ctx, tx, *MCPProxyEndpointEnvironment) error`
  - `RemoveEndpointEnvironment(ctx, tx, endpointUUID, envUUID) error`
  - `ListEndpointEnvironments(ctx, endpointUUID) ([]MCPProxyEndpointEnvironment, error)`
  - `GetEndpointEnvByProxyAndEnv(ctx, proxyUUID, envUUID) (*MCPProxyEndpointEnvironment, error)`
    — **the agent-binding resolver**; returns exactly one row thanks to `uq_proxy_env_single`,
    or `ErrRecordNotFound`.
  - `ListEndpointEnvironmentsByProxy(ctx, proxyUUID) ([]MCPProxyEndpointEnvironment, error)` —
    for delete teardown (collect all artifact UUIDs).
  - Return `gorm.ErrRecordNotFound` verbatim for not-found; wrap other errors. Follow the
    global rules: distinguish not-found from real errors, never silent fallback.
- **`repositories/mcp_proxy_repository.go`**: `GetByHandle` / `Get` now `Preload("Endpoints")`
  and `Preload("Endpoints.Environments")` so callers get the full graph. Stop relying on the
  `configuration.environments` blob.
- Wire the new repo into DI (`wiring/`) — add provider + regenerate `wire` via `make codegen`.
- Regenerate repository mocks (`repomocks`/moq) for the new interface (see §4.8).

### 4.4 Service — create / update / delete (`services/mcp_proxy_service.go`)

- **Constructor / struct**: add `endpointRepo repositories.MCPProxyEndpointRepository` field +
  parameter; update the `wire` provider and every test constructor call.
- **`Create` (`:105`)**:
  - Validate parent handle/name/version as today.
  - Replace `validateMCPEnvironments(req.Environments)` with `validateMCPEndpoints(req.Endpoints)`:
    per endpoint, SSRF-validate its upstream URL (`ssrf.ValidateURL`), validate handle
    non-empty/unique-within-proxy, validate the `environments` list (non-empty UUIDs).
  - **Cross-endpoint env uniqueness pre-check:** before insert, verify no two endpoints in the
    request target the same environment; return a clear `utils.ErrInvalidInput`-wrapped message
    (do not rely solely on the DB `23505`). Also catch `23505` → map to a friendly
    "environment already assigned to another endpoint" error (new sentinel, e.g.
    `utils.ErrMCPEnvAlreadyBound`).
  - Replace `buildMCPEnvironmentsForStorage` with `buildMCPEndpointsForStorage`: per endpoint,
    run `prepareMCPUpstreamAuthForStorage` (reuse as-is; encrypts plaintext → `SecretRef`),
    allocate one `artifact_uuid` per `(endpoint, env)` row (was per-env `ArtifactUUID`).
  - Transaction: insert parent `mcp_proxies` row (via existing `repo.Create`, now with a slim
    config), then per endpoint insert `mcp_proxy_endpoints`, then per target env insert
    `mcp_proxy_endpoint_environments`. All in the one `s.db.Transaction`.
  - After commit, reload full graph (`GetByHandle` with preloads) and call the renamed
    `deployMCPProxyEndpoints` (best-effort, §4.5).
- **Update** (locate the existing update method; it mirrors create): diff endpoints and their
  env lists. Adding an env → new join row + deploy. Removing an env → tear down that
  `(endpoint,env)` artifact + delete join row. Credential preservation on update stays via
  `prepareMCPUpstreamAuthForStorage(existing, updated)` — carry the existing endpoint's
  `SecretRef` when the client omits credentials. Use row locks (`GetByHandleForUpdate`) for the
  read-modify-write, per the global concurrency rules.
- **`Delete` (`:432`)**:
  - Still block when `env_agent_mcp_mapping` rows reference this proxy
    (`ErrMCPProxyHasMappings`).
  - Replace the `for _, env := range proxy.Configuration.Environments` artifact collection
    (`:446`) with `endpointRepo.ListEndpointEnvironmentsByProxy(proxyUUID)` → collect
    `artifact_uuid`s. Tear down gateway artifacts via `deleteMCPProxyEnvironmentArtifacts`
    (unchanged). Endpoint + join rows cascade-delete with the parent proxy (`ON DELETE CASCADE`).

### 4.5 Service — deployment (`services/mcp_proxy_deployment.go`)

- **Rename & rework `deployMCPProxyEnvironments` → `deployMCPProxyEndpoints`** (`:221`):
  - Iterate `for _, endpoint := range proxy.Endpoints { for _, ee := range endpoint.Environments { ... } }`
    instead of the `Configuration.Environments` map.
  - Per `(endpoint, ee)`: parse `ee.EnvironmentUUID`, require `ee.ArtifactUUID != uuid.Nil`,
    `resolveGatewayForEnvironment(ee.EnvironmentUUID, orgName)` (unchanged; skip on
    `errNoActiveGatewayForEnvironment`, log Info), build the deploy artifact, ensure the
    `artifacts` row, `deployMCPProxyToGateway`. Aggregate errors via `errors.Join`. Best-effort
    semantics unchanged.
- **`buildMCPProxyEnvArtifact` (`:140`)**: change signature to take
  `(source *MCPProxy, endpoint *MCPProxyEndpoint, ee *MCPProxyEndpointEnvironment)`. Flatten the
  endpoint's config (upstream/policies/caps/security) into the flat deployable `MCPProxy`. The
  artifact `UUID` = `ee.ArtifactUUID`. Version/context/vhost/specVersion still inherit from the
  parent proxy's shared metadata.
- **`mcpProxyEnvArtifactHandle` (`:130`)**: change to
  `{proxyHandle}-{endpointHandle}-{envUUIDWithoutHyphens}` (or `{endpointHandle}-{envSuffix}` if
  endpoint handles are globally unique) to preserve the `artifacts UNIQUE(handle, org)`
  invariant now that multiple endpoints exist under one proxy.
- **`ensureMCPProxyEnvArtifactRow` (`:193`)**: unchanged logic, but the `deployProxy` passed in
  now originates from an endpoint. `Kind` stays `KindMCPMapping`.
- **`deployMCPProxyToGateway` (`:88`)**, YAML builders (`generateMCPProxyDeploymentYAML`,
  `buildMCPProxyDeploymentYAML` `:390/:402`), policy injection (`appendMCPAPIKeyAuthPolicy`,
  `appendMCPBackendAuthPolicy`, merge/normalize) — **unchanged**; they already operate on a flat
  single-artifact `MCPProxy`.
- **Deletion broadcast** (`BroadcastMCPArtifactDeletion`, `gatewayIDsForDeletion`,
  `broadcastMCPProxyDeletion` `:324–388`) — unchanged; keyed on artifact UUID.

### 4.6 Agent binding (`services/agent_configuration_service.go`) — the delicate part

- **`buildAgentMCPConfigProxy` (`:4275`)**: replace the env-block lookup
  `findMCPEnvironmentConfig(source.Configuration.Environments, mapping.EnvironmentUUID)` (`:4304`)
  with a resolver that reads the endpoint layer:
  `endpointRepo.GetEndpointEnvByProxyAndEnv(source.UUID, mapping.EnvironmentUUID)` → returns the
  single `(endpoint, env)` row (guaranteed unique by `uq_proxy_env_single`), then load that
  endpoint's `configuration` to flatten upstream/policies/caps/security. The artifact identity
  `mapping.ArtifactUUID` is unchanged (agent still reuses the proxy's per-env artifact — which is
  now the endpoint's `(endpoint,env).artifact_uuid`).
  - **Signature change:** `buildAgentMCPConfigProxy` currently takes `source *models.MCPProxy`
    and reads its blob synchronously. It now needs the resolved endpoint config. Two options:
    (a) pre-resolve the endpoint+env row at each call site and pass `endpointCfg` +
    `artifactUUID` into the builder (keeps the builder pure, no repo/ctx dependency — preferred),
    or (b) inject the repo and make the builder do the lookup (adds ctx/error to a currently-pure
    function). **Choose (a):** resolve once per call site, pass the flattened inputs in.
  - Update **all ~10 call sites** (`:1338, :2305, :2499, :2556, :2610, :3016, :4950, :5096,
    :5345`) to first resolve the endpoint-env row for `(mapping.MCPProxyUUID,
    mapping.EnvironmentUUID)` and pass it down. Where a call site cannot find a row (proxy has no
    endpoint for that env), fail clearly (the current code lets upstream stay empty → "upstream
    URL is required"; keep an explicit, logged error with correlation context: config UUID, proxy
    UUID, env UUID).
- **`findMCPEnvironmentConfig` (`:4342`)**: delete (no longer used) or repurpose as a pure
  helper that picks an endpoint config from a preloaded slice.
- **`EnvAgentMCPMapping` create sites (`:1332, :2299`)** and **`buildMCPProxyMapping` (`:4355`)**
  — unchanged in shape; `ArtifactUUID` now sourced from the endpoint-env row's `artifact_uuid`
  when constructing the agent mapping.
- **`mapping.MCPProxyUUID` / `mapping.ArtifactUUID`** semantics preserved: proxy-level binding,
  agent reuses the endpoint's per-env deployed artifact.

### 4.7 API spec + controller (`docs/api_v1_openapi.yaml`, `controllers/mcp_proxy_controller.go`)

- Route surface unchanged (`api/mcp_proxy_routes.go`). RBAC unchanged.
- In the OpenAPI MCP proxy request/response schemas: replace the `environments` map with an
  `endpoints` array. Each endpoint schema: `id/handle`, `name`, `upstream` (url + auth; auth
  write-only `value`, never returned; `secretRef` internal), `policies`, `capabilities`,
  `security`, `environments` (array of env UUID strings), and read-only per-env
  `deploymentStatus`.
- Run `make codegen` to regenerate `oapi-codegen` models. Update the controller's decode/encode
  and any DTO↔model mapping (`convertModelMCPProxyToSpec` and its inverse) to the endpoint shape.
- **Response sanitization**: ensure `sanitizeMCPUpstreamAuthForResponse` (or equivalent) strips
  plaintext/`SecretRef` from **every endpoint** in the response, not the old env map.

### 4.8 Tests & mocks

- Regenerate repo mocks (moq) for `MCPProxyEndpointRepository` and the updated
  `MCPProxyRepository` — follow the service-unit-test conventions (no build tags, moq-generated
  mocks, strict CI lint incl. `nilnil`, `goheader`, `exhaustruct`, `errorlint`).
- Update existing MCP proxy service tests that construct `MCPProxyService` (new `endpointRepo`
  param) and that build `MCPProxyDTO` with `Environments`.
- New/updated coverage:
  - Create with one endpoint → multiple envs → one deploy per env.
  - Create rejects two endpoints targeting the same env (`ErrMCPEnvAlreadyBound`), both at the
    service pre-check and via DB `23505` mapping.
  - Update add/remove env → deploy / teardown of the right `(endpoint,env)` artifact.
  - Credential preservation on update when auth omitted.
  - Delete blocked by existing `env_agent_mcp_mapping`.
  - Agent-binding resolver `(proxy, env) → exactly one endpoint` and clear error when none.
  - No-active-gateway env skipped (best-effort), deploys on later update.

### 4.9 Console (out of scope for this service, flag only)

The React console (`console/`) builds MCP proxies with the `environments` map today. The DTO
change (`environments` → `endpoints`) is a breaking API change. **Coordinate a matching console
change** (new endpoint-oriented form + `apis/`/`hooks/` update). Not part of this service PR;
call out in the PR description and/or open a linked console issue.

---

## 5. Sequencing

1. Migration (§4.1)
2. Models (§4.2)
3. Repositories + DI wiring + mocks (§4.3, §4.8 mocks)
4. Service create/update/delete (§4.4)
5. Service deployment (§4.5)
6. Agent-binding resolver + all call sites (§4.6)
7. OpenAPI + controller + `make codegen` (§4.7)
8. Tests (§4.8)

Each step compiles before moving on where practical; §6 gates land at the end.

---

## 6. Verification gates

```bash
cd agent-manager/agent-manager-service
make codegen      # wire + oapi-codegen + sqlc regenerate; commit generated output
make fmt
make lint         # golangci-lint (also lints tests: nilnil, goheader, exhaustruct, errorlint)
make test         # unit + service tests
make dev-migrate  # apply the new migration against a local DB; confirm clean apply
```

Manual smoke (optional, via `make dev-up`): create a proxy with one endpoint targeting two
envs, confirm two gateway deployments; add a third env, confirm a third deploy; attempt a
second endpoint on an already-bound env, confirm rejection; bind an agent, confirm it resolves
to the endpoint's per-env artifact.

---

## 7. Open items to confirm during implementation

1. **Endpoint→artifacts FK (§3.2):** default is to drop it — artifacts rows belong to the
   per-(endpoint,env) deployments, not the endpoint config row. Confirm against how
   `ensureMCPProxyEnvArtifactRow` and the `deployments`/`deployment_status` FKs cascade.
2. **DTO env-status shape (§4.2/§4.7):** `map[envUuid]deploymentStatus` vs
   `[]{environmentUuid, deploymentStatus}` — pick one, mirror in OpenAPI and the console
   coordination note.
3. **Artifact handle format (§4.5):** `{proxy}-{endpoint}-{envSuffix}` vs `{endpoint}-{envSuffix}`
   — depends on whether endpoint handles are unique per org or only per proxy. Must keep
   `artifacts UNIQUE(handle, org)` satisfied.
4. **New sentinel error** `ErrMCPEnvAlreadyBound` (or similar) in `utils/` for the
   cross-endpoint env-uniqueness violation.

---

## 8. Non-goals

- No data migration from existing `mcp_proxies.configuration.environments` (decision #3).
- No change to `mcp_proxy_mappings` (decision #6).
- No change to `env_agent_mcp_mapping` schema or the agent proxy-level binding contract
  (decision #4).
- No console implementation in this PR (flagged in §4.9).
- No change to gateway deployment YAML / event contracts.

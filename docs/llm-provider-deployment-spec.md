# LLM Provider — Deployment Tab & Environment-Aware Gateway Selection

**Status:** Draft for review
**Area:** `console` (agent-manager console). One optional backend follow-up, called out separately.
**Related art:** [Mockup](https://claude.ai/code/artifact/831e749e-294d-406c-9d57-d340e2a01760)

---

## 1. Problem

An LLM provider's gateway placement can only be chosen once, at creation, through a flat
multi-select of every egress-capable gateway in the org. After that, the console offers no
way to see where the provider is deployed, deploy it somewhere new, or move it. Three
concrete failures follow:

1. **No post-create management.** `ViewLLMProvider.tsx:54` defines 8 tabs — Overview,
   Connection, Access Control, Security, API Keys, Rate Limiting, Guardrails, Consumers.
   None of them read or write `gateways`. The Overview tab renders
   `"No invoke URLs available. Deploy this provider to an AI gateway…"`
   (`LLMProviderOverviewTab.tsx:457`) with no affordance to do so.

2. **Silent create-time deploy failure strands providers with zero gateways.**
   `CreateLLMProvider` (`controllers/llm_controller.go:352-370`) collects
   `resp.Deployments []DeploymentResult` and **only logs** the failures — the HTTP response
   is a plain 200 with the provider body. A provider whose every deploy failed looks
   identical to one that succeeded. Since `GET` derives `gateways` from
   `deployment_status.status = DEPLOYED` only (`repositories/deployment_repository.go:400`),
   such a provider reports `gateways: []` forever, with no retry path in the UI. This is
   the "it's not been deployed to any" case.

3. **The flat picker can express an invalid selection.** The server enforces one gateway
   per `(provider, environment)` via `validateEgressPlacement`
   (`services/gateway_roles.go:169`), resolving each gateway's environments through
   `gateway_environment_mappings`. The console does not model environments at all, so two
   gateways sharing one environment are both selectable and the save 400s.

The backend already supports everything needed to fix (1) and (3): `PUT` with a non-nil
`gateways` array routes to `LLMProviderService.UpdateAndSync`
(`controllers/llm_controller.go:601`), which diffs against currently-deployed gateways and
deploys / updates / undeploys, validating placement up front and hard-failing *before*
touching existing deployments (`services/llm_provider_service.go:656-681`).

## 2. Prior art — how MCP proxies solve this

MCP proxies are environment-first. A binding is
`{environmentUuid, gatewayId?, deploymentStatus}` (`types/src/api/mcp-proxies.ts:74`); the
user picks *environments*, and `gatewayId` is requested only when it is genuinely ambiguous:

| Egress candidates in env | MCP behaviour |
|---|---|
| 0 | Environment is not offered |
| 1 | Auto-assigned; `gatewayId` omitted and inferred server-side |
| 2+ | `Select` rendered (`EndpointFormFields.tsx:521-567`); save blocked until chosen (`hasGatewaySelection`, `:218`) |
| already deployed | `Select` disabled, caption *"Placement is fixed once deployed. Delete and recreate the binding to change it."* |

Deployment state is visible as per-environment chips
(`MCPProxyOverviewTab.tsx:173-195`) and editable post-create via `EditMCPProxyDrawer` →
`EndpointsEditorSection`, which also renders the *"N of M environments have an endpoint"*
footer (`EndpointsEditorSection.tsx:235-240`).

**This spec adopts MCP's interaction model while keeping the LLM provider's flat
`gateways: string[]` wire format.** No API shape change is required.

## 3. Scope

**In scope**

- A new **Deployment** tab on the LLM provider detail page.
- Replacing the create form's flat gateway picker with the same environment-grouped control.
- A deployment summary card on the Overview tab.

**Out of scope**

- Adding an `environments` concept to the LLM provider API. Environments stay a client-side
  projection derived from `gateway.environments`.
- Per-environment `deploymentStatus` on the LLM provider response (see §10).
- Surfacing per-gateway deploy failures — deferred and scoped in §9.
- Any change to `llm_proxy` (the per-project proxy), the eval Monitor drawer flow, or
  MCP proxies.

## 4. Target design

### 4.1 New shared component: `EnvironmentGatewaySelector`

One component drives both the create form and the Deployment tab.

**Location:** `console/workspaces/libs/shared-component/src/components/EnvironmentGatewaySelector/`

Modeled on `EndpointFormFields.tsx:159-191` + `:521-567`, but reduced: no endpoint
name/URL/auth/capability concerns, and its value is a flat gateway-UUID list rather than a
per-environment binding array.

```ts
export interface EnvironmentGatewaySelectorProps {
  orgId: string;
  /** Gateway UUIDs currently selected. In the Deployment tab this is seeded from
   *  `providerData.gateways` (deployed-only, see §5). */
  value: string[];
  onChange: (gatewayIds: string[]) => void;
  /** Gateway UUIDs already deployed — rendered locked. Omit in the create form. */
  lockedGatewayIds?: string[];
  /** Blocks the caller's save action while any 2+-candidate environment is
   *  selected without a resolved gateway. */
  onValidityChange?: (isValid: boolean) => void;
  disabled?: boolean;
}
```

**Data sources**

| Data | Hook | Notes |
|---|---|---|
| Environments | `useListEnvironments({ orgName })` → `Environment[]` | `id`, `name`, `displayName` (`types/src/api/deployments.ts:151`) |
| Gateways | `useListGateways({ orgName }, { limit: 500 })` | Filter `gatewayType === "EGRESS" \|\| "BOTH"` |

Group gateways by `gateway.environments[0].id` into
`Record<string, GatewayResponse[]>` — same shape as `egressGatewaysByEnv`
(`EndpointFormFields.tsx:159-171`). A gateway belongs to exactly one
environment (business rule; the wire field is an array). A gateway with no
mapping is handled by the "Unmapped" row (§7).

> **Pass `{ limit: 500 }`.** `useListGateways` takes an optional query and
> `LLMProviderOverviewTab.tsx:147-150` already passes it; `EndpointFormFields.tsx:154`
> does not, and silently truncates at the server default. Do not repeat that.

**Do not filter on `gateway.status`.** The server's candidate set is not liveness-filtered,
so filtering here would hide a valid choice whenever a gateway is briefly disconnected —
the same reasoning already recorded at `AddLLMProviderForm.tsx:177-179`.

### 4.2 Per-environment row states

One row per environment in the org. **Every row leads with a checkbox** that includes or
excludes that environment; what follows the checkbox is keyed on `candidates.length`:

| State | Condition | Render |
|---|---|---|
| **Auto-assigned** | 1 candidate, not deployed | Checkbox + gateway name as static resolved text. No picker. |
| **Ambiguous** | 2+ candidates, not deployed | Checkbox + `Select` of candidates. While checked and unresolved: error caption *"Select an egress gateway for this environment."* and `onValidityChange(false)`. |
| **Deployed** | Any candidate is in `lockedGatewayIds` | Checkbox (checked; unchecking stages an undeploy) + `Select` `disabled` at the deployed gateway + green `Deployed` chip. Caption: *"Placement is fixed once deployed. To use a different gateway, uncheck this environment and save to undeploy, then select the new gateway and save again."* |
| **Unavailable** | 0 candidates | Checkbox disabled, row dimmed, caption *"No egress-capable gateway is attached to this environment."* |

The checkbox is the single inclusion control in all four states, so the emitted array is
always "the resolved gateway of every checked row". Unchecking a **Deployed** row is how an
undeploy is expressed — which is also the first half of the two-step gateway swap (§6).

Footer: **"N of M environments selected."** — "selected", not "deployed": N counts
unsaved staging too, and in the create form nothing is deployed yet. Only when `M > 1`, matching
`EndpointsEditorSection.tsx:235-240`.

`onChange` emits the flat union of resolved gateway UUIDs across all included
environments. Because at most one gateway per environment can be selected, the emitted
array can never violate the server's placement rule — which is the point.

### 4.3 Deployment tab

Insert into `TABS` in `ViewLLMProvider.tsx:54` at **index 2, after `"Connection"`**
(decided; deployment is a primary concern, not a trailing one).

> `LLMProviderAPIKeysTab` receives `onGoToSecurityTab={() => setTabIndex(TABS.indexOf("Security"))}`
> (`ViewLLMProvider.tsx:251`), which is index-derived and stays correct. Every other
> `TabPanel index={n}` is a literal and **must be renumbered** — this is the one mechanical
> trap in the change.

New file: `console/workspaces/pages/llm-providers/src/subComponents/LLMProviderDeploymentTab.tsx`

```ts
export type LLMProviderDeploymentTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  orgName: string | undefined;
  isLoading?: boolean;
  onUpdate: (fields: UpdateLLMProviderRequest) => Promise<LLMProviderResponse>;
  isUpdating: boolean;
};
```

Follows the established tab contract exactly — compare `LLMProviderConnectionTab.tsx`:
seed local state from `providerData` once per provider UUID via an
`initializedProviderIdRef` guard (`:74`, `:90-103`), derive `isDirty`, and render
Discard / Save with `disabled={!isDirty || isUpdating}`.

**Save:** `onUpdate({ gateways: nextGatewayIds })`.

`updateProvider` in `ViewLLMProvider.tsx:99-110` spreads `...providerData` into the body,
so `gateways` is *already* round-tripped on every existing tab save (harmless — it
reconciles to itself). This tab simply varies the field.

### 4.4 Overview tab summary card

Add a `Deployment` card to the existing grid in `LLMProviderOverviewTab.tsx:333-447`,
alongside Context / Upstream URL / Auth Type / Access Control / In Catalog. One chip per
environment the provider is deployed to, `color="success"`, mirroring
`MCPProxyOverviewTab.tsx:180-195`. Empty state: *"Not deployed"* with a link that switches
to the Deployment tab.

### 4.5 Create form

In `AddLLMProviderForm.tsx`, replace **both** gateway-picker branches — the
`showGatewaySelector` multi-select (`:642-666`) and the single-select fallback
(`:681-707`) — with `EnvironmentGatewaySelector`, keeping the existing
zero-egress-gateway warning `Alert` (`:673-678`).

The auto-select effect at `:190-194` becomes redundant (the selector auto-assigns
single-candidate environments itself) and should be deleted. Retain the submit guard at
`:740-744` that blocks an empty `gatewayIds`, and additionally gate on the selector's
`onValidityChange`.

`showGatewaySelector` currently only toggles between two equally environment-blind
controls. **Remove the prop entirely** and update its one consumer
(`MonitorLLMProviderDrawer.tsx:530`) — the eval drawer gets the same full selector as the
main create form. No `compact` variant; a second control is what created this divergence in
the first place.

## 5. Save semantics

```
PUT /organizations/{org}/llm-providers/{providerId}
{ ...providerData, "gateways": ["<uuid>", "<uuid>"] }
```

`UpdateAndSync` reconciles: deploy to added, update in place for retained, undeploy from
removed. Placement is validated for the whole requested set *before* any deploy or
undeploy runs, and a violation returns `400` with existing deployments untouched
(`services/llm_provider_service.go:648-681`).

**The response body does not confirm the outcome.** Two facts, both verified:

- `UpdateLLMProvider` discards `resp.Deployments` / `resp.Undeployments` — they are logged
  and dropped (`controllers/llm_controller.go:625-651`).
- The `PUT` response never calls `SetGateways`; only `GET` does
  (`controllers/llm_controller.go:487`).

So the tab **must not** trust the mutation result. `useUpdateLLMProvider` invalidates
`["llm-provider"]` (`hooks/llm-providers.ts:241-243`), so the refetched `GET` is the source
of truth for what actually deployed. The tab reconciles its local selection against that
refetch; a gateway that fails to deploy simply does not come back in `gateways`, and the
row reverts to undeployed.

> **Bug to fix in the same change:** `useUpdateLLMProvider` does **not** invalidate
> `["llm-deployments"]`, which `LLMProviderOverviewTab.tsx:143` uses to build invoke URLs.
> Deploying via the new tab therefore leaves the Overview tab's invoke URLs stale until
> remount. Add that invalidation to the hook's `onSuccess`.

## 6. Recoverability analysis

The goal of this change is that **no state reachable through the UI is a dead end**. Each
candidate dead end below was traced through the service and repository layers; all are
escapable console-side, so **no backend change is required to ship this tab**.

| Candidate dead end | Verdict | Evidence |
|---|---|---|
| Provider created with all deploys failed → `gateways: []` | **Recoverable.** A genuine deploy error (bad gateway UUID, org mismatch, placement violation, YAML gen failure) returns before any row is written, so nothing stale blocks a retry. The tab shows "Not deployed"; selecting an environment and saving re-enters the deploy branch of `UpdateAndSync`. | `llm_deployment_service.go:116-215`; `llm_provider_service.go:698-701` |
| Stale `deployment_status` row blocks re-deploy | **Cannot occur.** `deployment_status` is UPSERTed on `(artifact_uuid, ou_id, gateway_uuid)`, so a retry overwrites rather than conflicts. | `deployment_repository.go:131-144` |
| Undeploy leaves the environment permanently occupied | **Cannot occur.** Undeploy flips the status row to `UNDEPLOYED` via `SetCurrent`; `GetDeployedGatewaysByProvider` filters on `DEPLOYED`, so the environment frees immediately and a different gateway can be deployed there. | `llm_deployment_service.go:305-310`; `deployment_repository.go:400-411` |
| Repeated deploy/undeploy cycling exhausts the 25-row limit | **Cannot occur.** `ARCHIVED` is *derived* — a `deployments` row with no matching `deployment_status` row. Since only one status row exists per `(artifact, gateway)`, every prior deployment is archived by construction, so pruning always finds candidates. | `models/deployment.go:58`; `deployment_repository.go:100-124` |

**One design constraint falls out of this.** Swapping the gateway within an
already-deployed environment **cannot be done in a single save**: the placement accumulator
seeds from `currentGateways`, so a body containing both the outgoing and incoming gateway
fails validation and 400s before anything is touched
(`llm_provider_service.go:648-681`). The user must undeploy, save, then redeploy.

The locked-row design in §4.2 already enforces exactly this — a deployed row's `Select` is
disabled, so the only way to change it is to deselect the environment (save → undeploy)
and then reselect with a different gateway (save → deploy). The two mechanisms agree; the
locked-row caption should make the two-step explicit rather than leaving the user to
discover it via a 400.

## 7. Edge cases

| Case | Behaviour |
|---|---|
| Org has zero egress-capable gateways | Every row `Unavailable`; keep the existing warning Alert; Save disabled |
| Environment has zero egress gateways | Row disabled with explanatory caption (do not hide it — hiding makes the org look misconfigured in a way the user can't see) |
| Deployed gateway later deleted | It drops out of `gateways` on the next `GET`; the row reverts to undeployed. Acceptable |
| Gateway attached to 2+ environments | **Cannot happen** — a gateway belongs to exactly one environment (business rule; the wire field is an array only in shape). No cross-row conflict exists; the placement invariant reduces to replacing the previous same-environment selection when a row's candidate changes. |
| Provider deployed to a gateway with no environment mapping | Cannot be attributed to a row. Render as an extra "Unmapped" locked row so it is visible and not silently dropped from the emitted array on save |
| Deselecting the last environment | Emits `[]` → `UpdateAndSync` undeploys everything. Confirm via dialog; copy: *"This will undeploy the provider from all gateways. Invoke URLs will stop working."* |
| User wants a different gateway in a deployed environment | Not doable in one save (see §6). Locked row forces deselect → save → reselect → save. Caption must say so |

## 8. Files touched

| File | Change |
|---|---|
| `libs/shared-component/src/components/EnvironmentGatewaySelector/*` | **New.** Selector + index export |
| `pages/llm-providers/src/subComponents/LLMProviderDeploymentTab.tsx` | **New.** Tab wrapper |
| `pages/llm-providers/src/subComponents/ViewLLMProvider.tsx` | Add tab to `TABS`; add `TabPanel`; **renumber literal indices** |
| `pages/llm-providers/src/subComponents/LLMProviderOverviewTab.tsx` | Add Deployment summary card |
| `pages/llm-providers/src/subComponents/AddLLMProviderForm.tsx` | Replace both picker branches; drop `showGatewaySelector` and the `:190-194` auto-select effect |
| `pages/eval/src/subComponents/MonitorLLMProviderDrawer.tsx` | Drop the `showGatewaySelector={false}` prop |
| `libs/api-client/src/hooks/llm-providers.ts` | Invalidate `["llm-deployments"]` in `useUpdateLLMProvider` |

## 9. Deferred: silent per-gateway deploy failure

**Not in this change.** Scoped here so it can be picked up as separate work.

A create or update whose per-gateway deploys fail returns `200` with a normal provider
body. `CreateLLMProvider` and `UpdateLLMProvider` both collect
`[]DeploymentResult{GatewayID, Success, Error}` and only write them to the log
(`controllers/llm_controller.go:360-370`, `:625-651`). The client cannot tell a fully
successful deploy from a fully failed one except by refetching `GET` and noticing
`gateways` is shorter than requested — and even then it gets no reason.

Per §6 this is a **diagnosability** gap, not an unrecoverable one: the tab surfaces
"Not deployed" and the user can retry. That is why it is deferred.

Scope when picked up:
- Add `deployments` / `undeployments` (`[{gatewayId, success, error?}]`) to the `POST` and
  `PUT` LLM provider response schemas in the OpenAPI spec. Additive and backward-compatible.
- Have both controllers pass `resp.Deployments` / `resp.Undeployments` through instead of
  dropping them; call `SetGateways` on the `PUT` response so it matches `GET`.
- Console: render partial failure after save — "Deployed to 1 of 2 gateways" with the
  per-gateway reason — instead of relying on the refetch diff.
- Follow the `add-api-resource` skill for the spec-first + codegen workflow.

Note that a *broadcast* failure is deliberately swallowed and still records `DEPLOYED`
(`llm_deployment_service.go:249-257`, `:325-333`). So even after this work, `gateways`
means "the control plane believes it is deployed", not "the gateway acknowledged it".
Reporting true gateway-side health is a larger piece of work and is not implied here.

## 10. Decisions

All blocking questions are resolved; the spec is implementable as written.

| # | Question | Decision |
|---|---|---|
| 1 | Tab placement | **Index 2**, after Connection. Renumber the literal `TabPanel index={n}` props (§4.3) |
| 2 | Environment inclusion control | **Per-row checkbox** in all four states (§4.2) |
| 3 | Locked-row copy | Drafted in §4.2 — states the two-step in terms of check/save rather than MCP's "binding" noun. Copy review welcome; not blocking |
| 4 | One Save or two | **Single Save**, one `PUT` reconciled in one pass. Confirmation still required when the diff removes the last environment (§7) |
| 5 | `MonitorLLMProviderDrawer` | **Full selector**, same component as the create form. `showGatewaySelector` removed, no compact variant (§4.5) |
| 6 | Backend work needed for recoverability? | **No** — every candidate dead end is escapable console-side (§6) |
| 7 | Per-environment `deploymentStatus` like MCP? | **Not now.** Inference from `gateways` is adequate; revisit only if `Failed` must be distinguished from `Undeployed` |
| 8 | Surface per-gateway deploy failures? | **Yes, separately** — deferred and scoped in §9 |

**Assumption worth checking during review:** decision 4 keeps the confirm dialog only for
the remove-last-environment case (§7). If reviewers want a confirm on *any* undeploy — e.g.
the first half of a gateway swap — that is a one-line change to the same guard.

## 11. Test plan

**Unit — `EnvironmentGatewaySelector`**
- 1 candidate, checked → no `Select`; gateway present in `onChange`
- 1 candidate, unchecked → contributes nothing to `onChange`
- 2 candidates, checked → `Select` rendered; `onValidityChange(false)` until chosen
- 2 candidates, unchecked and unresolved → does **not** block validity
- 0 candidates → checkbox disabled, contributes nothing to `onChange`
- `lockedGatewayIds` → checkbox checked, `Select` disabled, `Deployed` chip present
- Unchecking a locked row → its gateway drops out of `onChange` (stages the undeploy)
- Choosing a different candidate in an environment → previous selection evicted from `onChange`, never joined
- `onChange` never emits 2 gateways sharing an environment (property test over generated fixtures)

**Unit — `LLMProviderDeploymentTab`**
- Seeds from `providerData.gateways`; `isDirty` false on mount
- Save calls `onUpdate` with exactly `{ gateways: [...] }`
- Provider with `gateways: []` renders all rows undeployed and Save enabled once a choice is made
- Post-save refetch returning fewer gateways than requested reconciles down (deploy-failure path)

**Integration**
- Create → deploy to env A → verify invoke URL appears on Overview without remount (guards the `["llm-deployments"]` invalidation fix)
- Add env B, save, verify both deployed
- Remove env A, save, verify undeployed and its invoke URL disappears
- Attempt a second gateway in an already-deployed environment → not offered by the UI

Follow `add-service-unit-test` conventions for any Go-side test; console tests follow the
existing pattern in `pages/add-new-agent/src/utils/buildAgentPayload.test.ts`.

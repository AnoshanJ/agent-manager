# LLM Provider Configuration — Full Context for Continuation

**Date:** 2026-05-25  
**Working directory:** `/Users/menakajayawardena/Work/AI/cloud-int/agent-manager/console`  
**Branch:** `main` (all changes are uncommitted, working tree only)

---

## What This Feature Is

An agent can be configured with one or more LLM provider configurations. Each config represents a logical LLM connection (e.g. "OpenAI for this agent"). The config maps to **different catalog-managed provider entries per environment** (Dev, Staging, Prod), but exposes the **same environment variable names** across all envs — the platform injects the actual URL/API key values at runtime per environment.

The config is stored as a single record (`AgentModelConfigResponse`) with `envMappings` keyed by environment name. Think of it as: one row in the listing = one LLM connection used by the agent across all its deployment environments.

---

## Data Model (TypeScript types)

**File:** `workspaces/libs/types/src/api/agent-model-configs.ts`

```ts
// The two env var keys are always "url" and "apikey"
interface EnvironmentVariableConfig {
  key: string;   // "url" | "apikey"
  name: string;  // user-editable, e.g. "OPENAI_URL"
}

// Per-environment mapping in a request
interface EnvModelConfigRequest {
  providerName?: string;     // catalog provider handle
  providerUuid?: string;
  configuration: {
    policies?: LLMPolicy[];  // guardrails, per-env
  };
}

// Full config response
interface AgentModelConfigResponse {
  uuid: string;
  name: string;              // auto-generated, not shown to users
  description?: string;      // unused (removed from UI)
  agentId: string;
  type: "llm" | "mcp" | "other";
  envMappings: Record<string, EnvProviderConfigMappings>;  // keyed by env name
  environmentVariables: EnvironmentVariableConfig[];       // shared across envs
  createdAt: string;
  updatedAt: string;
}

// Per-env response (richer than request)
interface EnvProviderConfigMappings {
  environmentName: string;
  configuration?: {
    providerName: string;
    proxyUuid: string;
    providerUuid?: string;
    url: string;
    authInfo?: { type: string; in: string; name: string; value?: string };
    policies?: LLMPolicy[];
    status?: string;
  };
}

// List item (no envMappings, no environmentVariables — summary only)
interface AgentModelConfigListItem {
  uuid: string;
  name: string;
  description?: string;
  agentId: string;
  type: AgentModelConfigType;
  organizationName: string;
  projectName: string;
  createdAt: string;
  updatedAt?: string;
}
```

**Key design rules confirmed during this session:**
- `name` is required by the data model but should NOT be shown to users. It is auto-generated from the provider template (e.g. `openai`, `openai-2` for duplicates).
- `description` is in the model but has been removed from the UI entirely — serves no purpose.
- `environmentVariables` (the `key`→`name` mappings) are **shared across all environments**. The same `OPENAI_URL` name is used in Dev, Staging, and Prod. The platform injects env-specific values at runtime.
- `policies` (guardrails) are **per-environment** — each env in `envMappings` has its own `policies` array.

---

## Files Changed (all uncommitted)

### 1. `workspaces/pages/configure-agent/src/AddLLMProvider.Component.tsx`
**Purpose:** Add / Edit form for a single LLM provider config.

**What was changed:**
- Removed `name` and `description` state and form fields entirely. "Basic Details" section deleted.
- Added `generateConfigName(templateId, existingNames)` — generates `openai`, `openai-2`, etc. from the provider's template ID, using existing config names to avoid collision.
- Added `useListAgentModelConfigs` hook call to get existing names for uniqueness check.
- Auto-generates env var names from the **selected provider's template** (not from a user-typed name). Uses `primaryTemplate` derived from the first env's selected provider.
- `guardrails` state changed from a flat `GuardrailSelection[]` to `guardrailsByEnv: Record<string, GuardrailSelection[]>` — guardrails are now properly per-env.
- The three guardrail handlers (`handleAddGuardrail`, `handleEditGuardrail`, `handleRemoveGuardrail`) now operate on `guardrailsByEnv[selectedEnvName]`.
- `handleSave` builds per-env policies from `guardrailsByEnv[env.name]` for each environment.
- `handleSave` no longer requires `name.trim()` — generates name automatically from resolved template.
- `isFormValid` is now just `hasAnyProvider` (no name check).
- `selectedEnvName` useMemo moved **before** the guardrails useMemo to fix a `ReferenceError: Cannot access 'selectedEnvName' before initialization`.
- Env tabs (when `environments.length > 1`) now show a warning dot (orange circle) on any tab with no provider selected.
- Framing text added above tabs: *"Select which catalog provider to use in each environment."*
- Unused imports removed: `FormControl`, `FormLabel`, `TextField`.

**Current state of the form (single env):**
```
Page: "Add LLM Provider"
├── LLM Service Provider (Form.Section)
│   ├── [subtitle: "Select which catalog provider..."] (only if >1 env)
│   ├── [Tabs with warning dots] (only if >1 env)
│   ├── Service Provider (Form.Subheader)
│   │   └── Selected provider card -or- "Select a Service Provider" button
│   └── Guardrails (GuardrailsSection — per selected env tab)
└── Environment Variables (Form.Section, only if hasAnyProvider && !isExternal)
    ├── [subtitle explaining shared names + runtime injection]
    └── Table: Variable Name (editable) | Description
        ├── OPENAI_URL
        └── OPENAI_API_KEY
    Cancel | Save (disabled until hasAnyProvider)
```

### 2. `workspaces/pages/configure-agent/src/ViewLLMProvider.Component.tsx`
**Purpose:** View/edit an existing config.

**What was changed:**
- Moved env var section OUT of an `<Alert severity="info">` into a proper `<Form.Section>` titled **"Environment Variable Names"** with subtitle: *"These variable names are injected into the agent at runtime with environment-specific values. Rename them here if your code already uses different names — then save."*
- Added `"(editable)"` caption to the Variable Name column header.
- Split the code snippet into its own separate `<Form.Section>` titled **"Integration Guide"** with subtitle.
- Added an unsaved-changes warning banner at the top: `<Alert severity="warning">` with inline **Discard** and **Save changes** buttons — appears whenever `isDirty` is true, replaces the old bottom-of-page Save/Cancel pattern.
- Added `<Form.Header>LLM Service Provider</Form.Header>` to the outer provider section so the env tabs are no longer floating orphaned.
- Added framing text above tabs (when `>1 env`): *"Each environment uses a separate catalog provider. The same variable names are injected in all environments with environment-specific values."*
- Removed redundant inner `<Form.Section><Form.Header>LLM Service Provider</Form.Header>` that was wrapping the provider card.
- `GuardrailsSection` is now inside the env-tabbed stack (after the provider card), so guardrails correctly reflect the selected environment. This was already correct in the data model (`guardrailsByEnv`), now the UI matches.

**Current layout:**
```
Page title: {config.name}  [config.description if set]

[Unsaved changes banner with Discard + Save changes] (if isDirty)

Environment Variable Names (Form.Section)
├── subtitle
└── Table: Variable Name (editable) | Description

Integration Guide (Form.Section)
├── subtitle
├── [Python] [AI Prompt] toggle
└── Code snippet (read-only, copyable)

LLM Service Provider (Form.Section)
├── [framing text + Tabs] (if >1 env)
├── [isExternal: Connect to your LLM Provider section]
├── Provider card (ProviderDisplay)
└── GuardrailsSection (per selected env)
```

### 3. `workspaces/pages/configure-agent/src/Configure/subComponents/AgentLLMProvidersSection.tsx`
**Purpose:** Listing table on the Configure Agent page.

**What was changed:**
- Removed "Description" column (nothing to show since description was removed from the UX).
- Renamed "Name" column to **"Provider"**.
- Row now shows: bold display name (capitalized, hyphens stripped, trailing `-N` suffix removed) + caption with the internal generated name (e.g. `openai`).
- `colSpan` updated from 4 → 3 to match new column count.
- Search placeholder updated: `"Search by name or type..."` (removed "description").
- Search filter no longer checks `description` field.

---

## Key Design Decisions Made in This Session

### Q: Is grouping multiple env providers under one config the right approach?
**Decision: The grouped approach is architecturally correct but the form is implemented wrong.**

The current tab-based form is wrong for multi-env. The correct UX is a **side-by-side table** showing all environments simultaneously, so users configure all envs in one view without hidden state:

```
┌─────────────────────────────────────────────────────────┐
│  Dev      │  [eeee - OpenAI ▾]  │  Guardrails: 1  │     │
│  Staging  │  [Select provider]  │  Guardrails: 0  │     │
│  Prod     │  [Select provider]  │  Guardrails: 0  │     │
└─────────────────────────────────────────────────────────┘
```

**This has NOT been implemented yet.** The current code still uses env tabs. This is the next major work item.

### Q: Do we need a name field?
**Decision: No. Remove from UI, auto-generate internally.**
- Generated from provider template: `openai`, `anthropic`, `azure-openai`
- Suffix added for duplicates: `openai-2`, `openai-3`
- Function: `generateConfigName(templateId, existingNames)` in `AddLLMProvider.Component.tsx`

### Q: Do we need a description field?
**Decision: No. Removed entirely.**

### Q: Are guardrails per-env or global?
**Decision: Per-env.** Both `AddLLMProvider` and `ViewLLMProvider` now use `guardrailsByEnv: Record<string, GuardrailSelection[]>`.

### Q: Are env var names per-env or shared?
**Decision: Shared across all envs.** Same `OPENAI_URL` / `OPENAI_API_KEY` names are used in all environments. The platform injects environment-specific values at runtime.

---

## Remaining Issues NOT Yet Fixed

### HIGH — Wrong form layout for multi-env (the main next task)
The current tab-based Add/Edit form should be replaced with a table layout where all environments are visible simultaneously as rows. Each row: env name | provider picker | guardrails count/button. This eliminates:
- Hidden state (partial configs silently save)
- Tab-switching confusion about what's per-env vs shared
- The unclear scope of env tabs

### MEDIUM — Save enabled with incomplete envs, no warning
`isFormValid = hasAnyProvider` — only one env needs a provider. With 3 envs, saving with only Dev configured is silently allowed. Should warn: "Staging and Prod have no provider selected."

### MEDIUM — "Service Provider" sub-section header inside the tab is redundant
The sub-header "Service Provider" (Form.Subheader) inside the tabbed area adds no value. Remove it.

### LOW — Warning dot on tab is too subtle
Orange dot next to env tab name works but a user might miss it. Consider "Default (not configured)" text or a completion summary "1 / 3 environments configured" above the tabs.

---

## Architecture Notes

- **Console stack:** React 19, TypeScript, Vite, MUI 7 / Oxygen UI, TanStack Query, Rush/pnpm monorepo
- **Package:** `workspaces/pages/configure-agent` — builds as a workspace package, consumed by `apps/webapp`
- **API hooks:** All in `workspaces/libs/api-client/src/hooks/agent-model-configs.ts`
  - `useListAgentModelConfigs` — list configs for an agent
  - `useGetAgentModelConfig` — get single config
  - `useCreateAgentModelConfig` — create
  - `useUpdateAgentModelConfig` — update
  - `useDeleteAgentModelConfig` — delete
- **Catalog providers:** `useListCatalogLLMProviders` — lists org-level catalog entries (the platform-managed providers users pick from)
- **Templates:** `useListLLMProviderTemplates` — metadata about provider types (OpenAI, Anthropic, etc.) including logo URLs
- **Environments:** `useListEnvironments` — org-level environment list (Default, Staging, Prod, etc.)
- **Dev server:** `make dev` from `agent-manager/console` — runs at `http://localhost:3000`
- **Test URL:** `http://localhost:3000/org/default/project/default/agents/ssdsdsdsd/configure/llm-providers/view/93451bd8-ef7c-4f35-954b-176cdb3e092e`

## Route Structure
```
/org/:orgId/project/:projectId/agents/:agentId/configure
  → AgentLLMProvidersSection (listing table)
  
/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers/add
  → AddLLMProviderComponent (create form)

/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers/edit/:configId
  → AddLLMProviderComponent (edit form, isEditMode=true)

/org/:orgId/project/:projectId/agents/:agentId/configure/llm-providers/view/:configId
  → ViewLLMProviderComponent (view + inline edit)
```

## Key Components

### `ProviderDisplay` (exported from `AddLLMProvider.Component.tsx`)
Reusable display component for a catalog provider entry. Used in both the Add/Edit form (inside the drawer and as the selected card) and the View page. Props:
```ts
{
  provider: { name, template?, version?, deployments?, security?, rateLimiting?, policies? } | null
  isSelected: boolean
  hideCheckbox?: boolean
  templateInfo?: { displayName: string; logoUrl?: string } | null
  fallbackLabel?: string
}
```

### `GuardrailsSection` (from `@agent-management-platform/llm-providers`)
Manages the list of guardrail policies for one environment. Props:
```ts
{
  guardrails: GuardrailSelection[]
  onAddGuardrail: (g: GuardrailSelection) => void
  onEditGuardrail: (g: GuardrailSelection) => void
  onRemoveGuardrail: (name: string, version: string) => void
}
```

---

## What the Next Agent Should Do

**Primary task:** Redesign the Add/Edit form (`AddLLMProvider.Component.tsx`) to replace the env-tab layout with a table/grid layout where all environments are shown as rows simultaneously.

**Design spec:**
```
LLM Service Provider (Form.Section)
├── For each environment — one row/card:
│   ├── Env name label
│   ├── Provider picker (selected card or "Select" button)
│   └── Guardrails (inline count + expand, or separate section below)
└── [warn if any env has no provider on Save attempt]

Environment Variables (Form.Section, shared — shown once below)
├── Auto-generated from template of first selected provider
└── Table: Variable Name (editable) | Description
```

**Constraints:**
- Data model stays the same — `envMappings` as `Record<string, EnvModelConfigRequest>`
- Guardrails are per-env
- Env var names are shared (one table, not per-env)
- Config name is still auto-generated (do not add a name field back)
- `isFormValid` should require ALL environments to have a provider (not just one)
- The drawer for provider selection already works — reuse it, just trigger it per-row with the env context

**Files to focus on:**
1. `workspaces/pages/configure-agent/src/AddLLMProvider.Component.tsx` — main rewrite
2. `workspaces/pages/configure-agent/src/ViewLLMProvider.Component.tsx` — same layout change for the view page's env section

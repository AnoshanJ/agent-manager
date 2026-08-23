export type AuthMode = "none" | "oidc";

export interface AppConfig {
  agentUrl: string;
  authMode: AuthMode;
  issuer: string;
  clientId: string;
  scopes: string;
  companyName: string;
}

const CHAT_PATH = "/chat";

export function chatEndpoint(base: string): string {
  const b = (base || "").trim().replace(/\/+$/, "");
  if (!b) return `http://localhost:10150${CHAT_PATH}`;
  return b.endsWith(CHAT_PATH) ? b : b + CHAT_PATH;
}

const AGENT_KEY = "insurance.agentUrl";

// Gated to mode "none": with a token attached, ?agent= would leak it to any origin.
function resolveAgentUrl(mode: AuthMode): string {
  const configured = import.meta.env.VITE_AGENT_URL ?? "";
  let override: string | null = null;
  try {
    const q = new URLSearchParams(window.location.search).get("agent");
    if (q === "reset") {
      window.localStorage.removeItem(AGENT_KEY);
    } else if (mode === "none") {
      if (q) {
        window.localStorage.setItem(AGENT_KEY, q);
        override = q;
      } else {
        override = window.localStorage.getItem(AGENT_KEY);
      }
    }
  } catch {
    override = null;
  }
  return chatEndpoint(override || configured);
}

const VALID_MODES: AuthMode[] = ["none", "oidc"];

function normalizedMode(): string {
  return (import.meta.env.VITE_AUTH_MODE ?? "").trim().toLowerCase();
}

function resolveMode(): AuthMode {
  const raw = normalizedMode();
  return raw === "oidc" ? "oidc" : "none";
}

const RAW_AUTH_MODE = import.meta.env.VITE_AUTH_MODE ?? "";
const AUTH_MODE = resolveMode();

export const CONFIG: AppConfig = {
  agentUrl: resolveAgentUrl(AUTH_MODE),
  authMode: AUTH_MODE,
  issuer: (import.meta.env.VITE_OIDC_ISSUER ?? "")
    .replace(/\/+$/, "")
    .replace(/\/\.well-known\/openid-configuration$/, ""),
  clientId: import.meta.env.VITE_OIDC_CLIENT_ID ?? "",
  scopes: import.meta.env.VITE_OIDC_SCOPES ?? "openid profile email",
  companyName: import.meta.env.VITE_COMPANY_NAME ?? "O2 Insurance",
};

export function configError(): string | null {
  const raw = normalizedMode();
  if (raw && !VALID_MODES.includes(raw as AuthMode)) {
    return `VITE_AUTH_MODE="${RAW_AUTH_MODE}" is not a recognised mode. Use "none" or "oidc".`;
  }
  if (CONFIG.authMode !== "oidc") return null;
  const missing = [
    CONFIG.issuer ? null : "VITE_OIDC_ISSUER",
    CONFIG.clientId ? null : "VITE_OIDC_CLIENT_ID",
  ].filter(Boolean);
  if (!missing.length) return null;
  return `VITE_AUTH_MODE=oidc needs ${missing.join(" and ")}. Set it in web/.env and restart the dev server.`;
}

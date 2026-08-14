/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import type { MCPProxy, SecurityConfig } from "@agent-management-platform/types";
import { resolveAuthenticationType } from "./mcpEndpointSecurity";
import {
  AGENTID_ENV_VAR_ROWS,
  resolveMCPEndpointForEnvironment,
  resolveMCPEnvVarSpec,
  resolveProxyAuthenticationType,
} from "./mcpEnvVarSpec";

// The three options the MCP Servers Security tab offers, in the exact shape it
// saves them (MCPProxySecurityTab always writes enabled: true and flips only the
// two sub-flags — so "none" is NOT the absence of a security object).
const apiKey: SecurityConfig = {
  enabled: true,
  apiKey: { enabled: true, key: "X-API-Key", in: "header" },
  identity: { enabled: false },
};
const identity: SecurityConfig = {
  enabled: true,
  apiKey: { enabled: false, key: "", in: "header" },
  identity: { enabled: true },
};
const none: SecurityConfig = {
  enabled: true,
  apiKey: { enabled: false, key: "", in: "header" },
  identity: { enabled: false },
};

function proxyWith(
  endpoints: { id: string; security?: SecurityConfig; envs?: string[] }[],
): Pick<MCPProxy, "endpoints"> {
  return {
    endpoints: endpoints.map((endpoint) => ({
      id: endpoint.id,
      security: endpoint.security,
      environments: (endpoint.envs ?? []).map((environmentUuid) => ({
        environmentUuid,
      })),
    })),
  };
}

describe("resolveAuthenticationType", () => {
  it("reads the three Security tab options back correctly", () => {
    expect(resolveAuthenticationType({ security: apiKey })).toBe("apiKey");
    expect(resolveAuthenticationType({ security: identity })).toBe("identity");
    expect(resolveAuthenticationType({ security: none })).toBe("");
  });

  it("treats a missing security object as none", () => {
    expect(resolveAuthenticationType({})).toBe("");
    expect(resolveAuthenticationType(undefined)).toBe("");
  });

  it("treats security.enabled false as none whatever the sub-flags say", () => {
    expect(
      resolveAuthenticationType({
        security: { enabled: false, apiKey: { enabled: true } },
      }),
    ).toBe("");
    expect(
      resolveAuthenticationType({
        security: { enabled: false, identity: { enabled: true } },
      }),
    ).toBe("");
  });
});

describe("resolveMCPEnvVarSpec", () => {
  it("offers both keys and no reference rows for an API-key endpoint", () => {
    const spec = resolveMCPEnvVarSpec("apiKey");
    expect(spec.editableKeys).toEqual(["url", "apikey"]);
    expect(spec.referenceRows).toEqual([]);
  });

  it("drops the api key and shows the AgentID rows for OAuth", () => {
    const spec = resolveMCPEnvVarSpec("identity");
    expect(spec.editableKeys).toEqual(["url"]);
    expect(spec.referenceRows).toEqual(AGENTID_ENV_VAR_ROWS);
  });

  // An unsecured endpoint has no credential of any kind, so the URL is the whole
  // contract — an API key field here would produce a permanently empty env var.
  it("offers only the url and no reference rows when security is none", () => {
    const spec = resolveMCPEnvVarSpec("");
    expect(spec.editableKeys).toEqual(["url"]);
    expect(spec.referenceRows).toEqual([]);
  });

  it("always offers the url, whatever the security", () => {
    for (const type of ["apiKey", "identity", ""] as const) {
      expect(resolveMCPEnvVarSpec(type).editableKeys).toContain("url");
    }
  });

  it("returns a fresh editableKeys array so callers cannot mutate the constant", () => {
    resolveMCPEnvVarSpec("apiKey").editableKeys.push("url");
    expect(resolveMCPEnvVarSpec("apiKey").editableKeys).toEqual(["url", "apikey"]);
  });

  it("names exactly the four AgentID variables the platform injects", () => {
    expect(AGENTID_ENV_VAR_ROWS.map((row) => row.name)).toEqual([
      "AMP_AGENTID_CLIENT_ID",
      "AMP_AGENTID_CLIENT_SECRET",
      "AMP_AGENTID_TOKEN_ENDPOINT",
      "AMP_AGENTID_SCOPES",
    ]);
  });
});

describe("resolveMCPEndpointForEnvironment", () => {
  it("picks the endpoint bound to the requested environment", () => {
    const proxy = proxyWith([
      { id: "e1", security: apiKey, envs: ["dev-uuid"] },
      { id: "e2", security: identity, envs: ["prod-uuid"] },
    ]);
    expect(resolveMCPEndpointForEnvironment(proxy, "prod-uuid")?.id).toBe("e2");
    expect(resolveMCPEndpointForEnvironment(proxy, "dev-uuid")?.id).toBe("e1");
  });

  it("is undefined when no endpoint is bound to that environment", () => {
    const proxy = proxyWith([{ id: "e1", security: identity, envs: ["dev-uuid"] }]);
    expect(resolveMCPEndpointForEnvironment(proxy, "staging-uuid")).toBeUndefined();
  });

  it("is undefined without an environment uuid", () => {
    const proxy = proxyWith([{ id: "e1", security: identity, envs: ["dev-uuid"] }]);
    expect(resolveMCPEndpointForEnvironment(proxy, undefined)).toBeUndefined();
  });

  it("is undefined for a missing or endpointless proxy", () => {
    expect(resolveMCPEndpointForEnvironment(null, "dev-uuid")).toBeUndefined();
    expect(resolveMCPEndpointForEnvironment(undefined, "dev-uuid")).toBeUndefined();
    expect(resolveMCPEndpointForEnvironment(proxyWith([]), "dev-uuid")).toBeUndefined();
  });

  // The reason this resolver returns the endpoint rather than its security: an
  // unsecured bound endpoint must read as "none", not as "look elsewhere".
  it("finds an endpoint that carries no security config at all", () => {
    const proxy = proxyWith([
      { id: "bare", envs: ["dev-uuid"] },
      { id: "oauth", security: identity, envs: ["prod-uuid"] },
    ]);
    const endpoint = resolveMCPEndpointForEnvironment(proxy, "dev-uuid");
    expect(endpoint?.id).toBe("bare");
    expect(resolveAuthenticationType(endpoint)).toBe("");
  });
});

describe("resolveProxyAuthenticationType", () => {
  it("is identity only when every endpoint uses identity", () => {
    expect(
      resolveProxyAuthenticationType(
        proxyWith([{ id: "a", security: identity }, { id: "b", security: identity }]),
      ),
    ).toBe("identity");
  });

  it("is apiKey when any endpoint needs a key, so mixed security keeps the field", () => {
    expect(
      resolveProxyAuthenticationType(
        proxyWith([{ id: "a", security: identity }, { id: "b", security: apiKey }]),
      ),
    ).toBe("apiKey");
  });

  it("is none when every endpoint is unsecured", () => {
    expect(
      resolveProxyAuthenticationType(
        proxyWith([{ id: "a", security: none }, { id: "b", security: none }]),
      ),
    ).toBe("");
  });

  it("is none for a mix of unsecured and OAuth, since neither needs a key", () => {
    expect(
      resolveProxyAuthenticationType(
        proxyWith([{ id: "a", security: none }, { id: "b", security: identity }]),
      ),
    ).toBe("");
  });

  it("is none when the proxy has no endpoints", () => {
    expect(resolveProxyAuthenticationType(proxyWith([]))).toBe("");
    expect(resolveProxyAuthenticationType(null)).toBe("");
    expect(resolveProxyAuthenticationType(undefined)).toBe("");
  });
});

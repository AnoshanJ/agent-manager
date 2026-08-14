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

import { useMemo } from "react";
import { useGetMCPProxy } from "@agent-management-platform/api-client";
import {
  resolveMCPEndpointForEnvironment,
  resolveMCPEnvVarSpec,
  resolveProxyAuthenticationType,
  type MCPEnvVarSpec,
} from "./mcpEnvVarSpec";
import {
  resolveAuthenticationType,
  type AuthenticationType,
} from "./mcpEndpointSecurity";

export interface UseMCPProxySecurityParams {
  orgName?: string;
  /** The MCP proxy the tool binding points at. Empty disables the fetch. */
  proxyId?: string | null;
  /**
   * Scope the answer to one environment's endpoint. Omit (or pass undefined,
   * which happens when the environment carries no uuid) to fall back to the
   * environment-agnostic every-endpoint rule.
   */
  environmentUuid?: string;
}

export interface UseMCPProxySecurityResult {
  /** How the resolved endpoint is secured: API key, OAuth, or none. */
  authenticationType: AuthenticationType;
  /** True when the resolved endpoint is secured with OAuth (AgentID). */
  usesIdentitySecurity: boolean;
  /** Which env vars to offer and which to show for reference. */
  spec: MCPEnvVarSpec;
  /** The proxy fetch is still in flight, so `spec` is not yet trustworthy. */
  isLoading: boolean;
}

/**
 * Resolves how an MCP proxy's endpoint is secured and, from that, which runtime
 * variables a tool binding needs. `MCPProxyListItem` carries no security data, so
 * the full proxy has to be fetched — this hook is the one place that does it.
 *
 * When `environmentUuid` is supplied and the proxy has an endpoint bound to it,
 * the answer is that endpoint's security, matching the server's per-environment
 * resolution. Otherwise it falls back to "every endpoint uses OAuth", which is
 * the sound answer when one binding covers all environments at once, and also
 * the safe answer when the environment simply has no uuid to match on.
 */
export function useMCPProxySecurity({
  orgName,
  proxyId,
  environmentUuid,
}: UseMCPProxySecurityParams): UseMCPProxySecurityResult {
  const { data: proxy, isLoading } = useGetMCPProxy({
    orgName,
    proxyId: proxyId ?? "",
  });

  const authenticationType = useMemo<AuthenticationType>(() => {
    if (!proxyId || !proxy) return "";
    const scopedEndpoint = resolveMCPEndpointForEnvironment(proxy, environmentUuid);
    if (scopedEndpoint) {
      return resolveAuthenticationType(scopedEndpoint);
    }
    return resolveProxyAuthenticationType(proxy);
  }, [proxy, proxyId, environmentUuid]);

  const spec = useMemo(
    () => resolveMCPEnvVarSpec(authenticationType),
    [authenticationType],
  );

  return {
    authenticationType,
    usesIdentitySecurity: authenticationType === "identity",
    spec,
    // No proxy selected yet means nothing to wait for, whatever the query says.
    isLoading: Boolean(proxyId) && isLoading,
  };
}

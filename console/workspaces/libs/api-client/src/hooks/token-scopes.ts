/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
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
import { useAuthHooks } from "@agent-management-platform/auth";
import { globalConfig } from "@agent-management-platform/types";

/**
 * Reads the scope claim off the current access token.
 *
 * It lives beside the query hooks because this package is where the token is
 * already read for every request — the scope set is a property of that same
 * token, not a separate resource to fetch.
 *
 * Callers gating a control on the environment tier should use
 * useAgentEnvironmentAccess from @agent-management-platform/shared-component
 * rather than testing scope strings by hand; it encodes the floor/production
 * rule in one place.
 *
 * The return shape is ScopeState from that module, declared there because it is
 * the rule's input rather than this hook's invention. It is not named here: a
 * second declaration of the same two fields is a second thing to keep in step,
 * and this package cannot import from shared-component without a cycle.
 */
export function useTokenScopes(): {
  /**
   * False when this deployment does not enforce RBAC. It mirrors the service's
   * RBAC_ENABLED switch (plus disableAuth, where there is no token at all): the
   * server gates nothing, so the console must not either.
   */
  enforced: boolean;
  scopes: ReadonlySet<string>;
} {
  const { userInfo } = useAuthHooks();
  const scopeStr = userInfo?.scope;
  return useMemo(
    () => ({
      enforced: !globalConfig.disableAuth && globalConfig.rbacEnabled,
      scopes: new Set((scopeStr ?? "").split(" ").filter(Boolean)),
    }),
    [scopeStr],
  );
}

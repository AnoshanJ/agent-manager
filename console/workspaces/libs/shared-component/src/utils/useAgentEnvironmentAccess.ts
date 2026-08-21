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

import { useCallback } from "react";
import { useTokenScopes } from "@agent-management-platform/api-client";

import {
  evaluateAgentEnvironmentAccess,
  type AccessDecision,
  type EnvironmentTier,
} from "./environmentTierAccess";

/**
 * Hook form of {@link evaluateAgentEnvironmentAccess}, bound to the current
 * token. Returns a stable callback so a caller can evaluate several
 * environments — the promotion targets of one card, say — in a single render.
 */
export function useAgentEnvironmentAccess(): (
  environment: EnvironmentTier | undefined,
  ...capabilities: string[]
) => AccessDecision {
  const state = useTokenScopes();
  return useCallback(
    (environment: EnvironmentTier | undefined, ...capabilities: string[]) =>
      evaluateAgentEnvironmentAccess(state, environment, ...capabilities),
    [state],
  );
}

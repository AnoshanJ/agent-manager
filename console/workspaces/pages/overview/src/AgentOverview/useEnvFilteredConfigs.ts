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

import { useCallback, useMemo, useState } from "react";
import type { AgentModelConfigListItem } from "@agent-management-platform/types";

/**
 * Model/MCP configs are agent-wide, but whether one actually applies to a
 * given environment is only knowable from its own envMappings — which the
 * list endpoint doesn't return, so each card resolves its own applicability
 * (see LLMProviderConfigCard/MCPProxyConfigCard) and reports it back here via
 * `reportResolved`. This hook keeps the first `previewLimit` configs (in list
 * order) that resolved as applicable to the current environment, so a config
 * that isn't deployed there never shows on that environment's card — no
 * falling back to another environment's data.
 */
export function useEnvFilteredConfigs(
    candidates: AgentModelConfigListItem[],
    previewLimit: number,
) {
    const [resolved, setResolved] = useState<Record<string, boolean>>({});

    const reportResolved = useCallback((configId: string, applicable: boolean) => {
        setResolved((prev) => (
            prev[configId] === applicable ? prev : { ...prev, [configId]: applicable }
        ));
    }, []);

    const visible = useMemo(
        () => candidates.filter((c) => resolved[c.uuid]).slice(0, previewLimit),
        [candidates, resolved, previewLimit],
    );

    const allResolved = candidates.length > 0 && candidates.every((c) => c.uuid in resolved);
    const isSettled = allResolved || visible.length >= previewLimit;

    return { visible, reportResolved, isSettled };
}

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

import { useEffect } from "react";

/**
 * Shared by LLMProviderConfigCard/MCPProxyConfigCard: picks this environment's
 * mapping out of a fetched config's envMappings, and reports back to
 * useEnvFilteredConfigs (via onResolved) whether the config applies to envId
 * once the fetch settles.
 */
export function useConfigEnvMapping<TMapping>(
    envMappings: Record<string, TMapping> | undefined,
    isLoadingConfig: boolean,
    envId: string,
    configId: string,
    onResolved: (configId: string, applicable: boolean) => void,
): TMapping | undefined {
    const envMapping = envMappings?.[envId];
    const isApplicable = !!envMapping;

    useEffect(() => {
        if (!isLoadingConfig) {
            onResolved(configId, isApplicable);
        }
    }, [isLoadingConfig, isApplicable, configId, onResolved]);

    return envMapping;
}

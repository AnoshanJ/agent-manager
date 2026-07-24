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

import { Box, Skeleton } from "@wso2/oxygen-ui";
import {
    useListAgentMCPConfigs,
    useListAgentModelConfigs,
} from "@agent-management-platform/api-client";
import { buildConfigureTabHref } from "./configureTabLink";
import { LLMProviderConfigCard } from "./LLMProviderConfigCard";
import { MCPProxyConfigCard } from "./MCPProxyConfigCard";
import { SectionHeader } from "./SectionHeader";
import { useEnvFilteredConfigs } from "./useEnvFilteredConfigs";

interface EnvConfigsSectionProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
}

const PREVIEW_LIMIT = 2;
// Configs are agent-wide but only some are deployed to any given environment
// (see useEnvFilteredConfigs), so more candidates than the preview limit are
// fetched to have enough headroom to find PREVIEW_LIMIT that actually apply
// to this environment.
const CANDIDATE_LIMIT = 10;

/**
 * Compact preview of the agent's Model Configs and MCP Proxies, rendered
 * right below the Invoke URL in EnvCapabilitiesSection — just enough to show
 * what's configured for this environment specifically, with a "View all"
 * link to the full list on the Configure Agent page.
 */
export const EnvConfigsSection: React.FC<EnvConfigsSectionProps> = ({
    orgId, projectId, agentId, envId,
}) => {
    const { data: modelData, isLoading: isLoadingModels } = useListAgentModelConfigs(
        { orgName: orgId, projName: projectId, agentName: agentId },
        { limit: CANDIDATE_LIMIT, offset: 0 },
    );
    const { data: mcpData, isLoading: isLoadingMCP } = useListAgentMCPConfigs(
        { orgName: orgId, projName: projectId, agentName: agentId },
        { limit: CANDIDATE_LIMIT, offset: 0 },
    );

    const modelConfigs = modelData?.configs ?? [];
    const mcpConfigs = mcpData?.configs ?? [];

    const {
        visible: visibleModelConfigs,
        reportResolved: reportModelResolved,
        isSettled: isModelSettled,
    } = useEnvFilteredConfigs(modelConfigs, PREVIEW_LIMIT);
    const {
        visible: visibleMcpConfigs,
        reportResolved: reportMcpResolved,
        isSettled: isMcpSettled,
    } = useEnvFilteredConfigs(mcpConfigs, PREVIEW_LIMIT);

    if (isLoadingModels || isLoadingMCP) {
        return <Skeleton variant="rounded" height={56} sx={{ mt: 2 }} />;
    }

    // Nothing to probe at all — not "none apply to this environment yet",
    // which is only knowable once the candidate cards below have resolved.
    if (modelConfigs.length === 0 && mcpConfigs.length === 0) {
        return null;
    }

    return (
        <>
            {modelConfigs.length > 0 && (
                <>
                    {isModelSettled ? (
                        visibleModelConfigs.length > 0 && (
                            <SectionHeader
                                title="LLM Providers"
                                viewAllHref={buildConfigureTabHref(orgId, projectId, agentId, "llm")}
                            />
                        )
                    ) : (
                        <Skeleton variant="rounded" height={56} sx={{ mt: 2 }} />
                    )}
                    <Box
                        display={isModelSettled ? "flex" : "none"}
                        flexDirection="column"
                        gap={1}
                        sx={{ mb: visibleModelConfigs.length > 0 ? 1.5 : 0 }}
                    >
                        {modelConfigs.map((config) => (
                            <LLMProviderConfigCard
                                key={config.uuid}
                                orgId={orgId}
                                projectId={projectId}
                                agentId={agentId}
                                envId={envId}
                                config={config}
                                visible={visibleModelConfigs.some((c) => c.uuid === config.uuid)}
                                onResolved={reportModelResolved}
                            />
                        ))}
                    </Box>
                </>
            )}
            {mcpConfigs.length > 0 && (
                <>
                    {isMcpSettled ? (
                        visibleMcpConfigs.length > 0 && (
                            <SectionHeader
                                title="MCP Proxies"
                                viewAllHref={buildConfigureTabHref(orgId, projectId, agentId, "tools")}
                            />
                        )
                    ) : (
                        <Skeleton variant="rounded" height={56} sx={{ mt: 2 }} />
                    )}
                    <Box
                        display={isMcpSettled ? "flex" : "none"}
                        flexDirection="column"
                        gap={1}
                        sx={{ mb: visibleMcpConfigs.length > 0 ? 1.5 : 0 }}
                    >
                        {mcpConfigs.map((config) => (
                            <MCPProxyConfigCard
                                key={config.uuid}
                                orgId={orgId}
                                projectId={projectId}
                                agentId={agentId}
                                envId={envId}
                                config={config}
                                visible={visibleMcpConfigs.some((c) => c.uuid === config.uuid)}
                                onResolved={reportMcpResolved}
                            />
                        ))}
                    </Box>
                </>
            )}
        </>
    );
};

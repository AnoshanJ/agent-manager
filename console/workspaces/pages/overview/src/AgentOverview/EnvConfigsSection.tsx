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

import {
    useListAgentMCPConfigs,
    useListAgentModelConfigs,
} from "@agent-management-platform/api-client";
import { buildConfigureTabHref } from "./configureTabLink";
import { EnvConfigGroup } from "./EnvConfigGroup";
import { LLMProviderConfigCard } from "./LLMProviderConfigCard";
import { MCPProxyConfigCard } from "./MCPProxyConfigCard";

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
 *
 * Each group collapses itself independently (see EnvConfigGroup) while its
 * own list is loading or empty, so LLM Providers can appear before MCP
 * Proxies (or vice versa) instead of both waiting on whichever list is
 * slower.
 */
export const EnvConfigsSection: React.FC<EnvConfigsSectionProps> = ({
    orgId, projectId, agentId, envId,
}) => {
    const { data: modelData } = useListAgentModelConfigs(
        { orgName: orgId, projName: projectId, agentName: agentId },
        { limit: CANDIDATE_LIMIT, offset: 0 },
    );
    const { data: mcpData } = useListAgentMCPConfigs(
        { orgName: orgId, projName: projectId, agentName: agentId },
        { limit: CANDIDATE_LIMIT, offset: 0 },
    );

    return (
        <>
            <EnvConfigGroup
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
                configs={modelData?.configs ?? []}
                title="LLM Providers"
                viewAllHref={buildConfigureTabHref(orgId, projectId, agentId, "llm")}
                previewLimit={PREVIEW_LIMIT}
                CardComponent={LLMProviderConfigCard}
            />
            <EnvConfigGroup
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
                configs={mcpData?.configs ?? []}
                title="MCP Proxies"
                viewAllHref={buildConfigureTabHref(orgId, projectId, agentId, "tools")}
                previewLimit={PREVIEW_LIMIT}
                CardComponent={MCPProxyConfigCard}
            />
        </>
    );
};

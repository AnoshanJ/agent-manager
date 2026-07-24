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

import { Box, Typography } from "@wso2/oxygen-ui";
import { CollapsibleSection } from "@agent-management-platform/shared-component";
import type { AgentModelConfigListItem } from "@agent-management-platform/types";
import { SectionHeader } from "./SectionHeader";
import { useEnvFilteredConfigs } from "./useEnvFilteredConfigs";

export interface ConfigCardProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    config: AgentModelConfigListItem;
    visible: boolean;
    onResolved: (configId: string, applicable: boolean) => void;
}

interface EnvConfigGroupProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    configs: AgentModelConfigListItem[];
    title: string;
    viewAllHref: string;
    previewLimit: number;
    CardComponent: React.ComponentType<ConfigCardProps>;
}

/**
 * One "LLM Providers" / "MCP Proxies" preview group: probes `configs` for
 * applicability to `envId` via useEnvFilteredConfigs, then renders up to
 * `previewLimit` of them as cards with a "View all" link.
 *
 * Stays collapsed (zero height) while probing and, once settled, only the
 * resolved-applicable configs stay mounted inside — cards that resolved as
 * inapplicable, or ranked past the preview limit, unmount instead of
 * lingering forever, so their per-config queries stop refetching in the
 * background. A group with nothing applicable to this environment simply
 * never expands.
 */
export const EnvConfigGroup: React.FC<EnvConfigGroupProps> = ({
    orgId, projectId, agentId, envId, configs, title, viewAllHref, previewLimit, CardComponent,
}) => {
    const {
        visible, reportResolved, isSettled, extraCount,
    } = useEnvFilteredConfigs(configs, previewLimit);

    if (configs.length === 0) {
        return null;
    }

    const activeConfigs = isSettled ? visible : configs;
    const show = isSettled && visible.length > 0;

    return (
        <CollapsibleSection show={show}>
            <SectionHeader title={title} viewAllHref={viewAllHref} />
            <Box display="flex" flexDirection="column" gap={1} sx={{ mb: extraCount > 0 ? 0.5 : 1.5 }}>
                {activeConfigs.map((config) => (
                    <CardComponent
                        key={config.uuid}
                        orgId={orgId}
                        projectId={projectId}
                        agentId={agentId}
                        envId={envId}
                        config={config}
                        visible={visible.some((c) => c.uuid === config.uuid)}
                        onResolved={reportResolved}
                    />
                ))}
            </Box>
            {extraCount > 0 && (
                <Typography variant="caption" color="text.disabled" sx={{ display: "block", mb: 1.5 }}>
                    +{extraCount} more
                </Typography>
            )}
        </CollapsibleSection>
    );
};

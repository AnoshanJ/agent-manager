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

import type { Configurations } from "@agent-management-platform/types";
import type { DeploymentStatus } from "@agent-management-platform/shared-component";
import { EnvAgentRolesGroupsSection } from "./EnvAgentRolesGroupsSection";
import { EnvCapabilitiesSection } from "./EnvCapabilitiesSection";
import { EnvConfigsSection } from "./EnvConfigsSection";
import { EnvMonitorsSection } from "./EnvMonitorsSection";
import { EnvObservabilitySection } from "./EnvObservabilitySection";
import { Divider } from "@wso2/oxygen-ui";

interface EnvironmentSectionsContentProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    configurations?: Configurations;
    external?: boolean;
    isolationTier?: string;
    deploymentStatus?: DeploymentStatus;
}

/**
 * Capabilities / Agent Identity / Agent Performance / Recent Traces sections
 * rendered as an EnvironmentCard's bottomContent, shared by
 * InternalAgentOverview and ExternalAgentOverview. EnvironmentCard renders
 * bottomContent unconditionally, and each section here decides for itself
 * whether it has anything to show.
 */
export function EnvironmentSectionsContent({
    orgId, projectId, agentId, envId, configurations, external, isolationTier, deploymentStatus,
}: EnvironmentSectionsContentProps) {
    return (
        <>
            <Divider sx={{ my: 1.5 }} />
            <EnvCapabilitiesSection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
                configurations={configurations}
                external={external}
                isolationTier={isolationTier}
                deploymentStatus={deploymentStatus}
            />
            <EnvAgentRolesGroupsSection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
            />
            <EnvConfigsSection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
            />
            {/* Monitors/Observability below still use a plain loading Skeleton
                rather than CollapsibleSection, deliberately — their skeletons
                are already sized close to the real content (metric tiles,
                per-card skeletons), so there's no mismatched-height jump to
                fix for them. */}
            <EnvMonitorsSection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
            />
            <EnvObservabilitySection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
                external={external}
            />
        </>
    );
}

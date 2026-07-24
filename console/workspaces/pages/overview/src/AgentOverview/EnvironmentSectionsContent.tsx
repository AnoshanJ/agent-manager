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
import { EnvAgentRolesGroupsSection } from "./EnvAgentRolesGroupsSection";
import { EnvCapabilitiesSection } from "./EnvCapabilitiesSection";
import { EnvConfigsSection } from "./EnvConfigsSection";
import { EnvMonitorsSection } from "./EnvMonitorsSection";
import { EnvObservabilitySection } from "./EnvObservabilitySection";

interface EnvironmentSectionsContentProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    configurations?: Configurations;
    external?: boolean;
}

/**
 * Capabilities / Agent Identity / Agent Performance / Recent Traces sections
 * rendered as an EnvironmentCard's bottomContent, shared by
 * InternalAgentOverview and ExternalAgentOverview.
 */
export function EnvironmentSectionsContent({
    orgId, projectId, agentId, envId, configurations, external,
}: EnvironmentSectionsContentProps) {
    return (
        <>
            <EnvCapabilitiesSection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
                configurations={configurations}
                external={external}
            />
            <EnvConfigsSection
                orgId={orgId}
                projectId={projectId}
                agentId={agentId}
                envId={envId}
            />
            <EnvAgentRolesGroupsSection
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

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

import { Box, Chip, Tooltip } from "@wso2/oxygen-ui";
import { Link as LinkOutlined, Tag } from "@wso2/oxygen-ui-icons-react";
import {
    DeploymentStatus,
    EnvStatus,
    formatRelativeTime,
    getAgentDeploymentPath,
    IsolationTierChip,
} from "@agent-management-platform/shared-component";
import { SectionHeader } from "./SectionHeader";

interface EnvDeploymentStatusSectionProps {
    orgId: string;
    projectId: string;
    agentId: string;
    external?: boolean;
    status?: DeploymentStatus;
    registeredAt?: string;
    isolationTier?: string;
    /** Deployed Agent Kind version (e.g. "v3"), relocated from EnvironmentCard's header. */
    deployedVersionLabel?: string | null;
}

/**
 * Deployment-status, deployed-version and sandbox-tier chips, relocated here
 * from EnvironmentCard's own header — a standalone section like Capabilities/
 * Configs/Roles, rendered unconditionally regardless of the deployment's
 * actual status (unlike `bottomContent`'s other sections, which self-gate
 * based on whether a deployment is live). "View Deployment" links out for
 * internal agents only — external agents aren't deployed through this
 * platform, so there's no deploy page to send them to.
 */
export const EnvDeploymentStatusSection: React.FC<EnvDeploymentStatusSectionProps> = ({
    orgId, projectId, agentId, external, status, registeredAt, isolationTier, deployedVersionLabel,
}) => {
    const deploymentPath = !external
        ? getAgentDeploymentPath(orgId, projectId, agentId)
        : undefined;

    return (
        <>
            <SectionHeader
                title="Deployment Status"
                viewAllHref={deploymentPath}
                viewAllLabel="View Deployment"
            />
            <Box display="flex" flexWrap="wrap" gap={1} sx={{ mb: 1.5 }}>
                {external ? (
                    <Tooltip title={formatRelativeTime(registeredAt)}>
                        <Chip
                            icon={<LinkOutlined size={16} />}
                            variant="outlined"
                            size="small"
                            label="Registered"
                            color="success"
                        />
                    </Tooltip>
                ) : (
                    <EnvStatus status={status} />
                )}
                {deployedVersionLabel && (
                    <Chip
                        icon={<Tag size={14} />}
                        label={deployedVersionLabel}
                        size="small"
                        variant="outlined"
                    />
                )}
                {!external && <IsolationTierChip tier={isolationTier} />}
            </Box>
        </>
    );
};

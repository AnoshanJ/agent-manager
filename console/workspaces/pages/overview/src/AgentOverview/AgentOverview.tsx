/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { useGetAgent } from "@agent-management-platform/api-client";
import { InternalAgentOverview } from "./InternalAgentOverview";
import { useParams } from "react-router-dom";
import { ExternalAgentOverview } from "./ExternalAgentOverview";
import { useState } from "react";
import { Box, Button, Chip, Skeleton, Stack } from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import { EditAgentDrawer } from "./EditAgentDrawer";
import {
    PageLayout,
    displayProvisionTypes,
    DescriptionCard,
    CreatedMetadata,
} from "@agent-management-platform/views";
import { LabelChips } from "@agent-management-platform/shared-component";

function AgentOverviewSkeleton() {
    return (
        <Box display="flex" flexDirection="column" gap={4} width="100%">
            <Skeleton variant="rounded" width="100%" height="40vh" />
        </Box>
    );
}

export function AgentOverview() {
    const { orgId, agentId, projectId } = useParams();
    const [editAgentDrawerOpen, setEditAgentDrawerOpen] = useState(false);
    const { data: agent, isLoading: isAgentLoading } = useGetAgent({
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
    });

    return (
        <>
            <PageLayout
                title={agent?.displayName ?? "Agent"}
                description={agent ? <CreatedMetadata createdAt={agent.createdAt} /> : undefined}
                isLoading={isAgentLoading}
                titleTail={
                    <Stack direction="row" spacing={0.5} alignItems="center" sx={{ width: "100%", minWidth: 0 }}>
                        <Chip
                            label={displayProvisionTypes(agent?.provisioning?.type)}
                            color="default"
                            size="small"
                            variant="outlined"
                            sx={{ flexShrink: 0 }}
                        />
                        <LabelChips labels={agent?.labels} />
                    </Stack>
                }
                actions={
                    <Button
                        variant="outlined"
                        size="small"
                        startIcon={<Edit size={16} />}
                        onClick={() => setEditAgentDrawerOpen(true)}
                        disabled={!agent}
                    >
                        Edit Agent
                    </Button>
                }
            >
                {isAgentLoading ? (
                    <AgentOverviewSkeleton />
                ) : (
                    <Box display="flex" flexDirection="column" gap={2}>
                        {agent?.description && (
                            <DescriptionCard title="Agent Description" content={agent.description} />
                        )}
                        {agent?.provisioning?.type === "internal" && <InternalAgentOverview />}
                        {agent?.provisioning?.type === "external" && <ExternalAgentOverview />}
                    </Box>
                )}
            </PageLayout>

            {agent && (
                <EditAgentDrawer
                    open={editAgentDrawerOpen}
                    onClose={() => setEditAgentDrawerOpen(false)}
                    agent={agent}
                    orgId={orgId || "default"}
                    projectId={projectId || "default"}
                />
            )}
        </>
    );
}

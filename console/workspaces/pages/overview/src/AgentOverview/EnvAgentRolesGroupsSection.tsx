/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useMemo } from "react";
import { Box } from "@wso2/oxygen-ui";
import {
  useAgentIdentityBinding,
  useListAgentIdentityAgents,
} from "@agent-management-platform/api-client";
import {
  CollapsibleSection,
  monospaceInputSx,
  OverviewSectionCard,
  RolesGroupsChips,
  useAgentRolesAndGroups,
} from "@agent-management-platform/shared-component";
import { TextInput } from "@agent-management-platform/views";
import { buildAgentIdHref } from "./agentIdLink";

interface EnvAgentRolesGroupsSectionProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

/**
 * Per-environment "Agent ID" card: the Thunder Agent ID plus the identity's
 * roles/groups, rendered inside an EnvironmentCard for both internal and
 * external agents — the client ID/secret/regenerate flow lives on the
 * agent-level "Agent ID" page instead, linked to via the "View all" button.
 * The Thunder Agent ID mirrors the one shown on that page, resolved from the
 * same useListAgentIdentityAgents response.
 */
export const EnvAgentRolesGroupsSection: React.FC<EnvAgentRolesGroupsSectionProps> = ({
  orgId, projectId, agentId, envId,
}) => {
  const { provisioned, isLoading: isLoadingIdentity } = useAgentIdentityBinding({
    orgId, projectId, agentId, envId,
  });

  const { roles, groups, isLoading } = useAgentRolesAndGroups({
    orgId, projectId, agentId, envId, enabled: provisioned,
  });

  // Same lookup the agent-id page uses: the Thunder Agent ID is a field on the
  // per-env identity-agents list, matched by agent + project name.
  const { data: identityAgentsData } = useListAgentIdentityAgents({
    orgName: orgId, envName: envId,
  });
  const thunderAgentId = useMemo(
    () => identityAgentsData?.agents.find(
      (item) => item.agentName === agentId && item.projectName === projectId,
    )?.thunderAgentId,
    [identityAgentsData, agentId, projectId],
  );

  const show = !isLoadingIdentity && provisioned;

  return (
    <CollapsibleSection show={show}>
      <OverviewSectionCard
        title="Agent ID"
        actionHref={buildAgentIdHref(orgId, projectId, agentId, envId)}
        sx={{ mb: 1.5 }}
      >
        {thunderAgentId && (
          <TextInput
            slotProps={{ input: { readOnly: true } }}
            label="Thunder Agent ID"
            value={thunderAgentId}
            copyable
            fullWidth
            size="small"
            sx={{ ...monospaceInputSx, mb: 1.5 }}
          />
        )}
        <Box>
          <RolesGroupsChips roles={roles} groups={groups} isLoading={isLoading} />
        </Box>
      </OverviewSectionCard>
    </CollapsibleSection>
  );
};

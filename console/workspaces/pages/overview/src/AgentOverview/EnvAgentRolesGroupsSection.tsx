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

import { Box } from "@wso2/oxygen-ui";
import { useAgentIdentityBinding } from "@agent-management-platform/api-client";
import { isAgentIdentityEnabled } from "@agent-management-platform/types";
import {
  CollapsibleSection,
  RolesGroupsChips,
  useAgentRolesAndGroups,
} from "@agent-management-platform/shared-component";
import { buildAgentIdHref } from "./agentIdLink";
import { SectionHeader } from "./SectionHeader";

interface EnvAgentRolesGroupsSectionProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

/**
 * Per-environment "Agent Identity" roles/groups display, rendered inside an
 * EnvironmentCard for both internal and external agents — the client
 * ID/secret/regenerate flow lives on the agent-level "Agent ID" page
 * instead, linked to via the "View all" button here (styled the same way as
 * the Agent Performance / Recent Traces section headers).
 */
export const EnvAgentRolesGroupsSection: React.FC<EnvAgentRolesGroupsSectionProps> = ({
  orgId, projectId, agentId, envId,
}) => {
  // useAgentIdentityBinding has no `enabled` option, so the ids are withheld
  // when Agent ID is disabled to keep the identity request from firing.
  const agentIdEnabled = isAgentIdentityEnabled();
  const { provisioned, isLoading: isLoadingIdentity } = useAgentIdentityBinding(
    agentIdEnabled
      ? { orgId, projectId, agentId, envId }
      : { orgId: "", projectId: "", agentId: "", envId: "" },
  );

  const { roles, groups, isLoading } = useAgentRolesAndGroups({
    orgId, projectId, agentId, envId, enabled: provisioned,
  });

  const show = agentIdEnabled && !isLoadingIdentity && provisioned;

  return (
    <CollapsibleSection show={show}>
      <SectionHeader
        title="Agent ID"
        viewAllHref={buildAgentIdHref(orgId, projectId, agentId, envId)}
      />

      <Box sx={{ mt: 1 }}>
        <RolesGroupsChips roles={roles} groups={groups} isLoading={isLoading} />
      </Box>
    </CollapsibleSection>
  );
};

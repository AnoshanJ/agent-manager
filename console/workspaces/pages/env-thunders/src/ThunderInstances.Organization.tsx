/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import { Navigate, Route, Routes, generatePath, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";
import { AgentIdentityEnvironmentProvider } from "./context/AgentIdentityEnvironmentContext";
import { ThunderInstancesTable } from "./subComponents/ThunderInstancesTable";
import { ViewThunderInstance } from "./subComponents/ViewThunderInstance";
import { AgentsOrganization } from "./AgentsOrganization";
import { RolesOrganization } from "./RolesOrganization";
import { GroupsOrganization } from "./GroupsOrganization";

export const ThunderInstancesOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();

  return (
    <AgentIdentityEnvironmentProvider>
      <Routes>
        <Route
          index
          element={
            <PageLayout
              title="Agent ID"
              description="Environment-scoped agent identities for agent authentication"
              disableIcon
            >
              <ThunderInstancesTable />
            </PageLayout>
          }
        />
        <Route path="view/:envName/*" element={<ViewThunderInstance />} />
        <Route path="agents/*" element={<AgentsOrganization />} />
        <Route path="roles/*" element={<RolesOrganization />} />
        <Route path="groups/*" element={<GroupsOrganization />} />
        <Route
          path="*"
          element={
            <Navigate
              to={generatePath(
                absoluteRouteMap.children.org.children.thunderInstances.path,
                { orgId },
              )}
            />
          }
        />
      </Routes>
    </AgentIdentityEnvironmentProvider>
  );
};

export default ThunderInstancesOrganization;

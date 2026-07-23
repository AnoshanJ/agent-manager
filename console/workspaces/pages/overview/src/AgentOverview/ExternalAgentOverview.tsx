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

import { globalConfig } from '@agent-management-platform/types';
import { Box, Button, Skeleton } from "@wso2/oxygen-ui";
import { Settings } from "@wso2/oxygen-ui-icons-react";
import { useParams, useSearchParams } from "react-router-dom";
import {
  useGetAgent,
  useListGateways,
} from "@agent-management-platform/api-client";
import {
  EnvironmentCard,
  usePipelineEnvironmentsState,
} from "@agent-management-platform/shared-component";
import { InstrumentationDrawer } from "./InstrumentationDrawer";
import { NoDataFound } from "@agent-management-platform/views";
import { EnvMonitorsSection } from "./EnvMonitorsSection";
import { EnvObservabilitySection } from "./EnvObservabilitySection";
import { EnvAgentRolesGroupsSection } from "./EnvAgentRolesGroupsSection";
import { EnvironmentTabsBar } from "./EnvironmentTabsBar";
import { useSelectedEnvironmentParam } from "./useSelectedEnvironmentParam";

export const ExternalAgentOverview = () => {
  const { agentId, orgId, projectId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  // Show only the environments in the current project's deployment pipeline,
  // ordered by the promotion chain. isLoading covers environments + project + pipelines.
  const { environments: sortedEnvironmentList, isLoading: isEnvironmentsLoading } =
    usePipelineEnvironmentsState(orgId, projectId);
  const { selectedEnvironment, selectEnvironment } =
    useSelectedEnvironmentParam(sortedEnvironmentList);
  const selectedEnvironmentId = selectedEnvironment?.id ?? "";

  // Per-env OTEL endpoint. The gateway mapped to the selected environment carries
  // the externally-reachable vhost; the OTEL RestApi is published at `<vhost>/otel`.
  // Falls back to globalConfig only when the gateway lookup hasn't resolved yet
  // (e.g. before an env is selected).
  const { data: envGatewayList } = useListGateways(
    { orgName: orgId ?? "" },
    { environment: selectedEnvironmentId },
  );
  const envGatewayVhost = envGatewayList?.gateways?.[0]?.vhost;
  const agentInstrumentationUrl = envGatewayVhost
    ? `${envGatewayVhost.replace(/\/$/, "")}/otel`
    : (globalConfig.instrumentationUrl || "http://default-default.gateway.localhost:19080/otel");

  const handleSetupAgent = (environmentName: string) => {
    selectEnvironment(environmentName);
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set("setup", "true");
        return next;
      },
      { replace: true },
    );
  };

  const closeSetupDrawer = () => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.delete("setup");
        return next;
      },
      { replace: true },
    );
  };

  return (
    <>
      <Box display="flex" flexDirection="column" gap={2}>
        {isEnvironmentsLoading ? (
          <Box display="flex" flexDirection="column" gap={2}>
            <Skeleton variant="rounded" height={100} />
            <Skeleton variant="rounded" height={100} />
          </Box>
        ) : sortedEnvironmentList.length === 0 ? (
          <NoDataFound
            message="No environments found"
            subtitle="Environments will appear here once they are created"
          />
        ) : (
          selectedEnvironment &&
          orgId &&
          projectId &&
          agentId && (
            <EnvironmentCard
              key={selectedEnvironment.name}
              orgId={orgId}
              projectId={projectId}
              agentId={agentId}
              environment={selectedEnvironment}
              showIsolationTier={sortedEnvironmentList.length > 1}
              tabsHeader={
                <EnvironmentTabsBar
                  environments={sortedEnvironmentList}
                  selectedName={selectedEnvironment.name}
                  onSelect={selectEnvironment}
                  dotColor={() => "success.main"}
                />
              }
              actions={
                <Button
                  variant="text"
                  size="small"
                  startIcon={<Settings size={16} />}
                  onClick={() => handleSetupAgent(selectedEnvironment.name)}
                >
                  Setup Agent
                </Button>
              }
              bottomContent={
                <>
                  <EnvAgentRolesGroupsSection
                    orgId={orgId}
                    projectId={projectId}
                    agentId={agentId}
                    envId={selectedEnvironment.name}
                  />
                  <EnvMonitorsSection
                    orgId={orgId}
                    projectId={projectId}
                    agentId={agentId}
                    envId={selectedEnvironment.name}
                  />
                  <EnvObservabilitySection
                    orgId={orgId}
                    projectId={projectId}
                    agentId={agentId}
                    envId={selectedEnvironment.name}
                    hideMetrics
                    external
                  />
                </>
              }
            />
          )
        )}
      </Box>
      <InstrumentationDrawer
        open={searchParams.get("setup") === "true" && selectedEnvironmentId !== ""}
        onClose={closeSetupDrawer}
        agentId={agentId ?? ""}
        orgName={orgId ?? "default"}
        projName={projectId ?? "default"}
        agentName={agentId ?? ""}
        environment={selectedEnvironment?.name}
        instrumentationUrl={agentInstrumentationUrl}
        componentUid={agent?.uuid}
        environmentUid={selectedEnvironmentId}
        autoGenerate
      />
    </>
  );
};

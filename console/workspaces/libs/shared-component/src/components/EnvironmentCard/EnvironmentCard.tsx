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

import { useMemo } from "react";
import {
  useGetAgent,
  useGetAgentBuilds,
  useListAgentDeployments,
  useListAgentKindVersions,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  Environment,
} from "@agent-management-platform/types";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  Skeleton,
  Tooltip,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import {
  CheckCircle as CheckCircleRounded,
  Circle as CircleOutlined,
  Rocket as RocketLaunchOutlined,
  Link as LinkOutlined,
  PauseCircle,
  Play,
  Tag,
  XCircle,
} from "@wso2/oxygen-ui-icons-react";
import { NoDataFound } from "@agent-management-platform/views";
import { generatePath, Link } from "react-router-dom";
import { formatRelativeTime } from "../../utils/format";
import { IsolationTierChip } from "../IsolationTierIndicator";

export enum DeploymentStatus {
  ACTIVE = "active",
  INACTIVE = "not-deployed",
  DEPLOYING = "in-progress",
  ERROR = "error",
  SUSPENDED = "suspended",
  FAILED = "failed",
}

export interface EnvironmentCardProps {
  environment?: Environment;
  orgId: string;
  projectId: string;
  agentId: string;
  actions?: React.ReactNode;
  /**
   * Rendered below the deployment status area. This card no longer lists
   * `currentDeployment.endpoints` itself (see EnvironmentCard.tsx history) —
   * a caller that wants endpoint/invoke-URL visibility must render it here,
   * as pages/overview's EnvCapabilitiesSection does.
   */
  bottomContent?: React.ReactNode;
  /**
   * Whether this is the first (root) environment of the deployment pipeline.
   * The root env is reached by deploying a build directly; downstream envs are
   * reached by promoting from the previous environment. Defaults to true so
   * callers without pipeline context keep the deploy-oriented wording.
   */
  isFirstEnvironment?: boolean;
  /** Replaces the environment name heading, e.g. a tab strip switching between sibling envs. */
  tabsHeader?: React.ReactNode;
  /** Shows the sandbox/isolation tier chip next to the status, alongside tabsHeader. */
  showIsolationTier?: boolean;
}

export const EnvStatus = ({
  status,
  suffix,
}: {
  status?: DeploymentStatus;
  suffix?: string;
}) => {
  const theme = useTheme();
  if (!status) {
    return null;
  }
  if (status === DeploymentStatus.ACTIVE) {
    return (
      <Chip
        icon={
          <CheckCircleRounded size={16} color={theme.vars?.palette?.success?.main} />
        }
        variant="outlined"
        size="small"
        label={suffix ? `Deployed · ${suffix}` : "Deployed"}
        color="success"
      />
    );
  }
  if (status === DeploymentStatus.INACTIVE) {
    return (
      <Chip
        icon={<CircleOutlined size={16} color={theme.vars?.palette?.text?.disabled} />}
        variant="outlined"
        size="small"
        label="Not Deployed"
        color="default"
      />
    );
  }
  if (status === DeploymentStatus.DEPLOYING) {
    return (
      <Chip
        icon={<CircularProgress size={16} color="warning" />}
        variant="outlined"
        size="small"
        label="Deploying"
        color="warning"
      />
    );
  }
  if (status === DeploymentStatus.ERROR) {
    return <Chip variant="outlined" size="small" label="Error" color="error" />;
  }
  if (status === DeploymentStatus.FAILED) {
    return <Chip variant="outlined" size="small" label="Error" color="error" />;
  }
  if (status === DeploymentStatus.SUSPENDED) {
    return (
      <Chip
        icon={<PauseCircle size={16} />}
        variant="outlined"
        size="small"
        label="Suspended"
        color="default"
      />
    );
  }
};

interface DeploymentStatusLinkProps {
  icon: React.ReactNode;
  label: string;
  color?: string;
  to: string;
  tooltip?: string;
}

/**
 * Icon + label, styled and linked like AgentInfoCard's "Last Build" status
 * (plain text over a Chip, hover:action.hover, links out) — used for the
 * deployment statuses here so clicking any of them jumps to the Deploy page.
 */
const DeploymentStatusLink = ({ icon, label, color, to, tooltip }: DeploymentStatusLinkProps) => {
  const content = (
    <Box display="flex" alignItems="center" gap={0.5} sx={{ color }}>
      {icon}
      <Typography variant="body2" fontWeight={600} color="inherit">
        {label}
      </Typography>
    </Box>
  );
  return (
    <Box
      component={Link}
      to={to}
      sx={{
        display: "flex",
        alignItems: "center",
        textDecoration: "none",
        color: "inherit",
        px: 1,
        py: 0.5,
        borderRadius: 1,
        "&:hover": { bgcolor: "action.hover" },
      }}
    >
      {tooltip ? <Tooltip title={tooltip}>{content}</Tooltip> : content}
    </Box>
  );
};

export const EnvironmentCard = (props: EnvironmentCardProps) => {
  const {
    environment,
    orgId,
    projectId,
    agentId,
    actions,
    bottomContent,
    isFirstEnvironment = true,
    tabsHeader,
    showIsolationTier,
  } = props;
  const theme = useTheme();
  const { data: agent, isLoading: isAgentLoading } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const isExternal = agent?.provisioning?.type === "external";

  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments(
      { orgName: orgId, projName: projectId, agentName: agentId },
      { enabled: !!orgId && !!projectId && !!agentId && !!agent && !isExternal }
    );

  const kindName = agent?.kindName;
  const { data: kindVersions } = useListAgentKindVersions({
    orgName: orgId,
    kindName: kindName ?? "",
  });

  const currentDeployment = deployments?.[environment?.name ?? ""];
  const envTitle = `${environment?.displayName ?? environment?.name ?? "Environment"} Environment`;

  const { data: buildsData } = useGetAgentBuilds({
    orgName: !isExternal ? orgId : "",
    projName: !isExternal ? projectId : "",
    agentName: !isExternal ? agentId : "",
  });

  const hasSuccessfulBuild = buildsData?.builds?.some(
    (b) => b.status === "Succeeded" || b.status === "Completed"
  ) ?? false;

  const deployedVersion = useMemo(() => {
    if (!currentDeployment?.imageId || !kindName) return null;
    const matched = kindVersions?.find((v) => v.imageId === currentDeployment.imageId);
    return matched?.version ?? null;
  }, [currentDeployment?.imageId, kindName, kindVersions]);

  const deployedVersionLabel = deployedVersion ? `v${deployedVersion}` : null;

  const latestKindVersion = useMemo(() => (
    kindVersions?.length
      ? [...kindVersions].sort(
        (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
      )[0]
      : undefined
  ), [kindVersions]);

  const isKindOutdated =
    !!kindName &&
    !!latestKindVersion &&
    !!deployedVersion &&
    deployedVersion !== latestKindVersion.version;

  if (isAgentLoading || isDeploymentsLoading) {
    return <Skeleton variant="rounded" height={100} />;
  }

  // ── External agent ────────────────────────────────────────────────────────
  if (isExternal) {
    return (
      <Card variant="outlined">
        <CardContent>
          <Box display="flex" flexDirection="row" gap={1} justifyContent="space-between" alignItems="center">
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
              {tabsHeader ?? <Typography variant="h6">{envTitle}</Typography>}
            </Box>
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
              {showIsolationTier && <IsolationTierChip tier={environment?.isolationTier} />}
              <Tooltip title={formatRelativeTime(agent?.createdAt)}>
                <Chip
                  icon={
                    <LinkOutlined size={16} color={theme.vars?.palette?.success?.main} />
                  }
                  variant="outlined"
                  size="small"
                  label="Registered"
                  color="success"
                />
              </Tooltip>
              {actions}
            </Box>
          </Box>
          {bottomContent}
        </CardContent>
      </Card>
    );
  }

  // ── Internal agent — not yet deployed ─────────────────────────────────────
  if (!currentDeployment) {
    return (
      <Card variant="outlined" sx={{ "&.MuiCard-root": { backgroundColor: "background.paper" } }}>
        <CardContent>
          <Box display="flex" flexDirection="row" gap={1} justifyContent="space-between" alignItems="center">
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
              {tabsHeader ?? <Typography variant="h6">{envTitle}</Typography>}
            </Box>
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
              {showIsolationTier && <IsolationTierChip tier={environment?.isolationTier} />}
              <EnvStatus status={DeploymentStatus.INACTIVE} />
            </Box>
          </Box>
        </CardContent>
      </Card>
    );
  }

  // ── Internal agent — deployment exists ────────────────────────────────────
  const deploymentStatus = currentDeployment.status as DeploymentStatus;
  const deploymentPath = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents
      .children.deployment.path,
    { orgId, projectId, agentId }
  );
  const deployedTimeTooltip = formatRelativeTime(currentDeployment?.lastDeployed);
  // Metrics/traces and monitor sections only carry meaningful data while the
  // deployment is serving traffic (active) or has failed while running (error).
  // For idle/transitional states (deploying, suspended) there is nothing live
  // to show, so we hide them and surface an empty state instead.
  const showObservability =
    deploymentStatus === DeploymentStatus.ACTIVE ||
    deploymentStatus === DeploymentStatus.ERROR ||
    deploymentStatus === DeploymentStatus.FAILED;
  return (
    <Card variant="outlined">
      <CardContent>
        <Box
          display="flex"
          flexDirection="row"
          gap={1}
          justifyContent="space-between"
          alignItems="center"
        >
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            {tabsHeader ?? (
              <Typography variant="h6">
                {environment?.displayName} Environment
              </Typography>
            )}
          </Box>
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            {showIsolationTier && <IsolationTierChip tier={environment?.isolationTier} />}
            {currentDeployment?.status === DeploymentStatus.ACTIVE && (
              <DeploymentStatusLink
                icon={<CheckCircleRounded size={16} />}
                label="Deployed"
                color={theme.vars?.palette?.success?.main}
                to={deploymentPath}
                tooltip={deployedTimeTooltip}
              />
            )}
            {(currentDeployment?.status === DeploymentStatus.ERROR ||
              currentDeployment?.status === DeploymentStatus.FAILED) && (
                <DeploymentStatusLink
                  icon={<XCircle size={16} />}
                  label="Error"
                  color={theme.vars?.palette?.error?.main}
                  to={deploymentPath}
                  tooltip={deployedTimeTooltip}
                />
              )}
            {currentDeployment?.status === DeploymentStatus.SUSPENDED && (
              <DeploymentStatusLink
                icon={<PauseCircle size={16} />}
                label="Suspended"
                color={theme.vars?.palette?.text?.secondary}
                to={deploymentPath}
                tooltip={deployedTimeTooltip}
              />
            )}
            {deployedVersionLabel && (
              <Chip
                icon={<Tag size={14} />}
                label={deployedVersionLabel}
                size="small"
                variant="outlined"
              />
            )}
            {currentDeployment?.status === DeploymentStatus.ACTIVE && actions}
          </Box>
        </Box>
        <Divider />
        <Box
          display="flex"
          width="100%"
          justifyContent="center"
          flexDirection="column"
          gap={1}
          pt={2}
          alignItems="center"
          sx={{
            // The Divider above already closes off the header/tabs row. Every
            // section in bottomContent is built on pages/overview's
            // SectionHeader, which unconditionally draws its own leading
            // <Divider> (an intentional boundary marker when a section isn't
            // first — see SectionHeader.tsx). Whichever section ends up
            // rendering first — usually Capabilities, but any section can be
            // first if earlier ones render null — would otherwise double up
            // with the Divider above. Hiding the first `hr` descendant here,
            // rather than each section knowing whether it's first, sidesteps
            // needing to lift each section's null/non-null render decision
            // back up to this component.
            "& > hr:first-of-type": { display: "none" },
          }}
        >
          {currentDeployment.status === DeploymentStatus.INACTIVE && (
            <NoDataFound
              disableBackground
              message="Not Deployed"
              icon={<RocketLaunchOutlined size={32} />}
              subtitle={
                hasSuccessfulBuild
                  ? isFirstEnvironment
                    ? "A successful build is available. Deploy it to get started."
                    : "Promote a deployment from the previous environment to get started."
                  : "No successful build found. Build the agent before deploying."
              }
              action={
                hasSuccessfulBuild && (
                  <Button
                    startIcon={<RocketLaunchOutlined size={16} />}
                    variant="outlined"
                    component={Link}
                    to={generatePath(
                      absoluteRouteMap.children.org.children.projects.children
                        .agents.children.deployment.path,
                      { orgId, projectId, agentId }
                    )}
                    size="small"
                  >
                    {isFirstEnvironment ? "Go to Deployment" : "Promote"}
                  </Button>
                )
              }
            />
          )}
          {currentDeployment.status === DeploymentStatus.DEPLOYING && (
            <NoDataFound disableBackground message="Deploying..." icon={<CircularProgress size={32} />} />
          )}
          {(currentDeployment.status === DeploymentStatus.ERROR ||
            currentDeployment.status === DeploymentStatus.FAILED) && (
              <Alert
                severity="error"
                sx={{ width: "100%" }}
                action={
                  <Button
                    component={Link}
                    to={generatePath(
                      absoluteRouteMap.children.org.children.projects.children
                        .agents.children.deployment.path,
                      { orgId, projectId, agentId }
                    )}
                    color="inherit"
                    size="small"
                  >
                    View Deployment
                  </Button>
                }
              >
                Deployment failed. Check the deployment page for more details.
              </Alert>
            )}
          {currentDeployment.status === DeploymentStatus.SUSPENDED && (
            <NoDataFound
              disableBackground
              message="Suspended"
              icon={<PauseCircle size={32} />}
              subtitle="This deployment is currently suspended. Resume it from the deployment page to make the agent available again."
              action={
                <Button
                  startIcon={<Play size={16} />}
                  variant="outlined"
                  component={Link}
                  to={generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.deployment.path,
                    { orgId, projectId, agentId }
                  )}
                  size="small"
                >
                  Go to Deployment
                </Button>
              }
            />
          )}
          {currentDeployment.status === DeploymentStatus.ACTIVE && isKindOutdated && (
            <Alert severity="warning" sx={{ width: "100%" }}>
              A newer version of this Agent Kind is available: <strong>v{latestKindVersion!.version}</strong>.{" "}
              Currently deployed: <strong>v{deployedVersion}</strong>.
            </Alert>
          )}
        </Box>
        {showObservability && bottomContent}
      </CardContent>
    </Card>
  );
};

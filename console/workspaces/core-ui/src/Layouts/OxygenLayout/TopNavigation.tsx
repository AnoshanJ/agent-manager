import {
  useListAgents,
  useListOrganizations,
  useListProjects,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  ButtonBase,
  Chip,
  ComplexSelect,
  Header,
  MenuItem,
  Stack,
  Tooltip,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { Building2, Plus } from "@wso2/oxygen-ui-icons-react";
import { useMemo, useState } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { asLink, LevelSwitcherCard } from "./LevelSwitcherCard";
import { useActiveAgentPage, useActiveOrgPage, useActiveProjectPage } from "./path-map";

const MAX_DISPLAY_NAME_LENGTH = 25;

const truncateName = (name: string) =>
  name.length > MAX_DISPLAY_NAME_LENGTH
    ? `${name.slice(0, MAX_DISPLAY_NAME_LENGTH)}…`
    : name;

export function TopNavigation() {
  const navigate = useNavigate();
  const theme = useTheme();
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const commonOrgPages = useActiveOrgPage();
  const commonProjectPages = useActiveProjectPage();
  const commonAgentPages = useActiveAgentPage();
  const [projectAnchorEl, setProjectAnchorEl] = useState<null | HTMLElement>(
    null,
  );
  const projectMenuOpen = Boolean(projectAnchorEl);

  const [agentAnchorEl, setAgentAnchorEl] = useState<null | HTMLElement>(null);
  const agentMenuOpen = Boolean(agentAnchorEl);

  // Get all organizations
  const { data: organizations } = useListOrganizations();
  const selectedOrganization = useMemo(() => {
    return organizations?.organizations?.find(
      (organization) => organization.name === orgId,
    );
  }, [organizations, orgId]);

  // Get all projects for the organization
  const { data: projects } = useListProjects({
    orgName: orgId,
  });

  const selectedProject = useMemo(() => {
    return projects?.projects?.find((project) => project.name === projectId);
  }, [projects, projectId]);

  // Get all agents for the project
  const { data: agents } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });

  const selectedAgent = useMemo(() => {
    return agents?.agents?.find((agent) => agent.name === agentId);
  }, [agents, agentId]);

  return (
    <>
      <Header.Switchers showDivider={false}>
        {organizations?.organizations && (
          <>
            {selectedOrganization && organizations.total > 1 && (
              <ComplexSelect
                value={orgId}
                size="small"
                sx={{ minWidth: 180 }}
                label="Organizations"
                renderValue={() => (
                  <ComplexSelect.MenuItem.Text
                    primary={selectedOrganization?.displayName}
                  />
                )}
              >
                {organizations.organizations.map((organization) => (
                  <ComplexSelect.MenuItem
                    key={organization.name}
                    value={organization.name}
                    {...asLink(
                      generatePath(absoluteRouteMap.children.org.path, {
                        orgId: organization.name,
                      }) + (commonOrgPages ? `/${commonOrgPages}` : ""),
                    )}
                  >
                    <ComplexSelect.MenuItem.Text
                      primary={organization.displayName ?? organization.name}
                    />
                  </ComplexSelect.MenuItem>
                ))}
              </ComplexSelect>
            )}
            {selectedOrganization && organizations.total == 1 && (
              <>
                <Tooltip title="Go to organization">
                  <ButtonBase
                   aria-label="Go to organization"
                   {...asLink(
                        generatePath(absoluteRouteMap.children.org.path, {
                          orgId: selectedOrganization.name,
                        }) + (commonOrgPages ? `/${commonOrgPages}` : ""),
                      )}

                  sx={{
                    color: theme.vars?.palette.text.primary,
                    border: `1px solid ${theme.vars?.palette.divider}`,
                    p: theme.spacing(1.75, 1.75),
                    borderRadius: theme.spacing(1),
                    "&:hover": {
                      border: `1px solid ${theme.vars?.palette.text.primary}`,
                    },
                  }}>
                    <Building2 size={22} />
                  </ButtonBase>
                </Tooltip>
              </>
            )}

          </>
        )}

        {projects?.projects && (
          <LevelSwitcherCard
            label="Projects"
            chevronTooltip={selectedProject ? "Switch project" : "Select or create a project"}
            anchorEl={projectAnchorEl}
            menuOpen={projectMenuOpen}
            onOpenMenu={(e) => setProjectAnchorEl(e.currentTarget)}
            onCloseMenu={() => setProjectAnchorEl(null)}
            selected={
              selectedProject && {
                to:
                  generatePath(
                    absoluteRouteMap.children.org.children.projects.path,
                    { orgId, projectId },
                  ) + (commonProjectPages ? `/${commonProjectPages}` : ""),
                goToTooltip: `Go to ${selectedProject.displayName}`,
                closeTooltip: "Close project",
                onClose: () =>
                  navigate(
                    generatePath(absoluteRouteMap.children.org.path, { orgId }),
                  ),
                content: (
                  <Typography variant="body1" noWrap sx={{ maxWidth: "100%" }}>
                    {truncateName(selectedProject.displayName)}
                  </Typography>
                ),
              }
            }
          >
            <MenuItem
              onClick={() => setProjectAnchorEl(null)}
              {...asLink(
                generatePath(
                  absoluteRouteMap.children.org.children.newProject.path,
                  { orgId },
                ),
              )}
            >
              <Plus size={20} style={{ marginRight: theme.spacing(1) }} />
              Create a Project
            </MenuItem>
            {projects.projects.map((project) => (
              <MenuItem
                key={project.name}
                selected={project.name === projectId}
                onClick={() => setProjectAnchorEl(null)}
                {...asLink(
                  generatePath(
                    absoluteRouteMap.children.org.children.projects.path,
                    { orgId, projectId: project.name },
                  ) + (commonProjectPages ? `/${commonProjectPages}` : ""),
                )}
              >
                {project.displayName}
              </MenuItem>
            ))}
          </LevelSwitcherCard>
        )}

        {agents?.agents && (
          <LevelSwitcherCard
            label="Agents"
            chevronTooltip={selectedAgent ? "Switch agent" : "Select or create an agent"}
            anchorEl={agentAnchorEl}
            menuOpen={agentMenuOpen}
            onOpenMenu={(e) => setAgentAnchorEl(e.currentTarget)}
            onCloseMenu={() => setAgentAnchorEl(null)}
            selected={
              selectedAgent && {
                to:
                  generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.path,
                    { orgId, projectId, agentId },
                  ) + (commonAgentPages ? `/${commonAgentPages}` : ""),
                goToTooltip: `Go to ${selectedAgent.displayName}`,
                closeTooltip: "Close agent",
                onClose: () =>
                  navigate(
                    generatePath(
                      absoluteRouteMap.children.org.children.projects.path,
                      { orgId, projectId },
                    ),
                  ),
                content: (
                  <Stack
                    direction="row"
                    gap={1}
                    alignItems="center"
                    sx={{ maxWidth: "100%", minWidth: 0 }}
                  >
                    <Typography variant="body1" noWrap sx={{ minWidth: 0 }}>
                      {truncateName(selectedAgent.displayName)}
                    </Typography>
                    {selectedAgent.provisioning.type === "external" && (
                      <Chip label={"External"} size="small" variant="outlined" />
                    )}
                  </Stack>
                ),
              }
            }
          >
            <MenuItem
              onClick={() => setAgentAnchorEl(null)}
              {...asLink(
                generatePath(
                  absoluteRouteMap.children.org.children.projects.children
                    .newAgent.path,
                  { orgId, projectId },
                ),
              )}
            >
              <Plus size={20} style={{ marginRight: theme.spacing(1) }} />
              Create an Agent
            </MenuItem>
            {agents.agents.map((agent) => (
              <MenuItem
                key={agent.name}
                selected={agent.name === agentId}
                onClick={() => setAgentAnchorEl(null)}
                {...asLink(
                  generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.path,
                    { orgId, projectId, agentId: agent.name },
                  ) + (commonAgentPages ? `/${commonAgentPages}` : ""),
                )}
              >
                <Stack direction="row" gap={1} alignItems="center">
                  {agent.displayName}
                  {agent.provisioning.type === "external" && (
                    <Chip label={"External"} size="small" variant="outlined" />
                  )}
                </Stack>
              </MenuItem>
            ))}
          </LevelSwitcherCard>
        )}
      </Header.Switchers>
    </>
  );
}

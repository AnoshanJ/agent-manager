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

import { useMemo, useState } from "react";
import { Box, Button, Chip, Grid, Tooltip, Typography, type ChipProps } from "@wso2/oxygen-ui";
import { Plug } from "@wso2/oxygen-ui-icons-react";
import { generatePath } from "react-router-dom";
import { useGetAgentEndpoints } from "@agent-management-platform/api-client";
import {
    CollapsibleSection,
    DeploymentStatus,
    extractOpenApiResources,
    getAgentDeploymentPath,
    IsolationTierChip,
    OverviewSectionCard,
    parseOpenApiSpecContent,
    type OpenApiResource,
} from "@agent-management-platform/shared-component";
import { absoluteRouteMap, type Configurations } from "@agent-management-platform/types";
import { TextInput } from "@agent-management-platform/views";
import { ConsumerConfigDrawer, type AuthMode } from "./ConsumerConfigDrawer";

interface EnvCapabilitiesSectionProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    configurations?: Configurations;
    external?: boolean;
    isolationTier?: string;
    deploymentStatus?: DeploymentStatus;
}

const METHOD_LABEL: Record<string, string> = {
    DELETE: "DEL",
};

const METHOD_COLOR: Record<string, ChipProps["color"]> = {
    GET: "success",
    POST: "warning",
    PUT: "info",
    PATCH: "info",
    DELETE: "error",
};

// Stable reference so an absent `oauthConfig.issuers` doesn't defeat
// ConsumerConfigDrawer's memoization with a new empty array every render.
const EMPTY_ISSUERS: string[] = [];

/**
 * Everything derived from `authMode` in one place — the Deploy page
 * (DeployCard.tsx) mirrors this same oauth/apikey/none branching for its own
 * security summary, so keeping every consumer of `authMode` here keyed off
 * one lookup (rather than four separate ternary chains) is what keeps the
 * wording in sync as it changes.
 */
function getAuthPresentation(
    authMode: AuthMode, authHeaderPrefix: string, oauthHeaderName: string,
): { label: string; tooltip: string; headerExample: string } {
    switch (authMode) {
        case "oauth":
            return {
                label: `OAuth2 (${authHeaderPrefix})`,
                tooltip: `Callers send an Authorization: ${authHeaderPrefix} <token> header validated `
                    + "by the gateway",
                headerExample: `${oauthHeaderName}: ${authHeaderPrefix} <token>`,
            };
        case "apikey":
            return {
                label: "API Key",
                tooltip: "Requests must include the header: x-api-key: <your-key>",
                headerExample: "x-api-key: <your-api-key>",
            };
        case "none":
            return {
                label: "None",
                tooltip: "Endpoint is publicly accessible without authentication",
                headerExample: "No authentication header required",
            };
    }
}

interface StatusPillProps {
    label: string;
    value: string;
    tooltip: string;
}

/** Chip badge shared by the Auth and CORS summaries below Invoke URL. */
const StatusPill: React.FC<StatusPillProps> = ({ label, value, tooltip }) => (
    <Tooltip title={tooltip}>
        <Chip
            variant="outlined"
            size="small"
            label={`${label}: ${value}`}
        />
    </Tooltip>
);

/**
 * Per-environment "Capabilities" list — the HTTP endpoints an agent exposes,
 * its invoke URL, and a read-only CORS/Authentication summary (no configure
 * action here; that lives on the Deploy page). Endpoints are parsed from each
 * endpoint's published OpenAPI schema. Links out to the full API Spec viewer
 * on the Try It page. Not applicable to external agents (they aren't deployed
 * through this platform, so there's nothing to fetch), so `external` withholds
 * `orgName` to keep useGetAgentEndpoints disabled instead of firing a request
 * that would just be discarded.
 */
export const EnvCapabilitiesSection: React.FC<EnvCapabilitiesSectionProps> = ({
    orgId, projectId, agentId, envId, configurations, external, isolationTier, deploymentStatus,
}) => {
    const [consumerConfigOpen, setConsumerConfigOpen] = useState(false);

    const { data: endpoints, isLoading } = useGetAgentEndpoints(
        { orgName: external ? "" : orgId, projName: projectId, agentName: agentId },
        { environment: envId },
    );

    // Single pass over the endpoint map: flattens every endpoint's OpenAPI
    // schema into a deduped method+path list, and separately picks the
    // externally-reachable endpoint as "the" invoke URL, falling back to
    // whichever entry is present when none is marked external.
    const { resources, invokeUrl } = useMemo(() => {
        const endpointList = Object.values(endpoints ?? {});
        const byKey = new Map<string, OpenApiResource>();
        endpointList.forEach((endpoint) => {
            const spec = parseOpenApiSpecContent(endpoint.schema?.content);
            extractOpenApiResources(spec).forEach((resource) => {
                byKey.set(`${resource.method} ${resource.path}`, resource);
            });
        });
        const externalEndpoint = endpointList.find(
            (endpoint) => endpoint.visibility?.toLowerCase() === "external",
        );
        return {
            resources: Array.from(byKey.values()),
            invokeUrl: (externalEndpoint ?? endpointList[0])?.url,
        };
    }, [endpoints]);

    // Mirrors DeployCard.tsx's authMode derivation so the wording matches the
    // Deploy page's own security summary.
    const authMode: AuthMode = configurations?.enableOAuthSecurity
        ? "oauth"
        : configurations?.enableApiKeySecurity
            ? "apikey"
            : "none";
    const authHeaderPrefix = configurations?.oauthConfig?.authHeaderPrefix || "Bearer";
    const oauthHeaderName = configurations?.oauthConfig?.headerName || "Authorization";
    const { label: authLabel, tooltip: authTooltip, headerExample: authHeaderExample } =
        getAuthPresentation(authMode, authHeaderPrefix, oauthHeaderName);
    const oauthIssuers = configurations?.oauthConfig?.issuers ?? EMPTY_ISSUERS;

    const corsEnabled = configurations?.corsConfig?.enabled ?? false;
    const corsOrigins = configurations?.corsConfig?.allowOrigin ?? [];
    const corsAllOrigins = corsOrigins.includes("*");
    const corsLabel = corsEnabled
        ? `Enabled · ${corsAllOrigins ? "all origins" : "allow-listed origins"}`
        : "Disabled";
    const corsTooltip = corsEnabled
        ? corsAllOrigins ? "Any origin may call this endpoint" : corsOrigins.join(", ")
        : "Cross-origin browser requests are blocked";

    // Not applicable to external agents at all: they aren't deployed through
    // this platform, so there's nothing to fetch, and the disabled query never
    // settles isLoading. Static per instance, so bailing out here (rather than
    // routing through CollapsibleSection like the loading/empty cases below)
    // skips building the JSX for a section that can never show.
    if (external) {
        return null;
    }

    // Points at the general tryOut route (not the api/chat sub-path) since
    // TestComponent picks Swagger vs. AgentChat based on agent type regardless
    // of which sub-path is requested — this is now the card's one "Try It"
    // entry point, replacing EnvironmentCard's own removed header button.
    const tryItHref = generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.tryOut.path,
        { orgId, projectId, agentId, envId },
    );
    const deploymentPath = getAgentDeploymentPath(orgId, projectId, agentId);

    // Resources need a parsed OpenAPI schema; invokeUrl only needs the
    // endpoint itself to have resolved a URL. A kind-type agent can have one
    // without the other (e.g. schema not registered yet) — show whichever is
    // actually available instead of hiding invokeUrl behind resources.
    // Also gated on the environment actually being deployed — an inactive
    // environment can still have a schema/URL left over from a prior
    // deployment, and surfacing those as if they were live would be misleading.
    const show = deploymentStatus === DeploymentStatus.ACTIVE
        && !isLoading && (resources.length > 0 || !!invokeUrl);

    return (
        <CollapsibleSection show={show}>
            {/*
             * 12-column grid: Invoke URL (8) beside Capabilities (4) on md+,
             * each wrapping to a full-width row on small screens where a
             * side-by-side split would squeeze the TextInput unreadably.
             */}
            <Grid container spacing={2} sx={{ mb: 1.5 }}>
                {invokeUrl && (
                    <Grid size={{ xs: 12, md: 8 }}>
                        <OverviewSectionCard
                            title="Invoke URL"
                            actionHref={deploymentPath}
                            actionLabel="Deployments"
                            headerAction={(
                                <Tooltip title="Open the consumer configuration">
                                    <Button
                                        size="small"
                                        variant="text"
                                        startIcon={<Plug size={14} />}
                                        onClick={() => setConsumerConfigOpen(true)}
                                        sx={{ minWidth: 0, fontSize: "0.75rem" }}
                                    >
                                        Connect
                                    </Button>
                                </Tooltip>
                            )}
                            variant="plain"
                            sx={{ height: "100%" }}
                        >
                            <TextInput
                                value={invokeUrl}
                                copyable
                                copyTooltipText="Copy URL"
                                slotProps={{ input: { readOnly: true } }}
                                sx={{ mb: 1 }}
                            />
                            <Box display="flex" flexWrap="wrap" gap={1}>
                                <StatusPill
                                    label="Auth"
                                    value={authLabel}
                                    tooltip={authTooltip}
                                />
                                <StatusPill
                                    label="CORS"
                                    value={corsLabel}
                                    tooltip={corsTooltip}
                                />
                                <IsolationTierChip tier={isolationTier} />
                            </Box>
                        </OverviewSectionCard>
                    </Grid>
                )}
                <Grid size={{ xs: 12, md: invokeUrl ? 4 : 12 }}>
                    <OverviewSectionCard
                        title="Agent Interface"
                        actionHref={tryItHref}
                        actionLabel="Try It"
                        variant="plain"
                        sx={{ height: "100%" }}
                    >
                        {resources.length === 0 ? (
                            <Typography
                                variant="caption"
                                color="text.disabled"
                                sx={{ display: "block", fontStyle: "italic" }}
                            >
                                Unable to find API schema
                            </Typography>
                        ) : (
                            <Box display="flex" flexWrap="wrap" gap={1}>
                                {resources.map((resource) => (
                                    <Box
                                        key={`${resource.method} ${resource.path}`}
                                        display="flex"
                                        alignItems="center"
                                        gap={0.75}
                                        sx={{
                                            border: "1px solid",
                                            borderColor: "divider",
                                            borderRadius: "999px",
                                            // The Chip already carries its own pill
                                            // padding on the left, so a smaller pl here
                                            // (vs. pr, which backs onto plain unpadded
                                            // text) keeps the inset even on both ends.
                                            pl: 0.5,
                                            pr: 1.25,
                                            py: 0.5,
                                        }}
                                    >
                                        <Chip
                                            label={
                                                METHOD_LABEL[resource.method] ?? resource.method
                                            }
                                            size="small"
                                            variant="outlined"
                                            color={METHOD_COLOR[resource.method] ?? "default"}
                                            sx={{ fontSize: "0.6875rem", fontWeight: 600 }}
                                        />
                                        <Typography variant="body2" sx={{ fontFamily: "monospace" }} noWrap>
                                            {resource.path}
                                        </Typography>
                                    </Box>
                                ))}
                            </Box>
                        )}
                    </OverviewSectionCard>
                </Grid>
            </Grid>
            {invokeUrl && (
                <ConsumerConfigDrawer
                    open={consumerConfigOpen}
                    onClose={() => setConsumerConfigOpen(false)}
                    orgId={orgId}
                    projectId={projectId}
                    agentId={agentId}
                    envId={envId}
                    invokeUrl={invokeUrl}
                    authMode={authMode}
                    authLabel={authLabel}
                    authHeaderExample={authHeaderExample}
                    oauthIssuers={oauthIssuers}
                />
            )}
        </CollapsibleSection>
    );
};

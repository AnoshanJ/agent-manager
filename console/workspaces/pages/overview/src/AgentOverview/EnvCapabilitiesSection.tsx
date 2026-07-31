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

import { useMemo } from "react";
import { Box, Chip, Divider, Tooltip, Typography, type ChipProps } from "@wso2/oxygen-ui";
import { generatePath } from "react-router-dom";
import { useGetAgentEndpoints } from "@agent-management-platform/api-client";
import {
    CollapsibleSection,
    extractOpenApiResources,
    parseOpenApiSpecContent,
    type OpenApiResource,
} from "@agent-management-platform/shared-component";
import { absoluteRouteMap, type Configurations } from "@agent-management-platform/types";
import { TextInput } from "@agent-management-platform/views";
import { SectionHeader, UppercaseCaptionLabel } from "./SectionHeader";

interface EnvCapabilitiesSectionProps {
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
    configurations?: Configurations;
    external?: boolean;
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
    orgId, projectId, agentId, envId, configurations, external,
}) => {
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
    const authMode: "none" | "apikey" | "oauth" = configurations?.enableOAuthSecurity
        ? "oauth"
        : configurations?.enableApiKeySecurity
            ? "apikey"
            : "none";
    const authHeaderPrefix = configurations?.oauthConfig?.authHeaderPrefix || "Bearer";
    const authLabel = authMode === "oauth"
        ? `OAuth2 (${authHeaderPrefix})`
        : authMode === "apikey"
            ? "API Key"
            : "None";
    const authTooltip = authMode === "oauth"
        ? `Callers send an Authorization: ${authHeaderPrefix} <token> header validated by the gateway`
        : authMode === "apikey"
            ? "Requests must include the header: x-api-key: <your-key>"
            : "Endpoint is publicly accessible without authentication";

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

    // Resources need a parsed OpenAPI schema; invokeUrl only needs the
    // endpoint itself to have resolved a URL. A kind-type agent can have one
    // without the other (e.g. schema not registered yet) — show whichever is
    // actually available instead of hiding invokeUrl behind resources.
    const show = !isLoading && (resources.length > 0 || !!invokeUrl);

    return (
        <CollapsibleSection show={show}>
            <SectionHeader title="Capabilities" viewAllHref={tryItHref} viewAllLabel="Try It" />
            {resources.length === 0 ? (
                <Typography
                    variant="caption"
                    color="text.disabled"
                    sx={{ display: "block", fontStyle: "italic", mb: 1.5 }}
                >
                    Unable to find API schema
                </Typography>
            ) : (
                <Box display="flex" flexWrap="wrap" gap={1} sx={{ mb: 1.5 }}>
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
                                // The Chip already carries its own pill padding on the
                                // left, so a smaller pl here (vs. pr, which backs onto
                                // plain unpadded text) keeps the inset even on both ends.
                                pl: 0.5,
                                pr: 1.25,
                                py: 0.5,
                            }}
                        >
                            <Chip
                                label={METHOD_LABEL[resource.method] ?? resource.method}
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
            {invokeUrl && (
                <Box sx={{ mb: 1.5 }}>
                    <Divider sx={{ mb: 1.5 }} />
                    <UppercaseCaptionLabel sx={{ display: "block", mb: 0.75 }}>
                        Invoke URL
                    </UppercaseCaptionLabel>
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
                    </Box>
                </Box>
            )}
        </CollapsibleSection>
    );
};

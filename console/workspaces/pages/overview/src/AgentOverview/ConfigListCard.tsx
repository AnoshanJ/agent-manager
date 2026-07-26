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

import { Avatar, Box, Card, CardContent, Skeleton, Typography, useTheme } from "@wso2/oxygen-ui";

interface ConfigListCardProps {
    avatarLabel: string;
    avatarColor: string;
    /** Provider logo (e.g. LLM provider template's metadata.logoUrl), shown
     * instead of the letter avatar when present — mirrors the logo Chip on
     * the LLM Providers listing page (LLMProviderTable.tsx). */
    avatarSrc?: string;
    title: string;
    /** Underlying provider/proxy name, shown muted next to the title. */
    providerLabel?: string;
    subtitle?: string;
    isLoadingSubtitle?: boolean;
}

/**
 * Presentational row card shared by the Model Configs and MCP Proxies
 * previews below Invoke URL — provider logo (or a colored letter fallback),
 * config name, and an optional secondary line (e.g. guardrails summary for
 * LLM configs). Uses the same Card variant="outlined" + CardContent pairing
 * as EmptyConfigCard.tsx elsewhere in Configure Agent, instead of a
 * hand-styled Box.
 */
export const ConfigListCard: React.FC<ConfigListCardProps> = ({
    avatarLabel, avatarColor, avatarSrc, title, providerLabel, subtitle, isLoadingSubtitle,
}) => {
    const theme = useTheme();
    const avatarBgcolor = avatarSrc ? theme.palette.grey[100] : avatarColor;
    const avatarTextColor = theme.palette.getContrastText(avatarBgcolor);

    return (
    <Card variant="outlined">
        <CardContent sx={{ display: "flex", alignItems: "center", gap: 1.5, "&:last-child": { pb: 2 } }}>
            <Avatar
                src={avatarSrc}
                sx={{
                    bgcolor: avatarBgcolor,
                    color: avatarTextColor,
                    width: 36,
                    height: 36,
                    fontSize: 14,
                }}
            >
                {avatarLabel}
            </Avatar>
            <Box minWidth={0}>
                <Box display="flex" alignItems="baseline" gap={0.75} minWidth={0}>
                    <Typography variant="body2" fontWeight={600} noWrap>
                        {title}
                    </Typography>
                    {providerLabel && (
                        <Typography variant="caption" color="text.disabled" noWrap>
                            {providerLabel}
                        </Typography>
                    )}
                </Box>
                {isLoadingSubtitle ? (
                    <Skeleton variant="text" width={140} height={16} />
                ) : subtitle ? (
                    <Typography variant="caption" color="text.secondary" noWrap sx={{ display: "block" }}>
                        {subtitle}
                    </Typography>
                ) : null}
            </Box>
        </CardContent>
    </Card>
    );
};

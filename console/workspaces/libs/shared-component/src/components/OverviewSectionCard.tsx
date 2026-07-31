/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type { ReactNode } from "react";
import { Box, Button, Card, Typography, type SxProps, type Theme } from "@wso2/oxygen-ui";
import { ChevronRight } from "@wso2/oxygen-ui-icons-react";
import { Link } from "react-router-dom";

interface OverviewSectionCardProps {
  /** Uppercase caption shown in the card's own header row. */
  title: string;
  /** Omit when there's nowhere to link out to — the action button is skipped entirely. */
  actionHref?: string;
  actionLabel?: string;
  sx?: SxProps<Theme>;
  children: ReactNode;
}

/**
 * Outlined Card with a built-in header row (uppercase title + a "View all"-style
 * action link in the top-right corner), used for the standalone cards on an
 * agent's overview page (e.g. Invoke URL, Capabilities). Bakes in the
 * Card/CardContent/header-row boilerplate so call sites only supply the body.
 */
export const OverviewSectionCard: React.FC<OverviewSectionCardProps> = ({
  title, actionHref, actionLabel = "View all", sx, children,
}) => (
  <Card variant="outlined" sx={{ px: 2, py: 1, mt:1, pb:3, ...sx }}>

      <Box display="flex" justifyContent="space-between" pb={1} alignItems="center">
        <Typography
          variant="caption"
          color="text.secondary"
          fontWeight={600}
          sx={{ textTransform: "uppercase", letterSpacing: "0.05em" }}
        >
          {title}
        </Typography>
        {actionHref && (
          <Button
            size="small"
            variant="text"
            endIcon={<ChevronRight size={14} />}
            component={Link}
            to={actionHref}
            sx={{ minWidth: 0, fontSize: "0.75rem" }}
          >
            {actionLabel}
          </Button>
        )}
      </Box>
      {children}
  </Card>
);

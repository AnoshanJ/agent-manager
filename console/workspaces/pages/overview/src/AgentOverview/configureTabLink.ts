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

import { generatePath } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

/**
 * Deep-links to the Configure Agent page with the given tab pre-selected.
 * Mirrors CONFIGURE_TAB_PARAM/CONFIGURE_TAB_KEYS in
 * pages/configure-agent/src/configureTabs.ts, which aren't part of that
 * package's public exports — keep the "llm"/"tools" values in sync if those
 * ever change.
 */
export function buildConfigureTabHref(
    orgId: string,
    projectId: string,
    agentId: string,
    tab: "llm" | "tools",
): string {
    const path = generatePath(
        absoluteRouteMap.children.org.children.projects.children.agents.children.configure.path,
        { orgId, projectId, agentId },
    );
    return `${path}?tab=${tab}`;
}

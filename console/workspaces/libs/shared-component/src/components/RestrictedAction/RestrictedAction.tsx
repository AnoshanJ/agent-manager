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

import type { ReactElement } from "react";
import { Tooltip } from "@mui/material";
import type { AccessDecision } from "../../utils/environmentTierAccess";

export interface RestrictedActionProps {
  decision: AccessDecision;
  /** The control itself. The caller still owns its `disabled` prop. */
  children: ReactElement;
}

/**
 * Explains why a control the caller's scopes do not reach is disabled.
 *
 * A disabled MUI control emits no pointer events, so a Tooltip placed directly
 * on it never opens; the reason has to hang off an element beside it. That is
 * all this component does — and only when the decision is a denial, so an
 * allowed control keeps exactly the markup it had.
 *
 * Disabling rather than hiding is deliberate: an absent Promote button is
 * already how the console says "this environment has no downstream target", and
 * a missing permission is a different thing to say.
 */
export function RestrictedAction({ decision, children }: RestrictedActionProps) {
  if (decision.allowed) return children;
  return (
    <Tooltip title={decision.reason}>
      <span style={{ display: "inline-flex" }}>{children}</span>
    </Tooltip>
  );
}

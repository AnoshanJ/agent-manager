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

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Button } from "@mui/material";
import { RestrictedAction } from "./RestrictedAction";

describe("RestrictedAction", () => {
  it("leaves an allowed control alone", () => {
    render(
      <RestrictedAction decision={{ allowed: true, reason: "" }}>
        <Button>Promote</Button>
      </RestrictedAction>,
    );
    const button = screen.getByRole("button", { name: "Promote" });
    expect(button).toBeEnabled();
  });

  // A disabled MUI Button emits no pointer events, so the reason has to hang off
  // a wrapper or the tooltip never opens — which is the whole point of this
  // component.
  it("explains a denied control on hover", async () => {
    render(
      <RestrictedAction
        decision={{
          allowed: false,
          missingScope: "amp:agent:env-production",
          reason: "You do not have permission to act on production environments.",
        }}
      >
        <Button>Promote</Button>
      </RestrictedAction>,
    );
    fireEvent.mouseOver(screen.getByRole("button", { name: "Promote" }).parentElement!);
    expect(
      await screen.findByText(
        "You do not have permission to act on production environments.",
      ),
    ).toBeInTheDocument();
  });

  // The caller passes no `disabled` of its own here: the denial alone has to
  // disable the control, or a wrapper whose tooltip says "you may not press
  // this" ships beside a button that can be pressed.
  it("disables a denied control the caller left enabled", () => {
    render(
      <RestrictedAction
        decision={{
          allowed: false,
          missingScope: "amp:agent:env-production",
          reason: "You do not have permission to act on production environments.",
        }}
      >
        <Button>Promote</Button>
      </RestrictedAction>,
    );
    expect(screen.getByRole("button", { name: "Promote" })).toBeDisabled();
  });
});

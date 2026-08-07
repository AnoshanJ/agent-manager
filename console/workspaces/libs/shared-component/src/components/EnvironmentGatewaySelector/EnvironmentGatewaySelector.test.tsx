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

import React, { useState } from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import type {
  Environment,
  GatewayResponse,
  GatewayType,
} from "@agent-management-platform/types";
// Only EnvironmentGatewaySelectorView is under test, but importing the module
// drags in api-client, whose auth dependency crashes at import time outside a
// configured app shell. Stub the module boundary; no hook behavior is mocked.
vi.mock("@agent-management-platform/api-client", () => ({
  useListEnvironments: vi.fn(() => ({ data: [], isLoading: false })),
  useListGateways: vi.fn(() => ({ data: undefined, isLoading: false })),
}));

import { EnvironmentGatewaySelectorView } from "./EnvironmentGatewaySelector";

const renderWithTheme = (component: React.ReactElement) =>
  render(<ThemeProvider theme={createTheme()}>{component}</ThemeProvider>);

const makeEnvironment = (id: string, name: string): Environment => ({
  id,
  name,
  displayName: name,
  dataplaneRef: "dp-1",
  isProduction: false,
  createdAt: "2026-01-01T00:00:00Z",
});

// A gateway belongs to exactly one environment; the wire shape is an array.
const makeGateway = (
  uuid: string,
  envId: string,
  gatewayType: GatewayType = "EGRESS",
): GatewayResponse => ({
  uuid,
  organizationName: "org",
  name: uuid,
  displayName: `Gateway ${uuid}`,
  gatewayType,
  vhost: "example.com",
  isCritical: false,
  status: "ACTIVE",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
  environments: [
    {
      id: envId,
      organizationName: "org",
      name: envId,
      displayName: envId,
      dataplaneRef: "dp-1",
      dnsPrefix: envId,
      isProduction: false,
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-01T00:00:00Z",
    },
  ],
});

// Controlled wrapper so onChange results flow back into value, mirroring how
// the create form and Deployment tab own the selection state.
const ControlledSelector: React.FC<{
  environments: Environment[];
  gateways: GatewayResponse[];
  initialValue?: string[];
  lockedGatewayIds?: string[];
  disabled?: boolean;
  onChangeSpy?: (ids: string[]) => void;
  onValidityChange?: (isValid: boolean) => void;
}> = ({
  environments,
  gateways,
  initialValue = [],
  lockedGatewayIds,
  disabled,
  onChangeSpy,
  onValidityChange,
}) => {
  const [value, setValue] = useState<string[]>(initialValue);
  return (
    <EnvironmentGatewaySelectorView
      environments={environments}
      gateways={gateways}
      value={value}
      onChange={(ids) => {
        setValue(ids);
        onChangeSpy?.(ids);
      }}
      lockedGatewayIds={lockedGatewayIds}
      onValidityChange={onValidityChange}
      disabled={disabled}
    />
  );
};

// `hidden: true` throughout: while an MUI Select menu is open (or still
// unmounting), the Modal marks the rest of the app aria-hidden, which would
// otherwise make every row invisible to role queries.
const getCheckbox = (name: string) =>
  screen.getByRole("checkbox", { name, hidden: true }) as HTMLInputElement;

const chooseOption = (selectName: string, optionName: string) => {
  fireEvent.mouseDown(
    screen.getByRole("combobox", { name: selectName, hidden: true }),
  );
  const listboxes = screen.getAllByRole("listbox", { hidden: true });
  const listbox = listboxes[listboxes.length - 1];
  fireEvent.click(
    within(listbox).getByRole("option", { name: optionName, hidden: true }),
  );
};

describe("EnvironmentGatewaySelectorView", () => {
  it("emits the sole candidate without rendering a Select when a 1-candidate row is checked", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(
      screen.queryByRole("combobox", { hidden: true }),
    ).not.toBeInTheDocument();
    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).toHaveBeenCalledWith(["gw-1"]);
  });

  it("contributes nothing while a 1-candidate row stays unchecked", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(getCheckbox("Alpha")).not.toBeChecked();
    expect(onChangeSpy).not.toHaveBeenCalled();
  });

  it("renders a Select for 2 candidates and reports invalid until one is chosen", () => {
    const onChangeSpy = vi.fn();
    const onValidityChange = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-a"),
        ]}
        onChangeSpy={onChangeSpy}
        onValidityChange={onValidityChange}
      />,
    );
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).not.toHaveBeenCalled();
    expect(onValidityChange).toHaveBeenLastCalledWith(false);
    expect(
      screen.getByText("Select an egress gateway for this environment."),
    ).toBeInTheDocument();
    chooseOption("Egress gateway for Alpha", "Gateway gw-2");
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-2"]);
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
  });

  it("does not block validity while a 2-candidate row is unchecked and unresolved", () => {
    const onValidityChange = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-a"),
        ]}
        onValidityChange={onValidityChange}
      />,
    );
    expect(onValidityChange).toHaveBeenLastCalledWith(true);
    expect(onValidityChange).not.toHaveBeenCalledWith(false);
  });

  it("disables the checkbox of a 0-candidate row and explains why", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[]}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(getCheckbox("Alpha")).toBeDisabled();
    expect(
      screen.getByText(
        "No egress-capable gateway is attached to this environment.",
      ),
    ).toBeInTheDocument();
    expect(onChangeSpy).not.toHaveBeenCalled();
  });

  it("renders a locked row checked with a disabled Select and a Deployed chip", () => {
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-a"),
        ]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
      />,
    );
    expect(getCheckbox("Alpha")).toBeChecked();
    expect(
      screen.getByRole("combobox", {
        name: "Egress gateway for Alpha",
        hidden: true,
      }),
    ).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByText("Deployed")).toBeInTheDocument();
  });

  it("drops a locked gateway from the emission when its row is unchecked, and re-adds it on re-check", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-a"),
        ]}
        initialValue={["gw-1"]}
        lockedGatewayIds={["gw-1"]}
        onChangeSpy={onChangeSpy}
      />,
    );
    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).toHaveBeenLastCalledWith([]);
    fireEvent.click(getCheckbox("Alpha"));
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-1"]);
  });

  it("evicts the previously selected gateway when a different candidate is chosen in the same environment", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[
          makeEnvironment("env-a", "Alpha"),
          makeEnvironment("env-b", "Beta"),
        ]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-a"),
          makeGateway("gw-3", "env-b"),
        ]}
        onChangeSpy={onChangeSpy}
      />,
    );
    fireEvent.click(getCheckbox("Alpha"));
    chooseOption("Egress gateway for Alpha", "Gateway gw-1");
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-1"]);

    // A gateway in a different environment coexists.
    fireEvent.click(getCheckbox("Beta"));
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-1", "gw-3"]);

    // Switching Alpha's candidate replaces gw-1 rather than joining it.
    chooseOption("Egress gateway for Alpha", "Gateway gw-2");
    expect(onChangeSpy).toHaveBeenLastCalledWith(["gw-3", "gw-2"]);
  });

  it("renders an unmapped selected gateway as a removable locked row", () => {
    const onChangeSpy = vi.fn();
    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        initialValue={["ghost-gw"]}
        onChangeSpy={onChangeSpy}
      />,
    );
    expect(screen.getByText("Unmapped")).toBeInTheDocument();
    const ghostCheckbox = getCheckbox("ghost-gw");
    expect(ghostCheckbox).toBeChecked();
    fireEvent.click(ghostCheckbox);
    expect(onChangeSpy).toHaveBeenLastCalledWith([]);
    expect(screen.queryByText("Unmapped")).not.toBeInTheDocument();
  });

  it("shows the selection footer only when there is more than one environment", () => {
    const { unmount } = renderWithTheme(
      <ControlledSelector
        environments={[
          makeEnvironment("env-a", "Alpha"),
          makeEnvironment("env-b", "Beta"),
        ]}
        gateways={[
          makeGateway("gw-1", "env-a"),
          makeGateway("gw-2", "env-b"),
        ]}
        initialValue={["gw-1"]}
      />,
    );
    expect(
      screen.getByText("1 of 2 environments selected."),
    ).toBeInTheDocument();
    unmount();

    renderWithTheme(
      <ControlledSelector
        environments={[makeEnvironment("env-a", "Alpha")]}
        gateways={[makeGateway("gw-1", "env-a")]}
        initialValue={["gw-1"]}
      />,
    );
    expect(screen.queryByText(/environments selected/)).not.toBeInTheDocument();
  });
});

// Deterministic LCG so any failure reproduces from its seed.
const makeLcg = (seed: number) => {
  let state = seed >>> 0;
  return () => {
    state = (state * 1664525 + 1013904223) >>> 0;
    return state / 2 ** 32;
  };
};

interface Fixture {
  environments: Environment[];
  gateways: GatewayResponse[];
  lockedGatewayIds: string[];
  initialValue: string[];
  envIdByUuid: Record<string, string | undefined>;
}

const generateFixture = (rand: () => number): Fixture => {
  const envCount = 2 + Math.floor(rand() * 4);
  const environments = Array.from({ length: envCount }, (_, i) =>
    makeEnvironment(`env-${i}`, `Env ${i}`),
  );
  const gatewayCount = 2 + Math.floor(rand() * 5);
  const gateways = Array.from({ length: gatewayCount }, (_, i) =>
    makeGateway(
      `gw-${i}`,
      environments[Math.floor(rand() * envCount)].id as string,
    ),
  );

  const envIdByUuid: Record<string, string | undefined> = {};
  gateways.forEach((gateway) => {
    envIdByUuid[gateway.uuid] = gateway.environments?.[0]?.id;
  });

  const lockedGatewayIds: string[] = [];
  const lockedEnvIds = new Set<string>();
  gateways.forEach((gateway) => {
    if (rand() >= 0.35) return;
    const envId = envIdByUuid[gateway.uuid];
    if (!envId || lockedEnvIds.has(envId)) return;
    lockedGatewayIds.push(gateway.uuid);
    lockedEnvIds.add(envId);
  });

  const initialValue = [...lockedGatewayIds];
  if (rand() < 0.3) {
    initialValue.push("ghost-gw");
    envIdByUuid["ghost-gw"] = undefined;
  }

  return { environments, gateways, lockedGatewayIds, initialValue, envIdByUuid };
};

const performRandomAction = (rand: () => number) => {
  const checkboxes = screen
    .queryAllByRole("checkbox", { hidden: true })
    .filter((checkbox) => !(checkbox as HTMLInputElement).disabled);
  const combos = screen
    .queryAllByRole("combobox", { hidden: true })
    .filter((combo) => combo.getAttribute("aria-disabled") !== "true");
  const total = checkboxes.length + combos.length;
  if (total === 0) return;
  const pick = Math.floor(rand() * total);
  if (pick < checkboxes.length) {
    fireEvent.click(checkboxes[pick]);
    return;
  }
  fireEvent.mouseDown(combos[pick - checkboxes.length]);
  const listboxes = screen.getAllByRole("listbox", { hidden: true });
  const options = within(listboxes[listboxes.length - 1])
    .getAllByRole("option", { hidden: true })
    .filter((option) => option.getAttribute("aria-disabled") !== "true");
  if (options.length > 0) {
    fireEvent.click(options[Math.floor(rand() * options.length)]);
  }
};

describe("EnvironmentGatewaySelectorView placement invariant", () => {
  it.each([7, 42, 1337, 20260807])(
    "never emits two gateways in the same environment (seed %i)",
    (seed) => {
      const rand = makeLcg(seed);
      const fixture = generateFixture(rand);
      const emissions: string[][] = [];
      renderWithTheme(
        <ControlledSelector
          environments={fixture.environments}
          gateways={fixture.gateways}
          initialValue={fixture.initialValue}
          lockedGatewayIds={fixture.lockedGatewayIds}
          onChangeSpy={(ids) => emissions.push(ids)}
        />,
      );

      let verified = 0;
      const verifyNewEmissions = () => {
        for (; verified < emissions.length; verified += 1) {
          const emitted = emissions[verified];
          const coveredEnvIds = new Set<string>();
          emitted.forEach((uuid) => {
            const envId = fixture.envIdByUuid[uuid];
            if (!envId) return;
            if (coveredEnvIds.has(envId)) {
              throw new Error(
                `seed ${seed}, emission ${verified}: env ${envId} covered ` +
                  `twice in [${emitted.join(", ")}]`,
              );
            }
            coveredEnvIds.add(envId);
          });
        }
      };

      for (let step = 0; step < 25; step += 1) {
        performRandomAction(rand);
        verifyNewEmissions();
      }
      expect(emissions.length).toBeGreaterThan(0);
    },
  );
});

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

import React, { useEffect, useMemo, useState } from "react";
import {
  Box,
  Checkbox,
  Chip,
  FormControl,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import type {
  Environment,
  GatewayResponse,
} from "@agent-management-platform/types";
import {
  useListEnvironments,
  useListGateways,
} from "@agent-management-platform/api-client";

export interface EnvironmentGatewaySelectorProps {
  orgId: string;
  /** Gateway UUIDs currently selected (controlled). */
  value: string[];
  onChange: (gatewayIds: string[]) => void;
  /** Gateway UUIDs already deployed — rendered locked. Omit in create form. */
  lockedGatewayIds?: string[];
  /** false while any checked 2+-candidate environment lacks a resolved gateway. */
  onValidityChange?: (isValid: boolean) => void;
  disabled?: boolean;
}

export interface EnvironmentGatewaySelectorViewProps
  extends Omit<EnvironmentGatewaySelectorProps, "orgId"> {
  environments: Environment[];
  gateways: GatewayResponse[];
  isLoading?: boolean;
}

// A gateway belongs to exactly one environment (business rule; the wire shape
// is an array). A gateway with no mapping surfaces via the "Unmapped" row.
const environmentIdOf = (gateway: GatewayResponse): string | undefined =>
  gateway.environments?.[0]?.id;

const gatewayLabel = (gateway: GatewayResponse): string =>
  gateway.displayName || gateway.name;

const AMBIGUOUS_CAPTION = "Select an egress gateway for this environment.";
const UNAVAILABLE_CAPTION =
  "No egress-capable gateway is attached to this environment.";
const DEPLOYED_CAPTION =
  "Placement is fixed once deployed. To use a different gateway, uncheck " +
  "this environment and save to undeploy, then select the new gateway and " +
  "save again.";

interface EnvironmentRow {
  envId: string;
  label: string;
  candidates: GatewayResponse[];
  lockedGateway?: GatewayResponse;
  resolvedGateway?: GatewayResponse;
  checked: boolean;
}

export const EnvironmentGatewaySelectorView: React.FC<
  EnvironmentGatewaySelectorViewProps
> = ({
  environments,
  gateways,
  isLoading,
  value,
  onChange,
  lockedGatewayIds = [],
  onValidityChange,
  disabled,
}) => {
  const [pendingEnvIds, setPendingEnvIds] = useState<Set<string>>(new Set());

  const valueSet = useMemo(() => new Set(value), [value]);
  const lockedSet = useMemo(() => new Set(lockedGatewayIds), [lockedGatewayIds]);
  const gatewayByUuid = useMemo(
    () => new Map(gateways.map((gateway) => [gateway.uuid, gateway])),
    [gateways],
  );

  // Egress-capable only: ingress gateways are not legal LLM placement targets
  // and the server rejects them. No status filter — the server's candidate set
  // is not liveness-filtered either, so filtering here would offer a narrower
  // set than the server accepts and hide a valid choice whenever a gateway is
  // briefly disconnected.
  const gatewaysByEnv = useMemo(() => {
    const map: Record<string, GatewayResponse[]> = {};
    gateways.forEach((gateway) => {
      if (gateway.gatewayType !== "EGRESS" && gateway.gatewayType !== "BOTH") {
        return;
      }
      const envId = environmentIdOf(gateway);
      if (!envId) return;
      (map[envId] ??= []).push(gateway);
    });
    return map;
  }, [gateways]);

  const rows = useMemo<EnvironmentRow[]>(
    () =>
      environments.flatMap((env) => {
        if (!env.id) return [];
        const candidates = gatewaysByEnv[env.id] ?? [];
        const lockedGateway = candidates.find((candidate) =>
          lockedSet.has(candidate.uuid),
        );
        const resolvedGateway = candidates.find((candidate) =>
          valueSet.has(candidate.uuid),
        );
        const checked = lockedGateway
          ? valueSet.has(lockedGateway.uuid)
          : resolvedGateway != null || pendingEnvIds.has(env.id);
        return [
          {
            envId: env.id,
            label: env.displayName || env.name,
            candidates,
            lockedGateway,
            resolvedGateway,
            checked,
          },
        ];
      }),
    [environments, gatewaysByEnv, lockedSet, valueSet, pendingEnvIds],
  );

  const unmappedSelectedUuids = useMemo(() => {
    const candidateUuids = new Set(
      rows.flatMap((row) => row.candidates.map((candidate) => candidate.uuid)),
    );
    return value.filter((uuid) => !candidateUuids.has(uuid));
  }, [rows, value]);

  const isValid = rows.every(
    (row) => !(row.checked && !row.lockedGateway && !row.resolvedGateway),
  );

  useEffect(() => {
    onValidityChange?.(isValid);
  }, [isValid, onValidityChange]);

  const setPending = (envId: string, pending: boolean) => {
    setPendingEnvIds((prev) => {
      const next = new Set(prev);
      if (pending) next.add(envId);
      else next.delete(envId);
      return next;
    });
  };

  const emitRemove = (uuid: string) => {
    onChange(value.filter((selected) => selected !== uuid));
  };

  // Evicting any same-environment gateway before adding keeps the emitted
  // array inside the server's one-gateway-per-environment placement rule.
  const emitAdd = (gateway: GatewayResponse) => {
    const envId = environmentIdOf(gateway);
    const next = value.filter((uuid) => {
      if (uuid === gateway.uuid) return false;
      const other = gatewayByUuid.get(uuid);
      if (!other || envId === undefined) return true;
      return environmentIdOf(other) !== envId;
    });
    onChange([...next, gateway.uuid]);
  };

  const handleToggle = (row: EnvironmentRow) => {
    if (row.lockedGateway) {
      if (row.checked) emitRemove(row.lockedGateway.uuid);
      else emitAdd(row.lockedGateway);
      return;
    }
    if (row.checked) {
      setPending(row.envId, false);
      if (row.resolvedGateway) emitRemove(row.resolvedGateway.uuid);
      return;
    }
    if (row.candidates.length === 1) emitAdd(row.candidates[0]);
    else setPending(row.envId, true);
  };

  const handleSelect = (row: EnvironmentRow, uuid: string) => {
    const gateway = gatewayByUuid.get(uuid);
    if (!gateway) return;
    setPending(row.envId, false);
    emitAdd(gateway);
  };

  if (isLoading) {
    return (
      <Stack spacing={1}>
        {[0, 1, 2].map((index) => (
          <Skeleton key={index} variant="rounded" height={40} />
        ))}
      </Stack>
    );
  }

  const deployedCount = rows.filter((row) =>
    row.candidates.some((candidate) => valueSet.has(candidate.uuid)),
  ).length;

  const renderRowContent = (row: EnvironmentRow) => {
    if (row.candidates.length === 0) {
      return (
        <Typography variant="caption" color="text.secondary">
          {UNAVAILABLE_CAPTION}
        </Typography>
      );
    }
    if (row.lockedGateway) {
      return (
        <>
          <FormControl fullWidth>
            <Select
              size="small"
              value={row.lockedGateway.uuid}
              disabled
              SelectDisplayProps={{
                "aria-label": `Egress gateway for ${row.label}`,
              }}
            >
              {row.candidates.map((candidate) => (
                <MenuItem key={candidate.uuid} value={candidate.uuid}>
                  {gatewayLabel(candidate)}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
            {DEPLOYED_CAPTION}
          </Typography>
        </>
      );
    }
    if (row.candidates.length === 1) {
      return (
        <Typography variant="body2" color="text.secondary">
          {gatewayLabel(row.candidates[0])}
        </Typography>
      );
    }
    return (
      <>
        <FormControl fullWidth>
          <Select
            size="small"
            value={row.resolvedGateway?.uuid ?? ""}
            disabled={disabled}
            onChange={(event) => handleSelect(row, event.target.value)}
            SelectDisplayProps={{
              "aria-label": `Egress gateway for ${row.label}`,
            }}
          >
            {row.candidates.map((candidate) => (
              <MenuItem key={candidate.uuid} value={candidate.uuid}>
                {gatewayLabel(candidate)}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        {row.checked && !row.resolvedGateway && (
          <Typography variant="caption" color="error" sx={{ mt: 0.5 }}>
            {AMBIGUOUS_CAPTION}
          </Typography>
        )}
      </>
    );
  };

  return (
    <Stack spacing={1.5}>
      {rows.map((row) => (
        <Stack
          key={row.envId}
          direction="row"
          spacing={1}
          alignItems="flex-start"
          sx={row.candidates.length === 0 ? { opacity: 0.5 } : undefined}
        >
          <Checkbox
            size="small"
            checked={row.checked}
            disabled={disabled || row.candidates.length === 0}
            onChange={() => handleToggle(row)}
            inputProps={{ "aria-label": row.label }}
            sx={{ p: 0.5 }}
          />
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Stack
              direction="row"
              spacing={1}
              alignItems="center"
              sx={{ mb: 0.5 }}
            >
              <Typography variant="body2">{row.label}</Typography>
              {row.lockedGateway && (
                <Chip
                  label="Deployed"
                  size="small"
                  variant="outlined"
                  color="success"
                />
              )}
            </Stack>
            {renderRowContent(row)}
          </Box>
        </Stack>
      ))}
      {unmappedSelectedUuids.map((uuid) => {
        const gateway = gatewayByUuid.get(uuid);
        const label = gateway ? gatewayLabel(gateway) : uuid;
        return (
          <Stack
            key={uuid}
            direction="row"
            spacing={1}
            alignItems="flex-start"
          >
            <Checkbox
              size="small"
              checked
              disabled={disabled}
              onChange={() => emitRemove(uuid)}
              inputProps={{ "aria-label": label }}
              sx={{ p: 0.5 }}
            />
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Stack direction="row" spacing={1} alignItems="center">
                <Typography variant="body2">{label}</Typography>
                <Chip label="Unmapped" size="small" variant="outlined" />
              </Stack>
            </Box>
          </Stack>
        );
      })}
      {rows.length > 1 && (
        <Typography variant="caption" color="text.secondary">
          {deployedCount} of {rows.length} environments deployed.
        </Typography>
      )}
    </Stack>
  );
};

export const EnvironmentGatewaySelector: React.FC<
  EnvironmentGatewaySelectorProps
> = ({ orgId, ...viewProps }) => {
  const { data: environments, isLoading: isLoadingEnvironments } =
    useListEnvironments({ orgName: orgId });
  // limit: 500 — without it the server's default page size silently truncates
  // the gateway list and environments would wrongly render as unavailable.
  const { data: gatewaysData, isLoading: isLoadingGateways } = useListGateways(
    { orgName: orgId },
    { limit: 500 },
  );
  return (
    <EnvironmentGatewaySelectorView
      environments={environments ?? []}
      gateways={gatewaysData?.gateways ?? []}
      isLoading={isLoadingEnvironments || isLoadingGateways}
      {...viewProps}
    />
  );
};

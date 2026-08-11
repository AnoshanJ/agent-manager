// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package services

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// mcpVarRows builds the pair of env var rows (url + apikey) that configuring an MCP
// connection persists for an environment, whether or not that environment turned out
// to be deployable.
func mcpVarRows(configUUID uuid.UUID, envUUIDs ...uuid.UUID) []models.AgentEnvConfigVariable {
	rows := make([]models.AgentEnvConfigVariable, 0, len(envUUIDs)*2)
	for _, envUUID := range envUUIDs {
		rows = append(
			rows,
			models.AgentEnvConfigVariable{ConfigUUID: configUUID, EnvironmentUUID: envUUID, VariableKey: "url", VariableName: "BOOKING_URL"},
			models.AgentEnvConfigVariable{ConfigUUID: configUUID, EnvironmentUUID: envUUID, VariableKey: "apikey", VariableName: "BOOKING_API_KEY"},
		)
	}
	return rows
}

// An environment the connection was configured for, but which never got a mapping
// because the proxy had no endpoint bound there at the time, must be reported as
// needing activation. This is the state promotion leaves behind: env var rows exist
// (so the vars are injected) but they resolve to empty strings forever.
func TestMCPEnvsNeedingActivation_ReportsEnvWithVarRowsButNoMapping(t *testing.T) {
	configUUID, proxyUUID := uuid.New(), uuid.New()
	devEnv, prodEnv := uuid.New(), uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: proxyUUID},
	}
	vars := mcpVarRows(configUUID, devEnv, prodEnv)

	got := mcpEnvsNeedingActivation(mappings, vars, proxyUUID)

	require.Equal(t, []uuid.UUID{prodEnv}, got,
		"prod has env var rows but no mapping — it must be reported for backfill")
}

// An environment that already has a mapping is fully bound; re-activating it would
// mint a duplicate API key and violate uq_env_mcp_mapping.
func TestMCPEnvsNeedingActivation_SkipsAlreadyMappedEnv(t *testing.T) {
	configUUID, proxyUUID := uuid.New(), uuid.New()
	devEnv := uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: proxyUUID},
	}

	got := mcpEnvsNeedingActivation(mappings, mcpVarRows(configUUID, devEnv), proxyUUID)

	require.Empty(t, got)
}

// The proxy to bind in an unmapped environment is inferred from the config's sibling
// environments. That inference is only sound when every existing mapping names the
// same proxy — a config deliberately bound to different proxies per environment
// records no intent for the unmapped one, so guessing would bind the wrong proxy.
func TestMCPEnvsNeedingActivation_SkipsConfigBoundToMultipleProxies(t *testing.T) {
	configUUID := uuid.New()
	proxyUUID, otherProxyUUID := uuid.New(), uuid.New()
	devEnv, stagingEnv, prodEnv := uuid.New(), uuid.New(), uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: proxyUUID},
		{ConfigUUID: configUUID, EnvironmentUUID: stagingEnv, MCPProxyUUID: otherProxyUUID},
	}
	vars := mcpVarRows(configUUID, devEnv, stagingEnv, prodEnv)

	got := mcpEnvsNeedingActivation(mappings, vars, proxyUUID)

	require.Empty(t, got, "ambiguous proxy intent must not be guessed")
}

// Backfill is driven from one proxy's update, so a config that has no mapping to that
// proxy at all is none of this proxy's business.
func TestMCPEnvsNeedingActivation_SkipsConfigNotBoundToThisProxy(t *testing.T) {
	configUUID := uuid.New()
	proxyUUID, otherProxyUUID := uuid.New(), uuid.New()
	devEnv, prodEnv := uuid.New(), uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: otherProxyUUID},
	}

	got := mcpEnvsNeedingActivation(mappings, mcpVarRows(configUUID, devEnv, prodEnv), proxyUUID)

	require.Empty(t, got)
}

// An environment with no env var rows was never part of the connection's requested
// environment set, so there is no binding intent to restore.
func TestMCPEnvsNeedingActivation_SkipsEnvNeverConfigured(t *testing.T) {
	configUUID, proxyUUID := uuid.New(), uuid.New()
	devEnv := uuid.New()

	mappings := []models.EnvAgentMCPMapping{
		{ConfigUUID: configUUID, EnvironmentUUID: devEnv, MCPProxyUUID: proxyUUID},
	}

	got := mcpEnvsNeedingActivation(mappings, mcpVarRows(configUUID, devEnv), proxyUUID)

	require.Empty(t, got)
}

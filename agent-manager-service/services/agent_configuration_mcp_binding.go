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
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// agentConfigListLimit caps the per-agent configuration listing used when rebuilding or
// auditing an agent's system-managed env vars. An agent has one configuration row per bound
// LLM provider / MCP proxy, so this is far above any real agent.
const agentConfigListLimit = 1000

// mcpEnvsNeedingActivation returns the environments where an agent's MCP connection was
// configured but never bound: env var rows exist (the environment was in the connection's
// requested set, so its URL/API-key variables are injected) while no EnvAgentMCPMapping
// does. That is the state provisionUnconfiguredMCPEnv leaves behind when the proxy has no
// endpoint in the environment yet — the variables are injected empty, and nothing has ever
// filled them in afterwards.
//
// The proxy to bind is inferred from the config's already-mapped environments, so the
// inference is only made when every existing mapping names proxyUUID. A config bound to
// different proxies per environment records no intent for the unmapped ones, and guessing
// there would bind the wrong proxy.
func mcpEnvsNeedingActivation(
	mappings []models.EnvAgentMCPMapping,
	vars []models.AgentEnvConfigVariable,
	proxyUUID uuid.UUID,
) []uuid.UUID {
	mappedEnvs := make(map[uuid.UUID]struct{}, len(mappings))
	boundToThisProxy := false
	for i := range mappings {
		if mappings[i].MCPProxyUUID != proxyUUID {
			return nil
		}
		boundToThisProxy = true
		mappedEnvs[mappings[i].EnvironmentUUID] = struct{}{}
	}
	if !boundToThisProxy {
		return nil
	}

	var unmapped []uuid.UUID
	seen := make(map[uuid.UUID]struct{}, len(vars))
	for i := range vars {
		envUUID := vars[i].EnvironmentUUID
		if _, alreadyMapped := mappedEnvs[envUUID]; alreadyMapped {
			continue
		}
		if _, duplicate := seen[envUUID]; duplicate {
			continue
		}
		seen[envUUID] = struct{}{}
		unmapped = append(unmapped, envUUID)
	}
	return unmapped
}

// ReconcileMCPBindingsForProxy binds agents to proxy in environments that have become
// deployable since the agent's MCP connection was configured — the case where an agent was
// promoted into an environment before the proxy had an endpoint there, leaving its URL and
// API-key variables injected but empty. Adding that endpoint is what triggers this call;
// without it the agent stays broken permanently, because nothing else ever revisits the
// binding.
//
// Best-effort per (config, environment): failures are collected and returned but never
// abort the proxy update that triggered the reconcile.
//
// A connection with no mapping in ANY environment is out of reach here: nothing links it to
// this proxy, since the link is the mapping row itself. That only happens when the proxy had
// no endpoint anywhere at the time the connection was configured; re-saving the connection
// binds it.
func (s *agentConfigurationService) ReconcileMCPBindingsForProxy(ctx context.Context, ouID, proxyHandle string) error {
	if s.envMCPMappingRepo == nil {
		return nil
	}
	// Reloaded rather than taken from the caller so the endpoint→environment rows this
	// reconcile reads are the ones the proxy update just committed.
	proxy, err := s.mcpProxyRepo.GetByHandle(ctx, proxyHandle, ouID)
	if err != nil {
		return fmt.Errorf("failed to load MCP proxy %q for binding reconcile: %w", proxyHandle, err)
	}
	if proxy == nil {
		return nil
	}

	proxyMappings, err := s.envMCPMappingRepo.ListByMCPProxy(ctx, proxy.UUID)
	if err != nil {
		return fmt.Errorf("failed to list agent bindings for MCP proxy %s: %w", proxy.UUID, err)
	}
	if len(proxyMappings) == 0 {
		return nil
	}

	envNameByUUID, err := s.mcpEnvironmentNames(ctx, ouID)
	if err != nil {
		return err
	}

	var errs []error
	for _, configUUID := range distinctConfigUUIDs(proxyMappings) {
		if err := s.reconcileConfigMCPBindings(ctx, ouID, proxy, configUUID, envNameByUUID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *agentConfigurationService) mcpEnvironmentNames(ctx context.Context, ouID string) (map[uuid.UUID]string, error) {
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments for MCP binding reconcile: %w", err)
	}
	names := make(map[uuid.UUID]string, len(envs))
	for _, env := range envs {
		envUUID, parseErr := uuid.Parse(env.UUID)
		if parseErr != nil {
			continue
		}
		names[envUUID] = env.Name
	}
	return names, nil
}

func distinctConfigUUIDs(mappings []models.EnvAgentMCPMapping) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(mappings))
	var configUUIDs []uuid.UUID
	for i := range mappings {
		if _, ok := seen[mappings[i].ConfigUUID]; ok {
			continue
		}
		seen[mappings[i].ConfigUUID] = struct{}{}
		configUUIDs = append(configUUIDs, mappings[i].ConfigUUID)
	}
	return configUUIDs
}

func (s *agentConfigurationService) reconcileConfigMCPBindings(
	ctx context.Context, ouID string, proxy *models.MCPProxy, configUUID uuid.UUID, envNameByUUID map[uuid.UUID]string,
) error {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, ouID)
	if err != nil {
		return fmt.Errorf("failed to load agent configuration %s: %w", configUUID, err)
	}
	if config == nil {
		return nil
	}

	mappings, err := s.envMCPMappingRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("failed to list MCP mappings for config %s: %w", configUUID, err)
	}
	vars, err := s.envVariableRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("failed to list env var rows for config %s: %w", configUUID, err)
	}

	candidates := mcpEnvsNeedingActivation(mappings, vars, proxy.UUID)
	if len(candidates) == 0 {
		return nil
	}

	envTemplates, err := s.reconcileEnvTemplates(ctx, config)
	if err != nil {
		return err
	}
	isExternalAgent, firstEnvName, err := s.agentDeploymentShape(ctx, ouID, config.ProjectName, config.AgentID)
	if err != nil {
		return err
	}

	var errs []error
	for _, envUUID := range candidates {
		envName := envNameByUUID[envUUID]
		if envName == "" {
			continue // environment since deleted
		}
		if !s.mcpEnvDeployable(ctx, proxy, ouID, envUUID) {
			continue
		}
		if err := s.activateMCPMappingForEnv(ctx, config, proxy, envUUID, envName, ouID,
			envTemplates, isExternalAgent, firstEnvName); err != nil {
			errs = append(errs, fmt.Errorf("failed to bind agent %q to MCP proxy in environment %s: %w", config.AgentID, envName, err))
			continue
		}
		s.logger.Info("Backfilled MCP binding for environment that became deployable",
			"agentName", config.AgentID, "configName", config.Name, "environment", envName, "mcpProxyUUID", proxy.UUID)
		// The agent's AgentID token scopes are derived from its MCP mappings, so the
		// binding just created changes them too.
		if err := s.agentIdentityInjection.ReconcileForEnvironment(ctx, ouID, config.ProjectName, config.AgentID, envName); err != nil {
			s.logger.Warn("Failed to refresh agent identity scopes after MCP binding backfill",
				"agentName", config.AgentID, "environment", envName, "error", err)
		}
	}
	return errors.Join(errs...)
}

// reconcileEnvTemplates rebuilds the config's env var templates from the names already
// persisted for it, so a backfill reuses the exact variable names the agent was promoted
// with (including any user overrides) rather than re-deriving defaults from the config name.
func (s *agentConfigurationService) reconcileEnvTemplates(ctx context.Context, config *models.AgentConfiguration) ([]EnvConfigTemplate, error) {
	existingVarNames, err := s.loadExistingVarNames(ctx, config.UUID)
	if err != nil {
		return nil, err
	}
	envTemplates, err := s.buildMCPMappingEnvironmentVariables(config.Name, varNamesToOverrides(existingVarNames))
	if err != nil {
		return nil, errors.Join(utils.ErrInvalidInput, err)
	}
	return envTemplates, nil
}

func (s *agentConfigurationService) agentDeploymentShape(ctx context.Context, ouID, projectName, agentName string) (isExternal bool, firstEnvName string, err error) {
	agentComp, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		return false, "", fmt.Errorf("failed to determine agent type for %s: %w", agentName, err)
	}
	isExternal = agentComp.Provisioning.Type == string(utils.ExternalAgent)
	if isExternal {
		return true, "", nil
	}
	if pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName); pipelineErr == nil && pipeline != nil {
		firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
	}
	return false, firstEnvName, nil
}

// mcpEnvDeployable reports whether proxy can back an agent binding in envUUID: it needs an
// endpoint bound to the environment, a shared gateway artifact owned by that binding, and an
// active gateway. Mirrors the deployability gate in createMCPConfig/updateMCPConfig.
func (s *agentConfigurationService) mcpEnvDeployable(
	ctx context.Context, proxy *models.MCPProxy, ouID string, envUUID uuid.UUID,
) bool {
	endpoint, _ := resolveMCPEndpointForEnv(proxy, envUUID.String())
	if endpoint == nil {
		return false
	}
	sharedArtifactUUID := mcpProxyEnvArtifactUUID(proxy, envUUID.String())
	if sharedArtifactUUID == uuid.Nil {
		return false
	}
	_, err := s.resolveGatewayForMCPArtifact(ctx, sharedArtifactUUID, ouID, envUUID)
	return err == nil
}

// ListUnresolvedMCPBindings returns the names of the agent's MCP connections that are
// configured for environmentName — so their URL and API-key variables are injected into the
// workload there — but resolve to no proxy URL, leaving those variables injected empty. An
// agent in this state starts and runs, but every call it makes through the connection fails.
func (s *agentConfigurationService) ListUnresolvedMCPBindings(
	ctx context.Context, agentID, ouID, projectName, environmentName string,
) (map[string]struct{}, error) {
	env, err := s.ocClient.GetEnvironment(ctx, ouID, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment %q: %w", environmentName, err)
	}
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		return nil, fmt.Errorf("invalid environment UUID %q: %w", env.UUID, err)
	}

	configs, err := s.agentConfigRepo.ListByAgent(ctx, ouID, projectName, agentID, agentConfigListLimit, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent configurations: %w", err)
	}

	unresolved := map[string]struct{}{}
	for i := range configs {
		config := &configs[i]
		if config.TypeID != models.AgentConfigTypeIDMCP {
			continue
		}
		vars, err := s.envVariableRepo.ListByConfigAndEnv(ctx, config.UUID, envUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to list env config variables for config %s: %w", config.UUID, err)
		}
		if len(vars) == 0 {
			continue // not configured for this environment at all
		}
		urlValue, err := s.systemManagedMCPURL(ctx, config, ouID, environmentName, envUUID)
		if err != nil {
			return nil, err
		}
		if urlValue == "" {
			unresolved[config.Name] = struct{}{}
		}
	}
	return unresolved, nil
}

// activateMCPMappingForEnv binds config to sourceProxy in a deployable environment: it
// creates the mapping row, mints the per-agent inbound API key against the proxy's shared
// gateway artifact when the proxy has api-key security enabled, and injects the resulting
// URL / API-key env vars. Nothing is deployed — the proxy already owns the environment's
// single gateway artifact.
//
// The env var rows are ensured rather than inserted outright: an environment that was
// previously unconfigured already has them, persisted blank by provisionUnconfiguredMCPEnv.
// Insert-only would silently no-op on the unique constraint and leave the API-key row
// pointing at no secret, so the secret reference is written explicitly afterwards.
//
// On any failure after the mapping row exists, the partially created binding is torn back
// down so a retry starts clean.
func (s *agentConfigurationService) activateMCPMappingForEnv(
	ctx context.Context,
	config *models.AgentConfiguration,
	sourceProxy *models.MCPProxy,
	envUUID uuid.UUID,
	envName, ouID string,
	envTemplates []EnvConfigTemplate,
	isExternalAgent bool,
	firstEnvName string,
) error {
	// A backfill binds an environment the agent was already promoted into, so its env var
	// rows are already there. Recorded before anything is written so rollback knows not to
	// delete rows this call did not create.
	existingVars, err := s.envVariableRepo.ListByConfigAndEnv(ctx, config.UUID, envUUID)
	if err != nil {
		return fmt.Errorf("failed to read existing MCP environment variables: %w", err)
	}
	envVarsCreatedHere := len(existingVars) == 0

	handle := mcpMappingProxyName(config.ProjectName, config.AgentID, config.Name, envName)
	mapping := &models.EnvAgentMCPMapping{
		ConfigUUID:      config.UUID,
		EnvironmentUUID: envUUID,
		MCPProxyUUID:    sourceProxy.UUID,
		ArtifactUUID:    uuid.New(),
	}
	deployedProxy := buildAgentMCPConfigProxy(config, mapping, sourceProxy, envName, ouID, handle)
	proxyMapping := buildMCPProxyMapping(sourceProxy.UUID, deployedProxy)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.envMCPMappingRepo.Create(ctx, tx, mapping, proxyMapping, handle, handle, mcpProxyArtifactVersion(sourceProxy), ouID)
	}); err != nil {
		return fmt.Errorf("failed to create MCP mapping: %w", err)
	}

	// Must precede credential provisioning: ensureMCPMappingCredentials points the API-key
	// row at the secret it mints, and fails when that row does not exist yet.
	if err := s.ensureMCPEnvVarRows(ctx, config.UUID, envUUID, envTemplates); err != nil {
		s.cleanupNewMCPMapping(ctx, config, mapping, envName, ouID, envVarsCreatedHere)
		return fmt.Errorf("failed to create MCP environment variables: %w", err)
	}

	if mcpProxyAPIKeySecurityEnabled(sourceProxy, envUUID.String()) {
		if _, err := s.ensureMCPMappingCredentials(ctx, config, mapping, envName, ouID); err != nil {
			s.cleanupNewMCPMapping(ctx, config, mapping, envName, ouID, envVarsCreatedHere)
			return err
		}
	} else if err := s.updateMCPMappingSecretReference(ctx, config.UUID, envUUID, ""); err != nil {
		s.cleanupNewMCPMapping(ctx, config, mapping, envName, ouID, envVarsCreatedHere)
		return fmt.Errorf("failed to clear MCP API key secret reference: %w", err)
	}

	if isExternalAgent {
		return nil
	}
	if err := s.injectMCPMappingEnvVars(ctx, config, mapping, sourceProxy, envName, ouID, envTemplates, firstEnvName); err != nil {
		s.logger.Warn("failed to inject MCP mapping env vars", "environment", envName, "err", err)
	}
	return nil
}

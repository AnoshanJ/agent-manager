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
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sync"

	"github.com/google/uuid"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// EnvironmentService defines the interface for environment operations
type EnvironmentService interface {
	CreateEnvironment(ctx context.Context, ouID string, req *models.CreateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error)
	GetEnvironment(ctx context.Context, ouID string, envID string) (*models.GatewayEnvironmentResponse, error)
	ListEnvironments(ctx context.Context, ouID string, limit, offset int32) (*models.EnvironmentListResponse, error)
	UpdateEnvironment(ctx context.Context, ouID string, envID string, req *models.UpdateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error)
	DeleteEnvironment(ctx context.Context, ouID string, envID string) error
	GetEnvironmentGateways(ctx context.Context, ouID string, envID string) ([]models.GatewayResponse, error)
	ListThunderInstances(ctx context.Context, ouID string) (*models.ThunderInstanceListResponse, error)
	// SetThunderSystemClientSecret stores (encrypted) the env-Thunder
	// system-client credential used to reach an environment's Thunder admin API,
	// keyed by ouID (stable, multi-tenant-safe).
	SetThunderSystemClientSecret(ctx context.Context, ouID, envName, clientID, clientSecret string) error
	// DeleteThunderSystemClientSecret removes that credential (idempotent), looked up by ouID.
	DeleteThunderSystemClientSecret(ctx context.Context, ouID, envName string) error
	// SetThunderURL registers an unguessable handle that replaces the predictable
	// <org>-<env> segment of this environment's externally-reachable env-Thunder
	// hostname, and returns the handle that ended up stored. When handle is empty,
	// one is generated automatically. Returns utils.ErrThunderHandleTaken if a
	// caller-supplied handle is already registered to a different environment.
	SetThunderURL(ctx context.Context, ouID, envName, handle string) (string, error)
	// GetThunderURL returns the registered handle for (ouID, envName), or
	// utils.ErrThunderHandleNotFound if none has been registered.
	GetThunderURL(ctx context.Context, ouID, envName string) (string, error)
	// DeleteThunderURL removes the handle (idempotent), freeing it for reuse.
	DeleteThunderURL(ctx context.Context, ouID, envName string) error
}

type environmentService struct {
	logger             *slog.Logger
	ocClient           occlient.OpenChoreoClient
	gatewayRepo        repositories.GatewayRepository
	thunderProber      thundersvc.Prober
	agentConfigService AgentConfigurationService
	envThunderRepo     repositories.EnvThunderSystemClientRepository
	envThunderURLRepo  repositories.EnvThunderURLRepository
	encryptionKey      []byte
}

// NewEnvironmentService creates a new environment service
func NewEnvironmentService(logger *slog.Logger, gatewayRepo repositories.GatewayRepository, ocClient occlient.OpenChoreoClient, thunderProber thundersvc.Prober, agentConfigService AgentConfigurationService, envThunderRepo repositories.EnvThunderSystemClientRepository, envThunderURLRepo repositories.EnvThunderURLRepository, encryptionKey []byte) EnvironmentService {
	return &environmentService{
		logger:             logger,
		gatewayRepo:        gatewayRepo,
		ocClient:           ocClient,
		thunderProber:      thunderProber,
		agentConfigService: agentConfigService,
		envThunderRepo:     envThunderRepo,
		envThunderURLRepo:  envThunderURLRepo,
		encryptionKey:      encryptionKey,
	}
}

func (s *environmentService) CreateEnvironment(ctx context.Context, ouID string, req *models.CreateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Creating environment in OpenChoreo", "name", req.Name, "ouID", ouID)

	if req.DataplaneRef == "" {
		s.logger.Warn("No dataplaneRef provided", "name", req.Name, "ouID", ouID)
	}

	// Reject unsupported isolation tiers up front. An unknown value would otherwise be
	// persisted verbatim on the Environment CR and silently fall back to runc at render
	// time (runtimeClassForIsolationTier returns "" for anything unrecognised), hiding the
	// typo from the user until they notice their agents are not actually isolated.
	switch req.IsolationTier {
	case "", utils.IsolationTierGvisor, utils.IsolationTierKata:
	default:
		return nil, fmt.Errorf("%w: unsupported isolation tier %q (allowed: gvisor, kata)", utils.ErrInvalidInput, req.IsolationTier)
	}

	ocReq := occlient.CreateEnvironmentRequest{
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		IsolationTier: req.IsolationTier,
		DataplaneRef:  req.DataplaneRef,
		IsProduction:  req.IsProduction,
		Gateway:       toOCClientGatewaySpec(req.Gateway),
	}

	env, err := s.ocClient.CreateEnvironment(ctx, ouID, ocReq)
	if err != nil {
		s.logger.Error("Failed to create environment in OpenChoreo", "ouID", ouID, "name", req.Name, "error", err)
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	return &models.GatewayEnvironmentResponse{
		UUID:             env.UUID,
		OrganizationName: ouID,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		Description:      req.Description,
		IsolationTier:    env.IsolationTier,
		DataplaneRef:     env.DataplaneRef,
		IsProduction:     env.IsProduction,
		Gateway:          env.Gateway,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.CreatedAt,
	}, nil
}

func (s *environmentService) GetEnvironment(ctx context.Context, ouID string, envID string) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Getting environment from OpenChoreo", "envID", envID, "ouID", ouID)

	// envID in this context is the environment name (not UUID)
	// since OpenChoreo API uses environment name as identifier
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to get environment from OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		// Check if it's a not-found error
		if errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// Convert OpenChoreo EnvironmentResponse to GatewayEnvironmentResponse
	return &models.GatewayEnvironmentResponse{
		UUID:             env.UUID,
		OrganizationName: ouID,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		Description:      env.Description,
		IsolationTier:    env.IsolationTier,
		DataplaneRef:     env.DataplaneRef,
		DNSPrefix:        env.DNSPrefix,
		IsProduction:     env.IsProduction,
		Gateway:          env.Gateway,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.CreatedAt,
	}, nil
}

func (s *environmentService) ListEnvironments(ctx context.Context, ouID string, limit, offset int32) (*models.EnvironmentListResponse, error) {
	s.logger.Info("Listing environments from OpenChoreo", "ouID", ouID, "limit", limit, "offset", offset)

	// Fetch environments directly from OpenChoreo
	ocEnvironments, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to list environments from OpenChoreo", "ouID", ouID, "error", err)
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	total := int32(len(ocEnvironments))

	// Apply pagination
	start := int(offset)
	end := start + int(limit)

	if start >= len(ocEnvironments) {
		// Offset is beyond available data
		return &models.EnvironmentListResponse{
			Environments: []models.GatewayEnvironmentResponse{},
			Total:        total,
			Limit:        limit,
			Offset:       offset,
		}, nil
	}

	if end > len(ocEnvironments) {
		end = len(ocEnvironments)
	}

	paginatedEnvs := ocEnvironments[start:end]

	// Convert OpenChoreo environment responses to gateway environment responses
	responses := make([]models.GatewayEnvironmentResponse, len(paginatedEnvs))
	for i, env := range paginatedEnvs {
		responses[i] = models.GatewayEnvironmentResponse{
			UUID:             env.UUID,
			OrganizationName: ouID,
			Name:             env.Name,
			DisplayName:      env.DisplayName,
			Description:      env.Description,
			IsolationTier:    env.IsolationTier,
			DataplaneRef:     env.DataplaneRef,
			DNSPrefix:        env.DNSPrefix,
			IsProduction:     env.IsProduction,
			Gateway:          env.Gateway,
			CreatedAt:        env.CreatedAt,
			UpdatedAt:        env.CreatedAt,
		}
	}

	return &models.EnvironmentListResponse{
		Environments: responses,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
	}, nil
}

func (s *environmentService) UpdateEnvironment(ctx context.Context, ouID string, envID string, req *models.UpdateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Updating environment in OpenChoreo", "envID", envID, "ouID", ouID)

	ocReq := occlient.UpdateEnvironmentRequest{
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		IsProduction: req.IsProduction,
		Gateway:      toOCClientGatewaySpec(req.Gateway),
	}

	env, err := s.ocClient.UpdateEnvironment(ctx, ouID, envID, ocReq)
	if err != nil {
		s.logger.Error("Failed to update environment in OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to update environment: %w", err)
	}

	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	return &models.GatewayEnvironmentResponse{
		UUID:             env.UUID,
		OrganizationName: ouID,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		Description:      description,
		DataplaneRef:     env.DataplaneRef,
		IsProduction:     env.IsProduction,
		Gateway:          env.Gateway,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.CreatedAt,
	}, nil
}

// DeleteEnvironment removes an environment from OpenChoreo and cleans up local DB state.
//
// OpenChoreo is the source of truth for "is anything actually deployed here": it refuses
// Environment deletion while ReleaseBindings or workloads still reference it, so we let
// the OC API server enforce that and surface whatever it returns.
//
// On success: delete the OpenChoreo Environment CR, then delete any gateway↔env mapping rows
func (s *environmentService) DeleteEnvironment(ctx context.Context, ouID string, envID string) error {
	s.logger.Info("Deleting environment", "envID", envID, "ouID", ouID)

	// envID is the environment name (matching OpenChoreo's identifier); resolve the UUID via OC
	// because the local DB doesn't have its own environments table.
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to look up environment", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			return utils.ErrEnvironmentNotFound
		}
		return fmt.Errorf("failed to look up environment: %w", err)
	}

	envUUID, parseErr := uuid.Parse(env.UUID)
	if parseErr != nil {
		s.logger.Error("Invalid env UUID from OpenChoreo", "uuid", env.UUID, "error", parseErr)
		return fmt.Errorf("invalid environment UUID: %w", parseErr)
	}

	// Block deletion if any deployment pipeline still references this environment in its
	// promotion paths (either as a source or a target environment).
	pipelines, err := s.ocClient.ListDeploymentPipelines(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to list deployment pipelines while checking environment references", "ouID", ouID, "envID", envID, "error", err)
		return fmt.Errorf("failed to verify environment references: %w", err)
	}
	var referencingPipelines []string
	for _, pipeline := range pipelines {
		if pipeline != nil && pipelineReferencesEnvironment(pipeline, env.Name) {
			referencingPipelines = append(referencingPipelines, pipeline.Name)
		}
	}
	if len(referencingPipelines) > 0 {
		s.logger.Warn("Cannot delete environment referenced by deployment pipelines", "ouID", ouID, "envID", envID, "pipelines", referencingPipelines)
		return fmt.Errorf("%w: %v", utils.ErrEnvironmentInUse, referencingPipelines)
	}

	// Delete in OpenChoreo first. If OC refuses (release bindings still exist, etc.) we surface
	// that error without having touched local state. A not-found from OC after UUID resolution
	// is treated as idempotent so we still clean up local gateway↔env mappings.
	if err := s.ocClient.DeleteEnvironment(ctx, ouID, env.Name); err != nil {
		s.logger.Error("Failed to delete environment in OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			s.logger.Warn("Environment already absent in OpenChoreo; continuing local cleanup",
				"ouID", ouID, "envID", envID, "envUUID", envUUID)
		} else {
			return fmt.Errorf("failed to delete environment in OpenChoreo: %w", err)
		}
	}

	// Cascade MCP-proxy cleanup: tear down every agent-scoped MCP mapping deployed into this
	// env and strip the env block from every org-level MCP proxy blueprint. Best-effort — the
	// environment is already gone from OpenChoreo, so cleanup errors are logged, not fatal.
	if s.agentConfigService != nil {
		if err := s.agentConfigService.CleanupEnvironmentMCPArtifacts(ctx, ouID, envUUID, env.Name); err != nil {
			s.logger.Warn("environment deleted; MCP artifact cleanup had errors",
				"ouID", ouID, "envID", envID, "envUUID", envUUID, "error", err)
		}
	}

	// Local cleanup: gateway↔env mapping rows. The gateway themselves are unaffected.
	deleted, err := s.gatewayRepo.DeleteEnvironmentMappingsByEnvironmentID(envUUID.String())
	if err != nil {
		s.logger.Error("Environment deleted in OpenChoreo but local gateway-mapping cleanup failed",
			"envUUID", envUUID, "error", err)
		return fmt.Errorf("environment deleted but gateway mapping cleanup failed: %w", err)
	}
	s.logger.Info("Deleted environment", "envID", envID, "envUUID", envUUID, "gatewayMappingsDeleted", deleted)
	return nil
}

func (s *environmentService) GetEnvironmentGateways(ctx context.Context, ouID string, envID string) ([]models.GatewayResponse, error) {
	s.logger.Info("Getting environment gateways", "envID", envID, "ouID", ouID)

	// Verify environment exists in OpenChoreo (envID is environment name)
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to get environment from OpenChoreo", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to verify environment: %w", err)
	}

	// Parse environment UUID
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		s.logger.Error("Failed to parse environment UUID", "uuid", env.UUID, "error", err)
		return nil, fmt.Errorf("invalid environment UUID: %w", err)
	}

	// Get gateway-environment mappings from repository
	mappings, err := s.gatewayRepo.GetEnvironmentMappingsByEnvironmentID(envUUID.String())
	if err != nil {
		s.logger.Error("Failed to get gateway mappings from repository", "environmentID", envUUID.String(), "error", err)
		return nil, fmt.Errorf("failed to get gateway mappings: %w", err)
	}

	// Fetch each gateway from the gateway repository
	responses := make([]models.GatewayResponse, 0, len(mappings))
	for _, mapping := range mappings {
		gatewayID := mapping.GatewayUUID.String()

		// Get gateway details from repository
		gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
		if err != nil {
			s.logger.Warn("Failed to get gateway from repository", "gatewayID", gatewayID, "error", err)
			continue
		}
		if gateway == nil {
			s.logger.Warn("Gateway not found", "gatewayID", gatewayID)
			continue
		}

		// Convert gateway model to response
		status := string(models.GatewayStatusInactive)
		if gateway.IsActive {
			status = string(models.GatewayStatusActive)
		}

		responses = append(responses, models.GatewayResponse{
			UUID:             gateway.UUID.String(),
			OrganizationName: ouID,
			Name:             gateway.Name,
			DisplayName:      gateway.DisplayName,
			GatewayType:      gateway.GatewayFunctionalityType,
			VHost:            gateway.Vhost,
			IsCritical:       gateway.IsCritical,
			Status:           status,
		})
	}

	return responses, nil
}

// pipelineReferencesEnvironment reports whether any promotion path in the pipeline
// references the given environment name, either as the source or as a target.
func pipelineReferencesEnvironment(pipeline *models.DeploymentPipelineResponse, envName string) bool {
	for _, path := range pipeline.PromotionPaths {
		if path.SourceEnvironmentRef == envName {
			return true
		}
		for _, target := range path.TargetEnvironmentRefs {
			if target.Name == envName {
				return true
			}
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Gateway spec translation: models.GatewaySpec (internal DTO) → occlient
// GatewaySpec (OC-bound request type). Direct field-by-field copy; the OC
// client layer constructs the runtime-only fields (gateway resource name/
// namespace, listener name).
// -----------------------------------------------------------------------------

func toOCClientGatewaySpec(g *models.GatewaySpec) *occlient.GatewaySpec {
	if g == nil {
		return nil
	}
	return &occlient.GatewaySpec{
		Ingress: toOCClientGatewayNetworkSpec(g.Ingress),
		Egress:  toOCClientGatewayNetworkSpec(g.Egress),
	}
}

func toOCClientGatewayNetworkSpec(n *models.GatewayNetworkSpec) *occlient.GatewayNetworkSpec {
	if n == nil {
		return nil
	}
	return &occlient.GatewayNetworkSpec{
		External: toOCClientGatewayEndpointSpec(n.External),
		Internal: toOCClientGatewayEndpointSpec(n.Internal),
	}
}

func toOCClientGatewayEndpointSpec(e *models.GatewayEndpointSpec) *occlient.GatewayEndpointSpec {
	if e == nil {
		return nil
	}
	return &occlient.GatewayEndpointSpec{
		HTTP:  toOCClientGatewayListenerSpec(e.HTTP),
		HTTPS: toOCClientGatewayListenerSpec(e.HTTPS),
		TLS:   toOCClientGatewayListenerSpec(e.TLS),
	}
}

func toOCClientGatewayListenerSpec(l *models.GatewayListenerSpec) *occlient.GatewayListenerSpec {
	if l == nil {
		return nil
	}
	return &occlient.GatewayListenerSpec{Port: l.Port, Host: l.Host}
}

// ListThunderInstances returns the Thunder OAuth2 identity provider info for every
// environment in the org whose env-Thunder instance is reachable.
//
// Reachability is determined by a live HTTP probe to the environment's JWKS endpoint,
// NOT by inferring from gateway mappings. Gateway mappings only prove a gateway was
// provisioned — not that Thunder was ever deployed (e.g. PROVISION_THUNDER=false or a
// failed provision both leave mappings behind with no Thunder instance). Advertising
// dead endpoints would cause the console Identity page to show broken issuer/token/JWKS
// URLs.
//
// maxThunderProbeConcurrency bounds how many environments' handle-read+probe run at
// once, so an org with hundreds of environments doesn't fire hundreds of simultaneous
// outbound HTTP probes (each itself fanning out to multiple candidate URLs — see
// thunderBaseURLCandidates).
const maxThunderProbeConcurrency = 10

func (s *environmentService) ListThunderInstances(ctx context.Context, ouID string) (*models.ThunderInstanceListResponse, error) {
	envs, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("list environments for org %s: %w", ouID, err)
	}

	// Thunder naming (release name, namespace, host, issuer URL) is keyed by the
	// org NAME the provisioning scripts deployed with (ORG_NAME, e.g. "default"),
	// which is the OpenChoreo namespace — not the OU id from the JWT. Probing or
	// advertising URLs derived from the OU id would point at instances that don't
	// exist.
	orgNamespace, err := ResolveNamespace(ctx, s.ocClient)
	if err != nil {
		return nil, err
	}

	// An environment with no registered handle has no address to probe, so it's
	// skipped without even trying. Probing is fanned out across goroutines (bounded
	// by maxThunderProbeConcurrency) rather than run sequentially, since each probe
	// can take a couple of seconds in the worst case.
	reachable := make([]bool, len(envs))
	handles := make([]string, len(envs))
	sem := make(chan struct{}, maxThunderProbeConcurrency)
	var wg sync.WaitGroup
	for i, env := range envs {
		if env == nil || env.Name == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, envName string) {
			defer wg.Done()
			defer func() { <-sem }()
			handle := s.readThunderHandle(ctx, ouID, envName)
			handles[idx] = handle
			if handle == "" {
				return // not provisioned — reachable[idx] stays false, no probe needed
			}
			reachable[idx] = s.thunderProber.Probe(ctx, orgNamespace, envName, handle)
		}(i, env.Name)
	}
	wg.Wait()

	instances := make([]models.ThunderInstanceResponse, 0, len(envs))
	for i, env := range envs {
		if env == nil || env.Name == "" {
			continue
		}

		// Reachability is the only reliable signal: environments created with PROVISION_THUNDER=false,
		// environments whose provisioning failed silently (non-fatal by design), and
		// environments with no registered handle all pass the gateway-mappings check but
		// have no Thunder instance.
		if !reachable[i] {
			s.logger.Debug("env-Thunder not reachable, skipping", "envName", env.Name)
			continue
		}

		// All three URLs must be externally reachable — this response is developer-facing
		// (console Identity page, copy-buttons). The internal svc.cluster.local addresses
		// only resolve inside the cluster and would cause DNS failures for developers.
		instances = append(instances, models.ThunderInstanceResponse{
			EnvName:      env.Name,
			DisplayName:  env.DisplayName,
			IsProduction: env.IsProduction,
			IssuerURL:    thundersvc.ThunderIssuerURL(handles[i]),
			TokenURL:     thundersvc.ThunderExternalTokenURL(handles[i]),
			JWKSURL:      thundersvc.ThunderExternalJWKSURL(handles[i]),
			Namespace:    thundersvc.ThunderNamespace(orgNamespace, env.Name),
		})
	}
	return &models.ThunderInstanceListResponse{ThunderInstances: instances}, nil
}

// readThunderHandle looks up envName's env-Thunder URL handle via
// ResolveThunderHandle. A read failure and "genuinely never provisioned" both
// collapse to "" here — callers (ListThunderInstances) treat that as the
// not-provisioned signal, not as an error to propagate from a transient hiccup.
func (s *environmentService) readThunderHandle(ctx context.Context, ouID, envName string) string {
	handle, err := ResolveThunderHandle(ctx, s.envThunderURLRepo, s.envThunderRepo, ouID, envName)
	if err != nil {
		s.logger.Error("failed to resolve env-thunder url handle", "ouID", ouID, "envName", envName, "error", err)
		return ""
	}
	return handle
}

// thunderHandlePattern matches a DNS-label-safe handle: lowercase alphanumeric,
// hyphens allowed but not leading/trailing — the same shape as ENV_NAME's own
// validation (deployments/scripts/add-environment.sh), so it's always a valid
// single DNS label once combined into "<handle>.<baseDomain>".
var thunderHandlePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// maxThunderHandleLen mirrors the 63-character DNS label limit any hostname
// built from the handle is subject to (see thundersvc.ThunderIssuerURL, which
// embeds the handle as-is into "<handle>.<baseDomain>").
const maxThunderHandleLen = 63

// minThunderHandleLen matches generatedThunderHandleLen — a caller-supplied
// handle shorter than what AMS itself would generate is trivially brute-forceable
// (e.g. a single character has only 36 possible values) and defeats the whole
// point of the feature, so it's a hard floor, not just UI advice. Genuinely
// guessable-but-long values ("productionenvironment") aren't something a format
// rule can catch — the console warns (non-blocking) against a common-word
// blocklist instead; see CreateEnvironmentDrawer.tsx's GUESSABLE_HANDLE_WORDS.
const minThunderHandleLen = generatedThunderHandleLen

// reservedThunderHandles blocks labels that would be confusing or misleading as
// a standalone hostname segment, as defense in depth beyond the format/length
// checks. Kept short and deliberately not the console's full blocklist
// (CreateEnvironmentDrawer.tsx's GUESSABLE_HANDLE_WORDS) — the API only
// hard-rejects names that are actively wrong/misleading as a hostname
// component; "guessable but not wrong" is a judgment call left to the
// non-blocking UI warning instead.
//
// Every entry here must be >= minThunderHandleLen characters, or it can never
// actually be submitted as a handle and the check is dead code (the platform's
// own fixed subdomains — console, api, thunder, observer, gateway, cp, agents —
// are all short enough that this length floor alone already keeps a handle
// from ever colliding with one of them; they don't need their own entries).
var reservedThunderHandles = map[string]bool{
	"kubernetes": true,
}

// validateThunderHandle checks handle's format. It does not check uniqueness —
// that's enforced atomically by the DB's unique constraint on Upsert.
func validateThunderHandle(handle string) error {
	if handle == "" {
		return fmt.Errorf("%w: handle is required", utils.ErrInvalidThunderHandle)
	}
	if len(handle) < minThunderHandleLen {
		return fmt.Errorf("%w: handle must be at least %d characters", utils.ErrInvalidThunderHandle, minThunderHandleLen)
	}
	if len(handle) > maxThunderHandleLen {
		return fmt.Errorf("%w: handle exceeds %d characters", utils.ErrInvalidThunderHandle, maxThunderHandleLen)
	}
	if !thunderHandlePattern.MatchString(handle) {
		return fmt.Errorf("%w: must be lowercase alphanumeric with hyphens, no leading/trailing hyphen", utils.ErrInvalidThunderHandle)
	}
	if reservedThunderHandles[handle] {
		return fmt.Errorf("%w: %q is a reserved word", utils.ErrInvalidThunderHandle, handle)
	}
	return nil
}

// generatedThunderHandleLen matches the task's own spec: an auto-generated handle
// is exactly 10 characters.
const generatedThunderHandleLen = 10

// thunderHandleAlphabet is the full lowercase-alphanumeric set (36 symbols) —
// wider than hex (16 symbols) for the same 10-character length, raising a
// generated handle's guess-space from 16^10 (40 bits) to 36^10 (~52 bits) without
// changing its length. Every symbol is already valid against thunderHandlePattern,
// so no post-generation validation is needed.
const thunderHandleAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// maxGenerateThunderHandleAttempts bounds the collision-retry loop in SetThunderURL.
// A collision is astronomically unlikely (36^10 ≈ 3.66 quadrillion possible
// values), so this only guards against a pathological run of bad luck — not a
// realistic operating condition.
const maxGenerateThunderHandleAttempts = 5

// generateThunderHandle returns a random, generatedThunderHandleLen-character
// handle drawn from thunderHandleAlphabet. Uses rand.Read + modulo rather than
// rejection sampling: 256 % 36 == 4 introduces a small bias toward the
// alphabet's first 4 symbols, but the resulting guess-space is still ~52 bits
// (36^10, see thunderHandleAlphabet's doc comment) — this is an
// unguessable-URL-segment generator, not a cryptographic key, so that bias
// isn't worth the extra complexity of unbiased sampling to close.
func generateThunderHandle() (string, error) {
	buf := make([]byte, generatedThunderHandleLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate thunder url handle: %w", err)
	}
	out := make([]byte, generatedThunderHandleLen)
	for i, b := range buf {
		out[i] = thunderHandleAlphabet[int(b)%len(thunderHandleAlphabet)]
	}
	return string(out), nil
}

// SetThunderURL registers handle for (ouID, envName) and returns the handle
// that actually ended up stored.
//
// If handle is empty, one is generated (see generateThunderHandle) — every
// environment gets a handle at provisioning time, whether the caller supplied
// one or not. There is no fallback to a pattern derived from org/env; a
// missing handle means "not provisioned" everywhere else in this codebase
// (ListThunderInstances, EnvThunderResolver), not "compute one on demand".
//
// Idempotent, and never changes an already-registered handle: Thunder's
// issuer is immutable once minted (baked into the running instance's
// Helm-installed publicUrl/jwt.issuer and its HTTPRoute hostname). A
// blank-handle call against an environment that already has one reuses that
// SAME handle rather than generating a new one; an explicit different handle
// against an already-registered environment is rejected (utils.ErrThunderHandleTaken,
// 409) rather than applied — a caller that genuinely wants to change it must
// call DeleteThunderURL first. The "already registered" check goes through
// ResolveThunderHandle, so an environment grandfathered in from before this
// feature existed is honored here too, not just on reads.
//
// Safe under concurrent first-time provisioning: the up-front
// ResolveThunderHandle read is only a fast path for the common case — it does
// not by itself stop two concurrent callers from both seeing no row (a TOCTOU
// window). The actual guarantee comes from claimThunderHandle's use of
// EnvThunderURLRepository.Insert, an insert-only write: whichever request's
// INSERT commits first in Postgres wins, and the loser discovers that via the
// database's own unique-constraint check — not a separate read-then-write
// step — and adopts the winner's value instead of overwriting it.
func (s *environmentService) SetThunderURL(ctx context.Context, ouID, envName, handle string) (string, error) {
	if ouID == "" {
		return "", fmt.Errorf("%w: ouID is required", utils.ErrInvalidInput)
	}
	if envName == "" {
		return "", fmt.Errorf("%w: envName is required", utils.ErrInvalidInput)
	}

	existing, err := ResolveThunderHandle(ctx, s.envThunderURLRepo, s.envThunderRepo, ouID, envName)
	if err != nil {
		return "", fmt.Errorf("failed to check for an existing thunder url handle for %s/%s: %w", ouID, envName, err)
	}
	if existing != "" {
		return s.reuseOrRejectThunderHandle(existing, handle, ouID, envName)
	}

	if handle != "" {
		if err := validateThunderHandle(handle); err != nil {
			return "", err
		}
		return s.claimThunderHandle(ctx, ouID, envName, handle, false)
	}

	for attempt := 1; attempt <= maxGenerateThunderHandleAttempts; attempt++ {
		generated, err := generateThunderHandle()
		if err != nil {
			return "", err
		}
		resolved, err := s.claimThunderHandle(ctx, ouID, envName, generated, true)
		if err == nil {
			return resolved, nil
		}
		if errors.Is(err, utils.ErrThunderHandleTaken) {
			// The GENERATED value itself coincidentally collided with a
			// DIFFERENT environment's handle — not a race for THIS
			// environment (claimThunderHandle already resolves that case
			// internally without returning an error to a blank-handle
			// caller). Try a fresh value.
			s.logger.Debug("generated thunder url handle collided, retrying", "ouID", ouID, "envName", envName, "attempt", attempt)
			continue
		}
		return "", err
	}
	return "", fmt.Errorf("failed to generate a unique thunder url handle for %s/%s after %d attempts", ouID, envName, maxGenerateThunderHandleAttempts)
}

// reuseOrRejectThunderHandle applies SetThunderURL's idempotency rule once a
// registered handle is known — whether seen via an up-front read, or
// discovered by losing an insert race (see claimThunderHandle): a blank or
// matching request reuses it as a no-op; a different EXPLICIT request is
// rejected, since Thunder's issuer is never silently changed once minted.
func (s *environmentService) reuseOrRejectThunderHandle(existing, requested, ouID, envName string) (string, error) {
	if requested == "" || requested == existing {
		s.logger.Info("Reusing already-registered env-thunder url handle", "ouID", ouID, "envName", envName)
		return existing, nil
	}
	return "", fmt.Errorf("%w: %s/%s already has a registered handle %q — call DeleteThunderURL first to change it",
		utils.ErrThunderHandleTaken, ouID, envName, existing)
}

// claimThunderHandle attempts to insert-claim handle for (ouID, envName) via
// EnvThunderURLRepository.Insert (see its doc comment for why this must be
// insert-only, never an upsert). If a concurrent request already won the
// SAME (ouID, envName) race first, it reads back the winning row:
//   - generated == true (the auto-generate path never asked for a SPECIFIC
//     value) always adopts the winner's handle — there's nothing to reject it
//     against.
//   - generated == false (an explicit caller-supplied handle) applies the
//     SAME reuse-or-reject rule a pre-existing row would get, so the outcome
//     never depends on which side of the race a given caller happened to land
//     on.
func (s *environmentService) claimThunderHandle(ctx context.Context, ouID, envName, handle string, generated bool) (string, error) {
	rec := &models.EnvThunderURL{OUID: ouID, EnvName: envName, ThunderHandle: handle}
	err := s.envThunderURLRepo.Insert(ctx, rec)
	if err == nil {
		if generated {
			s.logger.Info("Generated and stored env-thunder url handle", "ouID", ouID, "envName", envName)
		} else {
			s.logger.Info("Stored env-thunder url handle", "ouID", ouID, "envName", envName)
		}
		return handle, nil
	}
	if errors.Is(err, utils.ErrEnvThunderURLAlreadyClaimed) {
		winner, getErr := s.envThunderURLRepo.Get(ctx, ouID, envName)
		if getErr != nil {
			return "", fmt.Errorf("failed to read the winning thunder url handle for %s/%s after a claim race: %w", ouID, envName, getErr)
		}
		if generated {
			s.logger.Info("Lost a concurrent first-provisioning race; adopting the winning handle", "ouID", ouID, "envName", envName)
			return winner.ThunderHandle, nil
		}
		return s.reuseOrRejectThunderHandle(winner.ThunderHandle, handle, ouID, envName)
	}
	if errors.Is(err, utils.ErrThunderHandleTaken) {
		return "", err
	}
	return "", fmt.Errorf("failed to store thunder url handle for %s/%s: %w", ouID, envName, err)
}

// GetThunderURL returns the registered handle for (ouID, envName) — resolved
// via ResolveThunderHandle, so a pre-existing environment with no row yet but
// an existing system-client credential is grandfathered in rather than reported
// as not-provisioned — or utils.ErrThunderHandleNotFound if genuinely none has
// ever been registered.
func (s *environmentService) GetThunderURL(ctx context.Context, ouID, envName string) (string, error) {
	if ouID == "" {
		return "", fmt.Errorf("%w: ouID is required", utils.ErrInvalidInput)
	}
	handle, err := ResolveThunderHandle(ctx, s.envThunderURLRepo, s.envThunderRepo, ouID, envName)
	if err != nil {
		return "", fmt.Errorf("failed to read thunder url handle for %s/%s: %w", ouID, envName, err)
	}
	if handle == "" {
		return "", utils.ErrThunderHandleNotFound
	}
	return handle, nil
}

// DeleteThunderURL removes the handle for (ouID, envName). Idempotent — deleting a
// non-existent row is not an error. Freeing the handle lets a different environment
// (or a retry with a different handle for this same one) claim it.
func (s *environmentService) DeleteThunderURL(ctx context.Context, ouID, envName string) error {
	if ouID == "" {
		return fmt.Errorf("%w: ouID is required", utils.ErrInvalidInput)
	}
	if err := s.envThunderURLRepo.Delete(ctx, ouID, envName); err != nil {
		return fmt.Errorf("failed to delete thunder url handle for %s/%s: %w", ouID, envName, err)
	}
	s.logger.Info("Deleted env-thunder url handle", "ouID", ouID, "envName", envName)
	return nil
}

// SetThunderSystemClientSecret encrypts and upserts the env-Thunder system-client
// credential, keyed by ouID.
func (s *environmentService) SetThunderSystemClientSecret(ctx context.Context, ouID, envName, clientID, clientSecret string) error {
	if clientSecret == "" {
		return fmt.Errorf("%w: clientSecret is required", utils.ErrInvalidInput)
	}
	if ouID == "" {
		return fmt.Errorf("%w: ouID is required", utils.ErrInvalidInput)
	}
	encrypted, err := utils.EncryptBytes([]byte(clientSecret), s.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt env-thunder system-client secret for %s/%s: %w", ouID, envName, err)
	}
	cred := &models.EnvThunderSystemClient{
		OUID:                  ouID,
		EnvName:               envName,
		ClientID:              clientID,
		ClientSecretEncrypted: encrypted,
	}
	if err := s.envThunderRepo.Upsert(ctx, cred); err != nil {
		return fmt.Errorf("failed to store env-thunder system-client secret for %s/%s: %w", ouID, envName, err)
	}
	s.logger.Info("Stored env-thunder system-client secret", "ouID", ouID, "envName", envName)
	return nil
}

// DeleteThunderSystemClientSecret removes the credential for (ouID, envName).
// Idempotent — deleting a non-existent row is not an error.
func (s *environmentService) DeleteThunderSystemClientSecret(ctx context.Context, ouID, envName string) error {
	if ouID == "" {
		return fmt.Errorf("%w: ouID is required", utils.ErrInvalidInput)
	}
	if err := s.envThunderRepo.Delete(ctx, ouID, envName); err != nil {
		return fmt.Errorf("failed to delete env-thunder system-client secret for %s/%s: %w", ouID, envName, err)
	}
	s.logger.Info("Deleted env-thunder system-client secret", "ouID", ouID, "envName", envName)
	return nil
}

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
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	apiVersionLLMProxy = "gateway.api-platform.wso2.com/v1"
	kindLLMProxy       = "LlmProxy"

	// gatewayDefaultRequestTimeout and gatewayDefaultIdleTimeout mirror the gateway's own
	// RouteTimeoutMs/RouteIdleTimeoutMs defaults, applied when the provider being fronted
	// has no explicit resilience config of its own.
	gatewayDefaultRequestTimeout = "60s"
	gatewayDefaultIdleTimeout    = "300s"
)

// LLMProxyDeploymentService handles LLM proxy deployment business logic
type LLMProxyDeploymentService struct {
	deploymentRepo       repositories.DeploymentRepository
	proxyRepo            repositories.LLMProxyRepository
	providerRepo         repositories.LLMProviderRepository
	gatewayRepo          repositories.GatewayRepository
	gatewayEventsService *GatewayEventsService
}

// NewLLMProxyDeploymentService creates a new LLM proxy deployment service
func NewLLMProxyDeploymentService(
	deploymentRepo repositories.DeploymentRepository,
	proxyRepo repositories.LLMProxyRepository,
	providerRepo repositories.LLMProviderRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayEventsService *GatewayEventsService,
) *LLMProxyDeploymentService {
	return &LLMProxyDeploymentService{
		deploymentRepo:       deploymentRepo,
		proxyRepo:            proxyRepo,
		providerRepo:         providerRepo,
		gatewayRepo:          gatewayRepo,
		gatewayEventsService: gatewayEventsService,
	}
}

// doubleDuration parses a gateway duration string (e.g. "15s", "500ms") and returns it
// doubled in the same style it was written in (integer seconds stay integer seconds).
// Returns the input unchanged if it can't be parsed.
func doubleDuration(d string) string {
	parsed, err := time.ParseDuration(d)
	if err != nil {
		return d
	}
	doubled := parsed * 2
	if doubled%time.Second == 0 {
		return fmt.Sprintf("%ds", int64(doubled/time.Second))
	}
	return doubled.String()
}

// resilienceWithLLMProxyDefaults derives the LLM proxy's resilience from the provider it
// fronts, doubling each of the provider's timeout/idleTimeout (or the gateway's own default,
// if the provider has none set). A proxy call fully contains the provider call inside it, so
// its timeout must never fire first — matching the provider's budget 1:1 risks the outer
// (proxy) timeout winning the race and returning a spurious timeout for a request the
// provider would have completed. Any resilience value explicitly set on the proxy itself
// takes precedence and is left untouched.
func resilienceWithLLMProxyDefaults(proxyResilience, providerResilience *models.Resilience) *models.Resilience {
	providerTimeout := gatewayDefaultRequestTimeout
	providerIdleTimeout := gatewayDefaultIdleTimeout
	if providerResilience != nil {
		if providerResilience.Timeout != nil {
			providerTimeout = *providerResilience.Timeout
		}
		if providerResilience.IdleTimeout != nil {
			providerIdleTimeout = *providerResilience.IdleTimeout
		}
	}

	timeout := doubleDuration(providerTimeout)
	idleTimeout := doubleDuration(providerIdleTimeout)
	if proxyResilience != nil {
		if proxyResilience.Timeout != nil {
			timeout = *proxyResilience.Timeout
		}
		if proxyResilience.IdleTimeout != nil {
			idleTimeout = *proxyResilience.IdleTimeout
		}
	}
	return &models.Resilience{Timeout: &timeout, IdleTimeout: &idleTimeout}
}

// LLMProxyDeploymentYAML represents the deployment YAML
type LLMProxyDeploymentYAML struct {
	ApiVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   DeploymentMetadata     `yaml:"metadata" json:"metadata"`
	Spec       LLMProxyDeploymentSpec `yaml:"spec" json:"spec"`
}

// LLMProxyDeploymentSpec represents the spec section
type LLMProxyDeploymentSpec struct {
	DisplayName string                     `yaml:"displayName" json:"displayName"`
	Version     string                     `yaml:"version" json:"version"`
	Context     string                     `yaml:"context,omitempty" json:"context,omitempty"`
	VHost       string                     `yaml:"vhost,omitempty" json:"vhost,omitempty"`
	Provider    LLMProxyDeploymentProvider `yaml:"provider" json:"provider"`
	Resilience  *models.Resilience         `yaml:"resilience,omitempty" json:"resilience,omitempty"`
	Policies    []models.LLMPolicy         `yaml:"policies,omitempty" json:"policies,omitempty"`
	Security    *models.SecurityConfig     `yaml:"security,omitempty" json:"security,omitempty"`
}

// LLMProxyDeploymentProvider represents the provider configuration in the spec
type LLMProxyDeploymentProvider struct {
	ID   string               `yaml:"id" json:"id"`
	Auth *models.UpstreamAuth `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// DeployLLMProxy deploys an LLM proxy to a gateway
func (s *LLMProxyDeploymentService) DeployLLMProxy(proxyID string, req *models.DeployAPIRequest, ouID string) (*models.Deployment, error) {
	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: starting", "proxyID", proxyID, "ouID", ouID,
		"deploymentName", req.Name, "base", req.Base, "gatewayID", req.GatewayID)

	if req.Base == "" {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: base is required", "proxyID", proxyID)
		return nil, utils.ErrDeploymentBaseRequired
	}
	if req.GatewayID == "" {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: gateway ID is required", "proxyID", proxyID)
		return nil, utils.ErrDeploymentGatewayIDRequired
	}
	if req.Name == "" {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: deployment name is required", "proxyID", proxyID)
		return nil, utils.ErrDeploymentNameRequired
	}

	// Parse UUIDs
	gatewayUUID, err := uuid.Parse(req.GatewayID)
	if err != nil {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: invalid gateway UUID", "proxyID", proxyID, "gatewayID", req.GatewayID, "error", err)
		return nil, fmt.Errorf("invalid gateway UUID: %w", err)
	}

	// Validate gateway exists
	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: validating gateway", "proxyID", proxyID, "gatewayID", req.GatewayID)
	gateway, err := s.gatewayRepo.GetByUUID(req.GatewayID)
	if err != nil {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: failed to get gateway", "proxyID", proxyID, "gatewayID", req.GatewayID, "error", err)
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}
	if gateway == nil || gateway.OUID != ouID {
		slog.Warn("LLMProxyDeploymentService.DeployLLMProxy: gateway not found or org mismatch", "proxyID", proxyID, "gatewayID", req.GatewayID, "ouID", ouID)
		return nil, utils.ErrGatewayNotFound
	}

	// Get LLM proxy
	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: getting proxy", "proxyID", proxyID, "ouID", ouID)
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProxyDeploymentService.DeployLLMProxy: proxy not found", "proxyID", proxyID, "ouID", ouID)
			return nil, utils.ErrLLMProxyNotFound
		}
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: failed to get proxy", "proxyID", proxyID, "ouID", ouID, "error", err)
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		slog.Warn("LLMProxyDeploymentService.DeployLLMProxy: proxy not found", "proxyID", proxyID, "ouID", ouID)
		return nil, utils.ErrLLMProxyNotFound
	}

	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: proxy retrieved", "proxyID", proxyID, "proxyUUID", proxy.UUID)

	existing, err := s.deploymentRepo.GetDeployedGatewaysByProvider(proxy.UUID, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing proxy deployments: %w", err)
	}
	if err := validateEgressPlacement(s.gatewayRepo, gateway, existing); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
	}

	var baseDeploymentID *uuid.UUID
	var contentBytes []byte

	// Determine source: "current" or existing deployment
	if req.Base == "current" {
		slog.Info("LLMProxyDeploymentService.DeployLLMProxy: using current proxy configuration", "proxyID", proxyID)

		// Generate deployment YAML
		slog.Info("LLMProxyDeploymentService.DeployLLMProxy: generating deployment YAML", "proxyID", proxyID)
		deploymentYAML, err := s.generateLLMProxyDeploymentYAML(proxy, ouID)
		if err != nil {
			slog.Error("LLMProxyDeploymentService.DeployLLMProxy: failed to generate deployment YAML", "proxyID", proxyID, "error", err)
			return nil, fmt.Errorf("failed to generate deployment YAML: %w", err)
		}
		contentBytes = []byte(deploymentYAML)
	} else {
		slog.Info("LLMProxyDeploymentService.DeployLLMProxy: using existing deployment as base", "proxyID", proxyID, "baseDeploymentID", req.Base)

		// Use existing deployment as base
		baseUUID, err := uuid.Parse(req.Base)
		if err != nil {
			slog.Error("LLMProxyDeploymentService.DeployLLMProxy: invalid base deployment ID", "proxyID", proxyID, "baseDeploymentID", req.Base, "error", err)
			return nil, fmt.Errorf("invalid base deployment ID: %w", err)
		}

		baseDeployment, err := s.deploymentRepo.GetWithContent(req.Base, proxy.UUID.String(), ouID)
		if err != nil {
			slog.Warn("LLMProxyDeploymentService.DeployLLMProxy: base deployment not found", "proxyID", proxyID, "baseDeploymentID", req.Base, "error", err)
			return nil, utils.ErrBaseDeploymentNotFound
		}
		contentBytes = baseDeployment.Content
		baseDeploymentID = &baseUUID
		slog.Info("LLMProxyDeploymentService.DeployLLMProxy: base deployment retrieved", "proxyID", proxyID, "baseDeploymentID", req.Base)
	}

	// Create deployment
	deploymentID := uuid.New()
	deployed := models.DeploymentStatusDeployed

	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: creating deployment", "proxyID", proxyID,
		"deploymentID", deploymentID, "deploymentName", req.Name, "gatewayID", req.GatewayID)

	deployment := &models.Deployment{
		DeploymentID:     deploymentID,
		Name:             req.Name,
		ArtifactUUID:     proxy.UUID,
		OUID:             ouID,
		GatewayUUID:      gatewayUUID,
		BaseDeploymentID: baseDeploymentID,
		Content:          contentBytes,
		Metadata:         req.Metadata,
		Status:           &deployed,
	}

	hardLimit := maxDeploymentsPerAPI + deploymentLimitBuffer
	if err := s.deploymentRepo.CreateWithLimitEnforcement(deployment, hardLimit); err != nil {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: failed to create deployment", "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	slog.Info("LLMProxyDeploymentService.DeployLLMProxy: deployment created successfully", "proxyID", proxyID, "deploymentID", deploymentID)

	// Broadcast deployment event to gateway
	vhost := ""
	if proxy.Configuration.Vhost != nil {
		vhost = *proxy.Configuration.Vhost
	}

	deploymentEvent := &models.LLMProxyDeploymentEvent{
		ProxyID:        proxyID,
		DeploymentID:   deploymentID.String(),
		Vhost:          vhost,
		Environment:    "production",
		GatewayID:      req.GatewayID,
		OrganizationID: ouID,
		Status:         string(models.DeploymentStatusDeployed),
	}
	if err := s.gatewayEventsService.BroadcastLLMProxyDeploymentEvent(req.GatewayID, deploymentEvent); err != nil {
		slog.Error("LLMProxyDeploymentService.DeployLLMProxy: failed to broadcast deployment event",
			"proxyID", proxyID, "deploymentID", deploymentID, "gatewayID", req.GatewayID, "error", err)
		// Don't fail the deployment if broadcast fails - deployment is already persisted
	} else {
		slog.Info("LLMProxyDeploymentService.DeployLLMProxy: deployment event broadcast successfully",
			"proxyID", proxyID, "deploymentID", deploymentID, "gatewayID", req.GatewayID)
	}

	return deployment, nil
}

// UndeployLLMProxyDeployment undeploys a deployment
func (s *LLMProxyDeploymentService) UndeployLLMProxyDeployment(proxyID, deploymentID, gatewayID, ouID string) (*models.Deployment, error) {
	slog.Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: starting", "proxyID", proxyID,
		"deploymentID", deploymentID, "gatewayID", gatewayID, "ouID", ouID)

	// Get proxy
	slog.Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: getting proxy", "proxyID", proxyID, "ouID", ouID)
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: proxy not found", "proxyID", proxyID)
			return nil, utils.ErrLLMProxyNotFound
		}
		slog.Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to get proxy", "proxyID", proxyID, "error", err)
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		slog.Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: proxy not found", "proxyID", proxyID)
		return nil, utils.ErrLLMProxyNotFound
	}

	// Get deployment
	slog.Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: getting deployment", "proxyID", proxyID, "deploymentID", deploymentID)
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, proxy.UUID.String(), ouID)
	if err != nil {
		slog.Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to get deployment", "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		slog.Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: deployment not found", "proxyID", proxyID, "deploymentID", deploymentID)
		return nil, utils.ErrDeploymentNotFound
	}
	if deployment.GatewayUUID.String() != gatewayID {
		slog.Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: gateway ID mismatch", "proxyID", proxyID,
			"deploymentID", deploymentID, "expectedGatewayID", gatewayID, "actualGatewayID", deployment.GatewayUUID.String())
		return nil, utils.ErrGatewayIDMismatch
	}
	if deployment.Status == nil || *deployment.Status != models.DeploymentStatusDeployed {
		slog.Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: deployment not active", "proxyID", proxyID,
			"deploymentID", deploymentID, "status", deployment.Status)
		return nil, utils.ErrDeploymentNotActive
	}

	// Update status to undeployed
	slog.Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: setting status to undeployed", "proxyID", proxyID, "deploymentID", deploymentID)
	updatedAt, err := s.deploymentRepo.SetCurrent(proxy.UUID.String(), ouID, gatewayID, deploymentID, models.DeploymentStatusUndeployed)
	if err != nil {
		slog.Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to undeploy", "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to undeploy: %w", err)
	}

	undeployed := models.DeploymentStatusUndeployed
	deployment.Status = &undeployed
	deployment.UpdatedAt = &updatedAt

	slog.Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: undeployed successfully", "proxyID", proxyID, "deploymentID", deploymentID)

	// Broadcast undeployment event to gateway
	vhost := ""
	if proxy.Configuration.Vhost != nil {
		vhost = *proxy.Configuration.Vhost
	}

	undeploymentEvent := &models.LLMProxyUndeploymentEvent{
		ProxyID:        proxyID,
		Vhost:          vhost,
		Environment:    "production",
		GatewayID:      gatewayID,
		OrganizationID: ouID,
	}
	if err := s.gatewayEventsService.BroadcastLLMProxyUndeploymentEvent(gatewayID, undeploymentEvent); err != nil {
		slog.Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to broadcast undeployment event",
			"proxyID", proxyID, "deploymentID", deploymentID, "gatewayID", gatewayID, "error", err)
		// Don't fail the undeployment if broadcast fails - status is already updated
	} else {
		slog.Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: undeployment event broadcast successfully",
			"proxyID", proxyID, "deploymentID", deploymentID, "gatewayID", gatewayID)
	}

	return deployment, nil
}

// RestoreLLMProxyDeployment restores a previous deployment
func (s *LLMProxyDeploymentService) RestoreLLMProxyDeployment(proxyID, deploymentID, gatewayID, ouID string) (*models.Deployment, error) {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	// Get target deployment
	deployment, err := s.deploymentRepo.GetWithContent(deploymentID, proxy.UUID.String(), ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotFound
	}
	if deployment.GatewayUUID.String() != gatewayID {
		return nil, utils.ErrGatewayIDMismatch
	}

	// Check if already deployed
	currentDeploymentID, status, _, err := s.deploymentRepo.GetStatus(proxy.UUID.String(), ouID, gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment status: %w", err)
	}
	if currentDeploymentID == deploymentID && status == models.DeploymentStatusDeployed {
		return nil, utils.ErrDeploymentAlreadyDeployed
	}

	// Update status to deployed
	updatedAt, err := s.deploymentRepo.SetCurrent(proxy.UUID.String(), ouID, gatewayID, deploymentID, models.DeploymentStatusDeployed)
	if err != nil {
		return nil, fmt.Errorf("failed to restore deployment: %w", err)
	}

	deployed := models.DeploymentStatusDeployed
	deployment.Status = &deployed
	deployment.UpdatedAt = &updatedAt

	// Broadcast deployment event to gateway (restore is treated as a deployment)
	vhost := ""
	if proxy.Configuration.Vhost != nil {
		vhost = *proxy.Configuration.Vhost
	}

	deploymentEvent := &models.LLMProxyDeploymentEvent{
		ProxyID:        proxyID,
		DeploymentID:   deploymentID,
		Vhost:          vhost,
		Environment:    "production",
		GatewayID:      gatewayID,
		OrganizationID: ouID,
		Status:         string(models.DeploymentStatusDeployed),
	}
	if err := s.gatewayEventsService.BroadcastLLMProxyDeploymentEvent(gatewayID, deploymentEvent); err != nil {
		slog.Error("LLMProxyDeploymentService.RestoreLLMProxyDeployment: failed to broadcast deployment event",
			"proxyID", proxyID, "deploymentID", deploymentID, "gatewayID", gatewayID, "error", err)
		// Don't fail the restore if broadcast fails - status is already updated
	} else {
		slog.Info("LLMProxyDeploymentService.RestoreLLMProxyDeployment: deployment event broadcast successfully",
			"proxyID", proxyID, "deploymentID", deploymentID, "gatewayID", gatewayID)
	}

	return deployment, nil
}

// GetLLMProxyDeployments retrieves all deployments for a proxy
// GetDeployedGatewaysByProvider returns the gateway UUIDs where the given provider artifact is currently deployed.
func (s *LLMProxyDeploymentService) GetDeployedGatewaysByProvider(providerUUID uuid.UUID, ouID string) ([]string, error) {
	return s.deploymentRepo.GetDeployedGatewaysByProvider(providerUUID, ouID)
}

func (s *LLMProxyDeploymentService) GetLLMProxyDeployments(proxyID, ouID string, gatewayID *string, status *string) ([]*models.Deployment, error) {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	// Validate status if provided
	if status != nil {
		validStatuses := map[string]bool{
			string(models.DeploymentStatusDeployed):   true,
			string(models.DeploymentStatusUndeployed): true,
			string(models.DeploymentStatusArchived):   true,
		}
		if !validStatuses[*status] {
			return nil, utils.ErrInvalidDeploymentStatus
		}
	}

	// Get deployments
	deployments, err := s.deploymentRepo.GetDeploymentsWithState(proxy.UUID.String(), ouID, gatewayID, status, maxDeploymentsPerAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployments: %w", err)
	}

	return deployments, nil
}

// GetLLMProxyDeployment retrieves a specific deployment
func (s *LLMProxyDeploymentService) GetLLMProxyDeployment(proxyID, deploymentID, ouID string) (*models.Deployment, error) {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	// Get deployment
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, proxy.UUID.String(), ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotFound
	}

	return deployment, nil
}

// DeleteLLMProxyDeployment deletes a deployment
func (s *LLMProxyDeploymentService) DeleteLLMProxyDeployment(proxyID, deploymentID, ouID string) error {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrLLMProxyNotFound
		}
		return fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return utils.ErrLLMProxyNotFound
	}

	// Get deployment
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, proxy.UUID.String(), ouID)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return utils.ErrDeploymentNotFound
	}
	if deployment.Status != nil && *deployment.Status == models.DeploymentStatusDeployed {
		return utils.ErrDeploymentIsDeployed
	}

	// Delete deployment
	if err := s.deploymentRepo.Delete(deploymentID, proxy.UUID.String(), ouID); err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	return nil
}

// generateLLMProxyDeploymentYAML generates deployment YAML for an LLM proxy
func (s *LLMProxyDeploymentService) generateLLMProxyDeploymentYAML(proxy *models.LLMProxy, ouID string) (string, error) {
	if proxy == nil {
		return "", errors.New("proxy is required")
	}
	if proxy.Configuration.Provider == "" {
		return "", utils.ErrInvalidInput
	}

	// Get provider to validate it exists
	provider, err := s.providerRepo.GetByUUID(proxy.Configuration.Provider, ouID)
	if err != nil {
		return "", fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return "", utils.ErrLLMProviderNotFound
	}

	// Set default context if not provided
	contextValue := "/"
	if proxy.Configuration.Context != nil && *proxy.Configuration.Context != "" {
		contextValue = *proxy.Configuration.Context
	}

	vhostValue := ""
	if proxy.Configuration.Vhost != nil {
		vhostValue = *proxy.Configuration.Vhost
	}

	// Initialize policies slice
	policies := make([]models.LLMPolicy, 0, len(proxy.Configuration.Policies))

	// Transform security config to policy if enabled
	security := proxy.Configuration.Security
	if security != nil && isBoolTrue(security.Enabled) {
		if security.APIKey != nil && isBoolTrue(security.APIKey.Enabled) {
			key := strings.TrimSpace(security.APIKey.Key)
			if key == "" {
				return "", fmt.Errorf("invalid api key security configuration: key is required")
			}

			in := strings.ToLower(strings.TrimSpace(security.APIKey.In))
			if in != "header" && in != "query" {
				return "", fmt.Errorf("invalid api key security configuration: in must be 'header' or 'query', got %q", security.APIKey.In)
			}

			// Add API key auth as a policy
			addOrAppendPolicyPath(&policies, apiKeyAuthPolicyName, apiKeyAuthPolicyVersion, models.LLMPolicyPath{
				Path:    "/*",
				Methods: []string{"*"},
				Params: map[string]interface{}{
					"key": key,
					"in":  in,
				},
			})
		}
	}

	// Process and normalize policies
	for _, p := range proxy.Configuration.Policies {
		// Deep copy paths
		paths := make([]models.LLMPolicyPath, 0, len(p.Paths))
		for _, pp := range p.Paths {
			paths = append(paths, models.LLMPolicyPath{
				Path:    pp.Path,
				Methods: pp.Methods,
				Params:  pp.Params,
			})
		}
		// Add policy with normalized version
		policies = append(policies, models.LLMPolicy{
			Name:    p.Name,
			Version: normalizePolicyVersionToMajor(p.Version),
			Paths:   paths,
		})
	}

	// Build provider reference
	providerRef := LLMProxyDeploymentProvider{
		ID: provider.Artifact.Handle,
	}

	// Add upstream auth if configured
	if proxy.Configuration.UpstreamAuth != nil {
		providerRef.Auth = proxy.Configuration.UpstreamAuth
	}

	// Build deployment YAML
	deploymentYAML := LLMProxyDeploymentYAML{
		ApiVersion: apiVersionLLMProxy,
		Kind:       kindLLMProxy,
		Metadata: DeploymentMetadata{
			Name: proxy.Handle, // Use handle (artifact identifier) for metadata.name
		},
		Spec: LLMProxyDeploymentSpec{
			DisplayName: proxy.Configuration.Name,
			Version:     proxy.Configuration.Version,
			Context:     contextValue,
			VHost:       vhostValue,
			Provider:    providerRef,
			Resilience:  resilienceWithLLMProxyDefaults(proxy.Configuration.Resilience, provider.Configuration.Resilience),
			Policies:    policies,
		},
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(deploymentYAML)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

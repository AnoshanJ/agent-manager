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

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ErrMCPProxyNotDeployedToEnvironment means the proxy has no deployed
// (endpoint, environment) binding to anchor a resource identifier on.
var ErrMCPProxyNotDeployedToEnvironment = errors.New("MCP proxy is not deployed to this environment")

// MCPResourceServerIdentifier derives the protocol-stripped public URI the proxy
// is invoked at in the named environment — the value the env-Thunder resource
// server's identifier must carry.
func (s *MCPProxyService) MCPResourceServerIdentifier(ctx context.Context, ouID, envName string, proxy *models.MCPProxy) (string, error) {
	envs, err := s.infraManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		return "", fmt.Errorf("failed to list environments: %w", err)
	}
	envID := ""
	for _, env := range envs {
		if env.Name == envName {
			envID = env.UUID
			break
		}
	}
	if envID == "" {
		return "", fmt.Errorf("environment %q not found", envName)
	}

	_, ee := resolveMCPEndpointForEnv(proxy, envID)
	if ee == nil || ee.ArtifactUUID == uuid.Nil {
		return "", fmt.Errorf("%w: proxy %q, environment %q", ErrMCPProxyNotDeployedToEnvironment, proxyHandleOf(proxy), envName)
	}

	deployed, err := s.deploymentRepo.GetDeployedGatewaysByProvider(ee.ArtifactUUID, ouID)
	if err != nil {
		return "", fmt.Errorf("failed to list deployed gateways for MCP artifact %s: %w", ee.ArtifactUUID, err)
	}
	envUUID, err := uuid.Parse(envID)
	if err != nil {
		return "", fmt.Errorf("invalid environment UUID %q: %w", envID, err)
	}
	gateway, err := resolveEgressGatewayForArtifact(s.gatewayRepo, ouID, envUUID, deployed, nil)
	if err != nil {
		if errors.Is(err, errNoGatewayForEnvironment) || errors.Is(err, errNoEgressGatewayForEnvironment) {
			return "", fmt.Errorf("%w: proxy %q, environment %q", ErrMCPProxyNotDeployedToEnvironment, proxyHandleOf(proxy), envName)
		}
		return "", err
	}

	return stripURLScheme(buildMCPProxyURL(gateway, proxy.Configuration)), nil
}

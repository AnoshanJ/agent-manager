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

// The api-configuration trait's upstream port and base path are what the API
// gateway dials. Getting them wrong is not a cosmetic bug: the gateway reaches
// nothing and every request to the agent returns 503. Kind-sourced agents have no
// workflow parameters to read them from, which is exactly the case these cover.
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestEffectiveUpstreamInterface(t *testing.T) {
	ctx := context.Background()
	defaultPort := config.GetConfig().DefaultChatAPI.DefaultHTTPPort
	defaultBasePath := config.GetConfig().DefaultChatAPI.DefaultBasePath

	t.Run("reads the agent's own interface when it has one", func(t *testing.T) {
		s := &agentManagerService{logger: discardLogger()}

		port, basePath := s.effectiveUpstreamInterface(ctx, "org", &models.AgentResponse{
			InputInterface: &models.InputInterface{Port: 8084, BasePath: "/tasks"},
		})

		assert.Equal(t, int32(8084), port)
		assert.Equal(t, "/tasks", basePath)
	})

	// kindRepo returns a kind whose version 1.0.1 was built by build-1.
	kindRepo := func() *repomocks.AgentKindRepositoryMock {
		kindID := uuid.New()
		return &repomocks.AgentKindRepositoryMock{
			GetKindFunc: func(_ context.Context, _, kindName string) (*models.AgentKind, error) {
				return &models.AgentKind{
					ID: kindID, Name: kindName, ProjectName: "source-proj", AgentName: "source-agent",
				}, nil
			},
			GetVersionFunc: func(_ context.Context, _ uuid.UUID, versionTag string) (*models.AgentKindVersion, error) {
				return &models.AgentKindVersion{
					Version:   versionTag,
					BuildName: "build-1",
					Kind: &models.AgentKind{
						Name: "bal-task-assistant", ProjectName: "source-proj", AgentName: "source-agent",
					},
				}, nil
			},
		}
	}

	// The regression this exists for: a kind agent has no workflow parameters, so
	// its component reports no interface and the defaults would point the gateway
	// at a port nothing listens on. The build is the per-version record — reading
	// the source component instead would report whatever its port is today, which
	// the published image may never have served.
	t.Run("falls back to the build its kind version was published from", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetBuildFunc: func(_ context.Context, _, projectName, componentName, buildName string) (*models.BuildDetailsResponse, error) {
				assert.Equal(t, "source-proj", projectName)
				assert.Equal(t, "source-agent", componentName)
				assert.Equal(t, "build-1", buildName)
				return &models.BuildDetailsResponse{
					InputInterface: &models.InputInterface{Port: 8084, BasePath: "/tasks"},
				}, nil
			},
		}
		repo := kindRepo()
		s := &agentManagerService{
			ocClient: oc, agentKindService: newKindService(repo, oc), logger: discardLogger(),
		}

		port, basePath := s.effectiveUpstreamInterface(ctx, "org", &models.AgentResponse{
			KindName: "bal-task-assistant", KindVersion: "1.0.1",
		})

		assert.Equal(t, int32(8084), port)
		assert.Equal(t, "/tasks", basePath)
		require.Empty(t, oc.GetComponentCalls(), "the mutable source component must not be consulted")
	})

	t.Run("a build that can no longer be read leaves the defaults", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetBuildFunc: func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
				return nil, errors.New("workflow run not found")
			},
		}
		repo := kindRepo()
		s := &agentManagerService{
			ocClient: oc, agentKindService: newKindService(repo, oc), logger: discardLogger(),
		}

		port, basePath := s.effectiveUpstreamInterface(ctx, "org",
			&models.AgentResponse{KindName: "k", KindVersion: "1.0.1"})

		assert.Equal(t, defaultPort, port)
		assert.Equal(t, defaultBasePath, basePath)
	})

	// Agents created before the kind version was recorded have nothing to resolve
	// against; guessing a version would risk describing a different image.
	t.Run("a kind agent with no recorded version leaves the defaults", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{}
		repo := kindRepo()
		s := &agentManagerService{
			ocClient: oc, agentKindService: newKindService(repo, oc), logger: discardLogger(),
		}

		port, basePath := s.effectiveUpstreamInterface(ctx, "org", &models.AgentResponse{KindName: "k"})

		assert.Equal(t, defaultPort, port)
		assert.Equal(t, defaultBasePath, basePath)
		require.Empty(t, oc.GetBuildCalls())
	})

	// A source-built agent must never trigger a kind lookup.
	t.Run("a non-kind agent with no interface uses the defaults without extra calls", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{}
		s := &agentManagerService{ocClient: oc, logger: discardLogger()}

		port, basePath := s.effectiveUpstreamInterface(ctx, "org", &models.AgentResponse{})

		assert.Equal(t, defaultPort, port)
		assert.Equal(t, defaultBasePath, basePath)
		require.Empty(t, oc.GetBuildCalls())
	})
}

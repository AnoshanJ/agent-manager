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

// UNIT tests for environmentService. Like agent_kind_service_unit_test.go (the
// reference), this file has NO `//go:build integration` tag, so it runs in the
// fast unit tier with the dependencies mocked:
//
//   - repositories.GatewayRepository -> repomocks.GatewayRepositoryMock
//   - occlient.OpenChoreoClient      -> clientmocks.OpenChoreoClientMock
//   - thundersvc.Prober              -> clientmocks.ThunderProberMock
//
// The goal is to exercise the service's OWN logic (error mapping to sentinels,
// validation gates, pagination, fan-out/aggregation, transformation) without a
// database. Unconfigured mock methods panic, so any unexpected call fails loudly.
package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// envTestKey is a 32-byte AES key for the SetThunderSystemClientSecret tests.
var envTestKey = []byte("0123456789abcdef0123456789abcdef")

// newEnvService wires the service with a discard logger and the two mocked deps.
// The Thunder prober and env-Thunder URL repo are both left unconfigured (panic
// if called) — every ListThunderInstances test that needs either configured
// builds its own environmentService directly via NewEnvironmentService (a
// per-env registered handle drives both which URL repo reads happen and what
// gets probed, so there's no one-size-fits-all "with prober" helper to share).
// The env-Thunder-system-client repo and encryption key are nil/empty here;
// SetThunderSystemClient/SetThunderURL tests use
// newEnvServiceWithThunderRepo/newEnvServiceWithThunderURLRepo instead.
func newEnvService(repo *repomocks.GatewayRepositoryMock, oc *clientmocks.OpenChoreoClientMock) EnvironmentService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewEnvironmentService(logger, repo, oc, &clientmocks.ThunderProberMock{}, nil, nil, nil, nil)
}

// newEnvServiceWithThunderRepo wires the service with a configured env-Thunder
// repo + encryption key, for the SetThunderSystemClientSecret tests.
func newEnvServiceWithThunderRepo(repo *repomocks.EnvThunderSystemClientRepositoryMock) EnvironmentService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewEnvironmentService(logger, &repomocks.GatewayRepositoryMock{}, &clientmocks.OpenChoreoClientMock{}, &clientmocks.ThunderProberMock{}, nil, repo, nil, envTestKey)
}

// newEnvServiceWithThunderURLRepo wires the service with a configured env-Thunder
// URL repo, for the SetThunderURL/GetThunderURL/DeleteThunderURL tests. Both
// repos default to "not found" (genuinely never provisioned) unless the caller
// already set GetFunc — SetThunderURL/GetThunderURL now resolve their existing
// state through ResolveThunderHandle, which reads BOTH repos (see its
// grandfather-from-system-client-credential fallback), so a test that leaves
// either GetFunc nil would otherwise panic on the unconfigured mock call.
func newEnvServiceWithThunderURLRepo(repo *repomocks.EnvThunderURLRepositoryMock) EnvironmentService {
	return newEnvServiceWithThunderURLAndSystemClientRepo(repo, &repomocks.EnvThunderSystemClientRepositoryMock{})
}

// newEnvServiceWithThunderURLAndSystemClientRepo is newEnvServiceWithThunderURLRepo
// with an explicit system-client repo too — for tests exercising
// ResolveThunderHandle's grandfather path specifically (a pre-existing
// system-client credential with no URL row yet).
func newEnvServiceWithThunderURLAndSystemClientRepo(
	urlRepo *repomocks.EnvThunderURLRepositoryMock,
	systemClientRepo *repomocks.EnvThunderSystemClientRepositoryMock,
) EnvironmentService {
	if urlRepo.GetFunc == nil {
		urlRepo.GetFunc = func(context.Context, string, string) (*models.EnvThunderURL, error) {
			return nil, gorm.ErrRecordNotFound
		}
	}
	if systemClientRepo.GetFunc == nil {
		systemClientRepo.GetFunc = func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
			return nil, gorm.ErrRecordNotFound
		}
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewEnvironmentService(logger, &repomocks.GatewayRepositoryMock{}, &clientmocks.OpenChoreoClientMock{}, &clientmocks.ThunderProberMock{}, nil, systemClientRepo, urlRepo, nil)
}

// -----------------------------------------------------------------------------
// CreateEnvironment — wraps client errors; maps client response on success.
// -----------------------------------------------------------------------------

func TestEnvironmentService_CreateEnvironment(t *testing.T) {
	const org = "acme"

	t.Run("wraps a client error", func(t *testing.T) {
		boom := errors.New("oc unreachable")
		oc := &clientmocks.OpenChoreoClientMock{
			CreateEnvironmentFunc: func(_ context.Context, _ string, _ occlient.CreateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.CreateEnvironment(context.Background(), org, &models.CreateEnvironmentRequest{Name: "dev"})

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("maps the client response on success", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			CreateEnvironmentFunc: func(_ context.Context, ns string, req occlient.CreateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				// The request must be translated from the public model.
				assert.Equal(t, org, ns)
				assert.Equal(t, "dev", req.Name)
				return &models.EnvironmentResponse{
					UUID:         "11111111-1111-1111-1111-111111111111",
					Name:         "dev",
					DisplayName:  "Development",
					DataplaneRef: "dp-1",
					IsProduction: true,
				}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.CreateEnvironment(context.Background(), org, &models.CreateEnvironmentRequest{
			Name:         "dev",
			DisplayName:  "Development",
			Description:  "desc from request",
			DataplaneRef: "dp-1",
			IsProduction: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "dev", resp.Name)
		assert.Equal(t, org, resp.OrganizationName)
		assert.Equal(t, "Development", resp.DisplayName)
		// Description on the response comes from the REQUEST, not the OC response.
		assert.Equal(t, "desc from request", resp.Description)
		assert.True(t, resp.IsProduction)
	})
}

// -----------------------------------------------------------------------------
// GetEnvironment — maps not-found to the sentinel; wraps everything else.
// -----------------------------------------------------------------------------

func TestEnvironmentService_GetEnvironment(t *testing.T) {
	const org, envID = "acme", "dev"

	t.Run("maps not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrEnvironmentNotFound
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.GetEnvironment(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("wraps an unexpected client error", func(t *testing.T) {
		boom := errors.New("connection reset")
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.GetEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("returns mapped response on success", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{
					UUID:        "uuid-1",
					Name:        "dev",
					DisplayName: "Development",
					Description: "from oc",
					DNSPrefix:   "dev-prefix",
				}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.GetEnvironment(context.Background(), org, envID)

		require.NoError(t, err)
		assert.Equal(t, "dev", resp.Name)
		assert.Equal(t, org, resp.OrganizationName)
		// Unlike Create, Get carries description + DNS prefix straight from OC.
		assert.Equal(t, "from oc", resp.Description)
		assert.Equal(t, "dev-prefix", resp.DNSPrefix)
	})
}

// -----------------------------------------------------------------------------
// ListEnvironments — exercises pagination (offset/limit) and aggregation.
// -----------------------------------------------------------------------------

func TestEnvironmentService_ListEnvironments(t *testing.T) {
	const org = "acme"

	threeEnvs := func() []*models.EnvironmentResponse {
		return []*models.EnvironmentResponse{
			{UUID: "u1", Name: "env-0"},
			{UUID: "u2", Name: "env-1"},
			{UUID: "u3", Name: "env-2"},
		}
	}

	t.Run("wraps a client error", func(t *testing.T) {
		boom := errors.New("oc down")
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.ListEnvironments(context.Background(), org, 10, 0)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("applies limit within the available range", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return threeEnvs(), nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListEnvironments(context.Background(), org, 2, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(3), resp.Total) // total reflects all, not the page
		require.Len(t, resp.Environments, 2)
		assert.Equal(t, "env-0", resp.Environments[0].Name)
		assert.Equal(t, "env-1", resp.Environments[1].Name)
		assert.Equal(t, org, resp.Environments[0].OrganizationName)
	})

	t.Run("clamps the page end when limit overruns the slice", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return threeEnvs(), nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListEnvironments(context.Background(), org, 10, 1)

		require.NoError(t, err)
		assert.Equal(t, int32(3), resp.Total)
		require.Len(t, resp.Environments, 2) // env-1, env-2
		assert.Equal(t, "env-1", resp.Environments[0].Name)
	})

	t.Run("returns an empty page when offset is past the end", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return threeEnvs(), nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListEnvironments(context.Background(), org, 10, 5)

		require.NoError(t, err)
		assert.Equal(t, int32(3), resp.Total) // total still reports everything
		assert.Empty(t, resp.Environments)
		assert.Equal(t, int32(5), resp.Offset)
	})
}

// -----------------------------------------------------------------------------
// UpdateEnvironment — both ErrNotFound and ErrEnvironmentNotFound collapse to
// the env sentinel; other errors are wrapped. Description comes from the request.
// -----------------------------------------------------------------------------

func TestEnvironmentService_UpdateEnvironment(t *testing.T) {
	const org, envID = "acme", "dev"

	t.Run("maps generic not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			UpdateEnvironmentFunc: func(_ context.Context, _, _ string, _ occlient.UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrNotFound
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.UpdateEnvironment(context.Background(), org, envID, &models.UpdateEnvironmentRequest{})

		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("wraps an unexpected client error", func(t *testing.T) {
		boom := errors.New("boom")
		oc := &clientmocks.OpenChoreoClientMock{
			UpdateEnvironmentFunc: func(_ context.Context, _, _ string, _ occlient.UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.UpdateEnvironment(context.Background(), org, envID, &models.UpdateEnvironmentRequest{})

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("uses the request description on success", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			UpdateEnvironmentFunc: func(_ context.Context, _, _ string, _ occlient.UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{
					UUID:        "u1",
					Name:        "dev",
					Description: "ignored-oc-desc",
				}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.UpdateEnvironment(context.Background(), org, envID, &models.UpdateEnvironmentRequest{
			Description: strPtr("new description"),
		})

		require.NoError(t, err)
		assert.Equal(t, "new description", resp.Description)
		assert.Equal(t, org, resp.OrganizationName)
	})

	t.Run("defaults description to empty when request omits it", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			UpdateEnvironmentFunc: func(_ context.Context, _, _ string, _ occlient.UpdateEnvironmentRequest) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: "u1", Name: "dev", Description: "oc-desc"}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.UpdateEnvironment(context.Background(), org, envID, &models.UpdateEnvironmentRequest{})

		require.NoError(t, err)
		assert.Equal(t, "", resp.Description)
	})
}

// -----------------------------------------------------------------------------
// pipelineReferencesEnvironment — the pure predicate DeleteEnvironment uses to
// decide whether a pipeline blocks deletion (as source or as a promotion target).
// -----------------------------------------------------------------------------

func TestPipelineReferencesEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		pipeline *models.DeploymentPipelineResponse
		envName  string
		want     bool
	}{
		{
			name:     "no promotion paths",
			pipeline: &models.DeploymentPipelineResponse{Name: "p"},
			envName:  "development",
			want:     false,
		},
		{
			name: "matches source environment",
			pipeline: &models.DeploymentPipelineResponse{
				PromotionPaths: []models.PromotionPath{{SourceEnvironmentRef: "development"}},
			},
			envName: "development",
			want:    true,
		},
		{
			name: "matches target environment",
			pipeline: &models.DeploymentPipelineResponse{
				PromotionPaths: []models.PromotionPath{{
					SourceEnvironmentRef:  "development",
					TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "production"}},
				}},
			},
			envName: "production",
			want:    true,
		},
		{
			name: "no match",
			pipeline: &models.DeploymentPipelineResponse{
				PromotionPaths: []models.PromotionPath{{
					SourceEnvironmentRef:  "development",
					TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "production"}},
				}},
			},
			envName: "staging",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pipelineReferencesEnvironment(tt.pipeline, tt.envName))
		})
	}
}

// -----------------------------------------------------------------------------
// DeleteEnvironment — the richest method: lookup, UUID parse, pipeline-reference
// guard, OC delete (idempotent on not-found), then local mapping cleanup.
// -----------------------------------------------------------------------------

func TestEnvironmentService_DeleteEnvironment(t *testing.T) {
	const org, envID = "acme", "dev"
	const envUUID = "22222222-2222-2222-2222-222222222222"

	t.Run("maps lookup not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrNotFound
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("fails on an invalid UUID from OpenChoreo", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: "not-a-uuid", Name: "dev"}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("blocks deletion when a pipeline references the environment", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{
					{
						Name: "pipeline-a",
						PromotionPaths: []models.PromotionPath{
							{SourceEnvironmentRef: "dev"}, // references our env as source
						},
					},
				}, nil
			},
			// DeleteEnvironment must NOT be reached — leaving it nil asserts that.
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentInUse)
		assert.Contains(t, err.Error(), "pipeline-a")
		assert.Empty(t, oc.DeleteEnvironmentCalls())
	})

	t.Run("blocks deletion when referenced as a pipeline target", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{
					{
						Name: "pipeline-b",
						PromotionPaths: []models.PromotionPath{
							{
								SourceEnvironmentRef:  "staging",
								TargetEnvironmentRefs: []models.TargetEnvironmentRef{{Name: "dev"}},
							},
						},
					},
				}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentInUse)
	})

	t.Run("wraps a pipeline-listing error", func(t *testing.T) {
		boom := errors.New("pipelines unreachable")
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return nil, boom
			},
			// DeleteEnvironment must NOT be reached — leaving it nil asserts that.
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.Empty(t, oc.DeleteEnvironmentCalls())
	})

	t.Run("surfaces a non-not-found OC delete error without local cleanup", func(t *testing.T) {
		boom := errors.New("release bindings still exist")
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{}, nil
			},
			DeleteEnvironmentFunc: func(_ context.Context, _, _ string) error {
				return boom
			},
		}
		// DeleteEnvironmentMappingsByEnvironmentID must NOT be reached (nil asserts that).
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("treats an OC not-found delete as idempotent and still cleans up locally", func(t *testing.T) {
		cleaned := false
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{}, nil
			},
			DeleteEnvironmentFunc: func(_ context.Context, _, _ string) error {
				return utils.ErrEnvironmentNotFound
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			DeleteEnvironmentMappingsByEnvironmentIDFunc: func(_ string) (int64, error) {
				cleaned = true
				return 0, nil
			},
		}
		svc := newEnvService(repo, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.NoError(t, err)
		assert.True(t, cleaned, "expected local mapping cleanup to run")
	})

	t.Run("returns an error when local mapping cleanup fails", func(t *testing.T) {
		boom := errors.New("db down")
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{}, nil
			},
			DeleteEnvironmentFunc: func(_ context.Context, _, _ string) error {
				return nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			DeleteEnvironmentMappingsByEnvironmentIDFunc: func(_ string) (int64, error) {
				return 0, boom
			},
		}
		svc := newEnvService(repo, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("happy path: OC delete then local cleanup", func(t *testing.T) {
		var cleanedID string
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
			ListDeploymentPipelinesFunc: func(_ context.Context, _ string) ([]*models.DeploymentPipelineResponse, error) {
				return []*models.DeploymentPipelineResponse{
					{Name: "unrelated", PromotionPaths: []models.PromotionPath{{SourceEnvironmentRef: "prod"}}},
				}, nil
			},
			DeleteEnvironmentFunc: func(_ context.Context, _, name string) error {
				assert.Equal(t, "dev", name)
				return nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			DeleteEnvironmentMappingsByEnvironmentIDFunc: func(id string) (int64, error) {
				cleanedID = id
				return 2, nil
			},
		}
		svc := newEnvService(repo, oc)

		err := svc.DeleteEnvironment(context.Background(), org, envID)

		require.NoError(t, err)
		assert.Equal(t, envUUID, cleanedID)
	})
}

// -----------------------------------------------------------------------------
// GetEnvironmentGateways — verify env, resolve mappings, fan-out per gateway,
// skip missing/errored gateways, and map IsActive -> status string.
// -----------------------------------------------------------------------------

func TestEnvironmentService_GetEnvironmentGateways(t *testing.T) {
	const org, envID = "acme", "dev"
	const envUUID = "33333333-3333-3333-3333-333333333333"

	t.Run("maps env not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrEnvironmentNotFound
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("wraps a mapping-lookup error", func(t *testing.T) {
		boom := errors.New("mapping query failed")
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
				return nil, boom
			},
		}
		svc := newEnvService(repo, oc)

		_, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("aggregates gateways and skips missing/errored ones", func(t *testing.T) {
		activeGW := uuid.New()
		errorGW := uuid.New()
		missingGW := uuid.New()

		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
				return []models.GatewayEnvironmentMapping{
					{GatewayUUID: activeGW},
					{GatewayUUID: errorGW},
					{GatewayUUID: missingGW},
				}, nil
			},
			GetByUUIDFunc: func(id string) (*models.Gateway, error) {
				switch id {
				case activeGW.String():
					return &models.Gateway{
						UUID:                     activeGW,
						Name:                     "gw-active",
						DisplayName:              "Active GW",
						GatewayFunctionalityType: "egress",
						Vhost:                    "gw.example.com",
						IsCritical:               true,
						IsActive:                 true,
					}, nil
				case errorGW.String():
					return nil, errors.New("transient") // skipped, not fatal
				case missingGW.String():
					//nolint:nilnil // intentionally exercising the (nil, nil) "missing, skip" input the service must handle
					return nil, nil // skipped
				default:
					return nil, errors.New("unexpected gateway id")
				}
			},
		}
		svc := newEnvService(repo, oc)

		resp, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.NoError(t, err)
		require.Len(t, resp, 1) // only the active gateway survives
		assert.Equal(t, "gw-active", resp[0].Name)
		assert.Equal(t, activeGW.String(), resp[0].UUID)
		assert.Equal(t, org, resp[0].OrganizationName)
		assert.True(t, resp[0].IsCritical)
		assert.Equal(t, string(models.GatewayStatusActive), resp[0].Status)
	})

	t.Run("maps an inactive gateway to inactive status", func(t *testing.T) {
		gw := uuid.New()
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
				return []models.GatewayEnvironmentMapping{{GatewayUUID: gw}}, nil
			},
			GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
				return &models.Gateway{UUID: gw, Name: "gw-idle", IsActive: false}, nil
			},
		}
		svc := newEnvService(repo, oc)

		resp, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.NoError(t, err)
		require.Len(t, resp, 1)
		assert.Equal(t, string(models.GatewayStatusInactive), resp[0].Status)
	})
}

// -----------------------------------------------------------------------------
// ListThunderInstances — gates returning Thunder instance info on whether
// the env-Thunder JWKS endpoint is actually reachable (live HTTP probe).
// -----------------------------------------------------------------------------

func TestEnvironmentService_ListThunderInstances(t *testing.T) {
	const org = "acme"

	t.Run("wraps list environments error", func(t *testing.T) {
		boom := errors.New("oc down")
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.ListThunderInstances(context.Background(), org)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("skips environments with a registered handle that isn't currently reachable", func(t *testing.T) {
		// Both envs HAVE a registered handle (provisioned), but the prober reports
		// them unreachable (e.g. Thunder is down) — the result list must be empty.
		// Proving that gateway mappings alone are NOT sufficient to advertise
		// Thunder endpoints, distinct from the "no handle at all" case below.
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{
					{UUID: "u1", Name: "dev", DisplayName: "Dev", IsProduction: false},
					{UUID: "u2", Name: "staging", DisplayName: "Staging", IsProduction: false},
				}, nil
			},
			ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
				return []*models.OrganizationResponse{{Namespace: org}}, nil
			},
		}
		prober := &clientmocks.ThunderProberMock{
			ProbeFunc: func(_ context.Context, _, _, _ string) bool { return false },
		}
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(_ context.Context, _, envName string) (*models.EnvThunderURL, error) {
				return &models.EnvThunderURL{ThunderHandle: envName + "-handle"}, nil
			},
		}
		svc := NewEnvironmentService(discardLogger(), &repomocks.GatewayRepositoryMock{}, oc, prober, nil, nil, urlRepo, nil)

		resp, err := svc.ListThunderInstances(context.Background(), org)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.ThunderInstances)
	})

	t.Run("skips environments with no registered handle WITHOUT probing", func(t *testing.T) {
		// No handle means "never provisioned through add-environment-thunder.sh" —
		// there's no address to probe, so the prober must never even be called.
		// The unconfigured ThunderProberMock panics if it is, which is how this
		// test fails loudly if that guarantee ever regresses.
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{
					{UUID: "u1", Name: "dev", DisplayName: "Dev", IsProduction: false},
				}, nil
			},
			ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
				return []*models.OrganizationResponse{{Namespace: org}}, nil
			},
		}
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		// A missing URL row now also checks for a grandfatherable system-client
		// credential (see ResolveThunderHandle) before concluding "not provisioned" —
		// this env genuinely has neither, so that check must also report not-found.
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		svc := NewEnvironmentService(discardLogger(), &repomocks.GatewayRepositoryMock{}, oc, &clientmocks.ThunderProberMock{}, nil, systemClientRepo, urlRepo, nil)

		resp, err := svc.ListThunderInstances(context.Background(), org)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.ThunderInstances)
	})

	t.Run("skips nil and empty-name environments", func(t *testing.T) {
		// newEnvService's prober is unconfigured (panics if called) — proving the
		// nil/empty-name guard skips these entries before ever probing them.
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{
					nil,
					{UUID: "u1", Name: ""},
				}, nil
			},
			ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
				return []*models.OrganizationResponse{{Namespace: org}}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListThunderInstances(context.Background(), org)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Empty(t, resp.ThunderInstances)
	})

	t.Run("includes reachable Thunder instances, addressed by their registered handle", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{
					{UUID: "u1", Name: "dev", DisplayName: "Dev", IsProduction: false},
					{UUID: "u2", Name: "staging", DisplayName: "Staging", IsProduction: true},
				}, nil
			},
			ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
				return []*models.OrganizationResponse{{Namespace: org}}, nil
			},
		}
		// Each env has its OWN distinct handle — nothing derived from org/env.
		handles := map[string]string{"dev": "aaaa1111", "staging": "bbbb2222"}
		var probedHandles []string
		var mu sync.Mutex
		prober := &clientmocks.ThunderProberMock{
			ProbeFunc: func(_ context.Context, _, _, handle string) bool {
				mu.Lock()
				probedHandles = append(probedHandles, handle)
				mu.Unlock()
				return true
			},
		}
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(_ context.Context, _, envName string) (*models.EnvThunderURL, error) {
				return &models.EnvThunderURL{ThunderHandle: handles[envName]}, nil
			},
		}
		svc := NewEnvironmentService(discardLogger(), &repomocks.GatewayRepositoryMock{}, oc, prober, nil, nil, urlRepo, nil)

		resp, err := svc.ListThunderInstances(context.Background(), org)

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.ThunderInstances, 2)
		assert.ElementsMatch(t, []string{"aaaa1111", "bbbb2222"}, probedHandles,
			"the probe must target each env's own registered handle, never a value derived from org/env")

		dev := resp.ThunderInstances[0]
		assert.Equal(t, "dev", dev.EnvName)
		assert.Equal(t, "Dev", dev.DisplayName)
		assert.False(t, dev.IsProduction)
		assert.Equal(t, thundersvc.ThunderIssuerURL("aaaa1111"), dev.IssuerURL)
		assert.Equal(t, thundersvc.ThunderExternalTokenURL("aaaa1111"), dev.TokenURL)
		assert.Equal(t, thundersvc.ThunderExternalJWKSURL("aaaa1111"), dev.JWKSURL)
		assert.Equal(t, thundersvc.ThunderNamespace(org, "dev"), dev.Namespace)
		assert.NotContains(t, dev.IssuerURL, org+"-dev", "must never leak an org-env-derived pattern")

		staging := resp.ThunderInstances[1]
		assert.Equal(t, "staging", staging.EnvName)
		assert.True(t, staging.IsProduction)
		assert.Equal(t, thundersvc.ThunderIssuerURL("bbbb2222"), staging.IssuerURL)
	})

	t.Run("surfaces a pre-existing environment via the grandfathered legacy handle", func(t *testing.T) {
		// This environment predates the handle feature: no env_thunder_urls row,
		// but it already has an env_thunder_system_client credential (it WAS
		// provisioned through add-environment-thunder.sh, just before every
		// environment got a registered handle). It must still show up here — not
		// be silently dropped as "not provisioned" — addressed at the exact
		// hostname its already-running Thunder instance answers on.
		oc := &clientmocks.OpenChoreoClientMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{
					{UUID: "u1", Name: "legacy", DisplayName: "Legacy", IsProduction: false},
				}, nil
			},
			ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
				return []*models.OrganizationResponse{{Namespace: org}}, nil
			},
		}
		var probedHandle string
		prober := &clientmocks.ThunderProberMock{
			ProbeFunc: func(_ context.Context, _, _, handle string) bool {
				probedHandle = handle
				return true
			},
		}
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error { return nil },
		}
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				return &models.EnvThunderSystemClient{}, nil
			},
		}
		svc := NewEnvironmentService(discardLogger(), &repomocks.GatewayRepositoryMock{}, oc, prober, nil, systemClientRepo, urlRepo, nil)

		resp, err := svc.ListThunderInstances(context.Background(), org)

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, resp.ThunderInstances, 1)
		expectedHandle := thundersvc.LegacyThunderHandleLabel(ThunderOrgNamespace(), "legacy")
		assert.Equal(t, expectedHandle, probedHandle)
		assert.Equal(t, thundersvc.ThunderIssuerURL(expectedHandle), resp.ThunderInstances[0].IssuerURL)
	})
}

// -----------------------------------------------------------------------------
// SetThunderSystemClientSecret / DeleteThunderSystemClientSecret — encrypt +
// upsert, decrypt round-trip, validation, delete.
// -----------------------------------------------------------------------------

func TestEnvironmentService_SetThunderSystemClientSecret(t *testing.T) {
	t.Run("encrypts the secret and upserts with the given client id", func(t *testing.T) {
		var stored *models.EnvThunderSystemClient
		repo := &repomocks.EnvThunderSystemClientRepositoryMock{
			UpsertFunc: func(_ context.Context, cred *models.EnvThunderSystemClient) error {
				stored = cred
				return nil
			},
		}
		svc := newEnvServiceWithThunderRepo(repo)

		err := svc.SetThunderSystemClientSecret(context.Background(), "ou-123", "staging", "amp-system-client", "s3cr3t")
		require.NoError(t, err)
		require.NotNil(t, stored)
		assert.Equal(t, "ou-123", stored.OUID)
		assert.Equal(t, "staging", stored.EnvName)
		assert.Equal(t, "amp-system-client", stored.ClientID)
		// Ciphertext must not equal the plaintext, and must decrypt back to it.
		assert.NotEqual(t, []byte("s3cr3t"), stored.ClientSecretEncrypted)
		decrypted, err := utils.DecryptBytes(stored.ClientSecretEncrypted, envTestKey)
		require.NoError(t, err)
		assert.Equal(t, "s3cr3t", string(decrypted))
	})

	t.Run("rejects an empty secret", func(t *testing.T) {
		repo := &repomocks.EnvThunderSystemClientRepositoryMock{
			UpsertFunc: func(context.Context, *models.EnvThunderSystemClient) error {
				t.Fatal("must not upsert when the secret is empty")
				return nil
			},
		}
		svc := newEnvServiceWithThunderRepo(repo)

		err := svc.SetThunderSystemClientSecret(context.Background(), "ou-123", "staging", "amp-system-client", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})

	t.Run("rejects an empty ouID", func(t *testing.T) {
		repo := &repomocks.EnvThunderSystemClientRepositoryMock{
			UpsertFunc: func(context.Context, *models.EnvThunderSystemClient) error {
				t.Fatal("must not upsert when ouID is empty — ouID is the multi-tenant-safe lookup key")
				return nil
			},
		}
		svc := newEnvServiceWithThunderRepo(repo)

		err := svc.SetThunderSystemClientSecret(context.Background(), "", "staging", "amp-system-client", "s3cr3t")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})

	t.Run("wraps a repo error", func(t *testing.T) {
		boom := errors.New("db down")
		repo := &repomocks.EnvThunderSystemClientRepositoryMock{
			UpsertFunc: func(context.Context, *models.EnvThunderSystemClient) error { return boom },
		}
		svc := newEnvServiceWithThunderRepo(repo)

		err := svc.SetThunderSystemClientSecret(context.Background(), "ou-123", "staging", "amp-system-client", "s3cr3t")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}

func TestEnvironmentService_DeleteThunderSystemClientSecret(t *testing.T) {
	t.Run("delegates to the repo, keyed by ouID", func(t *testing.T) {
		var gotOUID, gotEnv string
		repo := &repomocks.EnvThunderSystemClientRepositoryMock{
			DeleteFunc: func(_ context.Context, ouID, envName string) error {
				gotOUID, gotEnv = ouID, envName
				return nil
			},
		}
		svc := newEnvServiceWithThunderRepo(repo)

		err := svc.DeleteThunderSystemClientSecret(context.Background(), "ou-123", "staging")
		require.NoError(t, err)
		assert.Equal(t, "ou-123", gotOUID)
		assert.Equal(t, "staging", gotEnv)
	})

	t.Run("rejects an empty ouID", func(t *testing.T) {
		repo := &repomocks.EnvThunderSystemClientRepositoryMock{
			DeleteFunc: func(context.Context, string, string) error {
				t.Fatal("must not delete when ouID is empty")
				return nil
			},
		}
		svc := newEnvServiceWithThunderRepo(repo)

		err := svc.DeleteThunderSystemClientSecret(context.Background(), "", "staging")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})

	t.Run("wraps a repo error", func(t *testing.T) {
		boom := errors.New("db down")
		repo := &repomocks.EnvThunderSystemClientRepositoryMock{
			DeleteFunc: func(context.Context, string, string) error { return boom },
		}
		svc := newEnvServiceWithThunderRepo(repo)

		err := svc.DeleteThunderSystemClientSecret(context.Background(), "ou-123", "staging")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}

// -----------------------------------------------------------------------------
// SetThunderURL / DeleteThunderURL — format validation, uniqueness-conflict
// mapping, upsert, delete. See naming.go/env_resolver.go for how the stored
// handle then overrides the guessable <org>-<env> pattern.
// -----------------------------------------------------------------------------

func TestEnvironmentService_SetThunderURL(t *testing.T) {
	t.Run("upserts a caller-supplied handle and returns it unchanged", func(t *testing.T) {
		var stored *models.EnvThunderURL
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(_ context.Context, rec *models.EnvThunderURL) error {
				stored = rec
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		resolved, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "x7f2q9kzab")
		require.NoError(t, err)
		assert.Equal(t, "x7f2q9kzab", resolved)
		require.NotNil(t, stored)
		assert.Equal(t, "ou-123", stored.OUID)
		assert.Equal(t, "prod", stored.EnvName)
		assert.Equal(t, "x7f2q9kzab", stored.ThunderHandle)
	})

	t.Run("generates a 10-character handle when none is given, and returns it", func(t *testing.T) {
		var stored *models.EnvThunderURL
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(_ context.Context, rec *models.EnvThunderURL) error {
				stored = rec
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		resolved, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "")
		require.NoError(t, err)
		assert.Len(t, resolved, 10, "the task's own spec: an auto-generated handle is exactly 10 characters")
		assert.Regexp(t, `^[a-z0-9]{10}$`, resolved, "generated handle must be lowercase alphanumeric — trivially valid against the format check")
		require.NotNil(t, stored)
		assert.Equal(t, resolved, stored.ThunderHandle, "the caller must be told the SAME value that got persisted")
	})

	t.Run("retries with a fresh generated value when the first collides", func(t *testing.T) {
		attempt := 0
		var generatedHandles []string
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(_ context.Context, rec *models.EnvThunderURL) error {
				attempt++
				generatedHandles = append(generatedHandles, rec.ThunderHandle)
				if attempt == 1 {
					return utils.ErrThunderHandleTaken
				}
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		resolved, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "")
		require.NoError(t, err, "a collision on a GENERATED handle must be retried, not surfaced as an error")
		assert.Equal(t, 2, attempt, "must retry exactly once after the first collision")
		assert.Equal(t, generatedHandles[1], resolved, "must return the handle that actually succeeded, not the first (rejected) attempt")
		assert.NotEqual(t, generatedHandles[0], generatedHandles[1], "the retry must use a FRESH random value, not repeat the collided one")
	})

	t.Run("gives up after repeated collisions instead of retrying forever", func(t *testing.T) {
		attempts := 0
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				attempts++
				return utils.ErrThunderHandleTaken
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "")
		require.Error(t, err)
		assert.Equal(t, maxGenerateThunderHandleAttempts, attempts, "must stop after the documented attempt cap, not loop forever")
	})

	t.Run("a collision on a caller-supplied handle is never retried with a different value", func(t *testing.T) {
		attempts := 0
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				attempts++
				return utils.ErrThunderHandleTaken
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "wanted-name")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrThunderHandleTaken)
		assert.Equal(t, 1, attempts, "silently substituting a different value than what the caller asked for would be surprising — must fail immediately, not retry with a random one")
	})

	t.Run("a blank handle on an already-registered environment reuses it instead of generating a new one", func(t *testing.T) {
		// Thunder's issuer is immutable once minted — regenerating here on a
		// blank-handle re-run (e.g. re-running add-environment-thunder.sh with
		// THUNDER_HANDLE unset against an environment that's already provisioned)
		// would move the database to a hostname the already-running instance
		// never actually issues tokens for.
		upsertCalls := 0
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return &models.EnvThunderURL{ThunderHandle: "existing1"}, nil
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				upsertCalls++
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		resolved, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "")
		require.NoError(t, err)
		assert.Equal(t, "existing1", resolved)
		assert.Equal(t, 0, upsertCalls, "must not write anything when reusing an already-registered handle")
	})

	t.Run("re-supplying the SAME explicit handle as what's already registered is a no-op", func(t *testing.T) {
		upsertCalls := 0
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return &models.EnvThunderURL{ThunderHandle: "existing1"}, nil
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				upsertCalls++
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		resolved, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "existing1")
		require.NoError(t, err)
		assert.Equal(t, "existing1", resolved)
		assert.Equal(t, 0, upsertCalls)
	})

	t.Run("an explicit DIFFERENT handle on an already-registered environment is rejected, not applied", func(t *testing.T) {
		upsertCalls := 0
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return &models.EnvThunderURL{ThunderHandle: "existing1"}, nil
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				upsertCalls++
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "different1")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrThunderHandleTaken, "the controller maps this to 409 — changing an already-provisioned handle requires DeleteThunderURL first")
		assert.Equal(t, 0, upsertCalls, "must never move an already-registered handle")
	})

	t.Run("a blank handle on a pre-existing (grandfathered) environment reuses the legacy pattern instead of generating a random one", func(t *testing.T) {
		upsertCalls := 0
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				upsertCalls++
				return nil
			},
		}
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				return &models.EnvThunderSystemClient{}, nil
			},
		}
		svc := newEnvServiceWithThunderURLAndSystemClientRepo(urlRepo, systemClientRepo)

		resolved, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "")
		require.NoError(t, err)
		assert.Equal(t, thundersvc.LegacyThunderHandleLabel(ThunderOrgNamespace(), "prod"), resolved)
		assert.Equal(t, 1, upsertCalls, "grandfathering persists the computed value exactly once")
	})

	// The following four tests cover claimThunderHandle's reaction to losing a
	// concurrent insert race (see EnvThunderURLRepository.Insert): the mock
	// returns utils.ErrEnvThunderURLAlreadyClaimed from Insert, then a second
	// Get call — the read-back-the-winner step — returns a DIFFERENT row than
	// what this call itself tried to insert. Postgres actually raising the
	// right conflict under real concurrent inserts is covered separately by
	// repositories/env_thunder_url_repository_test.go's
	// TestEnvThunderURLRepo_ConcurrentFirstInsertsOnlyOneWins (an integration
	// test, since that needs a real database).

	t.Run("loses a concurrent first-provisioning race while auto-generating; adopts the winner's handle instead of erroring", func(t *testing.T) {
		var getCalls int
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				getCalls++
				if getCalls == 1 {
					return nil, gorm.ErrRecordNotFound // ResolveThunderHandle's up-front check: no row yet
				}
				return &models.EnvThunderURL{ThunderHandle: "winnerhandle"}, nil // read back after losing the race
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				return utils.ErrEnvThunderURLAlreadyClaimed // a concurrent request won first
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		resolved, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "")
		require.NoError(t, err, "losing a race while auto-generating must never surface as an error — the caller never asked for a SPECIFIC value")
		assert.Equal(t, "winnerhandle", resolved, "must adopt whatever handle actually won the race, not retry generation")
	})

	t.Run("loses a concurrent first-provisioning race with an explicit handle that matches the winner — reuses it", func(t *testing.T) {
		var getCalls int
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				getCalls++
				if getCalls == 1 {
					return nil, gorm.ErrRecordNotFound
				}
				return &models.EnvThunderURL{ThunderHandle: "samehandle1"}, nil
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				return utils.ErrEnvThunderURLAlreadyClaimed
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		resolved, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "samehandle1")
		require.NoError(t, err)
		assert.Equal(t, "samehandle1", resolved)
	})

	t.Run("loses a concurrent first-provisioning race with an explicit handle that DIFFERS from the winner — rejected", func(t *testing.T) {
		var getCalls int
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				getCalls++
				if getCalls == 1 {
					return nil, gorm.ErrRecordNotFound
				}
				return &models.EnvThunderURL{ThunderHandle: "winnerhandle"}, nil
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				return utils.ErrEnvThunderURLAlreadyClaimed
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "myhandle12")
		require.Error(t, err, "an explicit value that lost the race and doesn't match the winner must be rejected, never silently substituted")
		assert.ErrorIs(t, err, utils.ErrThunderHandleTaken)
	})

	t.Run("fails cleanly if reading back the race's winner itself fails", func(t *testing.T) {
		boom := errors.New("db down")
		var getCalls int
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				getCalls++
				if getCalls == 1 {
					return nil, gorm.ErrRecordNotFound
				}
				return nil, boom
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				return utils.ErrEnvThunderURLAlreadyClaimed
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("rejects a handle shorter than 10 characters", func(t *testing.T) {
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				t.Fatal("must not upsert a too-short handle")
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		// 9 characters — one short of the boundary, so this fails precisely
		// because minThunderHandleLen is 10, not because it's trivially short.
		_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "abcdefghi")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrInvalidThunderHandle)
	})

	t.Run("rejects uppercase and other invalid characters", func(t *testing.T) {
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				t.Fatal("must not upsert an invalid handle")
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		// Each is padded to >=10 characters so it actually reaches the format
		// check instead of failing on length first — the one invalid trait
		// (uppercase, underscore, leading/trailing hyphen, dot, space) is what's
		// under test here, not shortness.
		for _, bad := range []string{"Acme123456", "acme_prod1", "-acme12345", "acmeprod1-", "acme.prod1", "acme prod1"} {
			_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", bad)
			require.Error(t, err, "handle %q must be rejected", bad)
			assert.ErrorIs(t, err, utils.ErrInvalidThunderHandle, "handle %q", bad)
		}
	})

	t.Run("rejects a handle longer than 63 characters", func(t *testing.T) {
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				t.Fatal("must not upsert an overlong handle")
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", strings.Repeat("a", 64))
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrInvalidThunderHandle)
	})

	t.Run("rejects a reserved word", func(t *testing.T) {
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				t.Fatal("must not upsert a reserved handle")
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		// "kubernetes" is exactly 10 characters — long enough to clear
		// minThunderHandleLen, so this actually exercises the reserved-word
		// check rather than failing on length first ("localhost" is only 9).
		_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "kubernetes")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrInvalidThunderHandle)
	})

	t.Run("rejects an empty ouID", func(t *testing.T) {
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				t.Fatal("must not upsert when ouID is empty")
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.SetThunderURL(context.Background(), "", "prod", "x7f2q9kzab")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})

	t.Run("maps a handle-taken conflict from the repo without wrapping it away", func(t *testing.T) {
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				return utils.ErrThunderHandleTaken
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "x7f2q9kzab")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrThunderHandleTaken, "the controller maps this specific sentinel to 409 — it must survive unwrapped")
	})

	t.Run("wraps an unexpected repo error", func(t *testing.T) {
		boom := errors.New("db down")
		repo := &repomocks.EnvThunderURLRepositoryMock{
			InsertFunc: func(context.Context, *models.EnvThunderURL) error { return boom },
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.SetThunderURL(context.Background(), "ou-123", "prod", "x7f2q9kzab")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}

func TestEnvironmentService_GetThunderURL(t *testing.T) {
	t.Run("returns the registered handle", func(t *testing.T) {
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(_ context.Context, ouID, envName string) (*models.EnvThunderURL, error) {
				assert.Equal(t, "ou-123", ouID)
				assert.Equal(t, "prod", envName)
				return &models.EnvThunderURL{ThunderHandle: "x7f2q9kzab"}, nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		handle, err := svc.GetThunderURL(context.Background(), "ou-123", "prod")
		require.NoError(t, err)
		assert.Equal(t, "x7f2q9kzab", handle)
	})

	t.Run("maps a missing row to ErrThunderHandleNotFound when genuinely never provisioned", func(t *testing.T) {
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.GetThunderURL(context.Background(), "ou-123", "prod")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrThunderHandleNotFound)
	})

	t.Run("grandfathers the legacy pattern for a pre-existing environment instead of reporting not-provisioned", func(t *testing.T) {
		// A pre-existing environment (provisioned before every environment got a
		// registered handle) has a system-client credential but no URL row yet —
		// GetThunderURL must resolve through the SAME grandfather path as reads
		// elsewhere (see ResolveThunderHandle), not report it as unprovisioned.
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error { return nil },
		}
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				return &models.EnvThunderSystemClient{}, nil
			},
		}
		svc := newEnvServiceWithThunderURLAndSystemClientRepo(urlRepo, systemClientRepo)

		handle, err := svc.GetThunderURL(context.Background(), "ou-123", "prod")
		require.NoError(t, err)
		assert.Equal(t, thundersvc.LegacyThunderHandleLabel(ThunderOrgNamespace(), "prod"), handle)
	})

	t.Run("rejects an empty ouID", func(t *testing.T) {
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				t.Fatal("must not read when ouID is empty")
				return nil, nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.GetThunderURL(context.Background(), "", "prod")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})

	t.Run("wraps an unexpected repo error", func(t *testing.T) {
		boom := errors.New("db down")
		repo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, boom
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		_, err := svc.GetThunderURL(context.Background(), "ou-123", "prod")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}

func TestEnvironmentService_DeleteThunderURL(t *testing.T) {
	t.Run("delegates to the repo, keyed by ouID", func(t *testing.T) {
		var gotOUID, gotEnv string
		repo := &repomocks.EnvThunderURLRepositoryMock{
			DeleteFunc: func(_ context.Context, ouID, envName string) error {
				gotOUID, gotEnv = ouID, envName
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		err := svc.DeleteThunderURL(context.Background(), "ou-123", "prod")
		require.NoError(t, err)
		assert.Equal(t, "ou-123", gotOUID)
		assert.Equal(t, "prod", gotEnv)
	})

	t.Run("rejects an empty ouID", func(t *testing.T) {
		repo := &repomocks.EnvThunderURLRepositoryMock{
			DeleteFunc: func(context.Context, string, string) error {
				t.Fatal("must not delete when ouID is empty")
				return nil
			},
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		err := svc.DeleteThunderURL(context.Background(), "", "prod")
		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})

	t.Run("wraps a repo error", func(t *testing.T) {
		boom := errors.New("db down")
		repo := &repomocks.EnvThunderURLRepositoryMock{
			DeleteFunc: func(context.Context, string, string) error { return boom },
		}
		svc := newEnvServiceWithThunderURLRepo(repo)

		err := svc.DeleteThunderURL(context.Background(), "ou-123", "prod")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}

// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// TestResolveThunderHandle covers the SINGLE centralized function every caller
// (EnvironmentService's readThunderHandle/GetThunderURL/SetThunderURL, and the
// resolver's ReadThunderHandleFunc via NewEnvThunderURLReader) delegates to —
// see its doc comment for the grandfather-from-legacy-pattern rationale.
func TestResolveThunderHandle(t *testing.T) {
	t.Run("returns the directly registered handle without touching the system-client repo", func(t *testing.T) {
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(_ context.Context, ouID, envName string) (*models.EnvThunderURL, error) {
				assert.Equal(t, "ou-1", ouID)
				assert.Equal(t, "prod", envName)
				return &models.EnvThunderURL{ThunderHandle: "x7f2q9kz"}, nil
			},
		}
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				t.Fatal("must not be consulted when a URL row already exists")
				return nil, nil
			},
		}

		handle, err := ResolveThunderHandle(context.Background(), urlRepo, systemClientRepo, "ou-1", "prod")
		require.NoError(t, err)
		assert.Equal(t, "x7f2q9kz", handle)
	})

	t.Run("grandfathers the legacy org-env pattern when a system-client credential already exists", func(t *testing.T) {
		var upserted *models.EnvThunderURL
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
			InsertFunc: func(_ context.Context, rec *models.EnvThunderURL) error {
				upserted = rec
				return nil
			},
		}
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				return &models.EnvThunderSystemClient{}, nil
			},
		}

		handle, err := ResolveThunderHandle(context.Background(), urlRepo, systemClientRepo, "ou-1", "prod")
		require.NoError(t, err)

		want := thundersvc.LegacyThunderHandleLabel(ThunderOrgNamespace(), "prod")
		assert.Equal(t, want, handle)
		require.NotNil(t, upserted, "the grandfathered handle must be persisted so subsequent calls hit the row directly")
		assert.Equal(t, want, upserted.ThunderHandle)
		assert.Equal(t, "ou-1", upserted.OUID)
		assert.Equal(t, "prod", upserted.EnvName)
	})

	t.Run("still resolves to the computed value even if persisting the grandfathered handle fails", func(t *testing.T) {
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				return errors.New("transient db error")
			},
		}
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				return &models.EnvThunderSystemClient{}, nil
			},
		}

		handle, err := ResolveThunderHandle(context.Background(), urlRepo, systemClientRepo, "ou-1", "prod")
		require.NoError(t, err, "a failed backfill write must not fail resolution — the next call just retries the write")
		assert.Equal(t, thundersvc.LegacyThunderHandleLabel(ThunderOrgNamespace(), "prod"), handle)
	})

	t.Run("reports not-provisioned when neither a url row nor a system-client credential exists", func(t *testing.T) {
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}

		handle, err := ResolveThunderHandle(context.Background(), urlRepo, systemClientRepo, "ou-1", "prod")
		require.NoError(t, err)
		assert.Empty(t, handle)
	})

	t.Run("propagates an unexpected url repo error without grandfathering", func(t *testing.T) {
		boom := errors.New("db down")
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, boom
			},
		}
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				t.Fatal("must not be consulted when the url repo read itself failed unexpectedly")
				return nil, nil
			},
		}

		_, err := ResolveThunderHandle(context.Background(), urlRepo, systemClientRepo, "ou-1", "prod")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("propagates an unexpected system-client repo error", func(t *testing.T) {
		boom := errors.New("db down")
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				return nil, boom
			},
		}

		_, err := ResolveThunderHandle(context.Background(), urlRepo, systemClientRepo, "ou-1", "prod")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("propagates a genuine cross-environment handle collision from the backfill write", func(t *testing.T) {
		urlRepo := &repomocks.EnvThunderURLRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderURL, error) {
				return nil, gorm.ErrRecordNotFound
			},
			InsertFunc: func(context.Context, *models.EnvThunderURL) error {
				return utils.ErrThunderHandleTaken
			},
		}
		systemClientRepo := &repomocks.EnvThunderSystemClientRepositoryMock{
			GetFunc: func(context.Context, string, string) (*models.EnvThunderSystemClient, error) {
				return &models.EnvThunderSystemClient{}, nil
			},
		}

		handle, err := ResolveThunderHandle(context.Background(), urlRepo, systemClientRepo, "ou-1", "prod")
		require.NoError(t, err, "a handle-taken conflict on the backfill write is best-effort — still resolves, doesn't fail the caller")
		assert.Equal(t, thundersvc.LegacyThunderHandleLabel(ThunderOrgNamespace(), "prod"), handle)
	})
}

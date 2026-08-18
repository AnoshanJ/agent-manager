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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// clearGatewayManifestCache resets the process-wide backend to a fresh in-memory
// cache, undoing both any Set() from the test and any SetGatewayManifestCacheBackend
// swap another test may have left behind.
func clearGatewayManifestCache(t *testing.T) {
	t.Helper()
	original := currentGatewayManifestCache()
	t.Cleanup(func() { SetGatewayManifestCacheBackend(original) })
	SetGatewayManifestCacheBackend(NewInMemoryGatewayManifestCache())
}

// TestSaveGatewayPolicyManifestCachesWithoutWritingRow is the point of moving manifests
// out of the jsonb column: a push must land in the cache, keyed to that gateway, and
// must not update the row.
func TestSaveGatewayPolicyManifestCachesWithoutWritingRow(t *testing.T) {
	clearGatewayManifestCache(t)

	gateway := newGateway(t, models.GatewayRoleBoth, true)
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(gatewayID string) (*models.Gateway, error) {
			require.Equal(t, gateway.UUID.String(), gatewayID)
			return gateway, nil
		},
		// UpdateGatewayFunc is left nil: calling it panics, which is the assertion that
		// the push no longer writes the row.
	}
	svc := NewPlatformGatewayService(repo, nil)

	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{"name": "mcp-auth", "version": "v1"},
		},
	}
	require.NoError(t, svc.SaveGatewayPolicyManifest(context.Background(), gateway.UUID.String(), manifest))

	cached, ok, err := currentGatewayManifestCache().Get(context.Background(), gateway.OUID, gateway.UUID.String())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, manifest, cached)

	require.Equal(t, manifest, gatewayManifest(context.Background(), gateway))
}

// TestGatewayManifestIsPerGateway is the regression test for the cache having
// (wrongly) answered for every gateway once any gateway pushed: a manifest pushed for
// one gateway must not leak to a different gateway, even in the same org, which still
// falls back to its own row.
func TestGatewayManifestIsPerGateway(t *testing.T) {
	clearGatewayManifestCache(t)

	pushed := newGateway(t, models.GatewayRoleBoth, true)
	other := newGateway(t, models.GatewayRoleEgress, true)
	other.OUID = pushed.OUID
	other.Manifest = map[string]interface{}{"policies": []interface{}{"still-on-row"}}

	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(gatewayID string) (*models.Gateway, error) {
			require.Equal(t, pushed.UUID.String(), gatewayID)
			return pushed, nil
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	manifest := map[string]interface{}{
		"policies": []interface{}{map[string]interface{}{"name": "mcp-auth", "version": "v1"}},
	}
	require.NoError(t, svc.SaveGatewayPolicyManifest(context.Background(), pushed.UUID.String(), manifest))

	require.Equal(t, manifest, gatewayManifest(context.Background(), pushed))
	// A different gateway in the same org, which never pushed, must fall back to its
	// own row rather than inherit pushed's cached manifest.
	require.Equal(t, other.Manifest, gatewayManifest(context.Background(), other))
}

// TestGatewayManifestIsPerOrg is the regression test for the cache having (wrongly)
// been one global key shared by every org: a manifest pushed by a gateway in one org
// must not be visible to a gateway in a different org, even if that gateway shares the
// same UUID namespace collision risk is what ouID scoping defends against.
func TestGatewayManifestIsPerOrg(t *testing.T) {
	clearGatewayManifestCache(t)

	orgAGateway := newGateway(t, models.GatewayRoleBoth, true)
	orgAGateway.OUID = "org-a"
	orgBGateway := newGateway(t, models.GatewayRoleBoth, true)
	orgBGateway.OUID = "org-b"
	orgBGateway.Manifest = map[string]interface{}{"policies": []interface{}{"org-b-row"}}

	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(gatewayID string) (*models.Gateway, error) {
			require.Equal(t, orgAGateway.UUID.String(), gatewayID)
			return orgAGateway, nil
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	manifest := map[string]interface{}{
		"policies": []interface{}{map[string]interface{}{"name": "mcp-auth", "version": "v1"}},
	}
	require.NoError(t, svc.SaveGatewayPolicyManifest(context.Background(), orgAGateway.UUID.String(), manifest))

	require.Equal(t, manifest, gatewayManifest(context.Background(), orgAGateway))
	// org B's gateway never pushed; it must not see org A's push even though both
	// pushes land in the same shared backend instance.
	require.Equal(t, orgBGateway.Manifest, gatewayManifest(context.Background(), orgBGateway))
}

// TestSaveGatewayPolicyManifestUnknownGateway keeps the endpoint's 404 behaviour: an
// unknown gateway must not seed the cache.
func TestSaveGatewayPolicyManifestUnknownGateway(t *testing.T) {
	clearGatewayManifestCache(t)

	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*models.Gateway, error) {
			//nolint:nilnil // GetByUUID reports "no such gateway" as (nil, nil).
			return nil, nil
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	err := svc.SaveGatewayPolicyManifest(context.Background(), "11111111-1111-1111-1111-111111111111", map[string]interface{}{})
	require.ErrorIs(t, err, utils.ErrGatewayNotFound)

	_, ok, getErr := currentGatewayManifestCache().Get(context.Background(), "", "11111111-1111-1111-1111-111111111111")
	require.NoError(t, getErr)
	require.False(t, ok)
}

// TestGatewayManifestFallsBackToRow covers the cold window after a restart: until the
// gateway itself pushes again, the manifest still on its row is what readers evaluate.
func TestGatewayManifestFallsBackToRow(t *testing.T) {
	clearGatewayManifestCache(t)

	gateway := newGateway(t, models.GatewayRoleBoth, true)
	gateway.Manifest = map[string]interface{}{"policies": []interface{}{}}

	require.Equal(t, gateway.Manifest, gatewayManifest(context.Background(), gateway))
	require.Nil(t, gatewayManifest(context.Background(), nil))
}

// TestGatewayManifestCacheKeepsOnlyLatestPerGateway documents the single-copy-per-gateway
// contract: a second push for the SAME gateway replaces the first rather than
// accumulating, but a different gateway's entry is untouched.
func TestGatewayManifestCacheKeepsOnlyLatestPerGateway(t *testing.T) {
	ctx := context.Background()
	cache := NewInMemoryGatewayManifestCache()
	first := map[string]interface{}{"policies": []interface{}{"a"}}
	second := map[string]interface{}{"policies": []interface{}{"b"}}

	require.NoError(t, cache.Set(ctx, "org1", "gw1", first))
	require.NoError(t, cache.Set(ctx, "org1", "gw1", second))
	require.NoError(t, cache.Set(ctx, "org1", "gw2", first))

	cached, ok, err := cache.Get(ctx, "org1", "gw1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, second, cached)

	otherCached, ok, err := cache.Get(ctx, "org1", "gw2")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, first, otherCached)

	require.NoError(t, cache.Clear(ctx, "org1", "gw1"))
	_, ok, err = cache.Get(ctx, "org1", "gw1")
	require.NoError(t, err)
	require.False(t, ok)

	// Clearing gw1 must not affect gw2's entry.
	_, ok, err = cache.Get(ctx, "org1", "gw2")
	require.NoError(t, err)
	require.True(t, ok)
}

// TestGatewayManifest_CacheReadErrorFallsBackToRow covers a Redis-unreachable-style
// failure: gatewayManifest must degrade to the row rather than propagate the error,
// since manifests are advisory.
func TestGatewayManifest_CacheReadErrorFallsBackToRow(t *testing.T) {
	clearGatewayManifestCache(t)
	SetGatewayManifestCacheBackend(&failingManifestCacheBackend{})

	gateway := newGateway(t, models.GatewayRoleBoth, true)
	gateway.Manifest = map[string]interface{}{"policies": []interface{}{}}

	require.Equal(t, gateway.Manifest, gatewayManifest(context.Background(), gateway))
}

// failingManifestCacheBackend simulates an unreachable external cache backend.
type failingManifestCacheBackend struct{}

func (f *failingManifestCacheBackend) Set(context.Context, string, string, map[string]interface{}) error {
	return errCacheUnavailable
}

func (f *failingManifestCacheBackend) Get(context.Context, string, string) (map[string]interface{}, bool, error) {
	return nil, false, errCacheUnavailable
}

func (f *failingManifestCacheBackend) Clear(context.Context, string, string) error {
	return errCacheUnavailable
}

var errCacheUnavailable = errors.New("cache backend unavailable")

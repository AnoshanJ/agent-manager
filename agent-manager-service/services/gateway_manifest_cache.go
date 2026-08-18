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
	"sync"
	"sync/atomic"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// GatewayManifestCacheBackend holds the latest policy manifest pushed by each gateway,
// scoped by (org, gateway) so one org's or one gateway's push can never answer for
// another's: gateways drift (rolling upgrades, heterogeneous fleets, a gateway that
// simply doesn't support a policy another one does — see gatewayHasMCPIdentityPolicies
// and intersectLLMPolicies, both of which depend on seeing each gateway's own manifest),
// and orgs are tenants that must never see each other's gateway state.
//
// Manifests are large and every gateway re-pushes its whole manifest on a fixed
// heartbeat, so persisting them wrote a multi-KB jsonb blob per push for data that is
// only ever read to answer "which policies does this gateway advertise?". The cache
// keeps exactly one copy per (org, gateway): the most recent push replaces the previous
// one for that gateway, no history and no TTL. Deleting a gateway therefore leaves its
// cache entry orphaned but harmless — nothing else keys off it.
//
// Two implementations exist:
//   - InMemoryGatewayManifestCache: process-local, the default. A restarted (or newly
//     scaled-up) replica starts empty and refills on the next push from any gateway.
//   - RedisGatewayManifestCache: shared across replicas. Required in HA deployments —
//     an in-process cache is inconsistent there, since each replica only ever sees the
//     pushes routed to it, and readers on other replicas would see a stale or empty
//     cache for gateways that never pushed to them.
//
// Manifests are advisory (they gate policy pickers, not traffic), so a cold window on
// either backend is tolerable.
type GatewayManifestCacheBackend interface {
	// Set replaces the cached manifest for one (ouID, gatewayID) with the one just pushed.
	Set(ctx context.Context, ouID, gatewayID string, manifest map[string]interface{}) error
	// Get returns the cached manifest for one (ouID, gatewayID), and whether that gateway
	// has pushed since startup (in-memory backend) or since the key was last cleared/evicted
	// (Redis backend).
	Get(ctx context.Context, ouID, gatewayID string) (map[string]interface{}, bool, error)
	// Clear drops the cached manifest for one (ouID, gatewayID), so reads fall back to the
	// gateway row again.
	Clear(ctx context.Context, ouID, gatewayID string) error
}

// manifestCacheKey builds the composite key a manifest is stored under. ouID is included
// even though gatewayID alone is already globally unique, so a cache dump or key scan is
// human-auditable per tenant and stays consistent with how every other cache/lookup in
// this codebase is scoped.
func manifestCacheKey(ouID, gatewayID string) string {
	return ouID + "\x00" + gatewayID
}

// InMemoryGatewayManifestCache is the process-local GatewayManifestCacheBackend. Safe
// as the default for a single replica; under HA, each replica gets its own independent
// copy, which readers on other replicas cannot see — use RedisGatewayManifestCache there.
type InMemoryGatewayManifestCache struct {
	mu        sync.RWMutex
	manifests map[string]map[string]interface{}
}

// NewInMemoryGatewayManifestCache creates an empty in-process manifest cache.
func NewInMemoryGatewayManifestCache() *InMemoryGatewayManifestCache {
	return &InMemoryGatewayManifestCache{
		manifests: make(map[string]map[string]interface{}),
	}
}

// Set implements GatewayManifestCacheBackend.
func (c *InMemoryGatewayManifestCache) Set(_ context.Context, ouID, gatewayID string, manifest map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.manifests[manifestCacheKey(ouID, gatewayID)] = manifest
	return nil
}

// Get implements GatewayManifestCacheBackend.
func (c *InMemoryGatewayManifestCache) Get(_ context.Context, ouID, gatewayID string) (map[string]interface{}, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	manifest, ok := c.manifests[manifestCacheKey(ouID, gatewayID)]
	return manifest, ok, nil
}

// Clear implements GatewayManifestCacheBackend.
func (c *InMemoryGatewayManifestCache) Clear(_ context.Context, ouID, gatewayID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.manifests, manifestCacheKey(ouID, gatewayID))
	return nil
}

// gatewayManifestCache is the process-wide manifest cache backend. It is a package
// var — rather than constructor-injected into every caller — because the writer (the
// gateway manifest push endpoint, via PlatformGatewayService) and the readers (the MCP
// and LLM policy pickers, package-level functions in this package) are constructed
// independently and neither currently threads a shared dependency between them.
// An atomic.Pointer (rather than a plain var) because SetGatewayManifestCacheBackend and
// every Get/Set below run concurrently with no other synchronization between them; in
// production SetGatewayManifestCacheBackend only ever runs once, before the server starts
// accepting requests, but tests swap backends directly, and this makes that swap race-free
// under `go test -race` regardless of test ordering.
var gatewayManifestCache = func() *atomic.Pointer[GatewayManifestCacheBackend] {
	p := &atomic.Pointer[GatewayManifestCacheBackend]{}
	def := GatewayManifestCacheBackend(NewInMemoryGatewayManifestCache())
	p.Store(&def)
	return p
}()

// SetGatewayManifestCacheBackend swaps the process-wide manifest cache backend. Called
// once at startup by wiring, based on config.GatewayManifestCache.Backend.
func SetGatewayManifestCacheBackend(backend GatewayManifestCacheBackend) {
	gatewayManifestCache.Store(&backend)
}

// currentGatewayManifestCache returns the active backend.
func currentGatewayManifestCache() GatewayManifestCacheBackend {
	return *gatewayManifestCache.Load()
}

// gatewayManifest returns the manifest to evaluate for a gateway: the cached copy once
// that gateway has pushed since this replica started (or, with the Redis backend, since
// any replica last received a push for it), otherwise the manifest still on its row. The
// fallback covers the cold window (and rows written before manifests moved out of the
// database); it stops being used once that gateway's first push lands. A cache read
// failure (e.g. Redis unreachable) degrades to the same row fallback rather than erroring
// the whole policy listing — manifests are advisory, so a stale read beats a hard failure.
func gatewayManifest(ctx context.Context, gateway *models.Gateway) map[string]interface{} {
	if gateway == nil {
		return nil
	}
	manifest, ok, err := currentGatewayManifestCache().Get(ctx, gateway.OUID, gateway.UUID.String())
	if err == nil && ok {
		return manifest
	}

	return gateway.Manifest
}

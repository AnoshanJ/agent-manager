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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// gatewayManifestCacheRedisKeyPrefix namespaces one key per (ouID, gatewayID) so every
// replica agrees on which manifest belongs to which gateway, regardless of which
// replica a given gateway's heartbeat happened to land on.
const gatewayManifestCacheRedisKeyPrefix = "amp:gateway-manifest-cache:v1"

// gatewayManifestCacheRedisKey builds the per-gateway Redis key. ouID and gatewayID are
// both server-generated identifiers (never raw user input), so no escaping is needed.
func gatewayManifestCacheRedisKey(ouID, gatewayID string) string {
	return gatewayManifestCacheRedisKeyPrefix + ":" + ouID + ":" + gatewayID
}

// RedisGatewayManifestCache is the Redis-backed GatewayManifestCacheBackend, for HA
// deployments where a process-local cache would leave replicas disagreeing about which
// policies gateways report (each replica only observes the manifest pushes routed to
// it by the load balancer).
type RedisGatewayManifestCache struct {
	client *redis.Client
}

// NewRedisGatewayManifestCache creates a Redis-backed manifest cache from config. It
// does not itself connect or ping — the first Get/Set surfaces any connectivity issue,
// consistent with how other lazily-dialed clients in this codebase behave.
func NewRedisGatewayManifestCache(cfg config.GatewayManifestCacheRedisConfig) *RedisGatewayManifestCache {
	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.TLSEnabled {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return &RedisGatewayManifestCache{client: redis.NewClient(opts)}
}

// Set implements GatewayManifestCacheBackend.
func (c *RedisGatewayManifestCache) Set(ctx context.Context, ouID, gatewayID string, manifest map[string]interface{}) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal gateway manifest for cache: %w", err)
	}
	// No TTL: matches the in-memory backend's no-expiry contract — the cache holds
	// whatever was last pushed until the next push replaces it, indefinitely.
	if err := c.client.Set(ctx, gatewayManifestCacheRedisKey(ouID, gatewayID), data, 0).Err(); err != nil {
		return fmt.Errorf("failed to write gateway manifest cache: %w", err)
	}
	return nil
}

// Get implements GatewayManifestCacheBackend.
func (c *RedisGatewayManifestCache) Get(ctx context.Context, ouID, gatewayID string) (map[string]interface{}, bool, error) {
	data, err := c.client.Get(ctx, gatewayManifestCacheRedisKey(ouID, gatewayID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read gateway manifest cache: %w", err)
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal cached gateway manifest: %w", err)
	}
	return manifest, true, nil
}

// Clear implements GatewayManifestCacheBackend.
func (c *RedisGatewayManifestCache) Clear(ctx context.Context, ouID, gatewayID string) error {
	if err := c.client.Del(ctx, gatewayManifestCacheRedisKey(ouID, gatewayID)).Err(); err != nil {
		return fmt.Errorf("failed to clear gateway manifest cache: %w", err)
	}
	return nil
}

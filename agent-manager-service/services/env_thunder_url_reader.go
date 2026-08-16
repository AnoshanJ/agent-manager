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

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ResolveThunderHandle is the SINGLE place every caller resolves an
// environment's env-Thunder URL handle — EnvironmentService's own
// readThunderHandle/GetThunderURL/SetThunderURL and the resolver's injected
// ReadThunderHandleFunc (via NewEnvThunderURLReader below) all delegate here so
// this logic can never drift apart between call sites.
//
// A missing row is NOT automatically "never provisioned": if a
// env_thunder_system_client credential already exists for (ouID, envName), this
// environment was provisioned BEFORE every environment got a registered handle
// (or a later registration step was lost) — its Thunder Helm release already
// has the legacy "<org>-<env>" pattern (see thundersvc.LegacyThunderHandleLabel)
// baked into its immutable issuer/publicUrl. This grandfathers that exact value
// in as the environment's handle (persisting it so subsequent calls hit the
// normal row directly), preserving the address it's actually still answering on
// with zero re-provisioning. Only when NEITHER row exists is the environment
// genuinely never provisioned, which returns ("", nil).
func ResolveThunderHandle(
	ctx context.Context,
	urlRepo repositories.EnvThunderURLRepository,
	systemClientRepo repositories.EnvThunderSystemClientRepository,
	ouID, envName string,
) (string, error) {
	row, err := urlRepo.Get(ctx, ouID, envName)
	if err == nil {
		return row.ThunderHandle, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("read env-thunder url handle for %s/%s: %w", ouID, envName, err)
	}

	if _, scErr := systemClientRepo.Get(ctx, ouID, envName); scErr != nil {
		if errors.Is(scErr, gorm.ErrRecordNotFound) {
			return "", nil // genuinely never provisioned
		}
		return "", fmt.Errorf("read env-thunder system-client for %s/%s: %w", ouID, envName, scErr)
	}

	// legacy is a pure function of (org namespace, envName), so it's identical
	// no matter which concurrent caller computes it — Insert (never an upsert,
	// see its doc comment) is still used rather than a blind write:
	//   - nil: this call won the claim; legacy is now the persisted value.
	//   - ErrEnvThunderURLAlreadyClaimed: a concurrent grandfather attempt for
	//     this SAME environment already persisted first — read back and adopt
	//     whatever it wrote (a self-race, not a real conflict).
	//   - ErrThunderHandleTaken: a DIFFERENT (ouID, envName) already owns this
	//     exact string. ThunderOrgNamespace() is config-pinned (not per-OU), so
	//     two different orgs each grandfathering a same-named environment (e.g.
	//     both have a "production") compute the identical legacy value — this
	//     is a real cross-tenant collision, not a rare edge case. Propagate the
	//     error rather than handing this caller another org's Thunder issuer,
	//     token, and JWKS endpoints.
	//   - any other error: a transient failure (e.g. the DB is down) —
	//     propagate it; the next call retries the whole resolution from scratch.
	legacy := thundersvc.LegacyThunderHandleLabel(ThunderOrgNamespace(), envName)
	grandfathered := &models.EnvThunderURL{OUID: ouID, EnvName: envName, ThunderHandle: legacy}
	if err := urlRepo.Insert(ctx, grandfathered); err != nil {
		if errors.Is(err, utils.ErrEnvThunderURLAlreadyClaimed) {
			winner, getErr := urlRepo.Get(ctx, ouID, envName)
			if getErr != nil {
				return "", fmt.Errorf("read env-thunder url handle for %s/%s after a grandfather claim race: %w", ouID, envName, getErr)
			}
			return winner.ThunderHandle, nil
		}
		return "", fmt.Errorf("failed to persist grandfathered thunder url handle for %s/%s: %w", ouID, envName, err)
	}
	return legacy, nil
}

// NewEnvThunderURLReader builds the resolver's DB-backed handle reader —
// ResolveThunderHandle widened to thundersvc.ReadThunderHandleFunc's shape.
// Lives in services (not wiring) for the same reason as
// NewEnvThunderSecretReader: app.Run's provisioning factory shares it without a cycle.
func NewEnvThunderURLReader(
	urlRepo repositories.EnvThunderURLRepository,
	systemClientRepo repositories.EnvThunderSystemClientRepository,
) thundersvc.ReadThunderHandleFunc {
	return func(ctx context.Context, ouID, envName string) (string, error) {
		return ResolveThunderHandle(ctx, urlRepo, systemClientRepo, ouID, envName)
	}
}

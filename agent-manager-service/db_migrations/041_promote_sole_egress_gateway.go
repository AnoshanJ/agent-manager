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

package dbmigrations

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Restore ingress coverage that migration 037 removed.
//
// 037's ai -> egress arm assumed 'ai' was only ever a second gateway installed beside
// a 'regular' one, which holds for the OSS paths but not for provisioners that register
// an environment's ONLY gateway as 'ai'. Those environments came out of 037 with no
// ingress-capable gateway, and resolveEnvGateways now filters on IngressGatewayRoles, so
// they can no longer be issued an inbound agent API key at all.
//
// Promote one egress gateway to 'both' in each environment that has none — which is what
// those environments effectively were before 037, whose resolver ignored the type column
// and keyed every gateway mapped to the environment. 'both' rather than 'ingress' so the
// gateway keeps carrying its existing LLM/MCP traffic. Environments that already hold an
// 'ingress' or 'both' gateway are untouched, leaving the regular+ai pair and split
// topology alone.
var migration041 = migration{
	ID: 41,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := PromoteSoleEgressGateways(tx); err != nil {
				return err
			}
			return AssertIngressCoveragePerEnvironment(tx)
		})
	},
}

// PromoteSoleEgressGateways promotes at most one gateway per environment that has no
// ingress-capable gateway.
//
// deleted_at IS NULL because raw SQL bypasses GORM's soft-delete scope. is_active is
// deliberately not filtered — it tracks WebSocket liveness and flaps, so a gateway that
// happens to be disconnected during the upgrade must still be promoted.
func PromoteSoleEgressGateways(tx *gorm.DB) error {
	const q = `
		WITH live AS (
			SELECT g.uuid,
			       g.created_at,
			       g.gateway_functionality_type AS role,
			       m.environment_uuid
			FROM gateways g
			JOIN gateway_environment_mappings m ON m.gateway_uuid = g.uuid
			WHERE g.deleted_at IS NULL
		),
		uncovered AS (
			SELECT DISTINCT l.environment_uuid
			FROM live l
			WHERE NOT EXISTS (
				SELECT 1 FROM live l2
				WHERE l2.environment_uuid = l.environment_uuid
				  AND l2.role IN ('ingress', 'both')
			)
		),
		winners AS (
			-- One candidate per environment, since the ingress slot holds exactly one.
			-- Ordered so the pick is deterministic across runs.
			SELECT uuid, environment_uuid
			FROM (
				SELECT l.uuid,
				       l.environment_uuid,
				       row_number() OVER (PARTITION BY l.environment_uuid
				                          ORDER BY l.created_at, l.uuid) AS rn
				FROM live l
				JOIN uncovered u USING (environment_uuid)
				WHERE l.role = 'egress'
			) r
			WHERE rn = 1
		),
		safe AS (
			-- The role lives on the gateway row, so a promotion applies to every
			-- environment the gateway serves. Promote only where it won in all of them,
			-- else it would breach the one-ingress cap in the ones it lost.
			SELECT wc.uuid
			FROM (SELECT uuid, count(*) AS wins FROM winners GROUP BY uuid) wc
			JOIN (SELECT gateway_uuid, count(*) AS total
			      FROM gateway_environment_mappings
			      GROUP BY gateway_uuid) mc ON mc.gateway_uuid = wc.uuid
			WHERE wc.wins = mc.total
		)
		UPDATE gateways
		SET gateway_functionality_type = 'both',
		    updated_at = now()
		WHERE uuid IN (SELECT uuid FROM safe);`

	if err := tx.Exec(q).Error; err != nil {
		return fmt.Errorf("migration 041: promoting sole egress gateways failed: %w", err)
	}
	return nil
}

// uncoveredEnvRow is one environment left without an ingress-capable gateway.
type uncoveredEnvRow struct {
	EnvironmentUUID string `gorm:"column:environment_uuid"`
	Gateways        string `gorm:"column:gateways"`
}

// AssertIngressCoveragePerEnvironment aborts the migration when an environment with
// gateways mapped to it holds none that can serve ingress. It is the lower bound to
// 037's AssertSingleIngressPerEnvironment upper bound, and the check whose absence let
// 037 leave environments silently unable to issue an inbound API key.
//
// What reaches here is a gateway shared across environments that lost the pick in one of
// them. The role is immutable once registered, so an operator has to resolve that in SQL
// either way; failing the upgrade surfaces it while it is still cheap. Environments with
// no gateways at all are not a regression and are not reported.
func AssertIngressCoveragePerEnvironment(tx *gorm.DB) error {
	const q = `
		SELECT m.environment_uuid,
		       string_agg(g.name || ' (' || g.gateway_functionality_type || ')', ', '
		                  ORDER BY g.name) AS gateways
		FROM gateway_environment_mappings m
		JOIN gateways g ON g.uuid = m.gateway_uuid
		WHERE g.deleted_at IS NULL
		  AND NOT EXISTS (
			SELECT 1
			FROM gateway_environment_mappings m2
			JOIN gateways g2 ON g2.uuid = m2.gateway_uuid
			WHERE m2.environment_uuid = m.environment_uuid
			  AND g2.deleted_at IS NULL
			  AND g2.gateway_functionality_type IN ('ingress', 'both')
		  )
		GROUP BY m.environment_uuid
		ORDER BY m.environment_uuid;`

	var rows []uncoveredEnvRow
	if err := tx.Raw(q).Scan(&rows).Error; err != nil {
		return fmt.Errorf("migration 041: ingress coverage check failed: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("migration 041 aborted: the environments below hold gateways but none that can " +
		"serve ingress ('ingress' or 'both'), so no agent in them can be issued an inbound API " +
		"key. The automatic promotion skipped these because the candidate gateway also serves an " +
		"environment that already has an ingress-capable gateway, and the role applies to the " +
		"gateway in every environment at once.\n\n" +
		"Promote one gateway per environment below, or register a new ingress gateway there. " +
		"This transaction has rolled back, so nothing has changed yet.\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "\n  environment %s: %s\n", r.EnvironmentUUID, r.Gateways)
		fmt.Fprintf(&b, "      UPDATE gateways SET gateway_functionality_type = 'both' WHERE uuid = '<pick one>';\n")
	}
	return errors.New(b.String())
}

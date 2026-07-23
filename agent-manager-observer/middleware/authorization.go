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

package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

// publisherImplicitPermissions is the fixed permission set granted to
// amp-publisher-* audience tokens, which carry no amp scopes of their own.
// Publishers may read traces and nothing else — the explicit form of the
// carve-out previously enforced by RejectPublisherAudience.
var publisherImplicitPermissions = []rbac.Permission{rbac.TraceRead}

// RequirePermission returns middleware that checks the JWTAuth-validated token
// carries the required amp scope. When rbacEnabled is false the scope check is
// skipped for ordinary tokens (zero-downtime rollout, mirroring
// agent-manager-service's RBAC_ENABLED flag) — but publisher-audience tokens
// are always confined to their implicit permission set, so the pre-authz
// publisher restrictions never regress while the kill-switch is off.
// Must run inside JWTAuth: it reads claims from the request context.
func RequirePermission(rbacEnabled bool, perm rbac.Permission) func(http.Handler) http.Handler {
	requiredScope := perm.Scope()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetTokenClaims(r.Context())
			if claims != nil && hasPublisherAudience(claims.Audience) {
				if !slices.Contains(publisherImplicitPermissions, perm) {
					writeAuthError(w, http.StatusForbidden, "insufficient permissions")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if !rbacEnabled {
				next.ServeHTTP(w, r)
				return
			}
			if claims == nil {
				writeAuthError(w, http.StatusForbidden, "missing token claims")
				return
			}
			if !slices.Contains(strings.Fields(claims.Scope), requiredScope) {
				writeAuthError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

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
	"log/slog"
	"net/http"
)

// RequireOrgMatch returns a middleware that enforces the "organization" query
// parameter matches the token's OU handle. It must be applied after JWTAuth so
// that token claims are available in the request context.
func RequireOrgMatch() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetTokenClaims(r.Context())
			if claims == nil {
				slog.Warn("RequireOrgMatch rejected", "reason", "missing token claims", "path", r.URL.Path)
				writeAuthError(w, http.StatusForbidden, "missing token claims")
				return
			}
			if claims.OuHandle == "" {
				slog.Warn("RequireOrgMatch rejected", "reason", "missing ou identity in token", "sub", claims.Sub, "path", r.URL.Path)
				writeAuthError(w, http.StatusForbidden, "missing ou identity in token")
				return
			}

			organization := r.URL.Query().Get("organization")
			if organization == "" {
				writeAuthError(w, http.StatusBadRequest, "organization is required")
				return
			}
			if organization != claims.OuHandle {
				slog.Warn("RequireOrgMatch rejected", "reason", "invalid organization access", "sub", claims.Sub, "tokenOu", claims.OuHandle, "queryOrg", organization)
				writeAuthError(w, http.StatusForbidden, "invalid organization access")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

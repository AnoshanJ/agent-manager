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
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// IsPublisherAudience reports whether the Bearer token in authHeader carries an
// amp-publisher-* audience. The token is parsed without verifying its signature
// — callers must ensure it was already validated by JWTAuth — purely to read
// the audience claim. A missing, non-Bearer, or unparseable token reports false
// (JWTAuth is responsible for rejecting those). It lets the am-obs-mcp tool
// handlers apply the publisher carve-out: the MCP go-sdk streamable transport
// hands tool handlers a context derived from the session-initializing request,
// not the current tool-call POST, so the per-call Authorization header (via
// req.Extra.Header) is the only trustworthy per-call token source there.
func IsPublisherAudience(authHeader string) bool {
	tokenString := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if tokenString == "" || tokenString == authHeader {
		return false
	}
	claims := &TokenClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(tokenString, claims); err != nil {
		return false
	}
	return hasPublisherAudience(claims.Audience)
}

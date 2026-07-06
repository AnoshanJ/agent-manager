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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func orgMatchRequest(t *testing.T, url string, claims *TokenClaims) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	if claims != nil {
		req = req.WithContext(context.WithValue(req.Context(), tokenClaimsCtxKey{}, claims))
	}
	return req
}

func TestRequireOrgMatch(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		claims     *TokenClaims
		wantStatus int
	}{
		{
			name:       "matching org passes",
			url:        "/api/v1/traces?organization=acme",
			claims:     &TokenClaims{Sub: "user1", OuHandle: "acme"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "mismatched org rejected",
			url:        "/api/v1/traces?organization=other-org",
			claims:     &TokenClaims{Sub: "user1", OuHandle: "acme"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing claims rejected",
			url:        "/api/v1/traces?organization=acme",
			claims:     nil,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing ou handle rejected",
			url:        "/api/v1/traces?organization=acme",
			claims:     &TokenClaims{Sub: "user1"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing organization param rejected",
			url:        "/api/v1/traces",
			claims:     &TokenClaims{Sub: "user1", OuHandle: "acme"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})

			rec := httptest.NewRecorder()
			RequireOrgMatch()(next).ServeHTTP(rec, orgMatchRequest(t, tt.url, tt.claims))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if wantCalled := tt.wantStatus == http.StatusOK; called != wantCalled {
				t.Fatalf("next handler called = %v, want %v", called, wantCalled)
			}
		})
	}
}

func TestValidateLocalDevExtractsOrgClaims(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &TokenClaims{
		Sub:      "user1",
		OuId:     "ou-123",
		OuHandle: "acme",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	tokenString, err := token.SignedString([]byte("dev-secret"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	claims, err := validateLocalDev(tokenString)
	if err != nil {
		t.Fatalf("validateLocalDev returned error: %v", err)
	}
	if claims.OuHandle != "acme" || claims.OuId != "ou-123" {
		t.Fatalf("claims = %+v, want OuHandle=acme OuId=ou-123", claims)
	}
}

func TestValidateLocalDevRejectsExpiredToken(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &TokenClaims{
		Sub: "user1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	})
	tokenString, err := token.SignedString([]byte("dev-secret"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	if _, err := validateLocalDev(tokenString); err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

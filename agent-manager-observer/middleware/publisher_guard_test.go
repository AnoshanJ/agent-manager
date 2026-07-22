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
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// signToken creates a throwaway HS256-signed token carrying the given
// audience claim. IsPublisherAudience only re-parses tokens without
// verifying the signature, so an arbitrary signing key is fine here.
func signToken(t *testing.T, aud string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"aud": aud})
	signed, err := token.SignedString([]byte("k"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func passThroughHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestIsPublisherAudience(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		want       bool
	}{
		{name: "publisher audience", authHeader: "Bearer " + signToken(t, "amp-publisher-acme"), want: true},
		{name: "normal audience", authHeader: "Bearer " + signToken(t, "localhost"), want: false},
		{name: "empty header", authHeader: "", want: false},
		{name: "non-bearer scheme", authHeader: "Basic dXNlcjpwYXNz", want: false},
		{name: "garbled token", authHeader: "Bearer not-a-valid-jwt", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPublisherAudience(tc.authHeader); got != tc.want {
				t.Errorf("IsPublisherAudience(%q) = %v, want %v", tc.authHeader, got, tc.want)
			}
		})
	}
}

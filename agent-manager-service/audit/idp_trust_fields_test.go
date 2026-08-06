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

package audit

import "testing"

// TestIdentityProviderTrustInputsAreRecorded pins the fields that decide which
// tokens a gateway will accept.
//
// The issuer alone does not answer that: jwksUri says where the signing keys
// are fetched from, and skipTlsVerify says whether that fetch validates the
// certificate. An operator pointing a trusted issuer at a new JWKS URI with TLS
// verification off is the change worth catching, and without these two keys the
// record showed only that the issuer was "updated".
func TestIdentityProviderTrustInputsAreRecorded(t *testing.T) {
	for _, action := range []Action{ActionGatewaySetIdentityProvider, ActionGatewayRemoveIdentityProvider} {
		e := Event{
			Action: action,
			Details: map[string]any{
				"identityProviderName": "corp-idp",
				"issuer":               "https://idp.example",
				"jwksUri":              "https://idp.example/.well-known/jwks.json",
				"skipTlsVerify":        true,
			},
		}

		redact(&e)

		for _, key := range []string{"identityProviderName", "issuer", "jwksUri", "skipTlsVerify"} {
			if _, ok := e.Details[key]; !ok {
				t.Errorf("%s: trust input %q was dropped; declare it in idpFields", action, key)
			}
		}
	}
}

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

import (
	"context"
	"testing"
)

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

// TestEnvelopeNamesAnActorTheHandlerAuthenticated is the regression for a
// coverage record that called an authenticated caller anonymous.
//
// On the internal gateway server there is no JWT: the audit Source is built
// before the handler runs, and the handler is what verifies the api-key. The
// envelope therefore recorded actorType "anonymous" with no id for a request
// that had authenticated — while the semantic record for the same surface,
// which passes an explicit Actor, named the gateway correctly. A reader could
// not tell which gateway pulled key material, which is the one question those
// records exist to answer.
func TestEnvelopeNamesAnActorTheHandlerAuthenticated(t *testing.T) {
	ctx := WithSource(context.Background(), Source{
		Surface: SurfaceInternal,
		IP:      "10.0.0.7",
		// No ActorID/ActorType/AuthMethod: this is what the middleware leaves
		// on a surface it cannot authenticate itself.
	})
	ctx, _ = NewRequestScope(ctx)

	before := BuildEvent(ctx, ActionAPIKeySync)
	if before.ActorType != ActorAnonymous || before.ActorID != "" {
		t.Fatalf("precondition: expected an unnamed actor before the handler runs, got %q/%q",
			before.ActorType, before.ActorID)
	}

	IdentifyActor(ctx, ActorGateway, "gw-42", "api-key")

	e := BuildEvent(ctx, ActionAPIKeySync)
	if e.ActorID != "gw-42" {
		t.Errorf("ActorID = %q, want the gateway the handler authenticated", e.ActorID)
	}
	if e.ActorType != ActorGateway {
		t.Errorf("ActorType = %q, want %q", e.ActorType, ActorGateway)
	}
	if e.AuthMethod != "api-key" {
		t.Errorf("AuthMethod = %q, want %q", e.AuthMethod, "api-key")
	}
}

// TestExplicitActorBeatsTheScope keeps precedence right: a semantic emit that
// names its own actor must not be overwritten by the scope.
func TestExplicitActorBeatsTheScope(t *testing.T) {
	ctx := WithSource(context.Background(), Source{Surface: SurfaceInternal})
	ctx, _ = NewRequestScope(ctx)
	IdentifyActor(ctx, ActorGateway, "gw-scope", "api-key")

	e := BuildEvent(ctx, ActionGatewayPushManifest, Actor(ActorGateway, "gw-explicit", ""))
	if e.ActorID != "gw-explicit" {
		t.Errorf("ActorID = %q; an explicit Actor option must win over the scope", e.ActorID)
	}
}

// TestJWTActorIsNotClobberedByAnEmptyScope guards the ordinary API surface:
// nothing calls IdentifyActor there, and the token subject must survive.
func TestJWTActorIsNotClobberedByAnEmptyScope(t *testing.T) {
	ctx := WithSource(context.Background(), Source{
		Surface: SurfaceAPI, ActorID: "alice@example.com", ActorType: ActorUser, AuthMethod: "jwt-bearer",
	})
	ctx, _ = NewRequestScope(ctx)

	e := BuildEvent(ctx, ActionAPIKeySync)
	if e.ActorID != "alice@example.com" || e.ActorType != ActorUser {
		t.Errorf("actor = %q/%q; an unused scope must not clear it", e.ActorType, e.ActorID)
	}
}

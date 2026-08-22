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

package authz

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// SEC-AUTHZ-001 — every guarded route rejects a token missing its scope.
//
// For each route the suite mints two tokens from the same IDP client:
//
//	deny  = every amp: scope EXCEPT the one this route requires  -> must be 403
//	allow = only the scope this route requires                   -> must NOT be 403
//
// The deny case catches an unguarded route. The allow case is the control that
// keeps the deny case honest: without it, a route that 403s for an unrelated
// reason (wrong permission constant, an extra guard) would look correctly
// protected. Both must hold for the route to be considered covered.
//
// Every request in this matrix is side-effect free by construction:
//   - GET/DELETE target deliberately absent resources (absentName) and 404.
//   - POST/PUT send NO request body, so the handler fails at JSON decode with
//     400 before touching any state.
//
// That matters because the deny case is expected to be stopped by middleware,
// but the allow case reaches the handler on purpose — and must not create,
// mutate, or delete anything when it gets there.

// absentName is used for every path parameter naming a resource. The
// "e2e-test-" prefix means the root cleanup suite would sweep it if a
// regression ever caused one of these requests to create something.
const absentName = "e2e-test-sec-absent"

// guardedRoute is one cell of the route × permission matrix.
type guardedRoute struct {
	// Method and Path form the request. Path is relative to the API base URL.
	Method string
	Path   string
	// Scopes are the scopes that satisfy this route. More than one means the
	// route is registered with OR semantics (HandleFuncWithValidationAndAnyAuthz),
	// so the deny token must lack all of them while any single one allows.
	Scopes []string
}

func (r guardedRoute) String() string {
	return fmt.Sprintf("%s %s [%s]", r.Method, r.Path, strings.Join(r.Scopes, " or "))
}

// guardedRoutes is a representative slice of the API surface: one entry per
// permission family, favouring the destructive and privilege-granting routes.
//
// It is deliberately NOT exhaustive. Exhaustiveness is enforced statically by
// TestEveryRouteIsAuthorized in agent-manager-service/api, which fails when a
// route is registered without an authz-bearing registrar method. This matrix
// checks the complementary property that the wiring actually behaves at
// runtime, on the routes where being wrong is most expensive.
func guardedRoutes(org string) []guardedRoute {
	agentBase := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", org, absentName)
	orgBase := fmt.Sprintf("/api/v1/orgs/%s", org)
	identityBase := orgBase + "/identities"

	return []guardedRoute{
		// --- Agent lifecycle -------------------------------------------------
		{http.MethodGet, agentBase, []string{"amp:agent:read"}},
		{http.MethodPost, agentBase, []string{"amp:agent:create"}},
		{http.MethodGet, agentBase + "/" + absentName, []string{"amp:agent:read"}},
		{http.MethodPut, agentBase + "/" + absentName, []string{"amp:agent:update"}},
		{http.MethodDelete, agentBase + "/" + absentName, []string{"amp:agent:delete"}},
		{http.MethodPost, agentBase + "/" + absentName + "/builds", []string{"amp:agent:build"}},
		{http.MethodPost, agentBase + "/" + absentName + "/deployments", []string{"amp:agent:deploy-non-production"}},
		{http.MethodPost, agentBase + "/" + absentName + "/promote", []string{"amp:agent:promote"}},
		{http.MethodPost, agentBase + "/" + absentName + "/deployments/state", []string{"amp:agent:suspend"}},
		{http.MethodPost, agentBase + "/" + absentName + "/publish-kind", []string{"amp:agent-kind:create"}},
		{
			http.MethodPost, agentBase + "/" + absentName + "/environments/" + absentName + "/api-keys",
			[]string{"amp:agent:api-key-manage"},
		},

		// --- Projects --------------------------------------------------------
		{http.MethodPost, orgBase + "/projects", []string{"amp:project:create"}},
		{http.MethodPut, orgBase + "/projects/" + absentName, []string{"amp:project:update"}},
		{http.MethodDelete, orgBase + "/projects/" + absentName, []string{"amp:project:delete"}},

		// --- Environments — the isolation-tier boundary ----------------------
		// Environment carries IsolationTier, from which the service derives the
		// pod runtimeClassName (runc / gVisor / Kata). Anyone who can update an
		// environment can silently downgrade every agent running in it.
		{http.MethodPost, orgBase + "/environments", []string{"amp:environment:create"}},
		{http.MethodPut, orgBase + "/environments/" + absentName, []string{"amp:environment:update"}},
		{http.MethodDelete, orgBase + "/environments/" + absentName, []string{"amp:environment:delete"}},

		// --- Identity — privilege granting -----------------------------------
		{http.MethodPost, identityBase + "/users/invite", []string{"amp:org:invite-member"}},
		{http.MethodDelete, identityBase + "/users/" + absentName, []string{"amp:org:remove-member"}},
		{http.MethodPost, identityBase + "/roles", []string{"amp:role:create"}},
		{http.MethodPut, identityBase + "/roles/" + absentName, []string{"amp:role:update"}},
		{http.MethodDelete, identityBase + "/roles/" + absentName, []string{"amp:role:delete"}},
		{http.MethodPost, identityBase + "/roles/" + absentName + "/permissions/add", []string{"amp:role:update"}},
		{http.MethodPost, identityBase + "/roles/" + absentName + "/assignees/add", []string{"amp:role:update"}},
		{http.MethodPost, identityBase + "/groups", []string{"amp:group:create"}},
		{http.MethodDelete, identityBase + "/groups/" + absentName, []string{"amp:group:delete"}},

		// --- Secrets and gateways --------------------------------------------
		{http.MethodPost, orgBase + "/git-secrets", []string{"amp:git-secret:create"}},
		{http.MethodDelete, orgBase + "/git-secrets/" + absentName, []string{"amp:git-secret:delete"}},
		{http.MethodPost, orgBase + "/gateways", []string{"amp:gateway:create"}},
		{http.MethodDelete, orgBase + "/gateways/" + absentName, []string{"amp:gateway:delete"}},
		{http.MethodPost, orgBase + "/gateways/" + absentName + "/tokens", []string{"amp:gateway:token-manage"}},

		// --- Agent kinds and catalog ------------------------------------------
		{http.MethodPut, orgBase + "/agent-kinds/" + absentName, []string{"amp:agent-kind:update"}},
		{http.MethodDelete, orgBase + "/agent-kinds/" + absentName, []string{"amp:agent-kind:delete"}},
		{http.MethodGet, orgBase + "/catalog", []string{"amp:catalog:read"}},

		// --- Evaluators --------------------------------------------------------
		{http.MethodPost, orgBase + "/evaluators/custom", []string{"amp:evaluator:create"}},
		{http.MethodDelete, orgBase + "/evaluators/custom/" + absentName, []string{"amp:evaluator:delete"}},
	}
}

var _ = Describe("SEC-AUTHZ-001: scope matrix", Label("security"), func() {
	for _, route := range guardedRoutes(framework.LoadConfig().DefaultOrg) {
		It(fmt.Sprintf("denies %s to a token without %s",
			route.Method+" "+route.Path, strings.Join(route.Scopes, "/")), func(ctx SpecContext) {
			By("calling the route with every scope EXCEPT the one it requires")
			deny := clientWithScopes(ctx, framework.ScopesExcept(route.Scopes...))
			resp := send(ctx, deny, route)
			defer resp.Body.Close()
			framework.ExpectForbidden(Default, resp, route.String())
		})

		It(fmt.Sprintf("allows %s to a token with %s",
			route.Method+" "+route.Path, route.Scopes[0]), func(ctx SpecContext) {
			By("calling the same route with only the scope it requires")
			allow := clientWithScopes(ctx, []string{route.Scopes[0]})
			resp := send(ctx, allow, route)
			defer resp.Body.Close()
			framework.ExpectNotForbidden(Default, resp, route.String())
		})
	}
})

// send issues the route's request. POST/PUT deliberately carry no body so the
// handler rejects them at JSON decode, before any state is touched.
func send(ctx SpecContext, client *framework.AMPClient, route guardedRoute) *http.Response {
	resp, err := client.DoWithContext(ctx, route.Method, route.Path, nil)
	Expect(err).NotTo(HaveOccurred(), "request failed: %s", route)
	return resp
}

// tokenCache avoids re-minting identical scope sets. Ginkgo runs a suite as N
// processes with separate memory, so this is a per-process cache; it still cuts
// the token round-trips for this matrix roughly in half.
var (
	tokenCache sync.Map
)

func clientWithScopes(ctx SpecContext, scopes []string) *framework.AMPClient {
	key := strings.Join(scopes, " ")

	if cached, ok := tokenCache.Load(key); ok {
		return cached.(*framework.AMPClient)
	}

	token, err := framework.FetchTokenWithScopes(ctx, Cfg, scopes)
	Expect(err).NotTo(HaveOccurred(), "failed to mint a token for %d scope(s)", len(scopes))

	// Guard against a silently-unreduced token. BeforeSuite proves the IDP
	// honours reduction in general; this proves it for this exact scope set,
	// so a spec can never pass because it was handed more privilege than asked.
	issued, err := framework.TokenScopes(token)
	Expect(err).NotTo(HaveOccurred(), "failed to decode the issued token")
	for _, s := range framework.AllScopes() {
		if !slices.Contains(scopes, s) {
			Expect(issued).NotTo(HaveKey(s),
				"the IDP issued %q which was not requested — this spec would be vacuous", s)
		}
	}

	c := framework.NewAMPClientWithToken(Cfg, token)
	actual, _ := tokenCache.LoadOrStore(key, c)
	return actual.(*framework.AMPClient)
}

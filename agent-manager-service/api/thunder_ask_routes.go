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

package api

import (
	"net/http"
	"strings"

	"golang.org/x/time/rate"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// thunderAskRateLimit caps thunder-ask throughput well above anything Caddy's
// own on-demand-TLS traffic could ever produce (new-cert issuance and renewals
// are rare events), while bounding how fast this reachable-from-the-public-
// internet endpoint (routed through the api host's HTTPRoute so Caddy can
// reach it — see caddyfile()) can be used to enumerate registered handles.
var thunderAskRateLimit = rate.NewLimiter(rate.Limit(5), 10)

// registerThunderAskRoute registers the endpoint Caddy's on-demand TLS "ask"
// mechanism calls before issuing a certificate for any hostname under the
// env-Thunder wildcard, including the legacy "<org>-<env>.thunder" shape
// LegacyThunderHandleLabel grandfathers pre-existing environments onto (see
// env_thunder_url_reader.go) — both are genuine registered rows, so neither is
// rejected for containing a "." the way a newly-generated handle never would.
// caddyfile() (deployments/vm/lib-vm.sh) matches the per-env gateway and
// deployed-agent wildcards BEFORE ever proxying here, so every request this
// handler actually sees is already known to be env-Thunder-shaped.
//
// Unauthenticated by design: Caddy's ask call carries no credentials, and the
// question answered here ("is this label a registered env-Thunder handle?")
// leaks nothing beyond what's already inferable by directly dialing the
// hostname and observing whether it resolves.
func registerThunderAskRoute(mux *http.ServeMux, environmentService services.EnvironmentService) {
	mux.HandleFunc("GET /internal/thunder-ask", func(w http.ResponseWriter, r *http.Request) {
		if !thunderAskRateLimit.Allow() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		domain := r.URL.Query().Get("domain")
		label, isEnvThunderHost := strings.CutSuffix(domain, "."+config.GetConfig().ThunderHostBaseDomain)
		if !isEnvThunderHost {
			w.WriteHeader(http.StatusOK)
			return
		}
		if label == "" {
			// The bare base domain is never a valid handle.
			w.WriteHeader(http.StatusForbidden)
			return
		}

		registered, err := environmentService.ThunderHandleRegistered(r.Context(), label)
		if err != nil {
			// Fail closed: an unreadable registry must never be treated as
			// authorization to issue the certificate.
			logger.GetLogger(r.Context()).Error("thunder-ask: failed to check handle registration", "handle", label, "error", err)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if !registered {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

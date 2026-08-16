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

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// registerThunderAskRoute registers the endpoint Caddy's on-demand TLS "ask"
// mechanism calls before issuing a certificate for any hostname under the
// env-Thunder wildcard (see deployments/vm/lib-vm.sh's caddyfile(), which also
// reuses this same ask endpoint for the per-env gateway and deployed-agent
// wildcards — this only ever tightens the env-Thunder case, every other
// hostname keeps today's always-allow behavior).
//
// Unauthenticated by design: Caddy's ask call carries no credentials, and the
// question answered here ("is this label a registered env-Thunder handle?")
// leaks nothing beyond what's already inferable by directly dialing the
// hostname and observing whether it resolves.
func registerThunderAskRoute(mux *http.ServeMux, environmentService services.EnvironmentService) {
	mux.HandleFunc("GET /internal/thunder-ask", func(w http.ResponseWriter, r *http.Request) {
		domain := r.URL.Query().Get("domain")
		label, isEnvThunderHost := strings.CutSuffix(domain, "."+config.GetConfig().ThunderHostBaseDomain)
		if !isEnvThunderHost {
			w.WriteHeader(http.StatusOK)
			return
		}
		if label == "" || strings.Contains(label, ".") {
			// The bare base domain, or anything with an extra label in
			// between, is never a valid single-segment handle.
			w.WriteHeader(http.StatusForbidden)
			return
		}

		available, err := environmentService.IsThunderHandleAvailable(r.Context(), label)
		if err != nil {
			// Fail closed: an unreadable registry must never be treated as
			// authorization to issue the certificate.
			logger.GetLogger(r.Context()).Error("thunder-ask: failed to check handle registration", "handle", label, "error", err)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if available {
			// "Available" means no environment has claimed this label.
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

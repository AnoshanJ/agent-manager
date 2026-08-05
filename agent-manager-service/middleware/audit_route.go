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
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// WithAudit returns a middleware that records one audit event per request for
// the given route.
//
// It is installed as the outermost per-route wrapper, so it observes the 400
// from path-parameter validation and the 403 from the permission check as well
// as whatever the handler itself returns. It sits inside the JWT middleware,
// which is applied at the mux level, so token claims are already on the context.
//
// The event it writes describes the request envelope: who called what, and what
// came back. It does not describe the domain effect — that is the semantic
// tier's job, and when a semantic event was emitted for a successful request
// this middleware stands down so the trail carries one record, not two.
func WithAudit(recorder audit.Recorder, meta audit.RouteMeta) func(http.HandlerFunc) http.HandlerFunc {
	if !meta.Audited {
		return func(next http.HandlerFunc) http.HandlerFunc { return next }
	}

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := newResponseRecorder(w)

			rbacEnabled := false
			if cfg := config.GetConfig(); cfg != nil {
				rbacEnabled = cfg.RBACEnabled
			}

			ctx := audit.WithRecorder(r.Context(), recorder)
			ctx = audit.WithSource(ctx, audit.Source{
				Surface:      audit.SurfaceAPI,
				IP:           utils.ClientIP(r),
				UserAgent:    r.UserAgent(),
				Method:       meta.Method,
				Pattern:      meta.Path,
				RBACEnforced: rbacEnabled,
			})
			ctx, scope := audit.NewRequestScope(ctx)

			defer func() {
				// RecovererOnPanic is the outermost middleware in the chain, so a
				// panic unwinds through here first. Record the 500 the recoverer
				// is about to write, then re-panic so its behaviour is unchanged.
				// Without this the request would be recorded as a success.
				panicked := recover()
				if panicked != nil {
					rec.setStatus(http.StatusInternalServerError)
				}

				emitEnvelope(ctx, recorder, meta, rec, scope, start)

				if panicked != nil {
					panic(panicked)
				}
			}()

			next(rec, r.WithContext(ctx))
		}
	}
}

// WithAuditRecorder installs an audit recorder and a surface description on
// every request passing through it.
//
// Used for surfaces that do not register through RouteRegistrar — the internal
// gateway server and MCP — so that emit sites there reach a real recorder
// instead of the "not installed" fallback. It records nothing by itself; the
// events come from the handlers.
func WithAuditRecorder(recorder audit.Recorder, surface audit.Surface) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := audit.WithRecorder(r.Context(), recorder)
			ctx = audit.WithSource(ctx, audit.Source{
				Surface:   surface,
				IP:        utils.ClientIP(r),
				UserAgent: r.UserAgent(),
				Method:    r.Method,
				// Surfaces without a registrar have no route pattern, so the
				// path is left to the handler to supply via a semantic emit.
			})
			ctx, _ = audit.NewRequestScope(ctx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// emitEnvelope writes the envelope event unless a semantic event already
// described this request.
func emitEnvelope(
	ctx context.Context,
	recorder audit.Recorder,
	meta audit.RouteMeta,
	rec *responseRecorder,
	scope *audit.RequestScope,
	start time.Time,
) {
	if scope.Suppressed() {
		return
	}
	// A semantic emit already recorded what happened. Keep the envelope for
	// failures: a request rejected before it reached the service emits nothing
	// semantic, and that rejection is exactly what must not go unrecorded.
	if scope.SemanticEmitted() && rec.Status() < http.StatusBadRequest {
		return
	}

	e := audit.BuildEvent(
		ctx, meta.Action,
		audit.Status(rec.Status()),
		audit.RequiredPermissions(meta.Perms...),
		audit.Detail("envelope", true),
	)
	e.DurationMs = time.Since(start).Milliseconds()
	recorder.Record(ctx, e)
}

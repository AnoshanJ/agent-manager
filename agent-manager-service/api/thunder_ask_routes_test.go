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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// stubThunderAskEnvironmentService implements services.EnvironmentService by
// embedding the interface (every unimplemented method panics if called) and
// overriding only IsThunderHandleAvailable, the one method this route uses.
type stubThunderAskEnvironmentService struct {
	services.EnvironmentService
	available    bool
	availableErr error
	calledWith   *string
}

func (s *stubThunderAskEnvironmentService) IsThunderHandleAvailable(_ context.Context, handle string) (bool, error) {
	s.calledWith = &handle
	return s.available, s.availableErr
}

func setupThunderAskMux(t *testing.T, svc services.EnvironmentService) *http.ServeMux {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.ThunderHostBaseDomain
	cfg.ThunderHostBaseDomain = "amp.example.com"
	t.Cleanup(func() { cfg.ThunderHostBaseDomain = orig })

	mux := http.NewServeMux()
	registerThunderAskRoute(mux, svc)
	return mux
}

func askThunderRoute(mux *http.ServeMux, domain string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/internal/thunder-ask?domain="+domain, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestThunderAskRoute_RegisteredHandleAllowed(t *testing.T) {
	svc := &stubThunderAskEnvironmentService{available: false}
	mux := setupThunderAskMux(t, svc)

	rec := askThunderRoute(mux, "abcd1234.amp.example.com")

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for a registered handle, got %d", rec.Code)
	}
	if svc.calledWith == nil || *svc.calledWith != "abcd1234" {
		t.Errorf("expected the bare label to be checked, got %v", svc.calledWith)
	}
}

func TestThunderAskRoute_UnregisteredHandleForbidden(t *testing.T) {
	svc := &stubThunderAskEnvironmentService{available: true}
	mux := setupThunderAskMux(t, svc)

	rec := askThunderRoute(mux, "never-claimed.amp.example.com")

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for an unregistered handle, got %d", rec.Code)
	}
}

func TestThunderAskRoute_FailsClosedOnServiceError(t *testing.T) {
	svc := &stubThunderAskEnvironmentService{availableErr: errors.New("db down")}
	mux := setupThunderAskMux(t, svc)

	rec := askThunderRoute(mux, "abcd1234.amp.example.com")

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 when the registry can't be read, got %d", rec.Code)
	}
}

func TestThunderAskRoute_NonThunderHostAlwaysAllowed(t *testing.T) {
	// The gateway and deployed-agent wildcards share this same ask endpoint
	// (see caddyfile()) — this route must never touch the handle registry for
	// hostnames outside the env-Thunder base domain.
	svc := &stubThunderAskEnvironmentService{}
	mux := setupThunderAskMux(t, svc)

	rec := askThunderRoute(mux, "some-org.gateway.example.com")

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for a non-Thunder host, got %d", rec.Code)
	}
	if svc.calledWith != nil {
		t.Errorf("expected the handle registry not to be consulted, got %v", svc.calledWith)
	}
}

func TestThunderAskRoute_EmptyLabelForbidden(t *testing.T) {
	svc := &stubThunderAskEnvironmentService{}
	mux := setupThunderAskMux(t, svc)

	rec := askThunderRoute(mux, ".amp.example.com")

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for an empty label, got %d", rec.Code)
	}
	if svc.calledWith != nil {
		t.Errorf("expected the handle registry not to be consulted, got %v", svc.calledWith)
	}
}

func TestThunderAskRoute_MultiLabelRemainderForbidden(t *testing.T) {
	svc := &stubThunderAskEnvironmentService{}
	mux := setupThunderAskMux(t, svc)

	rec := askThunderRoute(mux, "a.b.amp.example.com")

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for a multi-label remainder, got %d", rec.Code)
	}
	if svc.calledWith != nil {
		t.Errorf("expected the handle registry not to be consulted, got %v", svc.calledWith)
	}
}

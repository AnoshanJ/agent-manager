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

package controllers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// TestUpdateUser_CarriesOverTypeAndOUID guards against a regression where
// UpdateUser sent Thunder a PUT /users/{id} with an empty Type/OuID. Thunder's
// PUT is a full-replace operation that requires a valid type on every call, so an
// empty Type failed with USR-1021 "user type not found" instead of preserving the
// user's existing type — spec.UpdateUserRequest has no type/ouId field for callers
// to set explicitly, so the handler must carry both over from the fetched record.
func TestUpdateUser_CarriesOverTypeAndOUID(t *testing.T) {
	var gotReq thundersvc.UpdateUserRequest
	client := &clientmocks.IdentityClientMock{
		GetUserFunc: func(_ context.Context, userID string) (*thundersvc.ThunderUser, error) {
			return &thundersvc.ThunderUser{ID: userID, Type: "engineer", OuID: "ou-1"}, nil
		},
		UpdateUserFunc: func(_ context.Context, _ string, req thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
			gotReq = req
			return &thundersvc.ThunderUser{ID: "user-1", Type: req.Type, OuID: req.OuID}, nil
		},
	}
	ctrl := NewIdentityController(client)

	req := httptest.NewRequest(http.MethodPut, "/orgs/o1/identities/users/user-1",
		strings.NewReader(`{"attributes":{"given_name":"Updated"}}`))
	req.SetPathValue(utils.PathParamUserID, "user-1")
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}))
	w := httptest.NewRecorder()

	ctrl.UpdateUser(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "engineer", gotReq.Type)
	assert.Equal(t, "ou-1", gotReq.OuID)
	assert.Equal(t, "Updated", gotReq.Attributes["given_name"])
}

// TestUpdateCurrentUserProfile_RoutesPasswordToCredentialsEndpoint guards against a
// regression where a password change from the profile page was folded into the regular
// attribute-update call. Thunder rejects a password field there with USR-1028
// "Credential update not allowed" — it only accepts credential changes through the
// dedicated update-credentials endpoint, so the handler must split the two calls.
func TestUpdateCurrentUserProfile_RoutesPasswordToCredentialsEndpoint(t *testing.T) {
	var gotUpdateReq thundersvc.UpdateUserRequest
	var gotCredUserID, gotCredPassword string
	client := &clientmocks.IdentityClientMock{
		GetUserFunc: func(_ context.Context, userID string) (*thundersvc.ThunderUser, error) {
			return &thundersvc.ThunderUser{ID: userID, Type: "engineer", OuID: "ou-1"}, nil
		},
		UpdateUserFunc: func(_ context.Context, _ string, req thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
			gotUpdateReq = req
			return &thundersvc.ThunderUser{ID: "user-1", Type: req.Type, OuID: req.OuID}, nil
		},
		UpdateUserCredentialsFunc: func(_ context.Context, userID, password string) error {
			gotCredUserID = userID
			gotCredPassword = password
			return nil
		},
	}
	ctrl := NewIdentityController(client)

	req := httptest.NewRequest(http.MethodPut, "/orgs/o1/identities/users/user-1/profile",
		strings.NewReader(`{"attributes":{"given_name":"Updated","password":"newpass123"}}`))
	req.SetPathValue(utils.PathParamUserID, "user-1")
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}))
	req = req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(), &jwtassertion.TokenClaims{Sub: "user-1"}))
	w := httptest.NewRecorder()

	ctrl.UpdateCurrentUserProfile(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Updated", gotUpdateReq.Attributes["given_name"])
	_, passwordInAttrs := gotUpdateReq.Attributes["password"]
	assert.False(t, passwordInAttrs, "password must not be sent through the regular attribute-update call")
	assert.Equal(t, "user-1", gotCredUserID)
	assert.Equal(t, "newpass123", gotCredPassword)
}

// TestUpdateCurrentUserProfile_RejectsUpdatingAnotherUser guards the self-check
// that makes this a "current user" endpoint at all: without it, any
// authenticated caller could change another user's name, email, or password
// by putting a different ID in the path.
func TestUpdateCurrentUserProfile_RejectsUpdatingAnotherUser(t *testing.T) {
	client := &clientmocks.IdentityClientMock{
		GetUserFunc: func(_ context.Context, userID string) (*thundersvc.ThunderUser, error) {
			t.Fatalf("must not even fetch the target user once the self-check fails")
			return nil, nil //nolint:nilnil // unreachable: t.Fatalf halts the goroutine before this returns
		},
	}
	ctrl := NewIdentityController(client)

	req := httptest.NewRequest(http.MethodPut, "/orgs/o1/identities/users/victim-user/profile",
		strings.NewReader(`{"attributes":{"password":"attacker-set-password"}}`))
	req.SetPathValue(utils.PathParamUserID, "victim-user")
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}))
	req = req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(), &jwtassertion.TokenClaims{Sub: "attacker-user"}))
	w := httptest.NewRecorder()

	ctrl.UpdateCurrentUserProfile(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

// TestUpdateCurrentUserProfile_PasswordFailureReportsPartialSuccess guards the
// degrade path when the attribute update succeeds but the follow-up
// credential update fails: the response must say so explicitly (not a generic
// 500) since the name/email change already committed and a naive retry would
// resubmit those needlessly.
func TestUpdateCurrentUserProfile_PasswordFailureReportsPartialSuccess(t *testing.T) {
	updateUserCalled := false
	client := &clientmocks.IdentityClientMock{
		GetUserFunc: func(_ context.Context, userID string) (*thundersvc.ThunderUser, error) {
			return &thundersvc.ThunderUser{ID: userID, Type: "engineer", OuID: "ou-1"}, nil
		},
		UpdateUserFunc: func(_ context.Context, _ string, req thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
			updateUserCalled = true
			return &thundersvc.ThunderUser{ID: "user-1", Type: req.Type, OuID: req.OuID}, nil
		},
		UpdateUserCredentialsFunc: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("thunder update user credentials: HTTP 500: internal error")
		},
	}
	ctrl := NewIdentityController(client)

	req := httptest.NewRequest(http.MethodPut, "/orgs/o1/identities/users/user-1/profile",
		strings.NewReader(`{"attributes":{"given_name":"Updated","password":"newpass123"}}`))
	req.SetPathValue(utils.PathParamUserID, "user-1")
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}))
	req = req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(), &jwtassertion.TokenClaims{Sub: "user-1"}))
	w := httptest.NewRecorder()

	ctrl.UpdateCurrentUserProfile(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.True(t, updateUserCalled, "the name/email change must still have been attempted and committed")
	assert.Contains(t, w.Body.String(), "Profile updated, but password change failed")
}

// TestUpdateCurrentUserProfile_EmptyPasswordSkipsCredentialsCall covers a
// profile save where the password field is present but blank (e.g. a form
// that always includes the field but the user left it untouched): it must be
// treated as "no password change requested," not as "set the password to
// empty."
func TestUpdateCurrentUserProfile_EmptyPasswordSkipsCredentialsCall(t *testing.T) {
	credsCalled := false
	client := &clientmocks.IdentityClientMock{
		GetUserFunc: func(_ context.Context, userID string) (*thundersvc.ThunderUser, error) {
			return &thundersvc.ThunderUser{ID: userID, Type: "engineer", OuID: "ou-1"}, nil
		},
		UpdateUserFunc: func(_ context.Context, _ string, req thundersvc.UpdateUserRequest) (*thundersvc.ThunderUser, error) {
			return &thundersvc.ThunderUser{ID: "user-1", Type: req.Type, OuID: req.OuID}, nil
		},
		UpdateUserCredentialsFunc: func(_ context.Context, _, _ string) error {
			credsCalled = true
			return nil
		},
	}
	ctrl := NewIdentityController(client)

	req := httptest.NewRequest(http.MethodPut, "/orgs/o1/identities/users/user-1/profile",
		strings.NewReader(`{"attributes":{"given_name":"Updated","password":""}}`))
	req.SetPathValue(utils.PathParamUserID, "user-1")
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-1"}))
	req = req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(), &jwtassertion.TokenClaims{Sub: "user-1"}))
	w := httptest.NewRecorder()

	ctrl.UpdateCurrentUserProfile(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.False(t, credsCalled, "a blank password must not trigger a credentials update call")
}

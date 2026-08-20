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

package services

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const gatewayTestOUID = "ou-acme"

func gatewayFixture(name string) *models.Gateway {
	return &models.Gateway{
		UUID:        uuid.New(),
		Name:        name,
		DisplayName: name,
		OUID:        gatewayTestOUID,
		Vhost:       name + ".example.com",
	}
}

// The spec advertises the path param as "Gateway UUID or name", and a name used to
// be rejected with a bare error that matched no case in handleGatewayErrors and
// surfaced as HTTP 500.
func TestGetGateway_ResolvesByName(t *testing.T) {
	gateway := gatewayFixture("edge")
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(name, ouID string) (*models.Gateway, error) {
			assert.Equal(t, "edge", name)
			assert.Equal(t, gatewayTestOUID, ouID)
			return gateway, nil
		},
		// GetByUUIDFunc nil: a name must not be looked up as a UUID.
	}
	svc := NewPlatformGatewayService(repo, nil)

	resp, err := svc.GetGateway("edge", gatewayTestOUID)

	require.NoError(t, err)
	assert.Equal(t, gateway.UUID.String(), resp.ID)
	assert.Equal(t, "edge", resp.Name)
}

func TestGetGateway_ResolvesByUUID(t *testing.T) {
	gateway := gatewayFixture("edge")
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(gatewayID string) (*models.Gateway, error) {
			assert.Equal(t, gateway.UUID.String(), gatewayID)
			return gateway, nil
		},
		// GetByNameAndOrgIDFunc nil: a UUID must not fall through to a name lookup.
	}
	svc := NewPlatformGatewayService(repo, nil)

	resp, err := svc.GetGateway(gateway.UUID.String(), gatewayTestOUID)

	require.NoError(t, err)
	assert.Equal(t, gateway.UUID.String(), resp.ID)
}

func TestGetGateway_UnknownNameIsNotFound(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(_, _ string) (*models.Gateway, error) {
			return nil, utils.ErrGatewayNotFound
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway("ghost", gatewayTestOUID)

	assert.ErrorIs(t, err, utils.ErrGatewayNotFound)
}

func TestGetGateway_UnknownUUIDIsNotFound(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway(uuid.New().String(), gatewayTestOUID)

	assert.ErrorIs(t, err, utils.ErrGatewayNotFound)
}

// The name branch used to return the repository error verbatim, so a bare failure
// matched no case in handleGatewayErrors and surfaced as a 500 instead of a 404.
func TestGetGateway_UnknownNameFromGormIsNotFound(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(_, _ string) (*models.Gateway, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway("ghost", gatewayTestOUID)

	assert.ErrorIs(t, err, utils.ErrGatewayNotFound)
}

// A nil gateway with no error reached GetGateway's OUID check and panicked.
func TestGetGateway_NilGatewayWithoutErrorIsNotFound(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		GetByNameAndOrgIDFunc: func(_, _ string) (*models.Gateway, error) {
			return nil, nil //nolint:nilnil // the shape under test
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway("ghost", gatewayTestOUID)

	assert.ErrorIs(t, err, utils.ErrGatewayNotFound)
}

// A real repository failure must not be flattened into not-found.
func TestGetGateway_RepositoryErrorIsNotMaskedAsNotFound(t *testing.T) {
	boom := errors.New("connection refused")
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			return nil, boom
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway(uuid.New().String(), gatewayTestOUID)

	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, utils.ErrGatewayNotFound)
}

// Org isolation: a gateway UUID from another org reads as absent, not forbidden.
func TestGetGateway_OtherOrgGatewayIsNotFound(t *testing.T) {
	gateway := gatewayFixture("edge")
	gateway.OUID = "ou-other"
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			return gateway, nil
		},
	}
	svc := NewPlatformGatewayService(repo, nil)

	_, err := svc.GetGateway(gateway.UUID.String(), gatewayTestOUID)

	assert.ErrorIs(t, err, utils.ErrGatewayNotFound)
}

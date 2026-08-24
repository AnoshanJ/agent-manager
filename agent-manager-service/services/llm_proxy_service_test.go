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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A provider that LLMProviderService.Delete has already claimed via MarkDeleting
// (status DELETING) must reject new proxies. Otherwise a proxy could be created
// after Delete's own HasAssociatedProxies check passed, racing the undeploy/delete
// sequence below it (issue #1739 follow-up: TOCTOU between the proxies check and
// the delete completing).
func TestLLMProxyService_Create_RejectsWhenProviderIsBeingDeleted(t *testing.T) {
	providerUUID := uuid.New()
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(_, _ string) (*models.LLMProvider, error) {
			return &models.LLMProvider{UUID: providerUUID, Status: models.LLMProviderStatusDeleting}, nil
		},
	}
	proxyRepo := &repomocks.LLMProxyRepositoryMock{}
	svc := NewLLMProxyService(proxyRepo, providerRepo, make([]byte, 32))

	proxy := &models.LLMProxy{
		ProjectUUID: uuid.New(),
		Configuration: models.LLMProxyConfig{
			Name:     "my-proxy",
			Version:  "v1",
			Provider: providerUUID.String(),
		},
	}

	_, err := svc.Create("ou-acme", "creator", proxy)

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrLLMProviderBeingDeleted)
}

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func seededTemplateRepo(existing []*models.LLMProviderTemplate, updated *[]*models.LLMProviderTemplate) *repomocks.LLMProviderTemplateRepositoryMock {
	return &repomocks.LLMProviderTemplateRepositoryMock{
		CountFunc: func(string) (int, error) { return len(existing), nil },
		ListFunc: func(string, int, int) ([]*models.LLMProviderTemplate, error) {
			return existing, nil
		},
		UpdateFunc: func(t *models.LLMProviderTemplate) error {
			*updated = append(*updated, t)
			return nil
		},
		CreateFunc: func(*models.LLMProviderTemplate) error { return nil },
	}
}

// An org provisioned before the awsbedrock template gained an auth block keeps a
// row with Metadata.Auth == nil. Without a backfill the console goes on defaulting
// to the gateway-rejected "bearer" type, so the fix would reach new orgs only.
func TestSeedForOrgBackfillsMissingAuthOnExistingTemplate(t *testing.T) {
	existing := []*models.LLMProviderTemplate{{
		Handle: "awsbedrock",
		Name:   "AWS Bedrock",
		Metadata: &models.LLMProviderTemplateMetadata{
			EndpointURL:    "https://bedrock-runtime.us-east-1.amazonaws.com",
			OpenapiSpecURL: "https://example.invalid/openapi.yaml",
		},
	}}
	var updated []*models.LLMProviderTemplate

	seeder := NewLLMTemplateSeeder(seededTemplateRepo(existing, &updated), []*models.LLMProviderTemplate{{
		Handle: "awsbedrock",
		Name:   "AWS Bedrock",
		Metadata: &models.LLMProviderTemplateMetadata{
			EndpointURL: "https://bedrock-runtime.us-east-1.amazonaws.com",
			Auth: &models.LLMProviderTemplateAuth{
				Type:        "api-key",
				Header:      "Authorization",
				ValuePrefix: "Bearer ",
			},
		},
	}})

	require.NoError(t, seeder.SeedForOrg("org-1"))

	require.Len(t, updated, 1, "expected the existing template to be updated with the new auth block")
	require.NotNil(t, updated[0].Metadata.Auth, "auth block was not backfilled")
	assert.Equal(t, "api-key", updated[0].Metadata.Auth.Type)
	assert.Equal(t, "Authorization", updated[0].Metadata.Auth.Header)
	assert.Equal(t, "Bearer ", updated[0].Metadata.Auth.ValuePrefix)
}

// Operators may deliberately point a template at their own gateway credentials;
// re-seeding must not stomp that.
func TestSeedForOrgPreservesExistingAuth(t *testing.T) {
	existing := []*models.LLMProviderTemplate{{
		Handle: "awsbedrock",
		Name:   "AWS Bedrock",
		Metadata: &models.LLMProviderTemplateMetadata{
			Auth: &models.LLMProviderTemplateAuth{
				Type:   "api-key",
				Header: "X-Custom-Auth",
			},
		},
	}}
	var updated []*models.LLMProviderTemplate

	seeder := NewLLMTemplateSeeder(seededTemplateRepo(existing, &updated), []*models.LLMProviderTemplate{{
		Handle: "awsbedrock",
		Name:   "AWS Bedrock",
		Metadata: &models.LLMProviderTemplateMetadata{
			Auth: &models.LLMProviderTemplateAuth{
				Type:        "api-key",
				Header:      "Authorization",
				ValuePrefix: "Bearer ",
			},
		},
	}})

	require.NoError(t, seeder.SeedForOrg("org-1"))

	assert.Equal(t, "X-Custom-Auth", existing[0].Metadata.Auth.Header, "operator-set auth header was overwritten")
}

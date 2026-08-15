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
	"gopkg.in/yaml.v3"
)

func bedrockSigV4Provider() *models.LLMProvider {
	authType := "other"
	return &models.LLMProvider{
		TemplateHandle: "awsbedrock",
		Artifact:       &models.Artifact{Handle: "bedrock-provider"},
		Configuration: models.LLMProviderConfig{
			Name:    "Bedrock Provider",
			Version: "v1.0",
			Context: strPtr("/bedrock"),
			Upstream: &models.UpstreamConfig{
				Main: &models.UpstreamEndpoint{
					URL:  "https://bedrock-runtime.us-east-1.amazonaws.com",
					Auth: &models.UpstreamAuth{Type: &authType},
				},
			},
			Policies: []models.LLMPolicy{{
				Name:    "aws-authentication",
				Version: "v0.10.0",
				Paths: []models.LLMPolicyPath{{
					Path:    "/*",
					Methods: []string{"*"},
					Params: map[string]interface{}{
						"service":            "bedrock",
						"region":             "us-east-1",
						"authenticationType": "iam-user-access-key",
						"awsAccessKeyID":     "AKIAEXAMPLE",
						"awsSecretAccessKey": "secret",
					},
				}},
			}},
		},
	}
}

// The gateway turns upstream.auth into a static set-headers policy and errors on
// any type but "api-key". SigV4 cannot be a fixed header, so "other" means the
// auth block must be omitted and signing left to the aws-authentication policy.
func TestGenerateLLMProviderDeploymentYAML_OmitsAuthBlockForOtherType(t *testing.T) {
	service := &LLMProviderDeploymentService{}

	yamlStr, err := service.generateLLMProviderDeploymentYAML(bedrockSigV4Provider(), "test-org")
	require.NoError(t, err)

	var out LLMProviderDeploymentYAML
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &out))

	assert.Nil(t, out.Spec.Upstream.Auth, "upstream auth must be omitted for type \"other\"; the gateway rejects any type but api-key")
	assert.Equal(t, "https://bedrock-runtime.us-east-1.amazonaws.com", out.Spec.Upstream.URL)
}

// The signing policy must survive artifact generation intact, including its exact
// param keys: the policy schema sets additionalProperties:false.
func TestGenerateLLMProviderDeploymentYAML_KeepsAWSAuthenticationPolicy(t *testing.T) {
	service := &LLMProviderDeploymentService{}

	yamlStr, err := service.generateLLMProviderDeploymentYAML(bedrockSigV4Provider(), "test-org")
	require.NoError(t, err)

	var out LLMProviderDeploymentYAML
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &out))

	var found *models.LLMPolicy
	for i := range out.Spec.Policies {
		if out.Spec.Policies[i].Name == "aws-authentication" {
			found = &out.Spec.Policies[i]
			break
		}
	}
	require.NotNil(t, found, "aws-authentication policy missing from the deployment artifact")
	require.Len(t, found.Paths, 1)
	assert.Equal(t, "/*", found.Paths[0].Path)
	assert.Equal(t, "bedrock", found.Paths[0].Params["service"])
	assert.Equal(t, "us-east-1", found.Paths[0].Params["region"])
	assert.Equal(t, "iam-user-access-key", found.Paths[0].Params["authenticationType"])
}

// Regression guard: "other" handling sits on the code path every provider uses,
// so an ordinary api-key provider must be byte-identical to before.
func TestGenerateLLMProviderDeploymentYAML_APIKeyAuthUnaffected(t *testing.T) {
	service := &LLMProviderDeploymentService{}
	authType := "api-key"
	header := "Authorization"
	value := "Bearer sk-test"

	provider := &models.LLMProvider{
		TemplateHandle: "openai",
		Artifact:       &models.Artifact{Handle: "openai-provider"},
		Configuration: models.LLMProviderConfig{
			Name:    "OpenAI Provider",
			Version: "v1.0",
			Context: strPtr("/"),
			Upstream: &models.UpstreamConfig{
				Main: &models.UpstreamEndpoint{
					URL:  "https://api.openai.com",
					Auth: &models.UpstreamAuth{Type: &authType, Header: &header, Value: &value},
				},
			},
		},
	}

	yamlStr, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
	require.NoError(t, err)

	var out LLMProviderDeploymentYAML
	require.NoError(t, yaml.Unmarshal([]byte(yamlStr), &out))

	require.NotNil(t, out.Spec.Upstream.Auth, "api-key providers must keep their auth block")
	assert.Equal(t, "api-key", *out.Spec.Upstream.Auth.Type)
	assert.Equal(t, "Authorization", *out.Spec.Upstream.Auth.Header)
	assert.Equal(t, "Bearer sk-test", *out.Spec.Upstream.Auth.Value)
}

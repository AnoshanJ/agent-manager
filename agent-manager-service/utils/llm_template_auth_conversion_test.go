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

package utils_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The console decides whether to offer SigV4 credential sources from the auth
// policy on the list-templates response. Dropping it in conversion silently
// disables the whole feature while every other test still passes.
func TestConvertModelToSpecKeepsTemplateAuthPolicy(t *testing.T) {
	meta := &models.LLMProviderTemplateMetadata{
		EndpointURL: "https://bedrock-runtime.us-east-1.amazonaws.com",
		Auth: &models.LLMProviderTemplateAuth{
			Type:        "api-key",
			Header:      "Authorization",
			ValuePrefix: "Bearer ",
			Policy:      "aws-authentication",
		},
	}

	out := utils.ConvertModelToSpecLLMProviderTemplateMetadata(meta)

	require.NotNil(t, out)
	require.NotNil(t, out.Auth)
	require.NotNil(t, out.Auth.Policy, "auth policy dropped; the console never offers SigV4 credential sources")
	assert.Equal(t, "aws-authentication", *out.Auth.Policy)
}

// The upstream API key is masked in API responses (ConvertModelToSpecUpstreamConfig);
// an AWS secret key living in policy params deserves the same treatment, or any
// caller with read access to the provider retrieves a live AWS credential.
func TestConvertModelToSpecLLMPolicyMasksAWSSecrets(t *testing.T) {
	policy := models.LLMPolicy{
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
				"awsSecretAccessKey": "super-secret",
				"awsSessionToken":    "session-secret",
			},
		}},
	}

	out := utils.ConvertModelToSpecLLMPolicy(policy)

	params := out.Paths[0].Params
	assert.NotEqual(t, "super-secret", params["awsSecretAccessKey"], "AWS secret access key returned in cleartext")
	assert.NotEqual(t, "session-secret", params["awsSessionToken"], "AWS session token returned in cleartext")
	// Non-secret params stay readable so the UI can show the configuration.
	assert.Equal(t, "us-east-1", params["region"])
	assert.Equal(t, "iam-user-access-key", params["authenticationType"])
	assert.Equal(t, "AKIAEXAMPLE", params["awsAccessKeyID"])
}

// Masking must not corrupt the stored configuration it was built from.
func TestConvertModelToSpecLLMPolicyDoesNotMutateSource(t *testing.T) {
	params := map[string]interface{}{"awsSecretAccessKey": "super-secret"}
	policy := models.LLMPolicy{
		Name:    "aws-authentication",
		Version: "v0.10.0",
		Paths:   []models.LLMPolicyPath{{Path: "/*", Methods: []string{"*"}, Params: params}},
	}

	utils.ConvertModelToSpecLLMPolicy(policy)

	assert.Equal(t, "super-secret", params["awsSecretAccessKey"], "conversion mutated the caller's params map")
}

// Masking is only safe if the marker coming back on an update is dropped rather
// than stored. Otherwise a GET-then-PUT round trip -- exactly what the console
// does when saving guardrails -- overwrites the real AWS secret with the marker.
func TestConvertSpecToModelLLMPolicyDropsRedactedSecrets(t *testing.T) {
	specPolicy := spec.LLMPolicy{
		Name:    "aws-authentication",
		Version: "v0.10.0",
		Paths: []spec.LLMPolicyPath{{
			Path:    "/*",
			Methods: []string{"*"},
			Params: map[string]interface{}{
				"region":             "us-east-1",
				"awsSecretAccessKey": "***REDACTED***",
			},
		}},
	}

	out := utils.ConvertSpecToModelLLMPolicy(specPolicy)

	params := out.Paths[0].Params
	_, present := params["awsSecretAccessKey"]
	assert.False(t, present, "redaction marker stored as the secret; the real credential is destroyed on save")
	assert.Equal(t, "us-east-1", params["region"])
}

// A template round-tripped through the API must not lose its policy, or the
// seeder's "backfill only when Auth is nil" guard will never restore it.
func TestConvertSpecToModelKeepsTemplateAuthPolicy(t *testing.T) {
	original := &models.LLMProviderTemplateMetadata{
		Auth: &models.LLMProviderTemplateAuth{
			Type:   "api-key",
			Policy: "aws-authentication",
		},
	}

	roundTripped := utils.ConvertSpecToModelLLMProviderTemplateMetadata(
		utils.ConvertModelToSpecLLMProviderTemplateMetadata(original),
	)

	require.NotNil(t, roundTripped)
	require.NotNil(t, roundTripped.Auth)
	assert.Equal(t, "aws-authentication", roundTripped.Auth.Policy)
}

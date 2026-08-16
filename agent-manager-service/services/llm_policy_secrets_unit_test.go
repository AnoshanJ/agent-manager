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
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func awsPolicy(params map[string]interface{}) []models.LLMPolicy {
	return []models.LLMPolicy{{
		Name:    "aws-authentication",
		Version: "v0.10.0",
		Paths:   []models.LLMPolicyPath{{Path: "/*", Methods: []string{"*"}, Params: params}},
	}}
}

// Secrets are masked in read responses and stripped from writes, so an update
// built from a read arrives without them. Without a carry-forward the stored
// credential is silently destroyed by an unrelated edit -- saving a guardrail,
// renaming the provider -- and the provider stops authenticating to AWS.
func TestPreserveOmittedPolicySecretsCarriesForwardStoredSecret(t *testing.T) {
	stored := awsPolicy(map[string]interface{}{
		"region":             "us-east-1",
		"awsAccessKeyID":     "AKIAEXAMPLE",
		"awsSecretAccessKey": "stored-secret",
	})
	incoming := awsPolicy(map[string]interface{}{
		"region":         "us-east-1",
		"awsAccessKeyID": "AKIAEXAMPLE",
	})

	preserveOmittedPolicySecrets(incoming, stored)

	assert.Equal(t, "stored-secret", incoming[0].Paths[0].Params["awsSecretAccessKey"])
}

func TestPreserveOmittedPolicySecretsKeepsAnExplicitlyChangedSecret(t *testing.T) {
	stored := awsPolicy(map[string]interface{}{"awsSecretAccessKey": "old-secret"})
	incoming := awsPolicy(map[string]interface{}{"awsSecretAccessKey": "rotated-secret"})

	preserveOmittedPolicySecrets(incoming, stored)

	assert.Equal(t, "rotated-secret", incoming[0].Paths[0].Params["awsSecretAccessKey"])
}

// Removing the policy entirely is a deliberate act, not an omitted secret.
func TestPreserveOmittedPolicySecretsIgnoresPoliciesNotBeingUpdated(t *testing.T) {
	stored := awsPolicy(map[string]interface{}{"awsSecretAccessKey": "stored-secret"})
	incoming := []models.LLMPolicy{}

	preserveOmittedPolicySecrets(incoming, stored)

	assert.Empty(t, incoming)
}

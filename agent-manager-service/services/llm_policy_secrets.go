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

import "github.com/wso2/agent-manager/agent-manager-service/models"

// secretPolicyParamKeys are policy params masked in read responses. An update
// assembled from a read therefore arrives without them.
var secretPolicyParamKeys = []string{"awsSecretAccessKey", "awsSessionToken"}

// preserveOmittedPolicySecrets copies secret params the caller did not supply from
// the stored configuration into the incoming policies, matched by policy name and
// path. Without it, any update built from a masked read -- saving a guardrail,
// renaming the provider -- would silently drop the stored AWS credential. A value
// the caller did supply always wins, so rotation still works.
func preserveOmittedPolicySecrets(incoming, stored []models.LLMPolicy) {
	if len(incoming) == 0 || len(stored) == 0 {
		return
	}

	for i := range incoming {
		for j := range incoming[i].Paths {
			path := &incoming[i].Paths[j]
			storedParams := findStoredPolicyParams(stored, incoming[i].Name, path.Path)
			if storedParams == nil {
				continue
			}
			for _, key := range secretPolicyParamKeys {
				if _, supplied := path.Params[key]; supplied {
					continue
				}
				if value, ok := storedParams[key]; ok {
					if path.Params == nil {
						path.Params = map[string]interface{}{}
					}
					path.Params[key] = value
				}
			}
		}
	}
}

func findStoredPolicyParams(stored []models.LLMPolicy, name, path string) map[string]interface{} {
	for _, policy := range stored {
		if policy.Name != name {
			continue
		}
		for _, storedPath := range policy.Paths {
			if storedPath.Path == path {
				return storedPath.Params
			}
		}
	}
	return nil
}

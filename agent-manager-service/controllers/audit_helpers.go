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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
)

// beginConfigAPIKeyAudit records the intent to change an API key belonging to
// an agent's model or MCP configuration.
//
// These routes share one permission and one action with every other API-key
// route, so without the owner details a record cannot say which configuration
// the key belonged to. The key value is never passed in.
func beginConfigAPIKeyAudit(
	ctx context.Context,
	action audit.Action,
	ownerType, ouID, projName, agentName, envName, configID, keyName string,
) (*audit.Attempt, error) {
	return audit.Begin(ctx, action,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAPIKey, configID, keyName),
		audit.Project(projName),
		audit.Environment(envName),
		audit.Detail("ownerType", ownerType),
		audit.Detail("ownerName", agentName),
		audit.Detail("keyName", keyName),
	)
}

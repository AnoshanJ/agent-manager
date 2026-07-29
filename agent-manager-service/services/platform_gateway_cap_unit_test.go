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

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestNormalizeGatewayRole(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "canonical ingress", input: "INGRESS", want: models.GatewayRoleIngress},
		{name: "canonical egress", input: "EGRESS", want: models.GatewayRoleEgress},
		{name: "canonical both", input: "BOTH", want: models.GatewayRoleBoth},
		{name: "lowercase accepted", input: "both", want: models.GatewayRoleBoth},
		{name: "alias REGULAR maps to both", input: "REGULAR", want: models.GatewayRoleBoth},
		{name: "alias AI maps to egress", input: "AI", want: models.GatewayRoleEgress},
		{name: "event is rejected", input: "EVENT", wantErr: true},
		{name: "empty is rejected", input: "", wantErr: true},
		{name: "unknown is rejected", input: "sideways", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeGatewayRole(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

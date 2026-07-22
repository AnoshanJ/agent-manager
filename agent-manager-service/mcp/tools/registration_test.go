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

package tools

import (
	"context"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Verifies that every tool described by allToolSpecs is actually registered on a fully-wired MCP server.
func TestToolRegistration(t *testing.T) {
	clientSession, _ := setupTestServer(t)

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	registered := make(map[string]bool, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		registered[tool.Name] = true
	}

	for _, spec := range allToolSpecs {
		if !registered[spec.name] {
			t.Errorf("expected tool %q not registered", spec.name)
		}
	}
}

// Verifies every tool spec declares permissions and that the registry built by
// registration matches the specs exactly — a tool added without declaring its
// permission fails here (and addTool panics before this test even runs).
func TestToolPermissionsMatchSpecs(t *testing.T) {
	mock := NewMockToolsetHandler()
	toolsets := &Toolsets{
		ProjectToolset:    mock,
		AgentToolset:      mock,
		BuildToolset:      mock,
		DeploymentToolset: mock,
	}
	server := gomcp.NewServer(&gomcp.Implementation{Name: "t", Version: "0"}, nil)
	reg := toolsets.register(server)

	for _, spec := range allToolSpecs {
		if len(spec.permissions) == 0 {
			t.Errorf("tool spec %q declares no permissions", spec.name)
			continue
		}
		got := reg.permissions[spec.name]
		if len(got) != len(spec.permissions) {
			t.Errorf("tool %q: registered permissions %v, spec expects %v", spec.name, got, spec.permissions)
			continue
		}
		for i := range got {
			if got[i] != spec.permissions[i] {
				t.Errorf("tool %q: registered permissions %v, spec expects %v", spec.name, got, spec.permissions)
				break
			}
		}
	}
	if len(reg.permissions) != len(allToolSpecs) {
		t.Errorf("registry has %d tools, specs describe %d", len(reg.permissions), len(allToolSpecs))
	}
}

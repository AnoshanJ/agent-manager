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
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// setRBACEnabled flips the process-global RBAC switch for one test and
// restores it on cleanup. Tests using it must not run in parallel.
func setRBACEnabled(t *testing.T, enabled bool) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.RBACEnabled
	cfg.RBACEnabled = enabled
	t.Cleanup(func() { cfg.RBACEnabled = orig })
}

// callToolViaMiddleware runs a synthetic tools/call for toolName through the
// registry's middleware with a next handler that records whether it ran.
func callToolViaMiddleware(t *testing.T, reg *toolRegistry, ctx context.Context, toolName string) (result gomcp.Result, nextCalled bool) {
	t.Helper()
	next := func(_ context.Context, _ string, _ gomcp.Request) (gomcp.Result, error) {
		nextCalled = true
		return &gomcp.CallToolResult{}, nil
	}
	req := &gomcp.CallToolRequest{Params: &gomcp.CallToolParamsRaw{Name: toolName}}
	result, err := reg.authzMiddleware()(next)(ctx, "tools/call", req)
	if err != nil {
		t.Fatalf("middleware returned unexpected error: %v", err)
	}
	return result, nextCalled
}

func denialText(t *testing.T, result gomcp.Result) string {
	t.Helper()
	callResult, ok := result.(*gomcp.CallToolResult)
	if !ok {
		t.Fatalf("result is %T, want *gomcp.CallToolResult", result)
	}
	if !callResult.IsError {
		t.Fatal("expected IsError=true denial result")
	}
	if len(callResult.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(callResult.Content))
	}
	text, ok := callResult.Content[0].(*gomcp.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *gomcp.TextContent", callResult.Content[0])
	}
	return text.Text
}

func TestAddToolPanicsWithoutPermissions(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when registering a tool without permissions")
		}
		if !strings.Contains(r.(string), "no_perm_tool") {
			t.Fatalf("panic message %q does not name the tool", r)
		}
	}()
	server := gomcp.NewServer(&gomcp.Implementation{Name: "t", Version: "0"}, nil)
	addTool(newToolRegistry(), server, &gomcp.Tool{
		Name:        "no_perm_tool",
		Description: "a tool registered without permissions",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{}, nil, nil
	})
}

func TestAddToolRecordsPermissions(t *testing.T) {
	server := gomcp.NewServer(&gomcp.Implementation{Name: "t", Version: "0"}, nil)
	reg := newToolRegistry()
	addTool(reg, server, &gomcp.Tool{
		Name:        "two_perm_tool",
		Description: "a tool with two permissions",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{}, nil, nil
	}, rbac.AgentCreate, rbac.AgentTokenManage)

	got := reg.permissions["two_perm_tool"]
	if len(got) != 2 || got[0] != rbac.AgentCreate || got[1] != rbac.AgentTokenManage {
		t.Fatalf("registry permissions = %v, want [AgentCreate AgentTokenManage]", got)
	}
}

func TestAuthzMiddlewareDeniesUnregisteredTool(t *testing.T) {
	setRBACEnabled(t, false) // fail-closed applies even with RBAC disabled
	reg := newToolRegistry()
	result, nextCalled := callToolViaMiddleware(t, reg, context.Background(), "rogue_tool")
	if nextCalled {
		t.Fatal("next handler ran for an unregistered tool")
	}
	if got, want := denialText(t, result), `tool "rogue_tool" has no registered permissions`; got != want {
		t.Fatalf("denial text = %q, want %q", got, want)
	}
}

func TestAuthzMiddlewareSkipsCheckWhenRBACDisabled(t *testing.T) {
	setRBACEnabled(t, false)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	// No claims on context at all — must still pass with RBAC disabled.
	_, nextCalled := callToolViaMiddleware(t, reg, context.Background(), "some_tool")
	if !nextCalled {
		t.Fatal("next handler did not run with RBAC disabled")
	}
}

func TestAuthzMiddlewareDeniesMissingScope(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentRead.Scope(), // has read, not build
	})
	result, nextCalled := callToolViaMiddleware(t, reg, ctx, "some_tool")
	if nextCalled {
		t.Fatal("next handler ran despite missing scope")
	}
	if got, want := denialText(t, result), "insufficient permissions: this tool requires the amp:agent:build scope"; got != want {
		t.Fatalf("denial text = %q, want %q", got, want)
	}
}

func TestAuthzMiddlewareAllowsMatchingScope(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["some_tool"] = []rbac.Permission{rbac.AgentBuild}
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentBuild.Scope(),
	})
	_, nextCalled := callToolViaMiddleware(t, reg, ctx, "some_tool")
	if !nextCalled {
		t.Fatal("next handler did not run despite matching scope")
	}
}

func TestAuthzMiddlewareRequiresAllPermissions(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	reg.permissions["multi_tool"] = []rbac.Permission{rbac.AgentCreate, rbac.AgentTokenManage}
	// Only one of the two scopes present.
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentCreate.Scope(),
	})
	result, nextCalled := callToolViaMiddleware(t, reg, ctx, "multi_tool")
	if nextCalled {
		t.Fatal("next handler ran with only one of two required scopes")
	}
	if got, want := denialText(t, result), "insufficient permissions: this tool requires the amp:agent:token-manage scope"; got != want {
		t.Fatalf("denial text = %q, want %q", got, want)
	}
}

func TestAuthzMiddlewarePassesThroughOtherMethods(t *testing.T) {
	setRBACEnabled(t, true)
	reg := newToolRegistry()
	next := func(_ context.Context, _ string, _ gomcp.Request) (gomcp.Result, error) {
		return &gomcp.ListToolsResult{}, nil
	}
	req := &gomcp.ListToolsRequest{Params: &gomcp.ListToolsParams{}}
	result, err := reg.authzMiddleware()(next)(context.Background(), "tools/list", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := result.(*gomcp.ListToolsResult); !ok {
		t.Fatalf("tools/list result was intercepted: got %T", result)
	}
}

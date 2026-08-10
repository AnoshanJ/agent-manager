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

func TestBuildProxyURLUsesVhost(t *testing.T) {
	// RuntimeURL present but must be ignored: every agent gets the public URL.
	gateway := &models.Gateway{
		Vhost:      "https://dev-acme.gateway.example.com",
		RuntimeURL: "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893",
	}
	contextPath := "/llm/proxy"
	require.Equal(t, "https://dev-acme.gateway.example.com/llm/proxy", buildProxyURL(gateway, &contextPath))
	require.Equal(t, "https://dev-acme.gateway.example.com", buildProxyURL(gateway, nil))
}

func TestBuildMCPProxyURLUsesGatewayVhost(t *testing.T) {
	gateway := &models.Gateway{
		Vhost:      "https://gateway.example.com",
		RuntimeURL: "http://runtime.acme-dev:22893",
	}
	ctxPath := "/github"
	require.Equal(t, "https://gateway.example.com/github/mcp", buildMCPProxyURL(gateway, models.MCPProxyConfig{Context: &ctxPath}))
	require.Equal(t, "https://gateway.example.com/mcp", buildMCPProxyURL(gateway, models.MCPProxyConfig{}))
}

func TestBuildMCPProxyURLPrefersProxyVhostOverride(t *testing.T) {
	// The deployment spec forwards the proxy's own vhost to the gateway, so the
	// override — not the gateway default — is where the proxy is actually served.
	gateway := &models.Gateway{Vhost: "https://gateway.example.com"}
	vhost := "mcp.example.com"
	ctxPath := "/github"
	require.Equal(t, "https://mcp.example.com/github/mcp",
		buildMCPProxyURL(gateway, models.MCPProxyConfig{Vhost: &vhost, Context: &ctxPath}))
	full := "http://mcp.example.com"
	require.Equal(t, "http://mcp.example.com/mcp",
		buildMCPProxyURL(gateway, models.MCPProxyConfig{Vhost: &full}))
	empty := "  "
	require.Equal(t, "https://gateway.example.com/mcp",
		buildMCPProxyURL(gateway, models.MCPProxyConfig{Vhost: &empty}))
}

func TestStripURLScheme(t *testing.T) {
	require.Equal(t, "gw.example.com/github/mcp", stripURLScheme("https://gw.example.com/github/mcp"))
	require.Equal(t, "gw.example.com/mcp", stripURLScheme("gw.example.com/mcp"))
}

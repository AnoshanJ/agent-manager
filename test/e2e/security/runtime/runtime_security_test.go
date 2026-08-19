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

package runtime

import (
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	identityops "github.com/wso2/agent-manager/test/e2e/operations/agentidentity"
	"github.com/wso2/agent-manager/test/e2e/operations/build"
	"github.com/wso2/agent-manager/test/e2e/operations/configuration"
	"github.com/wso2/agent-manager/test/e2e/operations/deployment"
	mcpproxyops "github.com/wso2/agent-manager/test/e2e/operations/mcpproxy"
)

var _ = Describe("SEC-RUNTIME-001: deployed agent sandbox and AgentID", Label("security", "runtime", "sandbox", "agentid"), Ordered, func() {
	const (
		mcpConfigName = "security-mcp"
		echoTool      = "echo"
		addTool       = "add"
	)

	var (
		endpointURL string
		roleID      string
		roleName    string
		scopeRead   string
		scopeWrite  string
	)

	newProbeAPIKey := func() string {
		response := agentops.CreateAgentAPIKey(Default, adminClient,
			cfg.DefaultOrg, cfg.DefaultProject, agentName, cfg.DefaultEnv,
			framework.CreateAgentAPIKeyRequest{
				DisplayName: "security-probe",
				ExpiresAt:   time.Now().Add(2 * time.Hour).Format(time.RFC3339),
			})
		Expect(response.ApiKey).NotTo(BeEmpty())
		return response.ApiKey
	}

	expectMCPAuthorization := func(apiKey, tool string, wantAllowed bool) {
		Eventually(func(g Gomega) {
			result := agentops.InvokeSecurityProbe[framework.SecurityMCPProbeResponse](
				g, http.MethodPost, endpointURL+"/security/mcp/"+tool, apiKey)
			g.Expect(result.Authorized).To(Equal(wantAllowed),
				"tool=%s phase=%s status=%v error=%s", tool, result.Phase, result.HTTPStatus, result.Error)
			if wantAllowed {
				g.Expect(result.TokenMinted).To(BeTrue())
				g.Expect(result.HTTPStatus).NotTo(BeNil())
				g.Expect(*result.HTTPStatus).To(Equal(http.StatusOK))
				g.Expect(result.ResultReceived).To(BeTrue())
			}
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	}

	BeforeAll(func() {
		roleName = "e2e-security-probe-" + suffix
		DeferCleanup(func() {
			agentops.DeleteAgentBestEffort(adminClient, cfg.DefaultOrg, cfg.DefaultProject, agentName)
			identityops.DeleteRoleBestEffort(adminClient, cfg.DefaultOrg, cfg.DefaultEnv, roleID)
			mcpproxyops.DeleteMCPProxyBestEffort(adminClient, cfg.DefaultOrg, proxyID)
		})
	})

	It("creates an identity-secured MCP proxy with two independently scoped tools", func() {
		contextPath := "/" + proxyID
		description := "Disposable MCP proxy for AgentID scope isolation"
		version := "2025-06-18"
		upstream := framework.TestMCPServerURL
		proxy := mcpproxyops.CreateMCPProxy(Default, adminClient, cfg.DefaultOrg,
			framework.CreateMCPProxyRequest{
				ID:             proxyID,
				Name:           proxyID,
				Version:        "v1.0",
				Description:    &description,
				Context:        &contextPath,
				McpSpecVersion: &version,
				Endpoints: []framework.MCPProxyEndpoint{{
					ID:       "primary",
					Name:     "primary",
					Upstream: framework.UpstreamConfig{Main: &framework.UpstreamEndpoint{URL: &upstream}},
					Capabilities: &framework.MCPProxyCapabilities{Tools: []map[string]any{
						{
							"name":        echoTool,
							"description": "Echo a harmless test string",
							"inputSchema": map[string]any{
								"type":       "object",
								"properties": map[string]any{"message": map[string]any{"type": "string"}},
								"required":   []string{"message"},
							},
						},
						{
							"name":        addTool,
							"description": "Add two harmless test integers",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"a": map[string]any{"type": "number"},
									"b": map[string]any{"type": "number"},
								},
								"required": []string{"a", "b"},
							},
						},
					}},
					Security: &framework.SecurityConfig{
						Enabled:  true,
						Identity: &framework.SecurityIdentity{Enabled: true},
					},
					Environments: []framework.MCPEndpointEnvironment{{EnvironmentUUID: envUUID}},
				}},
			})
		Expect(proxy.ID).To(Equal(proxyID))

		readScope := mcpproxyops.CreateScope(Default, adminClient, cfg.DefaultOrg, proxyID,
			framework.MCPProxyScopeRequest{
				Action:      "read",
				Description: "Allows only the echo probe",
				Tools:       []string{echoTool},
			})
		writeScope := mcpproxyops.CreateScope(Default, adminClient, cfg.DefaultOrg, proxyID,
			framework.MCPProxyScopeRequest{
				Action:      "write",
				Description: "Allows only the add probe",
				Tools:       []string{addTool},
			})
		scopeRead = readScope.Scope
		scopeWrite = writeScope.Scope
		Expect(scopeRead).To(Equal(proxyID + ":read"))
		Expect(scopeWrite).To(Equal(proxyID + ":write"))
	})

	It("creates and deploys the deterministic security probe agent", func() {
		agent := agentops.CreateAgent(Default, adminClient, &agentops.CreateAgentParams{
			OrgName:     cfg.DefaultOrg,
			ProjectName: cfg.DefaultProject,
			Request:     framework.NewSecurityProbeAgentRequest(cfg, agentName),
		})
		Expect(agent.Name).To(Equal(agentName))
		Expect(agent.Provisioning.Type).To(Equal("internal"))
		Expect(agent.AgentType.SubType).To(Equal("custom-api"))

		build.WaitForBuildSuccess(adminClient, &build.WaitForBuildParams{
			OrgName:     cfg.DefaultOrg,
			ProjectName: cfg.DefaultProject,
			AgentName:   agentName,
			Timeout:     25 * time.Minute,
		})
		deployment.WaitForDeployed(adminClient, &deployment.WaitForDeploymentParams{
			OrgName:     cfg.DefaultOrg,
			ProjectName: cfg.DefaultProject,
			AgentName:   agentName,
			Environment: cfg.DefaultEnv,
			Timeout:     10 * time.Minute,
		})
		deployment.WaitForReadiness(adminClient, cfg.DefaultOrg, cfg.DefaultProject,
			agentName, cfg.DefaultEnv, 5*time.Minute)

		endpoints := deployment.GetEndpoints(Default, adminClient,
			cfg.DefaultOrg, cfg.DefaultProject, agentName, cfg.DefaultEnv)
		endpointURL = strings.TrimRight(deployment.FirstEndpointURL(endpoints), "/")
		Expect(endpointURL).NotTo(BeEmpty())
	})

	It("runs with the required container hardening controls", func() {
		key := newProbeAPIKey()
		posture := agentops.InvokeSecurityProbe[framework.SecurityRuntimeProbeResponse](
			Default, http.MethodGet, endpointURL+"/security/runtime", key)
		Expect(posture.NonRoot).To(BeTrue(), "agent process ran as root")
		Expect(posture.RootFilesystemReadOnly).To(BeTrue(), "agent root filesystem was writable")
		Expect(posture.TmpWritable).To(BeTrue(), "sandbox did not provide its bounded writable /tmp")
		Expect(posture.ServiceAccountTokenPresent).To(BeFalse(), "agent pod received a Kubernetes service-account token")
		Expect(posture.EffectiveCapabilitiesDrop).To(BeTrue(), "agent retained effective Linux capabilities")
		Expect(posture.NoNewPrivileges).To(BeTrue(), "NoNewPrivs is not active")
		Expect(posture.SeccompEnabled).To(BeTrue(), "RuntimeDefault seccomp is not active")
	})

	It("blocks the running agent from reaching the Kubernetes API network path", func() {
		key := newProbeAPIKey()
		result := agentops.InvokeSecurityProbe[framework.SecurityNetworkProbeResponse](
			Default, http.MethodPost, endpointURL+"/security/network/kubernetes-api", key)
		Expect(result.Target).To(Equal("kubernetes-api"))
		Expect(result.Outcome).To(Equal("blocked"),
			"any HTTP response means the sandbox reached the Kubernetes API; outcome=%s evidence=%s status=%v",
			result.Outcome, result.Evidence, result.HTTPStatus)
		Expect(result.Evidence).To(BeElementOf("connect_timeout", "connect_rejected"),
			"only failure to establish a connection to the injected, known-live Kubernetes API is blocking evidence")
		Expect(result.HTTPStatus).To(BeNil())
	})

	It("attaches the MCP proxy and denies both tools before any role assignment", func() {
		config := configuration.CreateAgentMCPConfig(Default, adminClient,
			cfg.DefaultOrg, cfg.DefaultProject, agentName,
			framework.CreateAgentModelConfigRequest{
				Name: mcpConfigName,
				EnvMappings: map[string]framework.EnvModelConfigRequest{
					cfg.DefaultEnv: {MCPProxyName: proxyID},
				},
				EnvironmentVariables: []framework.EnvironmentVariableConfig{
					{Key: "url", Name: "SECURITY_MCP_URL"},
					{Key: "apikey", Name: "SECURITY_MCP_API_KEY"},
				},
			})
		Expect(config.UUID).NotTo(BeEmpty())

		key := newProbeAPIKey()
		Eventually(func(g Gomega) {
			identity := agentops.InvokeSecurityProbe[framework.SecurityIdentityProbeResponse](
				g, http.MethodPost, endpointURL+"/security/identity", key)
			g.Expect(identity.Configured).To(BeTrue())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		for _, tool := range []string{echoTool, addTool} {
			result := agentops.InvokeSecurityProbe[framework.SecurityMCPProbeResponse](
				Default, http.MethodPost, endpointURL+"/security/mcp/"+tool, key)
			Expect(result.Authorized).To(BeFalse(), "unassigned AgentID reached %s", tool)
		}
	})

	It("grants only the read scope and keeps the write tool denied", func() {
		role := identityops.CreateRole(Default, adminClient, cfg.DefaultOrg, cfg.DefaultEnv,
			framework.AgentIdentityRoleRequest{
				Name:        roleName,
				Description: "Disposable partial-scope AgentID role",
				Scopes:      []string{scopeRead},
			})
		roleID = role.ID
		Expect(roleID).NotTo(BeEmpty())

		var thunderAgentID string
		Eventually(func(g Gomega) {
			agents := identityops.ListAgents(g, adminClient, cfg.DefaultOrg, cfg.DefaultEnv)
			for _, candidate := range agents.Agents {
				if candidate.AgentName == agentName && candidate.ProjectName == cfg.DefaultProject &&
					strings.EqualFold(candidate.Status, "completed") {
					thunderAgentID = candidate.ThunderAgentID
					break
				}
			}
			g.Expect(thunderAgentID).NotTo(BeEmpty())
		}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

		identityops.AddRoleAssignments(Default, adminClient, cfg.DefaultOrg, cfg.DefaultEnv, roleID,
			[]framework.AgentIdentityAssignment{{ID: thunderAgentID, Type: "agent"}})

		key := newProbeAPIKey()
		Eventually(func(g Gomega) {
			identity := agentops.InvokeSecurityProbe[framework.SecurityIdentityProbeResponse](
				g, http.MethodPost, endpointURL+"/security/identity", key)
			g.Expect(identity.TokenMinted).To(BeTrue())
			g.Expect(identity.GrantedScopes).To(ContainElement(scopeRead))
			g.Expect(identity.GrantedScopes).NotTo(ContainElement(scopeWrite))
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

		expectMCPAuthorization(key, echoTool, true)
		expectMCPAuthorization(key, addTool, false)
	})

	It("adds the second scope and allows both tools with newly minted tokens", func() {
		updated := identityops.UpdateRole(Default, adminClient, cfg.DefaultOrg, cfg.DefaultEnv, roleID,
			framework.AgentIdentityRoleRequest{Name: roleName, Scopes: []string{scopeRead, scopeWrite}})
		Expect(updated.ID).To(Equal(roleID))

		key := newProbeAPIKey()
		Eventually(func(g Gomega) {
			identity := agentops.InvokeSecurityProbe[framework.SecurityIdentityProbeResponse](
				g, http.MethodPost, endpointURL+"/security/identity", key)
			g.Expect(identity.GrantedScopes).To(ContainElements(scopeRead, scopeWrite))
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		expectMCPAuthorization(key, echoTool, true)
		expectMCPAuthorization(key, addTool, true)
	})

	It("removes only the read scope while the write scope remains usable", func() {
		updated := identityops.UpdateRole(Default, adminClient, cfg.DefaultOrg, cfg.DefaultEnv, roleID,
			framework.AgentIdentityRoleRequest{Name: roleName, Scopes: []string{scopeWrite}})
		Expect(updated.ID).To(Equal(roleID))

		key := newProbeAPIKey()
		Eventually(func(g Gomega) {
			identity := agentops.InvokeSecurityProbe[framework.SecurityIdentityProbeResponse](
				g, http.MethodPost, endpointURL+"/security/identity", key)
			g.Expect(identity.GrantedScopes).NotTo(ContainElement(scopeRead))
			g.Expect(identity.GrantedScopes).To(ContainElement(scopeWrite))
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		expectMCPAuthorization(key, echoTool, false)
		expectMCPAuthorization(key, addTool, true)
	})

	It("refreshes the deployed workload after AgentID credential rotation", func() {
		rotated := agentops.RegenerateAgentIdentitySecret(Default, adminClient,
			cfg.DefaultOrg, cfg.DefaultProject, agentName, cfg.DefaultEnv)
		Expect(rotated.ProvisioningType).To(Equal("internal"))
		Expect(rotated.ClientSecret).NotTo(BeEmpty())

		key := newProbeAPIKey()
		Eventually(func(g Gomega) {
			identity := agentops.InvokeSecurityProbe[framework.SecurityIdentityProbeResponse](
				g, http.MethodPost, endpointURL+"/security/identity", key)
			g.Expect(identity.Configured).To(BeTrue())
			g.Expect(identity.TokenMinted).To(BeTrue())
			g.Expect(identity.GrantedScopes).To(ContainElement(scopeWrite))
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
		expectMCPAuthorization(key, addTool, true)
	})

	It("removes the deployed identity after revocation and denies further MCP access", func() {
		revoked, raw := agentops.RevokeAgentIdentitySecret(Default, adminClient,
			cfg.DefaultOrg, cfg.DefaultProject, agentName, cfg.DefaultEnv)
		Expect(revoked.Status).To(Equal("revoked"))
		Expect(strings.ToLower(strings.ReplaceAll(raw, "_", ""))).NotTo(ContainSubstring("clientsecret"))

		key := newProbeAPIKey()
		Eventually(func(g Gomega) {
			identity := agentops.InvokeSecurityProbe[framework.SecurityIdentityProbeResponse](
				g, http.MethodPost, endpointURL+"/security/identity", key)
			g.Expect(identity.Configured).To(BeFalse())
			g.Expect(identity.TokenMinted).To(BeFalse())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		result := agentops.InvokeSecurityProbe[framework.SecurityMCPProbeResponse](
			Default, http.MethodPost, endpointURL+"/security/mcp/"+addTool, key)
		Expect(result.Authorized).To(BeFalse())
	})

})

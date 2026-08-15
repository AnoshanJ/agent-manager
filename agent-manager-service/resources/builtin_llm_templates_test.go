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

package resources_test

import (
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/resources"
)

func templateByHandle(t *testing.T, handle string) *models.LLMProviderTemplate {
	t.Helper()
	for _, tmpl := range resources.BuiltInLLMProviderTemplates {
		if tmpl.Handle == handle {
			return tmpl
		}
	}
	t.Fatalf("built-in template %q not found", handle)
	return nil
}

// The gateway accepts only "api-key" as an upstream auth type and errors on
// everything else, so a template without an auth block leaves the console
// defaulting to "bearer" and the deployment is rejected.
func TestAWSBedrockTemplateDeclaresAPIKeyAuth(t *testing.T) {
	tmpl := templateByHandle(t, "awsbedrock")

	if tmpl.Metadata == nil || tmpl.Metadata.Auth == nil {
		t.Fatal("awsbedrock template has no auth block; console falls back to the gateway-rejected \"bearer\" type")
	}

	auth := tmpl.Metadata.Auth
	if auth.Type != "api-key" {
		t.Errorf("auth type = %q, want %q", auth.Type, "api-key")
	}
	if auth.Header != "Authorization" {
		t.Errorf("auth header = %q, want %q", auth.Header, "Authorization")
	}
	if auth.ValuePrefix != "Bearer " {
		t.Errorf("auth value prefix = %q, want %q", auth.ValuePrefix, "Bearer ")
	}
}

// The console keys the SigV4 credential modes off this declaration rather than
// hardcoding the awsbedrock handle, so any future signing provider can opt in.
func TestAWSBedrockTemplateDeclaresAWSAuthenticationPolicy(t *testing.T) {
	tmpl := templateByHandle(t, "awsbedrock")

	if tmpl.Metadata == nil || tmpl.Metadata.Auth == nil {
		t.Fatal("awsbedrock template has no auth block")
	}
	if got := tmpl.Metadata.Auth.Policy; got != "aws-authentication" {
		t.Errorf("auth policy = %q, want %q", got, "aws-authentication")
	}
}

// Every built-in template must use the one upstream auth type the gateway
// accepts. See llm_transformer.go: any other value hits the default branch and
// fails the whole deployment.
func TestBuiltInTemplatesOnlyUseGatewaySupportedAuthType(t *testing.T) {
	for _, tmpl := range resources.BuiltInLLMProviderTemplates {
		if tmpl.Metadata == nil || tmpl.Metadata.Auth == nil {
			continue
		}
		if got := tmpl.Metadata.Auth.Type; got != "api-key" {
			t.Errorf("template %q: auth type = %q, want %q", tmpl.Handle, got, "api-key")
		}
	}
}

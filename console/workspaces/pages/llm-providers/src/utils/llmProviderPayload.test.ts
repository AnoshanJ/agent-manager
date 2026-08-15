/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import type { LLMProviderTemplateResponse } from "@agent-management-platform/types";
import type { AddLLMProviderFormValues } from "../subComponents/AddLLMProviderForm";
import {
  buildCreateLLMProviderRequest,
  mapLLMProviderTemplatesToCards,
} from "./llmProviderPayload";

// Shaped as the backend now returns it, so this covers the real path:
// template response -> card -> create request.
const bedrockTemplateResponse = {
  id: "awsbedrock",
  name: "AWS Bedrock",
  metadata: {
    endpointUrl: "https://bedrock-runtime.us-east-1.amazonaws.com",
    auth: {
      type: "api-key",
      header: "Authorization",
      valuePrefix: "Bearer ",
    },
  },
} as unknown as LLMProviderTemplateResponse;

const [bedrockTemplate] = mapLLMProviderTemplatesToCards([bedrockTemplateResponse]);

const baseValues: AddLLMProviderFormValues = {
  templateId: "awsbedrock",
  displayName: "Bedrock",
  version: "v1.0",
  description: "",
  context: "",
  upstreamUrl: "https://bedrock-runtime.us-east-1.amazonaws.com",
  apiKey: "ABSK-test-key",
  gatewayIds: [],
};

describe("buildCreateLLMProviderRequest for AWS Bedrock", () => {
  // Guards the regression this fixes: a template with no auth block maps to an
  // undefined authType, which the builder defaults to the rejected "bearer".
  it("would fall back to bearer if the template declared no auth block", () => {
    const [cardWithoutAuth] = mapLLMProviderTemplatesToCards([
      { id: "awsbedrock", name: "AWS Bedrock", metadata: {} } as unknown as LLMProviderTemplateResponse,
    ]);

    const req = buildCreateLLMProviderRequest(baseValues, [], [cardWithoutAuth]);

    expect(req.upstream?.main?.auth?.type).toBe("bearer");
  });

  // The gateway accepts only "api-key" and errors on every other upstream auth
  // type, so a bedrock provider built as "bearer" fails to deploy.
  it("builds an api-key auth block, not the gateway-rejected bearer type", () => {
    const req = buildCreateLLMProviderRequest(baseValues, [], [bedrockTemplate]);

    expect(req.upstream?.main?.auth?.type).toBe("api-key");
  });

  it("sends the Bedrock API key as a Bearer token in the Authorization header", () => {
    const req = buildCreateLLMProviderRequest(baseValues, [], [bedrockTemplate]);

    expect(req.upstream?.main?.auth?.header).toBe("Authorization");
    expect(req.upstream?.main?.auth?.value).toBe("Bearer ABSK-test-key");
  });
});

describe("SigV4 credential sources", () => {
  const sigV4Values: AddLLMProviderFormValues = {
    ...baseValues,
    apiKey: "",
    awsCredentialSource: "iam-user-access-key",
    awsRegion: "eu-west-1",
    awsAccessKeyId: "AKIAEXAMPLE",
    awsSecretAccessKey: "secret-value",
    awsAuthPolicyVersion: "v0.10.0",
  };

  // The gateway rejects every upstream auth type but api-key, and a SigV4
  // signature cannot be a static header, so the block must be delegated.
  it("delegates upstream auth to a policy instead of sending a header", () => {
    const req = buildCreateLLMProviderRequest(sigV4Values, [], [bedrockTemplate]);

    expect(req.upstream?.main?.auth?.type).toBe("other");
    expect(req.upstream?.main?.auth?.value).toBeFalsy();
  });

  it("attaches aws-authentication on /* with the credentials", () => {
    const req = buildCreateLLMProviderRequest(sigV4Values, [], [bedrockTemplate]);

    const policy = req.policies?.find((p) => p.name === "aws-authentication");
    expect(policy).toBeDefined();
    expect(policy?.version).toBe("v0.10.0");
    expect(policy?.paths[0].path).toBe("/*");
    expect(policy?.paths[0].params).toEqual({
      service: "bedrock",
      region: "eu-west-1",
      authenticationType: "iam-user-access-key",
      awsAccessKeyID: "AKIAEXAMPLE",
      awsSecretAccessKey: "secret-value",
    });
  });

  // The policy schema sets additionalProperties:false, so a key belonging to a
  // different credential mode fails validation at the gateway.
  it("omits key/secret fields for role-based and ambient modes", () => {
    const req = buildCreateLLMProviderRequest(
      {
        ...sigV4Values,
        awsCredentialSource: "irsa",
        awsAccessKeyId: "",
        awsSecretAccessKey: "",
        awsRoleArn: "arn:aws:iam::123456789012:role/bedrock",
      },
      [],
      [bedrockTemplate],
    );

    const params = req.policies?.find((p) => p.name === "aws-authentication")?.paths[0].params;
    expect(params).toEqual({
      service: "bedrock",
      region: "eu-west-1",
      authenticationType: "irsa",
      awsRoleARN: "arn:aws:iam::123456789012:role/bedrock",
    });
  });

  it("sends only the required params for the ambient credential chain", () => {
    const req = buildCreateLLMProviderRequest(
      {
        ...sigV4Values,
        awsCredentialSource: "default-credential-chain",
        awsAccessKeyId: "",
        awsSecretAccessKey: "",
      },
      [],
      [bedrockTemplate],
    );

    const params = req.policies?.find((p) => p.name === "aws-authentication")?.paths[0].params;
    expect(params).toEqual({
      service: "bedrock",
      region: "eu-west-1",
      authenticationType: "default-credential-chain",
    });
  });

  it("leaves the api-key path untouched", () => {
    const req = buildCreateLLMProviderRequest(
      { ...baseValues, awsCredentialSource: "api-key" },
      [],
      [bedrockTemplate],
    );

    expect(req.upstream?.main?.auth?.type).toBe("api-key");
    expect(req.policies?.find((p) => p.name === "aws-authentication")).toBeUndefined();
  });
});

describe("consumer API key security", () => {
  // Consumer security guards callers of the provider; the upstream credential
  // authenticates the provider to its backend. They are unrelated, but security
  // used to be gated on the upstream api key field being filled -- so a provider
  // authenticating by SigV4 (no api key) deployed wide open, and the monitor's
  // proxy provisioner then skipped minting a provider API key for it.
  it("is enabled for a provider with no upstream API key", () => {
    const req = buildCreateLLMProviderRequest(
      { ...baseValues, apiKey: "" },
      [],
      [bedrockTemplate],
    );

    expect(req.security?.enabled).toBe(true);
    expect(req.security?.apiKey?.enabled).toBe(true);
  });

  it("stays enabled for a provider that does have one", () => {
    const req = buildCreateLLMProviderRequest(baseValues, [], [bedrockTemplate]);

    expect(req.security?.enabled).toBe(true);
  });
});

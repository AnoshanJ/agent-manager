/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type {
  CreateLLMProviderRequest,
  LLMProviderTemplateResponse,
  UpstreamAuthType,
} from "@agent-management-platform/types";
import type {
  AddLLMProviderFormValues,
  GuardrailSelection,
  TemplateCard,
} from "../subComponents/AddLLMProviderForm";

export const toProviderId = (name: string): string =>
  name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

export function mapLLMProviderTemplatesToCards(
  templates: LLMProviderTemplateResponse[] | undefined,
): TemplateCard[] {
  return (
    templates?.map((t) => ({
      id: t.id,
      handle: t.id,
      name: t.name,
      description: t.description,
      image: t.metadata?.logoUrl,
      hasTemplateUrl: Boolean(t.metadata?.endpointUrl),
      endpointUrl: t.metadata?.endpointUrl,
      hasTemplateAuthType: Boolean(t.metadata?.auth?.type),
      hasTemplateAuthHeader: Boolean(t.metadata?.auth?.header),
      authType: t.metadata?.auth?.type,
      authHeader: t.metadata?.auth?.header,
      authValuePrefix: t.metadata?.auth?.valuePrefix,
      authPolicy: t.metadata?.auth?.policy,
    })) ?? []
  );
}

/** SigV4 signing name for the Bedrock runtime API. */
const AWS_BEDROCK_SIGNING_SERVICE = "bedrock";
export const AWS_AUTHENTICATION_POLICY = "aws-authentication";

const usesSigV4 = (values: AddLLMProviderFormValues): boolean =>
  Boolean(values.awsCredentialSource && values.awsCredentialSource !== "api-key");

/**
 * Builds the aws-authentication policy for SigV4 credential sources. The policy
 * schema sets additionalProperties:false, so only the keys belonging to the
 * selected credential source may be sent.
 */
function buildAWSAuthenticationPolicy(values: AddLLMProviderFormValues) {
  if (!usesSigV4(values)) return undefined;

  const params: Record<string, string> = {
    service: AWS_BEDROCK_SIGNING_SERVICE,
    region: values.awsRegion?.trim() ?? "",
    authenticationType: values.awsCredentialSource as string,
  };

  const set = (key: string, value: string | undefined) => {
    const trimmed = value?.trim();
    if (trimmed) params[key] = trimmed;
  };

  switch (values.awsCredentialSource) {
    case "iam-user-access-key":
      set("awsAccessKeyID", values.awsAccessKeyId);
      set("awsSecretAccessKey", values.awsSecretAccessKey);
      set("awsSessionToken", values.awsSessionToken);
      break;
    case "sts-assume-role":
      set("awsRoleARN", values.awsRoleArn);
      set("awsRoleExternalID", values.awsRoleExternalId);
      set("awsRoleSessionName", values.awsRoleSessionName);
      // Optional base credentials used to call sts:AssumeRole.
      set("awsAccessKeyID", values.awsAccessKeyId);
      set("awsSecretAccessKey", values.awsSecretAccessKey);
      break;
    case "irsa":
      set("awsRoleARN", values.awsRoleArn);
      set("awsRoleSessionName", values.awsRoleSessionName);
      break;
    default:
      // default-credential-chain resolves credentials ambiently.
      break;
  }

  return {
    name: AWS_AUTHENTICATION_POLICY,
    version: values.awsAuthPolicyVersion?.trim() || "v0",
    paths: [{ path: "/*", methods: ["*"], params }],
  };
}

export function buildCreateLLMProviderRequest(
  values: AddLLMProviderFormValues,
  guardrails: GuardrailSelection[],
  templates: TemplateCard[],
): CreateLLMProviderRequest {
  const normalizedDisplayName = values.displayName?.trim() || "";
  const providerId = toProviderId(normalizedDisplayName) || "llm-provider";
  const selectedTemplate = templates.find((tpl) => tpl.id === values.templateId);
  const templateHandle =
    selectedTemplate?.handle || selectedTemplate?.name || values.templateId;

  const guardrailPolicies = guardrails.map((g) => ({
    name: g.name,
    version: g.version,
    paths: [
      {
        path: "/*",
        methods: ["*"],
        params: g.settings ?? {},
      },
    ],
  }));

  const awsAuthPolicy = buildAWSAuthenticationPolicy(values);
  const allPolicies = awsAuthPolicy
    ? [...guardrailPolicies, awsAuthPolicy]
    : guardrailPolicies;
  const policies = allPolicies.length > 0 ? allPolicies : undefined;

  const contextPath = values.context?.trim() || ``;

  const authType: UpstreamAuthType =
    (selectedTemplate?.authType as UpstreamAuthType) ?? "bearer";
  const authHeader = selectedTemplate?.authHeader ?? "Authorization";
  const apiKey = values.apiKey?.trim() ?? "";
  const authValue = apiKey
    ? selectedTemplate?.authValuePrefix
      ? `${selectedTemplate.authValuePrefix}${apiKey}`
      : authType === "bearer"
        ? `Bearer ${apiKey}`
        : apiKey
    : "";

  return {
    id: providerId,
    name: normalizedDisplayName || providerId,
    version: values.version.trim(),
    context: contextPath,
    template: templateHandle,
    upstream: {
      main: {
        url: values.upstreamUrl?.trim(),
        // "other" tells the backend to omit the auth block from the gateway
        // artifact; the aws-authentication policy signs the request instead.
        auth: usesSigV4(values)
          ? { type: "other" as UpstreamAuthType }
          : values.apiKey
            ? {
                type: authType,
                header: authHeader,
                value: authValue,
              }
            : undefined,
      },
    },
    resilience:
      values.resilienceTimeout?.trim() || values.resilienceIdleTimeout?.trim()
        ? {
            timeout: values.resilienceTimeout?.trim() || undefined,
            idleTimeout: values.resilienceIdleTimeout?.trim() || undefined,
          }
        : undefined,
    description: values.description?.trim() || undefined,
    // Independent of the upstream credential: this guards callers of the
    // provider, and a provider authenticating upstream by SigV4 has no api key.
    security: {
      enabled: true,
      apiKey: {
        enabled: true,
        key: "X-API-Key",
        in: "header",
      },
    },
    policies,
    gateways:
      values.gatewayIds && values.gatewayIds.length > 0
        ? values.gatewayIds
        : undefined,
    accessControl: {
      exceptions: [],
      mode: "allow_all",
    },
  };
}

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

import React from "react";
import { Alert, Form, MenuItem, TextField } from "@wso2/oxygen-ui";
import type {
  AddLLMProviderFormValues,
  AWSCredentialSource,
} from "../form/schema";

type CredentialSourceOption = {
  value: AWSCredentialSource;
  label: string;
  description: string;
  /** Whether the option needs the aws-authentication policy on the gateway. */
  requiresPolicy: boolean;
};

export const CREDENTIAL_SOURCE_OPTIONS: CredentialSourceOption[] = [
  {
    value: "api-key",
    label: "API key",
    description:
      "A Bedrock API key, sent as a bearer token. Simplest option and stored encrypted.",
    requiresPolicy: false,
  },
  {
    value: "iam-user-access-key",
    label: "IAM user access key",
    description: "Sign requests with a long-lived access key and secret.",
    requiresPolicy: true,
  },
  {
    value: "sts-assume-role",
    label: "Assume an IAM role",
    description: "Exchange credentials for temporary ones via AWS STS.",
    requiresPolicy: true,
  },
  {
    value: "irsa",
    label: "IAM Roles for Service Accounts (IRSA)",
    description:
      "Use the gateway's Kubernetes service account. Requires an EKS cluster federated with AWS IAM.",
    requiresPolicy: true,
  },
  {
    value: "default-credential-chain",
    label: "Gateway's own credentials",
    description:
      "Resolve credentials from the gateway's environment: instance profile, task role, or pod identity.",
    requiresPolicy: true,
  },
];

export interface AWSAuthFieldsProps {
  values: AddLLMProviderFormValues;
  errors: Partial<Record<keyof AddLLMProviderFormValues, string | undefined>>;
  onChange: (
    field: keyof AddLLMProviderFormValues,
    value: string | string[],
  ) => void;
  /** False when no active gateway advertises aws-authentication. */
  policyAvailable: boolean;
  policyName: string;
}

export const AWSAuthFields: React.FC<AWSAuthFieldsProps> = ({
  values,
  errors,
  onChange,
  policyAvailable,
  policyName,
}) => {
  const source = values.awsCredentialSource ?? "api-key";
  const usesSigV4 = source !== "api-key";
  const selected = CREDENTIAL_SOURCE_OPTIONS.find((o) => o.value === source);

  const text = (
    field: keyof AddLLMProviderFormValues,
    label: string,
    opts: { secret?: boolean; placeholder?: string; helper?: string } = {},
  ) => (
    <Form.ElementWrapper label={label} name={field}>
      <TextField
        id={field}
        fullWidth
        type={opts.secret ? "password" : "text"}
        value={(values[field] as string) ?? ""}
        onChange={(e) => onChange(field, e.target.value)}
        placeholder={opts.placeholder}
        error={Boolean(errors[field])}
        helperText={errors[field] ?? opts.helper}
      />
    </Form.ElementWrapper>
  );

  return (
    <>
      <Form.ElementWrapper label="Credential source" name="awsCredentialSource">
        <TextField
          id="awsCredentialSource"
          select
          fullWidth
          value={source}
          onChange={(e) =>
            onChange(
              "awsCredentialSource",
              e.target.value as AWSCredentialSource,
            )
          }
          helperText={selected?.description}
        >
          {CREDENTIAL_SOURCE_OPTIONS.map((option) => (
            <MenuItem
              key={option.value}
              value={option.value}
              disabled={option.requiresPolicy && !policyAvailable}
            >
              {option.label}
            </MenuItem>
          ))}
        </TextField>
      </Form.ElementWrapper>

      {!policyAvailable && (
        <Alert severity="info">
          Signing with AWS credentials needs the <code>{policyName}</code> policy,
          which none of this organization&apos;s active gateways currently
          advertise. Upgrade the gateway to enable those options, or use an API
          key.
        </Alert>
      )}

      {usesSigV4 && (
        <>
          {text("awsRegion", "Region", {
            placeholder: "us-east-1",
            helper: "AWS region of the Bedrock endpoint.",
          })}

          {source === "iam-user-access-key" && (
            <>
              {text("awsAccessKeyId", "Access key ID", {
                placeholder: "AKIA...",
              })}
              {text("awsSecretAccessKey", "Secret access key", {
                secret: true,
              })}
              {text("awsSessionToken", "Session token (optional)", {
                secret: true,
                helper: "Only for temporary credentials.",
              })}
            </>
          )}

          {source === "sts-assume-role" && (
            <>
              {text("awsRoleArn", "Role ARN", {
                placeholder: "arn:aws:iam::123456789012:role/bedrock",
              })}
              {text("awsRoleExternalId", "External ID (optional)", {
                helper: "For cross-account role assumption.",
              })}
              {text("awsRoleSessionName", "Session name (optional)", {
                placeholder: "aws-authentication-session",
              })}
              {text("awsAccessKeyId", "Base access key ID (optional)", {
                helper:
                  "Leave empty to call AssumeRole with the gateway's own credentials.",
              })}
              {text("awsSecretAccessKey", "Base secret access key (optional)", {
                secret: true,
              })}
            </>
          )}

          {source === "irsa" &&
            text("awsRoleArn", "Role ARN (optional)", {
              helper:
                "Defaults to the AWS_ROLE_ARN injected into the gateway's service account.",
            })}
        </>
      )}
    </>
  );
};

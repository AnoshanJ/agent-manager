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

const AWS_REGIONAL_HOST = /\.([a-z]{2}(?:-[a-z]+)+-\d+)\.amazonaws\.com/;

/**
 * Extracts the AWS region from a regional endpoint host, e.g.
 * bedrock-runtime.us-east-1.amazonaws.com. Returns "" when the host is not a
 * regional AWS endpoint — SigV4 signs over the region, so a wrong guess fails
 * at AWS with an opaque signature error, and no prefill is better than a bad one.
 */
export function regionFromEndpoint(endpointUrl?: string): string {
  if (!endpointUrl) return "";
  return endpointUrl.match(AWS_REGIONAL_HOST)?.[1] ?? "";
}

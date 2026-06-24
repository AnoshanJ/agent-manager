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

/**
 * Config-kind external module mount point for gateway identity provider mode.
 *
 * Must match `MountPoints.IdentityProviderMode` in `@agent-management-platform/am-core-ui`.
 * That package depends on the gateways page, so it cannot be imported here without a
 * dependency cycle — pages reference mount points by their string value (the same way
 * `useExternalConfigModules("private-repo-support")` is used elsewhere).
 */
export const IDENTITY_PROVIDER_MODE_MOUNT_POINT = "identity-provider-mode";

/** Shape of the config value a host injects at {@link IDENTITY_PROVIDER_MODE_MOUNT_POINT}. */
export interface IdentityProviderModeConfig {
  enabled?: boolean;
}

/**
 * Identity providers are server-managed only when a host explicitly injects
 * `{ enabled: true }`. With nothing injected (self-hosted OSS), the dialog renders
 * the manage-identity-provider script instead of calling the REST API.
 */
export function isIdentityProviderServerManaged(
  modules: { value: object }[],
): boolean {
  return modules.some(
    (module) => (module.value as IdentityProviderModeConfig)?.enabled === true,
  );
}

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
import {
  IDENTITY_PROVIDER_MODE_MOUNT_POINT,
  isIdentityProviderServerManaged,
} from "./identityProviderMode";

describe("isIdentityProviderServerManaged", () => {
  it("is false when nothing is injected (self-hosted OSS renders the script)", () => {
    expect(isIdentityProviderServerManaged([])).toBe(false);
  });

  it("is true when a host injects { enabled: true }", () => {
    expect(
      isIdentityProviderServerManaged([{ value: { enabled: true } }]),
    ).toBe(true);
  });

  it("is false when a host injects { enabled: false }", () => {
    expect(
      isIdentityProviderServerManaged([{ value: { enabled: false } }]),
    ).toBe(false);
  });

  it("is false when the injected value omits enabled", () => {
    expect(isIdentityProviderServerManaged([{ value: {} }])).toBe(false);
  });

  it("is true when any injected module enables it", () => {
    expect(
      isIdentityProviderServerManaged([
        { value: { enabled: false } },
        { value: { enabled: true } },
      ]),
    ).toBe(true);
  });

  it("exposes the mount point string used to read the config module", () => {
    expect(IDENTITY_PROVIDER_MODE_MOUNT_POINT).toBe("identity-provider-mode");
  });
});

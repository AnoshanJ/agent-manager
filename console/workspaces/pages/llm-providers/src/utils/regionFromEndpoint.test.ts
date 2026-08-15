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
import { regionFromEndpoint } from "./awsRegion";

describe("regionFromEndpoint", () => {
  it("reads the region from a Bedrock runtime endpoint", () => {
    expect(
      regionFromEndpoint("https://bedrock-runtime.us-east-1.amazonaws.com"),
    ).toBe("us-east-1");
  });

  it("handles regions with a longer middle segment", () => {
    expect(
      regionFromEndpoint("https://bedrock-runtime.ap-southeast-2.amazonaws.com"),
    ).toBe("ap-southeast-2");
  });

  // Prefilling a wrong region is worse than prefilling none: SigV4 signs over
  // the region, so a bad guess fails at AWS with an opaque signature error.
  it("returns empty rather than guessing for a non-AWS endpoint", () => {
    expect(regionFromEndpoint("https://llm.internal.example.com")).toBe("");
    expect(regionFromEndpoint(undefined)).toBe("");
    expect(regionFromEndpoint("")).toBe("");
  });
});

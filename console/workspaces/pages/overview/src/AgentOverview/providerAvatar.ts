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

// Brand-ish colors for the handful of providers/config names users are most
// likely to see here; anything else falls back to a deterministic hash color
// so it's still stable across renders instead of random.
const KNOWN_PROVIDER_COLORS: Record<string, string> = {
    openai: "#000000",
    azure: "#0078D4",
    "azure-openai": "#0078D4",
    anthropic: "#B45AF2",
    claude: "#B45AF2",
    google: "#4285F4",
    gemini: "#4285F4",
    vertex: "#4285F4",
    mistral: "#FA520F",
    cohere: "#39594D",
    bedrock: "#FF9900",
    aws: "#FF9900",
    meta: "#0668E1",
    llama: "#0668E1",
};

const FALLBACK_COLORS = [
    "#5B5FEE", "#0EA5E9", "#EC4899", "#F59E0B", "#10B981", "#8B5CF6",
];

function hashString(value: string): number {
    let hash = 0;
    for (let i = 0; i < value.length; i += 1) {
        hash = (hash * 31 + value.charCodeAt(i)) | 0;
    }
    return Math.abs(hash);
}

/** Picks a stable avatar color for a provider/config name — a curated brand
 * color when recognized, otherwise a deterministic hash-based fallback. */
export function getProviderAvatarColor(name?: string): string {
    if (!name) return FALLBACK_COLORS[0];
    const key = name.trim().toLowerCase();
    const known = Object.keys(KNOWN_PROVIDER_COLORS).find((k) => key.includes(k));
    if (known) return KNOWN_PROVIDER_COLORS[known];
    return FALLBACK_COLORS[hashString(key) % FALLBACK_COLORS.length];
}

export function getAvatarInitial(name: string): string {
    return name.trim().charAt(0).toUpperCase() || "?";
}

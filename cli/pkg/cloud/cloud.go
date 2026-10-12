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

// Package cloud holds what amctl needs to reach Agent Manager on WSO2 Cloud, so that
// cloud-only knowledge stays out of the instance-agnostic login and API code.
package cloud

import (
	"net/url"
	"strings"
)

const (
	GatewayURLEnv     = "AMCTL_CLOUD_URL"
	DefaultGatewayURL = "https://production-wso2cloud.gateway.cloud.wso2.com"
	APIPath           = "/agent-manager-service-agent-manager-api"
	Resource          = "urn:wso2:amp"
	BaseDomain        = "cloud.wso2.com"
)

// APIURL returns the Agent Manager API base URL on WSO2 Cloud: the gateway in
// AMCTL_CLOUD_URL (production when unset) followed by APIPath. Use it to build the
// instance URL for amctl login --cloud.
func APIURL(getenv func(string) string) string {
	gateway := strings.TrimSpace(getenv(GatewayURLEnv))
	if gateway == "" {
		gateway = DefaultGatewayURL
	}
	return strings.TrimRight(gateway, "/") + APIPath
}

// IsAPIURL reports whether raw points at the Agent Manager API on a WSO2 Cloud gateway
// (a cloud.wso2.com host with a path under APIPath). Use it to catch a cloud URL passed
// to --url, which would log in without the cloud resource indicator and get no scopes.
func IsAPIURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != BaseDomain && !strings.HasSuffix(host, "."+BaseDomain) {
		return false
	}
	path := strings.TrimRight(u.Path, "/")
	return path == APIPath || strings.HasPrefix(path, APIPath+"/")
}

// GatewayOf returns the scheme and host of a WSO2 Cloud API URL, or "" if raw does not
// parse. Use it to tell users which AMCTL_CLOUD_URL value selects that environment.
func GatewayOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

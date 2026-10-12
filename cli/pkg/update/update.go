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

// Package update finds the latest published amctl, so commands can tell users when the
// binary they run is out of date.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
)

const (
	releasesURL = "https://api.github.com/repos/wso2/agent-manager/releases?per_page=100"
	// assetPrefix names the amctl binaries attached to a release: amctl_v<version>_<os>_<arch>.*
	assetPrefix = "amctl_v"
	// maxPages caps the releases walk (100 per page) so a long list cannot stall login.
	maxPages = 10
)

const tagPrefix = "amctl/v"

type Release struct {
	Version string
	URL     string
}

func Latest(ctx context.Context, httpc *http.Client) (Release, error) {
	// GitHub pages the list (newest first); follow it so amctl releases stay
	// discoverable however many other components release after them.
	var releases []githubRelease
	next := releasesURL
	// The list is newest first, so stopping at maxPages keeps the newest releases.
	for page := 0; next != "" && page < maxPages; page++ {
		batch, link, err := fetchReleases(ctx, httpc, next)
		if err != nil {
			return Release{}, err
		}
		releases = append(releases, batch...)
		next = link
	}

	platform := "_" + runtime.GOOS + "_" + runtime.GOARCH + "."
	var latest Release
	for _, r := range releases {
		if r.Draft || r.Prerelease {
			continue
		}
		version, ok := streamVersion(r.TagName)
		if !ok {
			continue
		}
		if latest.Version != "" && !Newer(latest.Version, version) {
			continue
		}
		candidate := Release{Version: version}
		ships := false
		for _, a := range r.Assets {
			if !strings.HasPrefix(a.Name, assetPrefix+version+"_") {
				continue
			}
			ships = true
			if strings.Contains(a.Name, platform) {
				candidate.URL = a.DownloadURL
			}
		}
		if !ships {
			continue
		}
		if candidate.URL == "" {
			candidate.URL = r.HTMLURL
		}
		latest = candidate
	}
	if latest.Version == "" {
		return Release{}, fmt.Errorf("no published amctl release")
	}
	return latest, nil
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// fetchReleases returns one page of releases and the URL of the next page, or "" on
// the last page.
func fetchReleases(ctx context.Context, httpc *http.Client, url string) ([]githubRelease, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	var page []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, "", fmt.Errorf("decode releases: %w", err)
	}
	return page, nextLink(resp.Header.Get("Link")), nil
}

// nextLink returns the rel="next" URL from a GitHub Link header, or "" if there is none.
func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		target, params, ok := strings.Cut(part, ";")
		if !ok || !strings.Contains(params, `rel="next"`) {
			continue
		}
		return strings.Trim(strings.TrimSpace(target), "<>")
	}
	return ""
}

func streamVersion(tag string) (string, bool) {
	version, ok := strings.CutPrefix(tag, tagPrefix)
	if !ok {
		return "", false
	}
	if _, valid := parse(version); !valid {
		return "", false
	}
	return version, true
}

func Newer(current, latest string) bool {
	c, ok := parse(current)
	if !ok {
		return false
	}
	l, ok := parse(latest)
	if !ok {
		return false
	}
	for i := range c.core {
		if c.core[i] != l.core[i] {
			return c.core[i] < l.core[i]
		}
	}
	// Same core: a pre-release (1.0.0-rc3) is older than its release (1.0.0).
	return c.pre != "" && l.pre == ""
}

type semver struct {
	core [3]int
	pre  string
}

func parse(v string) (semver, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	core, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var s semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		s.core[i] = n
	}
	s.pre = pre
	return s, true
}

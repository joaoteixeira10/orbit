// Package version exposes build/version info and a release checker that polls
// the app's own (public) GitHub releases API for a newer version.
package version

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/safedial"
)

// RepositoryURL is the default upstream repo (shown in About + used for the
// update check). It can be overridden via REPO_URL, or the check disabled
// entirely via UPDATE_CHECK=false (see config + NewChecker).
const RepositoryURL = "https://github.com/joaoteixeira10/orbit"

// These are overridden at build time via -ldflags, e.g.:
//
//	-X github.com/joaoteixeira10/orbit/api/internal/version.Version=1.2.3
//	-X github.com/joaoteixeira10/orbit/api/internal/version.Commit=abc1234
//	-X github.com/joaoteixeira10/orbit/api/internal/version.BuildTime=2026-05-29T12:00:00Z
//
// Version is bumped per release by scripts/set-version.sh (committed before the
// build) so the value is consistent across the repo. The build/release workflow
// also injects Version/Commit/BuildTime via -ldflags; the -ldflags Version wins
// at runtime, and Commit/BuildTime are only set that way.
var (
	Version   = "0.5.0"
	Commit    = ""
	BuildTime = ""
)

// Build describes the running binary.
type Build struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// Current returns build info. ldflags-injected values win; otherwise it falls
// back to the VCS stamps `go build` embeds (when building inside a git repo).
func Current() Build {
	b := Build{Version: Version, GoVersion: runtime.Version(), Commit: Commit, BuildTime: BuildTime}
	if b.Commit == "" || b.BuildTime == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					if b.Commit == "" {
						b.Commit = s.Value
					}
				case "vcs.time":
					if b.BuildTime == "" {
						b.BuildTime = s.Value
					}
				}
			}
		}
	}
	return b
}

// Checker polls a repo's releases endpoint (returning JSON with a `tag_name`
// field) for the latest version. The repo is configurable and the check can be
// disabled.
type Checker struct {
	client   *http.Client
	repoURL  string // repo web URL (shown in About)
	releases string // derived "latest release" API URL
	enabled  bool
	mu       sync.RWMutex
	latest   string
}

// NewChecker builds a release checker. repoURL defaults to RepositoryURL when
// empty; enabled=false disables the update check (the repo URL is still used for
// the About link). The releases API URL is derived from the repo URL.
func NewChecker(repoURL string, enabled bool) *Checker {
	if repoURL == "" {
		repoURL = RepositoryURL
	}
	return &Checker{
		client:   safedial.GuardedClient(10 * time.Second),
		repoURL:  repoURL,
		releases: releasesAPI(repoURL),
		enabled:  enabled,
	}
}

// releasesAPI derives the "latest release" API URL from a repo web URL:
//
//	https://github.com/owner/repo → https://api.github.com/repos/owner/repo/releases/latest
//	https://host/owner/repo       → https://host/api/v1/repos/owner/repo/releases/latest (Gitea/Forgejo, for a REPO_URL override)
//
// GitHub's latest release skips drafts and pre-releases.
func releasesAPI(repoURL string) string {
	u, err := url.Parse(repoURL)
	if err != nil || u.Host == "" {
		return ""
	}
	p := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	if p == "" {
		return ""
	}
	if h := strings.ToLower(u.Host); h == "github.com" || h == "www.github.com" {
		return "https://api.github.com/repos/" + p + "/releases/latest"
	}
	return u.Scheme + "://" + u.Host + "/api/v1/repos/" + p + "/releases/latest"
}

// Enabled reports whether the update check runs.
func (c *Checker) Enabled() bool { return c.enabled && c.releases != "" }

// RepoURL is the configured repository web URL (for the About link).
func (c *Checker) RepoURL() string { return c.repoURL }

// Latest returns the most recently fetched release version (without a leading
// "v"), or "" if unknown / not yet checked.
func (c *Checker) Latest() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

// UpdateAvailable reports whether a release newer than the running version is
// known, by semver order (so a dev build ahead of the latest release isn't
// flagged — PKI Management compared for inequality only).
func (c *Checker) UpdateAvailable() bool {
	l := c.Latest()
	return l != "" && newer(l, strings.TrimPrefix(Version, "v"))
}

// newer reports whether semver a > b, comparing MAJOR.MINOR.PATCH numerically;
// a pre-release (a "-..." suffix) sorts before its release. Unparseable
// versions are never newer.
func newer(a, b string) bool {
	pa, preA, okA := parseSemver(a)
	pb, preB, okB := parseSemver(b)
	if !okA || !okB {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return preA == "" && preB != "" // 1.2.0 > 1.2.0-rc1
}

func parseSemver(v string) (nums [3]int, pre string, ok bool) {
	v, pre, _ = strings.Cut(strings.TrimPrefix(v, "v"), "-")
	v, _, _ = strings.Cut(v, "+")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nums, "", false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nums, "", false
		}
		nums[i] = n
	}
	return nums, pre, true
}

// Check fetches the latest release tag and caches it. No-op when disabled.
func (c *Checker) Check(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.releases, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil // e.g. 404 when no releases exist yet
	}
	var body struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	tag := body.TagName
	if tag == "" {
		tag = body.Name
	}
	c.mu.Lock()
	c.latest = strings.TrimPrefix(tag, "v")
	c.mu.Unlock()
	return nil
}

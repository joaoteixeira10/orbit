package version

import (
	"context"
	"testing"
)

func TestNewCheckerDefaultEnabled(t *testing.T) {
	c := NewChecker("", true)
	if c.RepoURL() != RepositoryURL {
		t.Errorf("default repo = %q, want %q", c.RepoURL(), RepositoryURL)
	}
	if !c.Enabled() {
		t.Error("default checker should be enabled")
	}
}

func TestNewCheckerOverrideAndDisable(t *testing.T) {
	c := NewChecker("https://git.example.org/me/fork", false)
	if c.RepoURL() != "https://git.example.org/me/fork" {
		t.Errorf("repo override not applied: %q", c.RepoURL())
	}
	if c.Enabled() {
		t.Error("disabled checker should report Enabled()=false")
	}
	// A disabled Check is a no-op (no network) and returns nil.
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("disabled Check should no-op, got %v", err)
	}
}

// TestReleasesAPIDerivation: a GitHub repo uses api.github.com, any other host
// the Gitea-style /api/v1 path; a bad or empty URL gives no endpoint.
func TestReleasesAPIDerivation(t *testing.T) {
	cases := []struct{ repo, want string }{
		{"https://github.com/joaoteixeira10/orbit", "https://api.github.com/repos/joaoteixeira10/orbit/releases/latest"},
		{"https://github.com/me/fork.git", "https://api.github.com/repos/me/fork/releases/latest"},
		{"https://git.example.org/me/fork", "https://git.example.org/api/v1/repos/me/fork/releases/latest"},
		{"", ""},
		{"https://github.com/", ""},
	}
	for _, c := range cases {
		if got := releasesAPI(c.repo); got != c.want {
			t.Errorf("releasesAPI(%q) = %q, want %q", c.repo, got, c.want)
		}
	}
}

// TestDefaultRepo: unstamped, the update check reads GitHub's releases.
func TestDefaultRepo(t *testing.T) {
	if c := NewChecker("", true); c.RepoURL() != "https://github.com/joaoteixeira10/orbit" || !c.Enabled() {
		t.Errorf("default: %q enabled=%v", c.RepoURL(), c.Enabled())
	}
}

// TestNewerIsSemver: releases compare numerically, a release beats its
// pre-release, and garbage is never newer.
func TestNewerIsSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.10.0", "0.9.0", true},
		{"0.9.0", "0.10.0", false},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "1.0.0-rc1", true},
		{"1.0.0-rc1", "1.0.0", false},
		{"v2.0.1", "2.0.0", true},
		{"latest", "1.0.0", false},
	}
	for _, c := range cases {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

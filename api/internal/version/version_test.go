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

func TestReleasesAPIDerivation(t *testing.T) {
	got := releasesAPI("https://git.example.org/me/fork")
	want := "https://git.example.org/api/v1/repos/me/fork/releases/latest"
	if got != want {
		t.Errorf("releasesAPI = %q, want %q", got, want)
	}
	if u := releasesAPI(""); u != "" {
		t.Errorf("releasesAPI(\"\") = %q, want empty", u)
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

package helpdocs

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestGuides(t *testing.T) {
	s := From(fstest.MapFS{
		"USER-GUIDE.md":    {Data: []byte("# Orbit user guide\n\nIntro.\n\n## 14. Whiteboards\n\nDraw.\n\n```\n## not a heading\n```\n\n## Whiteboards\n\nAgain.\n")},
		"SSO-authentik.md": {Data: []byte("# SSO with Authentik (OIDC)\n\nIntro.\n\n## 3. Create the provider in Authentik\n\nIn **Applications → Providers**, the redirect URI `/api/v1/auth/oidc/callback`.\n\n## Troubleshooting\n\nRedirect mismatch: the slug.\n")},
		"CONFIGURATION.md": {Data: []byte("# Configuration\n\n## Uploads\n\n`UPLOAD_STORE` db, dir or s3; Authentik is not here.\n")},
		"ARCHITECTURE.md":  {Data: []byte("# Not a guide shown\n")},
	})
	var slugs []string
	for _, g := range s.List() {
		slugs = append(slugs, g.Slug)
	}
	if len(slugs) != 3 || slugs[0] != "user-guide" || slugs[1] != "configuration" || slugs[2] != "sso-authentik" {
		t.Fatalf("guides, in order, only those found: %v", slugs)
	}
	if _, _, ok := s.Get("architecture"); ok {
		t.Fatal("a file not listed isn't a guide")
	}

	// Anchors as GitHub (and the docs page) make them; a repeat gets "-1".
	secs := sections(Guide{Slug: "user-guide", Title: "User guide"}, s.text["user-guide"])
	var anchors []string
	for _, x := range secs {
		anchors = append(anchors, x.Anchor)
	}
	if want := []string{"orbit-user-guide", "14-whiteboards", "whiteboards"}; len(anchors) != 3 || anchors[0] != want[0] || anchors[1] != want[1] || anchors[2] != want[2] {
		t.Fatalf("anchors (a heading in a code block isn't one): %v", anchors)
	}
	if Anchor("SSO with Authentik (OIDC)") != "sso-with-authentik-oidc" || Anchor("Back up — and restore") != "back-up--and-restore" {
		t.Fatalf("anchor: %q %q", Anchor("SSO with Authentik (OIDC)"), Anchor("Back up — and restore"))
	}

	// The page: headings carry their anchors; a link to a guide goes to its
	// page, to another file keeps its text.
	s.text["configuration"] += "\nSee [Authentik](SSO-authentik.md#troubleshooting), [the blueprint](AUTH-BLUEPRINT.md) and [news](../CHANGELOG.md).\n"
	_, page, _ := s.Page("configuration")
	if !strings.Contains(page, `## <a data-anchor="uploads"></a>Uploads`) || !strings.Contains(page, "[Authentik](/docs/sso-authentik#troubleshooting)") || !strings.Contains(page, ", the blueprint and news.") {
		t.Fatalf("page: %s", page)
	}

	// The section that answers: every word, the heading counting most.
	hits := s.Search("How do I set up Authentik provider?", 5)
	if len(hits) == 0 || hits[0].Slug != "sso-authentik" || hits[0].Anchor != "3-create-the-provider-in-authentik" {
		t.Fatalf("search: %+v", hits)
	}
	if hits := s.Search("upload_store", 5); len(hits) == 0 || hits[0].Anchor != "uploads" {
		t.Fatalf("a setting's name: %+v", hits)
	}
	if hits := s.Search("the and", 5); hits != nil {
		t.Fatalf("only empty words: %+v", hits)
	}
}

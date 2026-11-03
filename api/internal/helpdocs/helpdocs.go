// Package helpdocs holds Orbit's guides (docs/*.md and the changelog), as the
// running version has them: copied into files/ at build (the Dockerfile) and
// embedded, or read from the repository's docs/ when run from source. The
// docs page shows them, and the assistants' orbit_help tool searches them.
package helpdocs

import (
	"embed"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

//go:embed all:files
var embedded embed.FS

// Guide is one guide: its address on the docs page, title and file.
type Guide struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	File  string `json:"file"`
}

// The guides shown, in this order (the others in docs/ are for those working
// on Orbit itself: its blueprint, architecture, roadmap, releasing).
var guides = []Guide{
	{"user-guide", "User guide", "USER-GUIDE.md"},
	{"configuration", "Configuration", "CONFIGURATION.md"},
	{"reverse-proxies", "Behind a reverse proxy", "reverse-proxies.md"},
	{"sso-authentik", "SSO with Authentik", "SSO-authentik.md"},
	{"sso-entra", "SSO with Microsoft Entra ID", "SSO-entra.md"},
	{"troubleshooting", "Troubleshooting", "TROUBLESHOOTING.md"},
	{"security", "Security", "SECURITY.md"},
	{"api", "API for scripts", "API.md"},
	{"changelog", "What's new", "CHANGELOG.md"},
}

// Set is the guides found, with their text.
type Set struct {
	list []Guide
	text map[string]string // slug → Markdown
}

// Load reads the guides: embedded at build, else from the repository (run
// from source: api/ is the working directory, docs/ beside it).
func Load() *Set {
	if sub, err := fs.Sub(embedded, "files"); err == nil {
		if s := From(sub); len(s.list) > 0 {
			return s
		}
	}
	return From(sourceFS{})
}

// sourceFS reads docs/*.md, and the changelog, from the repository.
type sourceFS struct{}

func (sourceFS) Open(name string) (fs.File, error) {
	if name == "CHANGELOG.md" {
		return os.DirFS("..").Open(name)
	}
	return os.DirFS("../docs").Open(name)
}

// From reads the guides from a file system holding docs' files and
// CHANGELOG.md side by side; a guide not there is left out.
func From(fsys fs.FS) *Set {
	s := &Set{text: map[string]string{}}
	for _, g := range guides {
		b, err := fs.ReadFile(fsys, g.File)
		if err != nil {
			continue
		}
		s.list = append(s.list, g)
		s.text[g.Slug] = string(b)
	}
	return s
}

// List is the guides, in order.
func (s *Set) List() []Guide { return append([]Guide(nil), s.list...) }

// Get is one guide and its Markdown.
func (s *Set) Get(slug string) (Guide, string, bool) {
	for _, g := range s.list {
		if g.Slug == slug {
			return g, s.text[slug], true
		}
	}
	return Guide{}, "", false
}

// Page is a guide's Markdown as the docs page shows it: each heading
// carries its anchor (a data-anchor, which the page's sanitiser keeps — an id
// like "title" it wouldn't), and links to another guide go to its page; a link
// to a file that isn't a guide keeps only its text.
func (s *Set) Page(slug string) (Guide, string, bool) {
	g, md, ok := s.Get(slug)
	if !ok {
		return g, "", false
	}
	bySlug := map[string]string{}
	for _, x := range s.list {
		bySlug[strings.ToLower(x.File)] = x.Slug
	}
	var out []string
	fence := ""
	seen := map[string]int{}
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			if fence == "" {
				fence = t[:3]
			} else if strings.HasPrefix(t, fence) {
				fence = ""
			}
			out = append(out, line)
			continue
		}
		if fence != "" {
			out = append(out, line)
			continue
		}
		if m := headingRE.FindStringSubmatch(line); m != nil {
			a := Anchor(plain(m[2]))
			if n := seen[a]; n > 0 {
				seen[a] = n + 1
				a = a + "-" + itoa(n)
			} else {
				seen[a] = 1
			}
			line = m[1] + ` <a data-anchor="` + a + `"></a>` + m[2]
		}
		out = append(out, mdLinkRE.ReplaceAllStringFunc(line, func(l string) string {
			p := mdLinkRE.FindStringSubmatch(l)
			if to, ok := bySlug[strings.ToLower(p[2])]; ok {
				return "[" + p[1] + "](/docs/" + to + p[3] + ")"
			}
			return p[1]
		}))
	}
	return g, strings.Join(out, "\n"), true
}

// A link to a Markdown file of the repository: [text](../X.md#part).
var mdLinkRE = regexp.MustCompile(`\[([^\]]*)\]\((?:\.\./|\./|docs/)*([\w.-]+\.md)(#[^)\s]*)?\)`)

// Section is a part of a guide under one heading.
type Section struct {
	Slug    string `json:"slug"`
	Guide   string `json:"guide"`
	Heading string `json:"heading"`
	Anchor  string `json:"anchor"` // the heading's id on the docs page
	Text    string `json:"text"`
	score   int
}

var headingRE = regexp.MustCompile(`^(#{1,4})\s+(.+?)\s*#*\s*$`)

// sections splits a guide at its headings (not inside code blocks).
func sections(g Guide, md string) []Section {
	var out []Section
	cur := Section{Slug: g.Slug, Guide: g.Title, Heading: g.Title}
	var body []string
	fence := ""
	seen := map[string]int{}
	flush := func() {
		cur.Text = strings.TrimSpace(strings.Join(body, "\n"))
		if cur.Text != "" || cur.Anchor != "" {
			out = append(out, cur)
		}
		body = nil
	}
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			if fence == "" {
				fence = t[:3]
			} else if strings.HasPrefix(t, fence) {
				fence = ""
			}
		}
		if fence == "" {
			if m := headingRE.FindStringSubmatch(line); m != nil {
				flush()
				h := plain(m[2])
				a := Anchor(h)
				if n := seen[a]; n > 0 {
					seen[a] = n + 1
					a = a + "-" + itoa(n)
				} else {
					seen[a] = 1
				}
				cur = Section{Slug: g.Slug, Guide: g.Title, Heading: h, Anchor: a}
				continue
			}
		}
		body = append(body, line)
	}
	flush()
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

var (
	linkRE   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	markRE   = regexp.MustCompile("[*`]")
	spaceRE  = regexp.MustCompile(`\s`)
	notIDsRE = regexp.MustCompile(`[^\p{L}\p{N}\s_-]`)
)

// plain is a heading's text without Markdown marks (a link as its text).
func plain(s string) string {
	return strings.TrimSpace(markRE.ReplaceAllString(linkRE.ReplaceAllString(s, "$1"), ""))
}

// Anchor is a heading's id as GitHub makes it (and the docs page): lower
// case, punctuation dropped, each space a "-".
func Anchor(heading string) string {
	return spaceRE.ReplaceAllString(notIDsRE.ReplaceAllString(strings.ToLower(strings.TrimSpace(heading)), ""), "-")
}

// fold lowers a text and drops its accents, for matching.
func fold(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, strings.ToLower(s))
	if err != nil {
		return strings.ToLower(s)
	}
	return out
}

// Search finds the sections that answer a question best: every word found
// first, a word in the heading counting most.
func (s *Set) Search(q string, limit int) []Section {
	var words []string
	for _, w := range strings.FieldsFunc(fold(q), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if len([]rune(w)) >= 2 && !stop[w] {
			words = append(words, w)
		}
	}
	if len(words) == 0 {
		return nil
	}
	var hits []Section
	for _, g := range s.list {
		for _, sec := range sections(g, s.text[g.Slug]) {
			head, body := fold(sec.Heading), fold(sec.Text)
			score, found := 0, 0
			for _, w := range words {
				h, b := strings.Count(head, w), strings.Count(body, w)
				if h+b > 0 {
					found++
				}
				score += 12*h + min(b, 8)
			}
			if found == 0 {
				continue
			}
			sec.score = score + 40*found
			if found == len(words) {
				sec.score += 100
			}
			hits = append(hits, sec)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// Words that say nothing about what is asked.
var stop = map[string]bool{
	"the": true, "and": true, "or": true, "to": true, "of": true, "in": true, "on": true, "for": true, "is": true, "it": true,
	"how": true, "do": true, "does": true, "can": true, "what": true, "with": true, "my": true, "an": true, "be": true, "set": true,
	"up": true, "orbit": true, "como": true, "que": true, "de": true, "da": true, "um": true, "uma": true, "no": true, "na": true,
}

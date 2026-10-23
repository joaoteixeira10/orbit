// Package linkpreview reads what a link is about — its title, description,
// site and picture (Open Graph, else the page's <title> and description) —
// for bookmarks. Links are anyone's to paste, so every request (each
// redirect hop too) goes through the public-only guarded client: no
// loopback, link-local or private-network addresses. Answers are capped;
// the picture is fetched by the server, once, so viewers never call the
// site.
package linkpreview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/safedial"
)

const (
	maxPage  = 1 << 20   // the head is near the top
	maxImage = 600 << 10 // a picture for a card
	maxHops  = 5
)

// Preview is what a link says about itself.
type Preview struct {
	URL         string // where it ended up (after redirects)
	Title       string
	Description string
	SiteName    string
	ImageURL    string
}

// Image is a fetched picture.
type Image struct {
	Type string
	Data []byte
}

// Fetcher reads link previews.
type Fetcher struct {
	Client *http.Client // must not follow redirects (Fetcher does, guarded)
}

// New is a Fetcher with the public-only guarded client.
func New() *Fetcher { return &Fetcher{Client: safedial.PublicClient(10 * time.Second)} }

// Normalize checks a link someone typed (https assumed).
func Normalize(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(raw) > 2000 {
		return "", errors.New("that isn't a web address")
	}
	u.Fragment = ""
	return u.String(), nil
}

// get follows redirects by hand (each hop through the guard).
func (f *Fetcher) get(ctx context.Context, target, accept string, limit int64) (string, string, []byte, error) {
	for hop := 0; hop <= maxHops; hop++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return "", "", nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; OrbitBookmarks/1.0)")
		req.Header.Set("Accept", accept)
		res, err := f.Client.Do(req)
		if err != nil {
			var ue *url.Error
			if errors.As(err, &ue) {
				err = ue.Err
			}
			return "", "", nil, err
		}
		if res.StatusCode >= 300 && res.StatusCode < 400 {
			loc := res.Header.Get("Location")
			_ = res.Body.Close()
			next, err := url.Parse(loc)
			if loc == "" || err != nil {
				return "", "", nil, errors.New("a redirect without an address")
			}
			u, _ := url.Parse(target)
			n := u.ResolveReference(next)
			if n.Scheme != "http" && n.Scheme != "https" {
				return "", "", nil, errors.New("redirected to a non-web address")
			}
			target = n.String()
			continue
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode >= 400 {
			return "", "", nil, errors.New("the site answered " + res.Status)
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, limit))
		return target, res.Header.Get("Content-Type"), body, err
	}
	return "", "", nil, errors.New("too many redirects")
}

// Fetch reads a link's preview (a page that isn't HTML: its address only).
func (f *Fetcher) Fetch(ctx context.Context, link string) (Preview, error) {
	final, ctype, body, err := f.get(ctx, link, "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5", maxPage)
	if err != nil {
		return Preview{URL: link}, err
	}
	p := Preview{URL: final}
	if !strings.Contains(ctype, "html") {
		return p, nil
	}
	if r, err := charset.NewReader(bytes.NewReader(body), ctype); err == nil {
		if b, err := io.ReadAll(r); err == nil {
			body = b
		}
	}
	parse(body, &p)
	if p.ImageURL != "" {
		base, _ := url.Parse(final)
		if u, err := url.Parse(p.ImageURL); err == nil {
			p.ImageURL = base.ResolveReference(u).String()
		}
	}
	return p, nil
}

// FetchImage downloads a picture (png, jpeg, gif, webp; capped).
func (f *Fetcher) FetchImage(ctx context.Context, src string) (Image, error) {
	_, _, data, err := f.get(ctx, src, "image/avif,image/webp,image/png,image/jpeg,image/gif;q=0.9", maxImage+1)
	if err != nil {
		return Image{}, err
	}
	if len(data) > maxImage || len(data) == 0 {
		return Image{}, errors.New("no picture, or too large")
	}
	typ := http.DetectContentType(data)
	switch typ {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return Image{Type: typ, Data: data}, nil
	}
	return Image{}, errors.New("not a picture")
}

func parse(body []byte, p *Preview) {
	z := html.NewTokenizer(bytes.NewReader(body))
	var inTitle bool
	var title, desc string
	meta := map[string]string{}
	for {
		switch z.Next() {
		case html.ErrorToken:
			finish(p, meta, title, desc)
			return
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "title":
				inTitle = true
			case "meta":
				var key, content string
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					switch strings.ToLower(string(k)) {
					case "property", "name":
						key = strings.ToLower(string(v))
					case "content":
						content = string(v)
					}
				}
				if key != "" && content != "" {
					if _, ok := meta[key]; !ok {
						meta[key] = strings.TrimSpace(content)
					}
				}
				if key == "description" && desc == "" {
					desc = strings.TrimSpace(content)
				}
			case "body":
				finish(p, meta, title, desc)
				return
			}
		case html.TextToken:
			if inTitle && title == "" {
				title = strings.TrimSpace(string(z.Text()))
			}
		case html.EndTagToken:
			if n, _ := z.TagName(); string(n) == "title" {
				inTitle = false
			}
		}
	}
}

func finish(p *Preview, meta map[string]string, title, desc string) {
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v := meta[k]; v != "" {
				return v
			}
		}
		return ""
	}
	p.Title = clip(first(pick("og:title", "twitter:title"), title), 300)
	p.Description = clip(first(pick("og:description", "twitter:description"), desc), 600)
	p.SiteName = clip(pick("og:site_name", "application-name"), 120)
	p.ImageURL = clip(pick("og:image", "og:image:url", "twitter:image", "twitter:image:src"), 2000)
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

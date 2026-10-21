// Package caldav keeps a workspace's calendar and a calendar on a CalDAV
// server (iCloud, Nextcloud, Fastmail, Radicale…) in step, both ways.
//
// client.go is the protocol, the few requests the sync needs: find the
// calendars (current-user-principal → calendar-home-set → its calendars,
// following absolute hrefs to other hosts, as iCloud's are), list a
// calendar's objects with their ETags, fetch some (calendar-multiget), and
// PUT / DELETE one conditionally. Every request goes through the
// SSRF-guarded dialer; redirects are followed by hand, keeping the method.
package caldav

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MaxObject caps one calendar object; maxReply one multistatus answer.
const (
	MaxObject = 1 << 20
	maxReply  = 32 << 20
)

// ErrAuth: the server refused the username or password.
var ErrAuth = errors.New("the server didn't accept the username or password (an app-specific password may be needed)")

// ErrPrecondition: the object changed on the server since it was read.
var ErrPrecondition = errors.New("changed on the server meanwhile")

// Client talks to one CalDAV account.
type Client struct {
	http     *http.Client
	username string
	password string
}

// NewClient builds a client; hc must not follow redirects (Client does).
func NewClient(hc *http.Client, username, password string) *Client {
	return &Client{http: hc, username: username, password: password}
}

// Calendar is a calendar collection that holds events.
type Calendar struct {
	URL   string `json:"url"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Object is one calendar object (an .ics resource).
type Object struct {
	URL  string
	ETag string
	Data []byte
}

// ---- WebDAV XML ----

type multistatus struct {
	Responses []response `xml:"DAV: response"`
}

type response struct {
	Href      string     `xml:"DAV: href"`
	Status    string     `xml:"DAV: status"`
	Propstats []propstat `xml:"DAV: propstat"`
}

type propstat struct {
	Prop   prop   `xml:"DAV: prop"`
	Status string `xml:"DAV: status"`
}

type prop struct {
	Principal    *hrefs        `xml:"DAV: current-user-principal"`
	Home         *hrefs        `xml:"urn:ietf:params:xml:ns:caldav calendar-home-set"`
	ResourceType *resourceType `xml:"DAV: resourcetype"`
	DisplayName  string        `xml:"DAV: displayname"`
	ETag         string        `xml:"DAV: getetag"`
	ContentType  string        `xml:"DAV: getcontenttype"`
	Components   *components   `xml:"urn:ietf:params:xml:ns:caldav supported-calendar-component-set"`
	Color        string        `xml:"http://apple.com/ns/ical/ calendar-color"`
	Data         string        `xml:"urn:ietf:params:xml:ns:caldav calendar-data"`
}

type hrefs struct {
	Href []string `xml:"DAV: href"`
}

type resourceType struct {
	Calendar *struct{} `xml:"urn:ietf:params:xml:ns:caldav calendar"`
}

type components struct {
	Comp []struct {
		Name string `xml:"name,attr"`
	} `xml:"urn:ietf:params:xml:ns:caldav comp"`
}

// ok reports a 2xx propstat / response status ("HTTP/1.1 200 OK").
func ok(status string) bool {
	f := strings.Fields(status)
	return len(f) >= 2 && strings.HasPrefix(f[1], "2")
}

// props merges a response's successful propstats.
func (r response) props() prop {
	var out prop
	for _, ps := range r.Propstats {
		if !ok(ps.Status) {
			continue
		}
		p := ps.Prop
		if p.Principal != nil {
			out.Principal = p.Principal
		}
		if p.Home != nil {
			out.Home = p.Home
		}
		if p.ResourceType != nil {
			out.ResourceType = p.ResourceType
		}
		if p.Components != nil {
			out.Components = p.Components
		}
		out.DisplayName = or(p.DisplayName, out.DisplayName)
		out.ETag = or(p.ETag, out.ETag)
		out.ContentType = or(p.ContentType, out.ContentType)
		out.Color = or(p.Color, out.Color)
		out.Data = or(p.Data, out.Data)
	}
	return out
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ---- Requests ----

// do sends a request, following up to three redirects with the same method
// and body, and answers the final response (the caller closes it).
func (c *Client) do(ctx context.Context, method, target string, body []byte, header http.Header) (*http.Response, error) {
	for hop := 0; ; hop++ {
		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		for k, v := range header {
			req.Header[k] = v
		}
		req.SetBasicAuth(c.username, c.password)
		res, err := c.http.Do(req)
		if err != nil {
			return nil, plainErr(err)
		}
		switch res.StatusCode {
		case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			loc := res.Header.Get("Location")
			_ = res.Body.Close()
			if hop >= 3 || loc == "" {
				return nil, errors.New("too many redirects")
			}
			next, err := resolve(target, loc)
			if err != nil {
				return nil, err
			}
			target = next
			continue
		case http.StatusUnauthorized, http.StatusForbidden:
			_ = res.Body.Close()
			return nil, ErrAuth
		}
		return res, nil
	}
}

// plainErr drops Go's "Get \"<url>\":" prefix (the URL may carry a user name).
func plainErr(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// resolve reads href against base (hrefs may be paths or whole URLs).
func resolve(base, href string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	h, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", err
	}
	u := b.ResolveReference(h)
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("the server pointed to a non-web address")
	}
	return u.String(), nil
}

// dav sends a PROPFIND / REPORT and reads the multistatus.
func (c *Client) dav(ctx context.Context, method, target, depth, body string) (*multistatus, error) {
	h := http.Header{"Content-Type": {"application/xml; charset=utf-8"}, "Depth": {depth}}
	res, err := c.do(ctx, method, target, []byte(xml.Header+body), h)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusMultiStatus {
		if res.StatusCode == http.StatusNotFound {
			return nil, errors.New("not found on the server")
		}
		return nil, fmt.Errorf("the server answered %d", res.StatusCode)
	}
	var ms multistatus
	if err := xml.NewDecoder(io.LimitReader(res.Body, maxReply)).Decode(&ms); err != nil {
		return nil, errors.New("the server's answer isn't WebDAV")
	}
	return &ms, nil
}

func (c *Client) one(ctx context.Context, target, body string) (prop, error) {
	ms, err := c.dav(ctx, "PROPFIND", target, "0", body)
	if err != nil {
		return prop{}, err
	}
	if len(ms.Responses) == 0 {
		return prop{}, errors.New("the server answered nothing")
	}
	return ms.Responses[0].props(), nil
}

// Calendars finds the account's calendars that hold events, starting from
// the address someone gave (the server, or a calendar's own address).
func (c *Client) Calendars(ctx context.Context, address string) ([]Calendar, error) {
	const principalQ = `<d:propfind xmlns:d="DAV:"><d:prop><d:current-user-principal/></d:prop></d:propfind>`
	var principal string
	var firstErr error
	for _, start := range starts(address) {
		p, err := c.one(ctx, start, principalQ)
		if err == nil && p.Principal != nil && len(p.Principal.Href) > 0 {
			principal, err = resolve(start, p.Principal.Href[0])
			if err == nil {
				break
			}
		}
		if errors.Is(err, ErrAuth) {
			return nil, err
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if principal == "" {
		if firstErr == nil {
			firstErr = errors.New("no CalDAV account found there")
		}
		return nil, fmt.Errorf("couldn't find the calendars: %w", firstErr)
	}
	p, err := c.one(ctx, principal, `<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><c:calendar-home-set/></d:prop></d:propfind>`)
	if err != nil {
		return nil, err
	}
	if p.Home == nil || len(p.Home.Href) == 0 {
		return nil, errors.New("the account has no calendars")
	}
	home, err := resolve(principal, p.Home.Href[0])
	if err != nil {
		return nil, err
	}
	ms, err := c.dav(ctx, "PROPFIND", home, "1", `<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" xmlns:a="http://apple.com/ns/ical/">`+
		`<d:prop><d:resourcetype/><d:displayname/><c:supported-calendar-component-set/><a:calendar-color/></d:prop></d:propfind>`)
	if err != nil {
		return nil, err
	}
	out := []Calendar{}
	for _, r := range ms.Responses {
		p := r.props()
		if p.ResourceType == nil || p.ResourceType.Calendar == nil || !holdsEvents(p.Components) {
			continue
		}
		u, err := resolve(home, r.Href)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(p.DisplayName)
		if name == "" {
			name = lastSegment(u)
		}
		out = append(out, Calendar{URL: u, Name: name, Color: color(p.Color)})
	}
	return out, nil
}

// starts: where to look for the principal — the address, then the server's
// /.well-known/caldav.
func starts(address string) []string {
	out := []string{address}
	if u, err := url.Parse(address); err == nil && u.Host != "" {
		w := url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/.well-known/caldav"}
		if w.String() != address {
			out = append(out, w.String())
		}
	}
	return out
}

func holdsEvents(c *components) bool {
	if c == nil || len(c.Comp) == 0 {
		return true // not said: any
	}
	for _, x := range c.Comp {
		if strings.EqualFold(x.Name, "VEVENT") {
			return true
		}
	}
	return false
}

// color reads Apple's calendar-color (#RRGGBB or #RRGGBBAA) as #rrggbb.
func color(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 9 {
		s = s[:7]
	}
	if len(s) != 7 || s[0] != '#' {
		return ""
	}
	return strings.ToLower(s)
}

func lastSegment(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return u
	}
	parts := strings.Split(strings.Trim(p.Path, "/"), "/")
	return parts[len(parts)-1]
}

// Listing is a calendar's objects: URL → ETag.
type Listing map[string]string

// List reads a calendar's objects and their ETags.
func (c *Client) List(ctx context.Context, calendar string) (Listing, error) {
	ms, err := c.dav(ctx, "PROPFIND", calendar, "1", `<d:propfind xmlns:d="DAV:"><d:prop><d:getetag/><d:getcontenttype/><d:resourcetype/></d:prop></d:propfind>`)
	if err != nil {
		return nil, err
	}
	self, _ := url.Parse(calendar)
	out := Listing{}
	for _, r := range ms.Responses {
		u, err := resolve(calendar, r.Href)
		if err != nil {
			continue
		}
		if pu, _ := url.Parse(u); pu != nil && self != nil && strings.TrimSuffix(pu.Path, "/") == strings.TrimSuffix(self.Path, "/") {
			continue // the calendar itself
		}
		p := r.props()
		if p.ETag == "" || (p.ContentType != "" && !strings.Contains(p.ContentType, "calendar")) {
			continue
		}
		out[u] = p.ETag
	}
	return out, nil
}

// Get fetches objects (calendar-multiget, in batches).
func (c *Client) Get(ctx context.Context, calendar string, urls []string) ([]Object, error) {
	var out []Object
	for len(urls) > 0 {
		n := min(len(urls), 50)
		batch := urls[:n]
		urls = urls[n:]
		var b strings.Builder
		b.WriteString(`<c:calendar-multiget xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:getetag/><c:calendar-data/></d:prop>`)
		for _, u := range batch {
			href := u
			if p, err := url.Parse(u); err == nil {
				href = p.EscapedPath()
			}
			b.WriteString("<d:href>")
			_ = xml.EscapeText(&b, []byte(href))
			b.WriteString("</d:href>")
		}
		b.WriteString(`</c:calendar-multiget>`)
		ms, err := c.dav(ctx, "REPORT", calendar, "1", b.String())
		if err != nil {
			return nil, err
		}
		for _, r := range ms.Responses {
			p := r.props()
			if p.Data == "" || len(p.Data) > MaxObject {
				continue
			}
			u, err := resolve(calendar, r.Href)
			if err != nil {
				continue
			}
			out = append(out, Object{URL: u, ETag: p.ETag, Data: []byte(p.Data)})
		}
	}
	return out, nil
}

// Put writes an object: etag "" creates it (it mustn't exist), else
// replaces that version. It answers the new ETag ("" when the server
// doesn't say).
func (c *Client) Put(ctx context.Context, target, etag string, data []byte) (string, error) {
	h := http.Header{"Content-Type": {"text/calendar; charset=utf-8"}}
	if etag == "" {
		h.Set("If-None-Match", "*")
	} else {
		h.Set("If-Match", etag)
	}
	res, err := c.do(ctx, "PUT", target, data, h)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	switch {
	case res.StatusCode == http.StatusPreconditionFailed:
		return "", ErrPrecondition
	case res.StatusCode/100 != 2:
		return "", fmt.Errorf("the server refused the event (%d)", res.StatusCode)
	}
	return res.Header.Get("ETag"), nil
}

// Delete removes an object (if it's still that version; etag "" any).
func (c *Client) Delete(ctx context.Context, target, etag string) error {
	h := http.Header{}
	if etag != "" {
		h.Set("If-Match", etag)
	}
	res, err := c.do(ctx, "DELETE", target, nil, h)
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	switch {
	case res.StatusCode == http.StatusPreconditionFailed:
		return ErrPrecondition
	case res.StatusCode == http.StatusNotFound || res.StatusCode/100 == 2:
		return nil
	}
	return fmt.Errorf("the server refused to delete it (%d)", res.StatusCode)
}

// HTTPClient is the guarded client the sync uses (no redirects: Client
// follows them itself).
func HTTPClient(dial func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Client {
	return &http.Client{
		Timeout:       60 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			DialContext:           dial,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   2,
			IdleConnTimeout:       30 * time.Second,
		},
	}
}

package gitsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/uploads"
)

// Pictures and files in an issue's comments, both ways.
//
// From git: a comment's pictures (and, on Gitea, the files attached under
// it) are fetched with the list's account and kept as the task's files, and
// the comment shows Orbit's copies — the server's own addresses wouldn't
// open from Orbit's page (relative, or a private repository's). Pictures
// from anywhere else are left as they are (Orbit shows those after a click).
//
// To git: a file added to a task is sent to its issue as a comment with the
// file — attached under it on Gitea, uploaded to the project on GitLab. A
// GitHub app can't upload a file to an issue: there the comment only names
// it.

const (
	maxCommentFiles = 10
	// MaxFileBytes is the largest file brought from or sent to git.
	MaxFileBytes = 25 << 20
)

var (
	mdRef   = regexp.MustCompile(`(!?)\[([^\]\n]*)\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)
	htmlImg = regexp.MustCompile(`(?i)<img\b[^>]*>`)
	attrSrc = regexp.MustCompile(`(?i)\bsrc\s*=\s*"([^"]+)"`)
	attrAlt = regexp.MustCompile(`(?i)\balt\s*=\s*"([^"]*)"`)
)

// ghSigned is GitHub's pictures in a comment's HTML, in order: the addresses
// that open without signing in (for a few minutes).
func ghSigned(html string) []string {
	var out []string
	for _, tag := range htmlImg.FindAllString(html, -1) {
		if m := attrSrc.FindStringSubmatch(tag); m != nil && isGitHubFile(m[1]) {
			out = append(out, strings.ReplaceAll(m[1], "&amp;", "&"))
		}
	}
	return out
}

// isGitHubFile: a picture or file people added to GitHub (not any link).
func isGitHubFile(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Host)
	switch {
	case h == "github.com":
		return strings.HasPrefix(u.Path, "/user-attachments/") || strings.Contains(u.Path, "/assets/")
	case strings.HasSuffix(h, ".githubusercontent.com"):
		return strings.Contains(h, "user-images") || strings.Contains(h, "user-attachments")
	}
	return false
}

// serverFile is where a reference in a comment is fetched from: the git
// server's own file (with the account), or false — not the server's.
func (c *Client) serverFile(ref, owner, repo string) (fetch string, ok bool) {
	base, err := url.Parse(c.base)
	if err != nil {
		return "", false
	}
	u, err := base.Parse(ref)
	if err != nil || !strings.EqualFold(u.Host, base.Host) {
		return "", false
	}
	switch {
	case c.GitLab():
		// /uploads/<secret>/<name>, or …/<project>/uploads/<secret>/<name>
		i := strings.Index(u.Path, "/uploads/")
		if i < 0 {
			return "", false
		}
		parts := strings.SplitN(strings.TrimPrefix(u.Path[i:], "/uploads/"), "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", false
		}
		return apiRoot(c.base, c.kind) + glProject(owner, repo) + "/uploads/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]), true
	case !c.GitHub():
		if !strings.Contains(u.Path, "/attachments/") {
			return "", false
		}
		return u.String(), true
	}
	return "", false
}

// absolute is a reference made to work from anywhere: the server's address
// before a path of its own.
func (c *Client) absolute(ref string) string {
	base, err := url.Parse(c.base)
	if err != nil {
		return ref
	}
	if u, err := base.Parse(ref); err == nil {
		return u.String()
	}
	return ref
}

// fetch reads a file: with the account (the server's), or as it is (a
// signed address on another host gets no token).
func (c *Client) fetch(ctx context.Context, addr string, withAccount bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return nil, err
	}
	if withAccount {
		switch {
		case c.GitLab(), c.GitHub():
			req.Header.Set("Authorization", "Bearer "+c.token)
		default:
			req.Header.Set("Authorization", "token "+c.token)
		}
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", req.URL.Host, res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("larger than %d MB", MaxFileBytes>>20)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty")
	}
	return data, nil
}

// keepFile is a fetched file as the task's (the one already kept from that
// address, if any).
func (e *Engine) keepFile(t *models.Task, ownerID, name, gitURL string, data []byte) (models.Upload, error) {
	var u models.Upload
	if e.db.Where("target = ? AND target_id = ? AND git_url = ? AND "+uploads.Shown, uploads.Task, t.ID, gitURL).Limit(1).Find(&u).RowsAffected > 0 {
		return u, nil
	}
	u = models.Upload{WorkspaceID: t.WorkspaceID, OwnerID: ownerID, Target: uploads.Task, TargetID: t.ID, Name: name, GitURL: gitURL}
	if u.Name == "" {
		u.Name = "file"
	}
	err := uploads.Create(e.db, &u, data, e.Quota)
	return u, err
}

// fileName is a name for a fetched file: its alt text if it looks like one,
// else the last part of its address.
func fileName(alt, addr string) string {
	alt = strings.TrimSpace(alt)
	if alt != "" && strings.Contains(alt, ".") && !strings.ContainsAny(alt, "/\\") {
		return alt
	}
	if u, err := url.Parse(addr); err == nil {
		if b := path.Base(u.Path); b != "" && b != "/" && b != "." {
			if alt != "" && !strings.Contains(b, ".") {
				return alt
			}
			return b
		}
	}
	if alt != "" {
		return alt
	}
	return "file"
}

// localComment is a comment's text with its pictures and files kept in
// Orbit (the server's own, and GitHub's); the rest as it was.
func (e *Engine) localComment(ctx context.Context, c *Client, t *models.Task, rc Comment, body, ownerID string) string {
	owner, repo, _ := strings.Cut(t.GitRepo, "/")
	signed := []string(nil)
	if c.GitHub() {
		signed = ghSigned(rc.BodyHTML)
	}
	files := 0
	ghIndex := 0
	kept := map[string]string{} // address in the comment → Orbit's
	take := func(ref, alt string, picture bool) (string, bool) {
		if to, ok := kept[ref]; ok {
			return to, true
		}
		var fetch string
		var withAccount bool
		switch {
		case c.GitHub() && picture && isGitHubFile(ref):
			if ghIndex >= len(signed) {
				return "", false
			}
			fetch = signed[ghIndex]
			ghIndex++
		case !c.GitHub():
			f, ok := c.serverFile(ref, owner, repo)
			if !ok {
				return "", false
			}
			fetch, withAccount = f, true
		default:
			return "", false
		}
		if files >= maxCommentFiles {
			return "", false
		}
		files++
		data, err := c.fetch(ctx, fetch, withAccount)
		if err != nil {
			e.log.Debug("git comment file not fetched")
			return "", false
		}
		u, err := e.keepFile(t, ownerID, fileName(alt, ref), c.absolute(ref), data)
		if err != nil {
			return "", false
		}
		kept[ref] = uploads.URL(u)
		return kept[ref], true
	}
	// <img> tags (GitHub's resized pictures) first, as Markdown pictures.
	body = htmlImg.ReplaceAllStringFunc(body, func(tag string) string {
		m := attrSrc.FindStringSubmatch(tag)
		if m == nil {
			return tag
		}
		alt := ""
		if a := attrAlt.FindStringSubmatch(tag); a != nil {
			alt = a[1]
		}
		if to, ok := take(m[1], alt, true); ok {
			return "![" + alt + "](" + to + ")"
		}
		return tag
	})
	body = mdRef.ReplaceAllStringFunc(body, func(s string) string {
		m := mdRef.FindStringSubmatch(s)
		bang, text, ref := m[1], m[2], m[3]
		if to, ok := take(ref, text, bang == "!"); ok {
			return bang + "[" + text + "](" + to + ")"
		}
		if !c.GitHub() && strings.HasPrefix(ref, "/") {
			// The server's, not brought: at least its full address.
			return bang + "[" + text + "](" + c.absolute(ref) + ")"
		}
		return s
	})
	// Gitea: files attached under the comment, not in its text.
	for _, a := range rc.Assets {
		if a.URL == "" || files >= maxCommentFiles || strings.Contains(rc.Body, a.UUID) && a.UUID != "" {
			continue
		}
		to, ok := take(a.URL, a.Name, false)
		if !ok {
			continue
		}
		var mark string
		if u, err := url.Parse(to); err == nil && isPictureName(a.Name) {
			mark = "![" + a.Name + "](" + u.String() + ")"
		} else {
			mark = "📎 [" + a.Name + "](" + to + ")"
		}
		body = strings.TrimRight(body, "\n") + "\n\n" + mark
	}
	return body
}

func isPictureName(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	}
	return false
}

// ---- To git ----

// upload sends a file to the server: under a Gitea comment (its address),
// or to a GitLab project (the Markdown that shows it).
func (c *Client) upload(ctx context.Context, apiPath, field, name string, data []byte, out any) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(field, name)
	if err != nil {
		return err
	}
	if _, err := part.Write(data); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiRoot(c.base, c.kind)+apiPath, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	if c.GitLab() {
		req.Header.Set("Authorization", "Bearer "+c.token)
	} else {
		req.Header.Set("Authorization", "token "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		var m struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(b, &m)
		return &APIError{Status: res.StatusCode, Message: m.Message}
	}
	return json.Unmarshal(b, out)
}

// PushFile sends a file added to a task to its issue, as a comment with the
// file (a no-op for tasks without one), and remembers that comment on it.
func (e *Engine) PushFile(ctx context.Context, actor, actorName string, t *models.Task, u *models.Upload, data []byte) error {
	if t.GitIssueID == 0 || t.GitNumber == 0 || u.GitURL != "" {
		return nil
	}
	owner, repo, ok := strings.Cut(t.GitRepo, "/")
	if !ok {
		return nil
	}
	c, viaBot, err := e.writer(actor, e.linkOf(t), t.GitHost, t.WorkspaceID)
	if err != nil {
		return err
	}
	if len(data) > MaxFileBytes {
		return fmt.Errorf("%s is larger than %d MB: it stays in Orbit", u.Name, MaxFileBytes>>20)
	}
	say := func(body string) string {
		if viaBot {
			return signed(actorName, body)
		}
		return body
	}
	var rc Comment
	gitURL := ""
	switch {
	case c.GitLab():
		var up struct {
			URL      string `json:"url"`
			Markdown string `json:"markdown"`
		}
		if err := c.upload(ctx, glProject(owner, repo)+"/uploads", "file", u.Name, data, &up); err != nil {
			return err
		}
		if rc, err = c.CreateComment(ctx, owner, repo, t.GitNumber, say("📎 "+up.Markdown)); err != nil {
			return err
		}
		gitURL = c.absolute(up.URL)
	case c.GitHub():
		if rc, err = c.CreateComment(ctx, owner, repo, t.GitNumber, say("📎 **"+u.Name+"** — a file added in Orbit (GitHub doesn't take files from apps)")); err != nil {
			return err
		}
		gitURL = "github:comment"
	default:
		if rc, err = c.CreateComment(ctx, owner, repo, t.GitNumber, say("📎 **"+u.Name+"**")); err != nil {
			return err
		}
		var a Asset
		if err := c.upload(ctx, fmt.Sprintf("%s/issues/comments/%d/assets", repoPath(owner, repo), rc.ID), "attachment", u.Name, data, &a); err != nil {
			_ = c.DeleteComment(ctx, owner, repo, t.GitNumber, rc.ID)
			return err
		}
		gitURL = a.URL
	}
	u.GitURL, u.GitCommentID = gitURL, rc.ID
	return e.db.Model(&models.Upload{}).Where("id = ?", u.ID).Updates(map[string]any{"git_url": gitURL, "git_comment_id": rc.ID}).Error
}

// DropFile removes the comment a task's file was sent to git with (when the
// file goes in Orbit).
func (e *Engine) DropFile(ctx context.Context, actor string, t *models.Task, u models.Upload) error {
	if u.GitCommentID == 0 || t.GitNumber == 0 {
		return nil
	}
	owner, repo, ok := strings.Cut(t.GitRepo, "/")
	if !ok {
		return nil
	}
	c, _, err := e.writer(actor, e.linkOf(t), t.GitHost, t.WorkspaceID)
	if err != nil {
		return err
	}
	return c.DeleteComment(ctx, owner, repo, t.GitNumber, u.GitCommentID)
}

var orbitFile = regexp.MustCompile(`/api/v1/uploads/([0-9a-f-]{36})/[^)\s"]*`)

// remoteLinks is a comment's text for git: Orbit's copies of the server's
// files back to the server's addresses.
func (e *Engine) remoteLinks(body string) string {
	return orbitFile.ReplaceAllStringFunc(body, func(s string) string {
		var u models.Upload
		id := orbitFile.FindStringSubmatch(s)[1]
		if e.db.Select("git_url").First(&u, "id = ?", id).Error == nil && strings.Contains(u.GitURL, "://") {
			return u.GitURL
		}
		return s
	})
}

// stale: a comment brought before its pictures were (a server's relative
// address, or a GitHub picture, still in it).
func stale(body string) bool {
	for _, m := range mdRef.FindAllStringSubmatch(body, -1) {
		ref := m[3]
		if strings.HasPrefix(ref, "/") && !strings.HasPrefix(ref, "/api/v1/uploads/") || m[1] == "!" && isGitHubFile(ref) {
			return true
		}
	}
	for _, tag := range htmlImg.FindAllString(body, -1) {
		if m := attrSrc.FindStringSubmatch(tag); m != nil && isGitHubFile(m[1]) {
			return true
		}
	}
	return false
}

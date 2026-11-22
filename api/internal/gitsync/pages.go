package gitsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/uploads"
)

// A workspace's pages copied into a git repository, as Markdown files — one
// way. Each page is "<title>.md" (its sub-pages in a folder "<title>/"
// beside it), with its title, id, dates and tags at the top; links to the
// workspace's other pages point at their files, and the pictures and files
// in the pages go into "_files/". Restricted and locked pages, templates,
// and what's under them, stay out. Unchanged files aren't written again;
// only files the copy wrote are ever deleted (a README beside them stays).

const (
	maxCopiedFile  = 10 << 20  // a picture or file larger stays a link to Orbit
	maxCopiedFiles = 100 << 20 // all of them, at most
)

var (
	pageLink   = regexp.MustCompile(`\]\(((?:https?://[^)\s/]+)?/w/([0-9a-f-]{36})/p/([0-9a-f-]{36}))([^)\s]*)\)`)
	uploadLink = regexp.MustCompile(`/api/v1/uploads/([0-9a-f-]{36})/[^)\s"]*`)
	unsafeName = regexp.MustCompile(`[/\\:*?"<>|\x00-\x1f]+`)
)

// fileName is a page's title as a file name (readable, and safe anywhere).
func pageFileName(title string) string {
	s := strings.TrimSpace(unsafeName.ReplaceAllString(title, "-"))
	s = strings.Trim(s, ". ")
	if r := []rune(s); len(r) > 80 {
		s = strings.TrimSpace(string(r[:80]))
	}
	if s == "" {
		s = "Untitled"
	}
	return s
}

// rel is the address of to as seen from the folder of from (both paths in
// the repository), for a Markdown link.
func rel(from, to string) string {
	fd := strings.Split(path.Dir(from), "/")
	if path.Dir(from) == "." {
		fd = nil
	}
	td := strings.Split(to, "/")
	i := 0
	for i < len(fd) && i < len(td)-1 && fd[i] == td[i] {
		i++
	}
	parts := []string{}
	for range fd[i:] {
		parts = append(parts, "..")
	}
	parts = append(parts, td[i:]...)
	out := make([]string, len(parts))
	for j, p := range parts {
		out[j] = url.PathEscape(p)
	}
	return strings.Join(out, "/")
}

// PagesFiles is what a workspace's pages are as files under folder.
func (e *Engine) PagesFiles(wsID, folder string) (map[string][]byte, error) {
	var pages []models.Page
	if err := e.db.Where("workspace_id = ? AND trashed_at IS NULL", wsID).Order("rank").Find(&pages).Error; err != nil {
		return nil, err
	}
	kids := map[string][]*models.Page{}
	var roots []*models.Page
	for i := range pages {
		p := &pages[i]
		if p.ParentID != nil && *p.ParentID != "" {
			kids[*p.ParentID] = append(kids[*p.ParentID], p)
		} else {
			roots = append(roots, p)
		}
	}
	hidden := func(p *models.Page) bool { return p.Restricted || p.Sealed || p.Template }
	base := cleanPath(folder)
	join := func(parts ...string) string {
		if base != "" {
			parts = append([]string{base}, parts...)
		}
		return path.Join(parts...)
	}
	where := map[string]string{} // page id → its file
	var order []*models.Page
	var walk func(dir string, list []*models.Page)
	walk = func(dir string, list []*models.Page) {
		taken := map[string]bool{}
		for _, p := range list {
			if hidden(p) {
				continue
			}
			name := pageFileName(p.Title)
			for n := 2; taken[strings.ToLower(name)]; n++ {
				name = fmt.Sprintf("%s (%d)", pageFileName(p.Title), n)
			}
			taken[strings.ToLower(name)] = true
			where[p.ID] = path.Join(dir, name+".md")
			order = append(order, p)
			walk(path.Join(dir, name), kids[p.ID])
		}
	}
	walk("", roots)

	out := map[string][]byte{}
	copied := int64(0)
	for _, p := range order {
		me := where[p.ID]
		body := pageLink.ReplaceAllStringFunc(p.Content, func(s string) string {
			m := pageLink.FindStringSubmatch(s)
			if to, ok := where[m[3]]; ok && m[2] == wsID {
				return "](" + rel(me, to) + ")"
			}
			return s
		})
		body = uploadLink.ReplaceAllStringFunc(body, func(s string) string {
			id := uploadLink.FindStringSubmatch(s)[1]
			var u models.Upload
			if e.db.First(&u, "id = ? AND workspace_id = ?", id, wsID).Error != nil || u.Size > maxCopiedFile || copied+u.Size > maxCopiedFiles {
				return s
			}
			at := path.Join("_files", u.ID+"-"+uploads.CleanName(u.Name))
			if _, done := out[join(at)]; !done {
				data, err := uploads.Read(e.db, u)
				if err != nil {
					return s
				}
				out[join(at)] = data
				copied += u.Size
			}
			return rel(me, at)
		})
		var tags []string
		_ = json.Unmarshal([]byte(p.Tags), &tags)
		title, _ := json.Marshal(p.Title)
		head := "---\ntitle: " + string(title) + "\norbit_id: " + p.ID +
			"\ncreated: " + p.CreatedAt.UTC().Format(time.RFC3339) + "\nupdated: " + p.UpdatedAt.UTC().Format(time.RFC3339)
		if len(tags) > 0 {
			t, _ := json.Marshal(tags)
			head += "\ntags: " + string(t)
		}
		out[join(me)] = []byte(head + "\n---\n\n" + strings.TrimRight(body, "\n") + "\n")
	}
	return out, nil
}

func fileSums(files map[string][]byte) map[string]string {
	out := make(map[string]string, len(files))
	for p, b := range files {
		s := sha256.Sum256(b)
		out[p] = hex.EncodeToString(s[:])
	}
	return out
}

// CopyPages copies a workspace's pages to its repository now (nothing is
// written when nothing changed since the last copy).
func (e *Engine) CopyPages(ctx context.Context, r *models.PagesRepo) error {
	files, err := e.PagesFiles(r.WorkspaceID, r.Folder)
	if err == nil {
		err = e.copyFiles(ctx, r, files)
	}
	now := time.Now()
	upd := map[string]any{"last_error": ""}
	if err != nil {
		upd["last_error"] = clip(err.Error(), 500)
	} else {
		upd["copied_at"], upd["files"], upd["commit"] = &now, r.Files, r.Commit
		r.CopiedAt = &now
	}
	r.LastError, _ = upd["last_error"].(string)
	e.db.Model(&models.PagesRepo{}).Where("id = ?", r.ID).Updates(upd)
	return err
}

func (e *Engine) copyFiles(ctx context.Context, r *models.PagesRepo, files map[string][]byte) error {
	sums := fileSums(files)
	before := map[string]string{}
	_ = json.Unmarshal([]byte(r.Files), &before)
	if len(before) == len(sums) && r.LastError == "" && r.CopiedAt != nil {
		same := true
		for p, s := range sums {
			if before[p] != s {
				same = false
				break
			}
		}
		if same {
			return nil
		}
	}
	c, err := e.ClientFor(r.UserID, r.Host)
	if err != nil {
		return err
	}
	var gone []string
	for p := range before {
		if _, ok := files[p]; !ok {
			gone = append(gone, p)
		}
	}
	sort.Strings(gone)
	var ws models.Workspace
	e.db.Select("name").First(&ws, "id = ?", r.WorkspaceID)
	sha, err := c.Commit(ctx, r.Owner, r.Repo, r.Branch, "Pages of "+ws.Name+" (from Orbit)", files, gone)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(sums)
	r.Files = string(b)
	if sha != "" {
		r.Commit = sha
	}
	return nil
}

// CopyAllPages copies the pages of every workspace whose copy is automatic
// (the timer; each only when something changed).
func (e *Engine) CopyAllPages(ctx context.Context) {
	var repos []models.PagesRepo
	e.db.Where("auto = ?", true).Find(&repos)
	for i := range repos {
		if ctx.Err() != nil {
			return
		}
		_ = e.CopyPages(ctx, &repos[i])
	}
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

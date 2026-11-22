package gitsync

import (
	"context"
	"crypto/sha1" //nolint:gosec // git's own object names, not security
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Commit writes files to a branch in one commit, through the server's API
// (no git on Orbit's side): put are paths and their bytes, del paths to
// remove. Files already there with the same bytes aren't written again; a
// path to delete that isn't there is skipped. It returns the commit made
// ("" when nothing differed).
func (c *Client) Commit(ctx context.Context, owner, repo, branch, message string, put map[string][]byte, del []string) (string, error) {
	have, err := c.treeOf(ctx, owner, repo, branch)
	if err != nil {
		return "", err
	}
	var paths []string
	for p, b := range put {
		if have[p] != blobSHA(b) {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	var gone []string
	for _, p := range del {
		if _, ok := have[p]; ok {
			if _, kept := put[p]; !kept {
				gone = append(gone, p)
			}
		}
	}
	sort.Strings(gone)
	if len(paths) == 0 && len(gone) == 0 {
		return "", nil
	}
	switch {
	case c.GitHub():
		return c.ghCommit(ctx, owner, repo, branch, message, put, paths, gone)
	case c.GitLab():
		return c.glCommit(ctx, owner, repo, branch, message, put, paths, gone, have)
	}
	return c.giteaCommit(ctx, owner, repo, branch, message, put, paths, gone, have)
}

// blobSHA is git's name for a file's bytes (what a tree lists).
func blobSHA(b []byte) string {
	h := sha1.New() //nolint:gosec
	_, _ = h.Write([]byte("blob " + strconv.Itoa(len(b)) + "\x00"))
	_, _ = h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// treeOf is every file on a branch: path → its blob's name.
func (c *Client) treeOf(ctx context.Context, owner, repo, branch string) (map[string]string, error) {
	out := map[string]string{}
	if c.GitLab() {
		for page := 1; page <= 100; page++ {
			var batch []struct {
				ID   string `json:"id"`
				Path string `json:"path"`
				Type string `json:"type"`
			}
			err := c.do(ctx, "GET", glProject(owner, repo)+"/repository/tree", url.Values{"ref": {branch}, "recursive": {"true"}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}, nil, &batch)
			if err != nil {
				return nil, fmt.Errorf("reading branch %s: %w", branch, err)
			}
			for _, e := range batch {
				if e.Type == "blob" {
					out[e.Path] = e.ID
				}
			}
			if len(batch) < 100 {
				break
			}
		}
		return out, nil
	}
	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	q := url.Values{"recursive": {"1"}}
	if !c.GitHub() {
		q.Set("per_page", "10000")
	}
	if err := c.do(ctx, "GET", repoPath(owner, repo)+"/git/trees/"+url.PathEscape(branch), q, nil, &tree); err != nil {
		return nil, fmt.Errorf("reading branch %s: %w", branch, err)
	}
	for _, e := range tree.Tree {
		if e.Type == "blob" {
			out[e.Path] = e.SHA
		}
	}
	return out, nil
}

// giteaCommit: Gitea's (and Forgejo's) several-files change, in one commit.
func (c *Client) giteaCommit(ctx context.Context, owner, repo, branch, message string, put map[string][]byte, paths, gone []string, have map[string]string) (string, error) {
	type op struct {
		Operation string `json:"operation"`
		Path      string `json:"path"`
		Content   string `json:"content,omitempty"`
		SHA       string `json:"sha,omitempty"`
	}
	var files []op
	for _, p := range paths {
		o := op{Operation: "create", Path: p, Content: base64.StdEncoding.EncodeToString(put[p])}
		if sha, ok := have[p]; ok {
			o.Operation, o.SHA = "update", sha
		}
		files = append(files, o)
	}
	for _, p := range gone {
		files = append(files, op{Operation: "delete", Path: p, SHA: have[p]})
	}
	var out struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	err := c.do(ctx, "POST", repoPath(owner, repo)+"/contents", nil, map[string]any{"branch": branch, "message": message, "files": files}, &out)
	return out.Commit.SHA, err
}

// ghCommit: GitHub's way — the files as blobs, a tree on the branch's, a
// commit of it, the branch moved to it.
func (c *Client) ghCommit(ctx context.Context, owner, repo, branch, message string, put map[string][]byte, paths, gone []string) (string, error) {
	rp := repoPath(owner, repo)
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := c.do(ctx, "GET", rp+"/git/ref/heads/"+url.PathEscape(branch), nil, nil, &ref); err != nil {
		return "", fmt.Errorf("the branch %s: %w", branch, err)
	}
	var head struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := c.do(ctx, "GET", rp+"/git/commits/"+ref.Object.SHA, nil, nil, &head); err != nil {
		return "", err
	}
	type entry struct {
		Path string  `json:"path"`
		Mode string  `json:"mode"`
		Type string  `json:"type"`
		SHA  *string `json:"sha"`
	}
	var entries []entry
	for _, p := range paths {
		var blob struct {
			SHA string `json:"sha"`
		}
		if err := c.do(ctx, "POST", rp+"/git/blobs", nil, map[string]string{"content": base64.StdEncoding.EncodeToString(put[p]), "encoding": "base64"}, &blob); err != nil {
			return "", err
		}
		sha := blob.SHA
		entries = append(entries, entry{Path: p, Mode: "100644", Type: "blob", SHA: &sha})
	}
	for _, p := range gone {
		entries = append(entries, entry{Path: p, Mode: "100644", Type: "blob", SHA: nil}) // nil: removed
	}
	var tree struct {
		SHA string `json:"sha"`
	}
	if err := c.do(ctx, "POST", rp+"/git/trees", nil, map[string]any{"base_tree": head.Tree.SHA, "tree": entries}, &tree); err != nil {
		return "", err
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := c.do(ctx, "POST", rp+"/git/commits", nil, map[string]any{"message": message, "tree": tree.SHA, "parents": []string{ref.Object.SHA}}, &commit); err != nil {
		return "", err
	}
	err := c.do(ctx, "PATCH", rp+"/git/refs/heads/"+url.PathEscape(branch), nil, map[string]any{"sha": commit.SHA}, nil)
	return commit.SHA, err
}

// glCommit: GitLab's several-files commit.
func (c *Client) glCommit(ctx context.Context, owner, repo, branch, message string, put map[string][]byte, paths, gone []string, have map[string]string) (string, error) {
	type action struct {
		Action   string `json:"action"`
		FilePath string `json:"file_path"`
		Content  string `json:"content,omitempty"`
		Encoding string `json:"encoding,omitempty"`
	}
	var actions []action
	for _, p := range paths {
		a := action{Action: "create", FilePath: p, Content: base64.StdEncoding.EncodeToString(put[p]), Encoding: "base64"}
		if _, ok := have[p]; ok {
			a.Action = "update"
		}
		actions = append(actions, a)
	}
	for _, p := range gone {
		actions = append(actions, action{Action: "delete", FilePath: p})
	}
	var out struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, "POST", glProject(owner, repo)+"/repository/commits", nil, map[string]any{"branch": branch, "commit_message": message, "actions": actions}, &out)
	return out.ID, err
}

// cleanPath is a folder as kept: no leading or trailing slash, no "..".
func cleanPath(p string) string {
	var parts []string
	for _, s := range strings.Split(strings.ReplaceAll(p, "\\", "/"), "/") {
		if s = strings.TrimSpace(s); s != "" && s != "." && s != ".." {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "/")
}

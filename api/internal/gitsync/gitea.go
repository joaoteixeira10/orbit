package gitsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The REST API of Gitea / Forgejo (api/v1) and GitHub (api.github.com, or
// /api/v3 on GitHub Enterprise), as much as the sync needs. Gitea's API was
// modelled on GitHub's, so one client speaks both; where they differ it
// asks c.kind: GitHub sets labels by name, pages with per_page, lists pull
// requests among issues, has no due dates on issues, and no project-board
// REST API (Projects are GraphQL now): labels stand for columns there.

// Kinds of server.
const (
	KindGitea  = "gitea" // Gitea and Forgejo
	KindGitHub = "github"
)

// Issue is an issue (or pull request) on the server.
type Issue struct {
	ID          int64      `json:"id"`     // global
	Number      int64      `json:"number"` // in the repository
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	State       string     `json:"state"` // open | closed
	HTMLURL     string     `json:"html_url"`
	Labels      []Label    `json:"labels"`
	Assignees   []User     `json:"assignees"`
	DueDate     *string    `json:"due_date"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	PullRequest *struct {
		Merged   bool       `json:"merged"`
		MergedAt *time.Time `json:"merged_at"` // GitHub
	} `json:"pull_request"`
	Repository *struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// Due is the issue's due date as YYYY-MM-DD (the date as the server wrote it).
func (i Issue) Due() *string {
	if i.DueDate == nil || len(*i.DueDate) < 10 {
		return nil
	}
	d := (*i.DueDate)[:10]
	return &d
}

// GitState is open, closed or (a pull request) merged.
func (i Issue) GitState() string {
	if i.PullRequest != nil && (i.PullRequest.Merged || i.PullRequest.MergedAt != nil) {
		return "merged"
	}
	return i.State
}

// Label is a repository label.
type Label struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// User is an account on the server.
type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// Repo is a repository.
type Repo struct {
	ID          int64  `json:"id"`
	FullName    string `json:"full_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Private     bool   `json:"private"`
	HTMLURL     string `json:"html_url"`
	Owner       User   `json:"owner"`
}

// Project is a project board (Gitea with the projects API).
type Project struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

// Column is a column of a project board.
type Column struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Color   string `json:"color"`
	Default bool   `json:"default"`
	Sorting int64  `json:"sorting"`
}

// ErrNoProjects: the server has no projects API (Gitea before it, Forgejo).
var ErrNoProjects = errors.New("this server's API has no project boards; use labels as columns instead")

// APIError is an answer of the server that isn't a success.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("the git server answered %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("the git server answered %d", e.Status)
}

// Client talks to one server as one user.
type Client struct {
	base  string // https://host[/path], no trailing slash
	kind  string // KindGitea | KindGitHub
	token string
	http  *http.Client

	labelNames map[string]map[int64]string // GitHub: owner/repo → label id → name
}

// apiRoot is where a server's REST API is.
func apiRoot(base, kind string) string {
	if kind != KindGitHub {
		return base + "/api/v1"
	}
	if isGitHubCom(base) {
		return "https://api.github.com"
	}
	return base + "/api/v3" // GitHub Enterprise
}

func isGitHubCom(base string) bool {
	u, err := url.Parse(base)
	return err == nil && (u.Host == "github.com" || u.Host == "www.github.com")
}

// GitHub reports whether the client speaks to GitHub.
func (c *Client) GitHub() bool { return c.kind == KindGitHub }

// HasDue reports whether the server's issues have due dates (GitHub's don't).
func (c *Client) HasDue() bool { return c.kind != KindGitHub }

func (c *Client) do(ctx context.Context, method, path string, q url.Values, in, out any) error {
	u := apiRoot(c.base, c.kind) + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return err
	}
	if c.GitHub() {
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	} else {
		req.Header.Set("Authorization", "token "+c.token)
		req.Header.Set("Accept", "application/json")
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var m struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &m)
		return &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(m.Message)}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// isNotJSON: the answer wasn't the API's JSON (a web page, say).
func isNotJSON(err error) bool {
	var se *json.SyntaxError
	return errors.As(err, &se)
}

func isStatus(err error, code int) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == code
}

// Me is the token's account.
func (c *Client) Me(ctx context.Context) (User, error) {
	var u User
	err := c.do(ctx, "GET", "/user", nil, nil, &u)
	return u, err
}

// SearchRepos finds repositories the user can see (most recently updated
// first).
func (c *Client) SearchRepos(ctx context.Context, q string) ([]Repo, error) {
	if c.GitHub() {
		// The user's repositories (own, collaborator, organisations'), most
		// recently updated first, filtered here.
		q = strings.ToLower(strings.TrimSpace(q))
		var out []Repo
		for page := 1; page <= 3 && len(out) < 30; page++ {
			var batch []Repo
			err := c.do(ctx, "GET", "/user/repos", url.Values{"sort": {"updated"}, "per_page": {"100"}, "page": {strconv.Itoa(page)},
				"affiliation": {"owner,collaborator,organization_member"}}, nil, &batch)
			if err != nil {
				return nil, err
			}
			for _, r := range batch {
				if q == "" || strings.Contains(strings.ToLower(r.FullName), q) {
					out = append(out, r)
				}
			}
			if len(batch) < 100 {
				break
			}
		}
		if len(out) > 30 {
			out = out[:30]
		}
		return out, nil
	}
	var res struct {
		Data []Repo `json:"data"`
	}
	err := c.do(ctx, "GET", "/repos/search", url.Values{"q": {q}, "limit": {"30"}, "sort": {"updated"}, "order": {"desc"}}, nil, &res)
	return res.Data, err
}

func repoPath(owner, repo string) string {
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

// Issues lists a repository's issues (not pull requests), open and closed,
// updated since (zero: all), every page.
func (c *Client) Issues(ctx context.Context, owner, repo string, since time.Time) ([]Issue, error) {
	var all []Issue
	size := 50
	if c.GitHub() {
		size = 100
	}
	for page := 1; page <= 100; page++ {
		q := url.Values{"state": {"all"}, "page": {strconv.Itoa(page)}}
		if c.GitHub() {
			q.Set("per_page", strconv.Itoa(size))
		} else {
			q.Set("type", "issues")
			q.Set("limit", strconv.Itoa(size))
		}
		if !since.IsZero() {
			q.Set("since", since.UTC().Format(time.RFC3339))
		}
		var batch []Issue
		if err := c.do(ctx, "GET", repoPath(owner, repo)+"/issues", q, nil, &batch); err != nil {
			return nil, err
		}
		for _, is := range batch {
			if is.PullRequest == nil { // GitHub lists pull requests too
				all = append(all, is)
			}
		}
		if len(batch) < size {
			break
		}
	}
	return all, nil
}

// Issue is one issue (or pull request) by its number.
func (c *Client) Issue(ctx context.Context, owner, repo string, number int64) (Issue, error) {
	var i Issue
	err := c.do(ctx, "GET", fmt.Sprintf("%s/issues/%d", repoPath(owner, repo), number), nil, nil, &i)
	if err == nil && i.PullRequest != nil {
		// The issues endpoint doesn't say whether a pull request was merged.
		var pr struct {
			Merged bool `json:"merged"`
		}
		if c.do(ctx, "GET", fmt.Sprintf("%s/pulls/%d", repoPath(owner, repo), number), nil, nil, &pr) == nil {
			i.PullRequest.Merged = pr.Merged
		}
	}
	return i, err
}

// IssueInput is what Orbit sets on an issue. Nil fields aren't changed.
type IssueInput struct {
	Title     *string   `json:"title,omitempty"`
	Body      *string   `json:"body,omitempty"`
	State     *string   `json:"state,omitempty"`
	DueDate   *string   `json:"due_date,omitempty"` // RFC 3339
	UnsetDue  bool      `json:"unset_due_date,omitempty"`
	Assignees *[]string `json:"assignees,omitempty"` // nil: unchanged; empty: none
	Labels    []int64   `json:"labels,omitempty"`    // create only
}

// DueTime is how a date is sent: the end of that day, UTC.
func DueTime(date string) string { return date + "T23:59:59Z" }

// CreateIssue opens an issue.
func (c *Client) CreateIssue(ctx context.Context, owner, repo string, in IssueInput) (Issue, error) {
	var i Issue
	var body any = in
	if c.GitHub() {
		names, err := c.names(ctx, owner, repo, in.Labels)
		if err != nil {
			return i, err
		}
		gh := map[string]any{"title": in.Title}
		if in.Body != nil {
			gh["body"] = *in.Body
		}
		if in.Assignees != nil {
			gh["assignees"] = *in.Assignees
		}
		if len(names) > 0 {
			gh["labels"] = names
		}
		body = gh
	}
	err := c.do(ctx, "POST", repoPath(owner, repo)+"/issues", nil, body, &i)
	return i, err
}

// EditIssue changes an issue.
func (c *Client) EditIssue(ctx context.Context, owner, repo string, number int64, in IssueInput) (Issue, error) {
	var i Issue
	if c.GitHub() {
		in.DueDate, in.UnsetDue = nil, false
	}
	err := c.do(ctx, "PATCH", fmt.Sprintf("%s/issues/%d", repoPath(owner, repo), number), nil, in, &i)
	return i, err
}

// SetLabels replaces an issue's labels.
func (c *Client) SetLabels(ctx context.Context, owner, repo string, number int64, ids []int64) error {
	var labels any = ids
	if ids == nil {
		labels = []int64{}
	}
	if c.GitHub() {
		names, err := c.names(ctx, owner, repo, ids)
		if err != nil {
			return err
		}
		labels = names
	}
	return c.do(ctx, "PUT", fmt.Sprintf("%s/issues/%d/labels", repoPath(owner, repo), number), nil, map[string]any{"labels": labels}, nil)
}

// names turns label ids into names (GitHub sets labels by name).
func (c *Client) names(ctx context.Context, owner, repo string, ids []int64) ([]string, error) {
	out := []string{}
	if len(ids) == 0 {
		return out, nil
	}
	byID := c.labelNames[owner+"/"+repo]
	if byID == nil {
		if _, err := c.Labels(ctx, owner, repo); err != nil {
			return nil, err
		}
		byID = c.labelNames[owner+"/"+repo]
	}
	for _, id := range ids {
		if n, ok := byID[id]; ok {
			out = append(out, n)
		}
	}
	return out, nil
}

// Labels lists a repository's labels.
func (c *Client) Labels(ctx context.Context, owner, repo string) ([]Label, error) {
	var all []Label
	size, sizeKey := 50, "limit"
	if c.GitHub() {
		size, sizeKey = 100, "per_page"
	}
	for page := 1; page <= 20; page++ {
		var batch []Label
		if err := c.do(ctx, "GET", repoPath(owner, repo)+"/labels", url.Values{sizeKey: {strconv.Itoa(size)}, "page": {strconv.Itoa(page)}}, nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < size {
			break
		}
	}
	byID := map[int64]string{}
	for _, l := range all {
		byID[l.ID] = l.Name
	}
	if c.labelNames == nil {
		c.labelNames = map[string]map[int64]string{}
	}
	c.labelNames[owner+"/"+repo] = byID
	return all, nil
}

// Projects lists a repository's open project boards (ErrNoProjects when
// the server has no projects API).
func (c *Client) Projects(ctx context.Context, owner, repo string) ([]Project, error) {
	if c.GitHub() {
		return nil, ErrNoProjects
	}
	var ps []Project
	err := c.do(ctx, "GET", repoPath(owner, repo)+"/projects", url.Values{"state": {"open"}, "limit": {"50"}}, nil, &ps)
	if isStatus(err, http.StatusNotFound) || isStatus(err, http.StatusMethodNotAllowed) {
		return nil, ErrNoProjects
	}
	return ps, err
}

// Columns lists a project's columns, in their order.
func (c *Client) Columns(ctx context.Context, owner, repo string, project int64) ([]Column, error) {
	var cs []Column
	err := c.do(ctx, "GET", fmt.Sprintf("%s/projects/%d/columns", repoPath(owner, repo), project), url.Values{"limit": {"50"}}, nil, &cs)
	return cs, err
}

// ColumnIssues lists the issues in a project column.
func (c *Client) ColumnIssues(ctx context.Context, owner, repo string, project, column int64) ([]Issue, error) {
	var all []Issue
	for page := 1; page <= 40; page++ {
		var batch []Issue
		if err := c.do(ctx, "GET", fmt.Sprintf("%s/projects/%d/columns/%d/issues", repoPath(owner, repo), project, column),
			url.Values{"limit": {"50"}, "page": {strconv.Itoa(page)}}, nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 50 {
			break
		}
	}
	return all, nil
}

// MoveIssue puts an issue (by its global id) in a project column, adding it
// to the project if it isn't there.
func (c *Client) MoveIssue(ctx context.Context, owner, repo string, project, issueID, column int64) error {
	err := c.do(ctx, "POST", fmt.Sprintf("%s/projects/%d/issues/%d/move", repoPath(owner, repo), project, issueID), nil, map[string]any{"column_id": column}, nil)
	if isStatus(err, http.StatusNotFound) || isStatus(err, http.StatusUnprocessableEntity) {
		return c.do(ctx, "POST", fmt.Sprintf("%s/projects/%d/columns/%d/issues/%d", repoPath(owner, repo), project, column, issueID), nil, nil, nil)
	}
	return err
}

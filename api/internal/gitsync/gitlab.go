package gitsync

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GitLab (gitlab.com or a self-managed one, REST API v4) speaks differently
// from Gitea and GitHub, so the client translates: a repository is a
// project named by its full path ("group/sub/project"), an issue by its iid
// (its number in the project), a merge request apart from issues; states
// are opened / closed and change with state_event; labels go by name,
// assignees by user id; comments are notes, edited under their issue;
// webhooks carry a token, not a signature. Project boards are GitLab's issue
// boards, lists of labels — so labels stand for columns, as on GitHub.

const KindGitLab = "gitlab"

// GitLab reports whether the client speaks to GitLab.
func (c *Client) GitLab() bool { return c.kind == KindGitLab }

func isGitLabCom(base string) bool {
	u, err := url.Parse(base)
	return err == nil && (u.Host == "gitlab.com" || u.Host == "www.gitlab.com")
}

// glProject is the API path of a project: its full path, escaped as one.
func glProject(owner, repo string) string {
	return "/projects/" + url.PathEscape(owner+"/"+repo)
}

type glUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func (u glUser) user() User { return User{ID: u.ID, Login: u.Username} }

type glLabel struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (l glLabel) label() Label {
	return Label{ID: l.ID, Name: l.Name, Color: strings.TrimPrefix(l.Color, "#")}
}

type glMilestone struct {
	ID      int64   `json:"id"`
	IID     int64   `json:"iid"`
	Title   string  `json:"title"`
	State   string  `json:"state"`
	DueDate *string `json:"due_date"`
}

func (m glMilestone) milestone() Milestone {
	out := Milestone{ID: m.ID, Number: m.IID, Title: m.Title, State: m.State}
	if m.DueDate != nil {
		if d, err := time.Parse("2006-01-02", *m.DueDate); err == nil {
			out.DueOn = &d
		}
	}
	return out
}

type glIssue struct {
	ID          int64        `json:"id"`
	IID         int64        `json:"iid"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	State       string       `json:"state"` // opened | closed | merged | locked
	WebURL      string       `json:"web_url"`
	Labels      []glLabel    `json:"labels"` // asked for with_labels_details
	Assignees   []glUser     `json:"assignees"`
	DueDate     *string      `json:"due_date"`
	UpdatedAt   time.Time    `json:"updated_at"`
	ClosedAt    *time.Time   `json:"closed_at"`
	MergedAt    *time.Time   `json:"merged_at"`
	Milestone   *glMilestone `json:"milestone"`
	References  struct {
		Full string `json:"full"` // group/project#12
	} `json:"references"`
}

// issue is a GitLab issue (or, pull, merge request) as the sync knows them.
func (g glIssue) issue(pull bool) Issue {
	i := Issue{ID: g.ID, Number: g.IID, Title: g.Title, Body: g.Description, HTMLURL: g.WebURL, DueDate: g.DueDate, UpdatedAt: g.UpdatedAt, ClosedAt: g.ClosedAt}
	i.State = "open"
	if g.State == "closed" || g.State == "merged" || g.State == "locked" {
		i.State = "closed"
	}
	for _, l := range g.Labels {
		i.Labels = append(i.Labels, l.label())
	}
	for _, a := range g.Assignees {
		i.Assignees = append(i.Assignees, a.user())
	}
	if g.Milestone != nil {
		m := g.Milestone.milestone()
		i.Milestone = &m
	}
	if pull {
		i.PullRequest = &struct {
			Merged   bool       `json:"merged"`
			MergedAt *time.Time `json:"merged_at"`
		}{Merged: g.State == "merged", MergedAt: g.MergedAt}
	}
	return i
}

func (c *Client) glMe(ctx context.Context) (User, error) {
	var u glUser
	err := c.do(ctx, "GET", "/user", nil, nil, &u)
	return u.user(), err
}

func (c *Client) glSearchRepos(ctx context.Context, q string) ([]Repo, error) {
	var ps []struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		Path              string `json:"path"` // the slug: the repo part of its address
		PathWithNamespace string `json:"path_with_namespace"`
		Description       string `json:"description"`
		Visibility        string `json:"visibility"`
		WebURL            string `json:"web_url"`
	}
	v := url.Values{"membership": {"true"}, "order_by": {"last_activity_at"}, "per_page": {"30"}, "with_issues_enabled": {"true"}}
	if q = strings.TrimSpace(q); q != "" {
		v.Set("search", q)
		v.Set("search_namespaces", "true")
	}
	if err := c.do(ctx, "GET", "/projects", v, nil, &ps); err != nil {
		return nil, err
	}
	out := make([]Repo, 0, len(ps))
	for _, p := range ps {
		owner := p.PathWithNamespace
		if i := strings.LastIndex(owner, "/"); i >= 0 {
			owner = owner[:i]
		}
		out = append(out, Repo{ID: p.ID, FullName: p.PathWithNamespace, Name: p.Path, Description: p.Description, Private: p.Visibility != "public", HTMLURL: p.WebURL, Owner: User{Login: owner}})
	}
	return out, nil
}

func (c *Client) glIssues(ctx context.Context, owner, repo string, since time.Time) ([]Issue, error) {
	var all []Issue
	for page := 1; page <= 100; page++ {
		v := url.Values{"state": {"all"}, "per_page": {"100"}, "page": {strconv.Itoa(page)}, "with_labels_details": {"true"}}
		if !since.IsZero() {
			v.Set("updated_after", since.UTC().Format(time.RFC3339))
		}
		var batch []glIssue
		if err := c.do(ctx, "GET", glProject(owner, repo)+"/issues", v, nil, &batch); err != nil {
			return nil, err
		}
		for _, g := range batch {
			all = append(all, g.issue(false))
		}
		if len(batch) < 100 {
			break
		}
	}
	return all, nil
}

func (c *Client) glItem(ctx context.Context, owner, repo string, number int64, pull bool) (Issue, error) {
	kind := "issues"
	if pull {
		kind = "merge_requests"
	}
	var g glIssue
	err := c.do(ctx, "GET", fmt.Sprintf("%s/%s/%d", glProject(owner, repo), kind, number), url.Values{"with_labels_details": {"true"}}, nil, &g)
	return g.issue(pull), err
}

// glUserIDs turns logins into GitLab user ids (assignees go by id there).
func (c *Client) glUserIDs(ctx context.Context, logins []string) ([]int64, error) {
	ids := []int64{}
	for _, l := range logins {
		if id, ok := c.userIDs[l]; ok {
			ids = append(ids, id)
			continue
		}
		var us []glUser
		if err := c.do(ctx, "GET", "/users", url.Values{"username": {l}}, nil, &us); err != nil {
			return nil, err
		}
		if len(us) == 0 {
			continue
		}
		if c.userIDs == nil {
			c.userIDs = map[string]int64{}
		}
		c.userIDs[l] = us[0].ID
		ids = append(ids, us[0].ID)
	}
	return ids, nil
}

// glBody is an IssueInput as GitLab takes it.
func (c *Client) glBody(ctx context.Context, owner, repo string, in IssueInput, create bool) (map[string]any, error) {
	b := map[string]any{}
	if in.Title != nil {
		b["title"] = *in.Title
	}
	if in.Body != nil {
		b["description"] = *in.Body
	}
	if in.State != nil && !create {
		b["state_event"] = map[string]string{"closed": "close", "open": "reopen"}[*in.State]
	}
	if in.DueDate != nil && len(*in.DueDate) >= 10 {
		b["due_date"] = (*in.DueDate)[:10]
	} else if in.UnsetDue {
		b["due_date"] = ""
	}
	if in.Assignees != nil {
		ids, err := c.glUserIDs(ctx, *in.Assignees)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 && !create {
			ids = []int64{0} // GitLab: 0 takes them all off
		}
		b["assignee_ids"] = ids
	}
	if len(in.Labels) > 0 {
		names, err := c.names(ctx, owner, repo, in.Labels)
		if err != nil {
			return nil, err
		}
		b["labels"] = strings.Join(names, ",")
	}
	if in.Milestone != nil {
		b["milestone_id"] = *in.Milestone // 0 takes it off
	}
	return b, nil
}

func (c *Client) glCreateIssue(ctx context.Context, owner, repo string, in IssueInput) (Issue, error) {
	body, err := c.glBody(ctx, owner, repo, in, true)
	if err != nil {
		return Issue{}, err
	}
	var g glIssue
	err = c.do(ctx, "POST", glProject(owner, repo)+"/issues", nil, body, &g)
	return g.issue(false), err
}

func (c *Client) glEditIssue(ctx context.Context, owner, repo string, number int64, in IssueInput) (Issue, error) {
	body, err := c.glBody(ctx, owner, repo, in, false)
	if err != nil {
		return Issue{}, err
	}
	var g glIssue
	err = c.do(ctx, "PUT", fmt.Sprintf("%s/issues/%d", glProject(owner, repo), number), url.Values{"with_labels_details": {"true"}}, body, &g)
	return g.issue(false), err
}

func (c *Client) glSetLabels(ctx context.Context, owner, repo string, number int64, ids []int64) error {
	names, err := c.names(ctx, owner, repo, ids)
	if err != nil {
		return err
	}
	return c.do(ctx, "PUT", fmt.Sprintf("%s/issues/%d", glProject(owner, repo), number), nil, map[string]any{"labels": strings.Join(names, ",")}, nil)
}

func (c *Client) glLabels(ctx context.Context, owner, repo string) ([]Label, error) {
	var all []Label
	for page := 1; page <= 20; page++ {
		var batch []glLabel
		if err := c.do(ctx, "GET", glProject(owner, repo)+"/labels", url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}}, nil, &batch); err != nil {
			return nil, err
		}
		for _, l := range batch {
			all = append(all, l.label())
		}
		if len(batch) < 100 {
			break
		}
	}
	return all, nil
}

func (c *Client) glMilestones(ctx context.Context, owner, repo string) ([]Milestone, error) {
	var ms []glMilestone
	if err := c.do(ctx, "GET", glProject(owner, repo)+"/milestones", url.Values{"state": {"active"}, "per_page": {"100"}}, nil, &ms); err != nil {
		return nil, err
	}
	out := make([]Milestone, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.milestone())
	}
	return out, nil
}

type glNote struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	Author    glUser    `json:"author"`
	System    bool      `json:"system"` // "changed the due date…": not a comment
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (n glNote) comment() Comment {
	return Comment{ID: n.ID, Body: n.Body, User: n.Author.user(), CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt}
}

func (c *Client) glComments(ctx context.Context, owner, repo string, number int64) ([]Comment, error) {
	var out []Comment
	for page := 1; page <= 20; page++ {
		var batch []glNote
		err := c.do(ctx, "GET", fmt.Sprintf("%s/issues/%d/notes", glProject(owner, repo), number), url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}, "sort": {"asc"}}, nil, &batch)
		if err != nil {
			return nil, err
		}
		for _, n := range batch {
			if !n.System {
				out = append(out, n.comment())
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	return out, nil
}

func (c *Client) glNoteCall(ctx context.Context, method, path, body string) (Comment, error) {
	var n glNote
	var in any
	if body != "" {
		in = map[string]string{"body": body}
	}
	err := c.do(ctx, method, path, nil, in, &n)
	return n.comment(), err
}

func (c *Client) glCreateHook(ctx context.Context, owner, repo, target, secret string) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	err := c.do(ctx, "POST", glProject(owner, repo)+"/hooks", nil, map[string]any{
		"url": target, "token": secret, "issues_events": true, "confidential_issues_events": true, "note_events": true,
		"confidential_note_events": true, "push_events": false, "enable_ssl_verification": true,
	}, &out)
	return out.ID, err
}

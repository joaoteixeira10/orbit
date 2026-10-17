// Package gitsync keeps task lists and git issues in step, both ways, on
// Gitea and Forgejo (GitHub later: a second client with the same methods).
//
// A list linked to a repository (models.GitLink) holds its issues as tasks:
// the background sync pulls what changed on git (titles, descriptions, due
// dates, labels as tags, the assignee, open / closed, the column), and every
// change made in Orbit is pushed at once as the person who made it (their
// own token; the person who linked the list when they have none). A list's
// columns are open / done, labels, or a project board's columns. Any task
// can also be linked by hand to one issue or pull request, which then only
// syncs open / closed.
package gitsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/safedial"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// closedWindow: closed issues older than this aren't imported (open ones
// always are).
const closedWindow = 14 * 24 * time.Hour

// Engine syncs lists and issues.
type Engine struct {
	db      *gorm.DB
	sealer  *keystore.Sealer
	allowed map[string]bool // base URLs that may be on a private network
	log     *zap.Logger
	// Changed is told which workspace's tasks a background sync changed.
	Changed func(workspaceID string)

	mu    sync.Mutex
	locks map[string]*sync.Mutex // one sync or push per list at a time
	// clientFor makes the HTTP client for a base URL (tests replace it).
	clientFor func(base string) *http.Client
}

// New builds the engine. allowed are base URLs users may connect even on a
// private network (GIT_SERVERS); every other server must be public.
func New(db *gorm.DB, sealer *keystore.Sealer, allowed []string, log *zap.Logger) *Engine {
	e := &Engine{db: db, sealer: sealer, allowed: map[string]bool{}, log: log, locks: map[string]*sync.Mutex{}}
	for _, a := range allowed {
		if b, err := normalize(a); err == nil {
			e.allowed[b] = true
		}
	}
	e.clientFor = func(base string) *http.Client {
		if e.allowed[base] {
			return &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		}
		return safedial.PublicClient(20 * time.Second)
	}
	return e
}

// UseHTTP makes every client use h (tests, against an httptest server).
func (e *Engine) UseHTTP(h *http.Client) { e.clientFor = func(string) *http.Client { return h } }

// Allowed lists the servers named in GIT_SERVERS.
func (e *Engine) Allowed() []string {
	out := make([]string, 0, len(e.allowed))
	for b := range e.allowed {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}

func (e *Engine) lock(listID string) func() {
	e.mu.Lock()
	m := e.locks[listID]
	if m == nil {
		m = &sync.Mutex{}
		e.locks[listID] = m
	}
	e.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// normalize is a base URL as stored: scheme://host[:port][/path], lower-case
// host, no trailing slash.
func normalize(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("give the server's address, like https://gitea.example.org")
	}
	return u.Scheme + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/"), nil
}

// NormalizeBase checks and normalises a server address a user gives: https,
// or plain http only for a server named in GIT_SERVERS.
func (e *Engine) NormalizeBase(raw string) (string, error) {
	b, err := normalize(raw)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(b, "http://") && !e.allowed[b] {
		return "", errors.New("the server must use https")
	}
	return b, nil
}

func (e *Engine) client(acc models.GitAccount) (*Client, error) {
	if e.sealer == nil {
		return nil, errors.New("git tokens can't be read on this instance")
	}
	tok, err := e.sealer.Unseal(acc.TokenSealed)
	if err != nil {
		return nil, errors.New("this git token can't be read; connect the account again")
	}
	return &Client{base: acc.BaseURL, token: string(tok), http: e.clientFor(acc.BaseURL)}, nil
}

// Connect checks a token against the server and saves (or refreshes) the
// user's account there.
func (e *Engine) Connect(ctx context.Context, userID, base, token string) (models.GitAccount, error) {
	base, err := e.NormalizeBase(base)
	if err != nil {
		return models.GitAccount{}, err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return models.GitAccount{}, errors.New("give a personal access token")
	}
	c := &Client{base: base, token: token, http: e.clientFor(base)}
	me, err := c.Me(ctx)
	if err != nil {
		if isStatus(err, http.StatusUnauthorized) || isStatus(err, http.StatusForbidden) {
			return models.GitAccount{}, errors.New("the server refused this token")
		}
		return models.GitAccount{}, fmt.Errorf("couldn't reach %s: %w", base, err)
	}
	sealed, err := e.sealer.Seal([]byte(token))
	if err != nil {
		return models.GitAccount{}, err
	}
	now := time.Now()
	acc := models.GitAccount{UserID: userID, Kind: "gitea", BaseURL: base}
	e.db.Where("user_id = ? AND base_url = ?", userID, base).First(&acc)
	acc.Login, acc.TokenSealed, acc.LastError, acc.CheckedAt = me.Login, sealed, "", &now
	return acc, e.db.Save(&acc).Error
}

// ClientFor is a user's client on a server.
func (e *Engine) ClientFor(userID, base string) (*Client, error) {
	var acc models.GitAccount
	if e.db.Where("user_id = ? AND base_url = ?", userID, base).First(&acc).Error != nil {
		return nil, fmt.Errorf("connect your account on %s first (Settings → Git)", base)
	}
	return e.client(acc)
}

// clientAs acts as actor on a link's server, else as whoever linked it.
func (e *Engine) clientAs(actor string, link *models.GitLink, host string) (*Client, error) {
	if c, err := e.ClientFor(actor, host); err == nil {
		return c, nil
	}
	if link != nil {
		return e.linkerClient(*link)
	}
	return nil, fmt.Errorf("connect your account on %s first (Settings → Git)", host)
}

// errLinkerLeft: the person who linked a list left its workspace, so their
// token isn't used for it any more.
var errLinkerLeft = errors.New("whoever linked this list has left the workspace: link it again with your own account")

// member reports whether a user still belongs to a workspace.
func (e *Engine) member(wsID, userID string) bool {
	var n int64
	e.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ? AND user_id = ?", wsID, userID).Count(&n)
	return n > 0
}

// linkerClient acts as whoever linked a list — only while they're still a
// member of its workspace.
func (e *Engine) linkerClient(link models.GitLink) (*Client, error) {
	if !e.member(link.WorkspaceID, link.LinkedBy) {
		return nil, errLinkerLeft
	}
	return e.ClientFor(link.LinkedBy, link.Host)
}

// ColumnMap is a list column → a label name ("labels" mode) or a project
// column id ("project" mode).
type ColumnMap map[string]string

// DecodeColumnMap reads a link's column map.
func DecodeColumnMap(s string) ColumnMap {
	m := ColumnMap{}
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

// Encode writes it.
func (m ColumnMap) Encode() string {
	b, _ := json.Marshal(m)
	return string(b)
}

func decodeIDs(s string) map[int64]bool {
	var ids []int64
	_ = json.Unmarshal([]byte(s), &ids)
	out := map[int64]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// Ignore stops an issue from coming back into a list (its task was deleted
// in Orbit).
func (e *Engine) Ignore(listID string, issueID int64) {
	var link models.GitLink
	if issueID == 0 || e.db.Where("list_id = ?", listID).First(&link).Error != nil {
		return
	}
	ids := decodeIDs(link.IgnoreIDs)
	ids[issueID] = true
	list := make([]int64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	b, _ := json.Marshal(list)
	e.db.Model(&link).Update("ignore_ids", string(b))
}

func decodeTags(s string) []string {
	var t []string
	_ = json.Unmarshal([]byte(s), &t)
	return t
}

func encodeTags(t []string) string {
	if t == nil {
		t = []string{}
	}
	b, _ := json.Marshal(t)
	return string(b)
}

// ---- Pull --------------------------------------------------------------------

// workspaceLogins maps logins on a server to the workspace's members who
// connected an account there (and back).
func (e *Engine) workspaceLogins(wsID, host string) (byLogin map[string]string, byUser map[string]string) {
	var rows []struct {
		UserID string
		Login  string
	}
	e.db.Table("git_accounts").Select("git_accounts.user_id, git_accounts.login").
		Joins("JOIN workspace_members ON workspace_members.user_id = git_accounts.user_id AND workspace_members.workspace_id = ?", wsID).
		Where("git_accounts.base_url = ?", host).Scan(&rows)
	byLogin, byUser = map[string]string{}, map[string]string{}
	for _, r := range rows {
		byLogin[strings.ToLower(r.Login)] = r.UserID
		byUser[r.UserID] = r.Login
	}
	return byLogin, byUser
}

type columns struct {
	all       []planner.Column
	firstOpen planner.Column
	firstDone planner.Column
}

func (cs columns) byID(id string) (planner.Column, bool) {
	return planner.FindColumn(cs.all, id)
}

// target is the column an issue belongs in.
func target(link models.GitLink, cm ColumnMap, cs columns, is Issue, projectCol string, current string) planner.Column {
	cur, haveCur := cs.byID(current)
	if is.State == "closed" {
		if haveCur && cur.Done {
			return cur
		}
		return cs.firstDone
	}
	switch link.Mode {
	case "project":
		if c, ok := cs.byID(projectCol); ok && projectCol != "" {
			return c
		}
	case "labels":
		labels := map[string]bool{}
		for _, l := range is.Labels {
			labels[strings.ToLower(l.Name)] = true
		}
		for _, c := range cs.all {
			if name := cm[c.ID]; name != "" && labels[strings.ToLower(name)] && !c.Done {
				return c
			}
		}
		// None of the column labels: the column without one (Open).
		for _, c := range cs.all {
			if cm[c.ID] == "" && !c.Done {
				return c
			}
		}
	}
	if haveCur && !cur.Done {
		return cur
	}
	return cs.firstOpen
}

// SyncLink pulls a linked list's issues into its tasks.
func (e *Engine) SyncLink(ctx context.Context, link models.GitLink) error {
	defer e.lock(link.ListID)()
	err := e.pull(ctx, link)
	now := time.Now()
	up := map[string]any{"synced_at": &now, "last_error": ""}
	if err != nil {
		up = map[string]any{"last_error": truncate(err.Error(), 500)}
	}
	e.db.Model(&models.GitLink{}).Where("id = ?", link.ID).Updates(up)
	return err
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (e *Engine) pull(ctx context.Context, link models.GitLink) error {
	c, err := e.linkerClient(link)
	if err != nil {
		return err
	}
	var list models.TaskList
	if err := e.db.First(&list, "id = ?", link.ListID).Error; err != nil {
		return err
	}
	e.retry(ctx, link)
	all := planner.DecodeColumns(list.Columns)
	cs := columns{all: all, firstOpen: planner.FirstOpen(all), firstDone: planner.FirstDone(all)}
	cm := DecodeColumnMap(link.ColumnMap)
	ignore := decodeIDs(link.IgnoreIDs)
	byLogin, byUser := e.workspaceLogins(link.WorkspaceID, link.Host)

	// The issues to look at, and (project) which board column each is in.
	var issues []Issue
	projectCol := map[int64]string{}
	if link.Mode == "project" {
		back := map[string]string{} // project column id → list column id
		for col, pc := range cm {
			back[pc] = col
		}
		pcs, err := c.Columns(ctx, link.Owner, link.Repo, link.ProjectID)
		if err != nil {
			return err
		}
		for _, pc := range pcs {
			iss, err := c.ColumnIssues(ctx, link.Owner, link.Repo, link.ProjectID, pc.ID)
			if err != nil {
				return err
			}
			for _, is := range iss {
				projectCol[is.ID] = back[strconv.FormatInt(pc.ID, 10)]
				issues = append(issues, is)
			}
		}
	} else {
		var since time.Time
		if link.SyncedAt != nil {
			since = link.SyncedAt.Add(-2 * time.Minute)
		}
		if issues, err = c.Issues(ctx, link.Owner, link.Repo, since); err != nil {
			return err
		}
	}
	if len(issues) == 0 {
		return nil
	}

	// Labels the repository has (tags that aren't one stay Orbit's own), and
	// those that stand for columns (not tags).
	repoLabels := map[string]bool{}
	if labels, err := c.Labels(ctx, link.Owner, link.Repo); err == nil {
		for _, l := range labels {
			repoLabels[strings.ToLower(l.Name)] = true
		}
	}
	columnLabels := map[string]bool{}
	if link.Mode == "labels" {
		for _, name := range cm {
			columnLabels[strings.ToLower(name)] = true
		}
	}

	var tasks []models.Task
	e.db.Where("list_id = ? AND git_issue_id <> 0 AND git_host = ? AND git_repo = ?", link.ListID, link.Host, link.Owner+"/"+link.Repo).Find(&tasks)
	have := map[int64]*models.Task{}
	for i := range tasks {
		have[tasks[i].GitIssueID] = &tasks[i]
	}

	changed := false
	now := time.Now()
	for _, is := range issues {
		if ignore[is.ID] {
			continue
		}
		t := have[is.ID]
		if t != nil && t.GitDirty {
			continue // Orbit's change goes first
		}
		if t == nil {
			if is.State == "closed" && (is.ClosedAt == nil || now.Sub(*is.ClosedAt) > closedWindow) {
				continue
			}
			t = &models.Task{WorkspaceID: link.WorkspaceID, ListID: link.ListID, CreatedBy: link.LinkedBy, Tags: "[]"}
		} else if t.GitSyncedAt != nil && !is.UpdatedAt.After(*t.GitSyncedAt) {
			// Nothing new on the issue; on a board it may still have moved.
			if link.Mode != "project" {
				continue
			}
			col := target(link, cm, cs, is, projectCol[is.ID], t.ColumnID)
			if col.ID == t.ColumnID {
				continue
			}
			e.placeIn(t, col)
			e.db.Model(&models.Task{}).Where("id = ?", t.ID).Updates(map[string]any{"column_id": t.ColumnID, "completed_at": t.CompletedAt, "rank": t.Rank})
			changed = true
			continue
		}
		fresh := t.ID == ""
		t.Title = truncate(strings.TrimSpace(is.Title), 500)
		if t.Title == "" {
			t.Title = fmt.Sprintf("#%d", is.Number)
		}
		t.Notes = is.Body
		t.Due = is.Due()
		if t.Due == nil {
			t.DueTime = nil
		}
		var tags []string
		for _, l := range is.Labels {
			if !columnLabels[strings.ToLower(l.Name)] {
				tags = append(tags, l.Name)
			}
		}
		for _, own := range decodeTags(t.Tags) {
			if !repoLabels[strings.ToLower(own)] {
				tags = append(tags, own)
			}
		}
		t.Tags = encodeTags(tags)
		// The issue's assignees who are members with an account there; those
		// git can't name (no account there) stay assigned.
		var who []string
		for _, a := range is.Assignees {
			if uid, ok := byLogin[strings.ToLower(a.Login)]; ok {
				who = append(who, uid)
			}
		}
		for _, uid := range t.AssigneeList() {
			if byUser[uid] == "" {
				who = append(who, uid)
			}
		}
		t.SetAssignees(who)
		t.GitHost, t.GitRepo, t.GitNumber, t.GitIssueID = link.Host, link.Owner+"/"+link.Repo, is.Number, is.ID
		t.GitURL, t.GitState, t.GitPull = is.HTMLURL, is.GitState(), false
		synced := is.UpdatedAt
		t.GitSyncedAt = &synced
		col := target(link, cm, cs, is, projectCol[is.ID], t.ColumnID)
		if fresh || col.ID != t.ColumnID {
			e.placeIn(t, col)
		}
		if is.State == "closed" && t.CompletedAt == nil {
			ct := now
			if is.ClosedAt != nil {
				ct = *is.ClosedAt
			}
			t.CompletedAt = &ct
		}
		if err := e.db.Save(t).Error; err != nil {
			return err
		}
		changed = true
	}
	if changed && e.Changed != nil {
		e.Changed(link.WorkspaceID)
	}
	return nil
}

// retry sends the changes of a list's tasks that didn't reach git, as whoever
// linked the list.
func (e *Engine) retry(ctx context.Context, link models.GitLink) {
	if !e.member(link.WorkspaceID, link.LinkedBy) {
		return // their token isn't used any more (errLinkerLeft)
	}
	var dirty []models.Task
	e.db.Where("list_id = ? AND git_dirty = ? AND parent_id IS NULL", link.ListID, true).Limit(50).Find(&dirty)
	for i := range dirty {
		t := &dirty[i]
		var err error
		if t.GitIssueID == 0 {
			err = e.createIssue(ctx, link.LinkedBy, link, t)
		} else {
			err = e.push(ctx, link.LinkedBy, e.linkOf(t), t, All)
		}
		_ = e.dirtyOn(t, err)
	}
}

// placeIn puts a task at the end of a column, completed or not as the
// column says.
func (e *Engine) placeIn(t *models.Task, col planner.Column) {
	t.ColumnID = col.ID
	var last struct{ Max *float64 }
	e.db.Model(&models.Task{}).Select("MAX(rank) AS max").Where("list_id = ? AND column_id = ? AND parent_id IS NULL", t.ListID, col.ID).Scan(&last)
	t.Rank = planner.Rank(last.Max, nil)
	switch {
	case col.Done && t.CompletedAt == nil:
		now := time.Now()
		t.CompletedAt = &now
	case !col.Done:
		t.CompletedAt = nil
	}
}

// ---- Push --------------------------------------------------------------------

// Change says what changed on a task in Orbit.
type Change struct {
	Title, Notes, Due, State, Assignee, Tags, Column bool
}

// Any reports whether anything that reaches git changed.
func (c Change) Any() bool {
	return c.Title || c.Notes || c.Due || c.State || c.Assignee || c.Tags || c.Column
}

func (e *Engine) linkOf(t *models.Task) *models.GitLink {
	var link models.GitLink
	if e.db.Where("list_id = ?", t.ListID).First(&link).Error != nil {
		return nil
	}
	if link.Host != t.GitHost || link.Owner+"/"+link.Repo != t.GitRepo {
		return nil // linked by hand to an issue of another repository
	}
	return &link
}

// All is every change (a retry sends the whole task).
var All = Change{Title: true, Notes: true, Due: true, State: true, Assignee: true, Tags: true, Column: true}

// Push sends a task's changes to its issue, as actor. A task linked by hand
// (not in a linked list) only sends open / closed. When it fails the task is
// marked to be sent again by the background sync.
func (e *Engine) Push(ctx context.Context, actor string, t *models.Task, ch Change) error {
	if t.GitIssueID == 0 || !ch.Any() {
		return nil
	}
	link := e.linkOf(t)
	if link != nil {
		defer e.lock(link.ListID)()
	}
	return e.dirtyOn(t, e.push(ctx, actor, link, t, ch))
}

// dirtyOn marks a task whose change didn't reach git (or clears the mark).
func (e *Engine) dirtyOn(t *models.Task, err error) error {
	if dirty := err != nil; dirty != t.GitDirty {
		t.GitDirty = dirty
		e.db.Model(&models.Task{}).Where("id = ?", t.ID).Update("git_dirty", dirty)
	}
	return err
}

func (e *Engine) push(ctx context.Context, actor string, link *models.GitLink, t *models.Task, ch Change) error {
	if link == nil {
		ch = Change{State: ch.State}
		if !ch.State || t.GitPull && t.GitState == "merged" {
			return nil
		}
	}
	owner, repo, ok := strings.Cut(t.GitRepo, "/")
	if !ok {
		return nil
	}
	c, err := e.clientAs(actor, link, t.GitHost)
	if err != nil {
		return err
	}
	in := IssueInput{}
	edit := false
	if ch.Title {
		in.Title, edit = &t.Title, true
	}
	if ch.Notes {
		in.Body, edit = &t.Notes, true
	}
	if ch.State {
		state := "open"
		if t.CompletedAt != nil {
			state = "closed"
		}
		in.State, edit = &state, true
	}
	if ch.Due {
		if t.Due != nil {
			d := DueTime(*t.Due)
			in.DueDate = &d
		} else {
			in.UnsetDue = true
		}
		edit = true
	}
	if ch.Assignee && link != nil {
		_, byUser := e.workspaceLogins(t.WorkspaceID, t.GitHost)
		logins := []string{}
		ids := t.AssigneeList()
		for _, uid := range ids {
			if l, ok := byUser[uid]; ok {
				logins = append(logins, l)
			}
		}
		if len(ids) == 0 || len(logins) > 0 {
			in.Assignees, edit = &logins, true
		}
	}
	var is Issue
	if edit {
		if is, err = c.EditIssue(ctx, owner, repo, t.GitNumber, in); err != nil {
			return err
		}
	}
	if link != nil && (ch.Tags || (ch.Column && link.Mode == "labels")) {
		ids, err := e.labelIDs(ctx, c, *link, t)
		if err != nil {
			return err
		}
		if err := c.SetLabels(ctx, owner, repo, t.GitNumber, ids); err != nil {
			return err
		}
		is = Issue{} // changed since
	}
	if link != nil && ch.Column && link.Mode == "project" {
		if pc, ok := DecodeColumnMap(link.ColumnMap)[t.ColumnID]; ok && pc != "" {
			id, _ := strconv.ParseInt(pc, 10, 64)
			if err := c.MoveIssue(ctx, owner, repo, link.ProjectID, t.GitIssueID, id); err != nil {
				return err
			}
			is = Issue{}
		}
	}
	// Remember the issue as it is now, so the next pull doesn't bring our own
	// change back as news.
	if is.ID == 0 {
		if is, err = c.Issue(ctx, owner, repo, t.GitNumber); err != nil {
			return err
		}
	}
	synced := is.UpdatedAt
	t.GitSyncedAt, t.GitState = &synced, is.GitState()
	return e.db.Model(&models.Task{}).Where("id = ?", t.ID).Updates(map[string]any{"git_synced_at": &synced, "git_state": t.GitState}).Error
}

// labelIDs are the labels an issue should carry for a task: its tags that
// are repository labels, plus (labels mode) its column's label.
func (e *Engine) labelIDs(ctx context.Context, c *Client, link models.GitLink, t *models.Task) ([]int64, error) {
	labels, err := c.Labels(ctx, link.Owner, link.Repo)
	if err != nil {
		return nil, err
	}
	byName := map[string]int64{}
	for _, l := range labels {
		byName[strings.ToLower(l.Name)] = l.ID
	}
	cm := DecodeColumnMap(link.ColumnMap)
	columnLabels := map[string]bool{}
	if link.Mode == "labels" {
		for _, n := range cm {
			columnLabels[strings.ToLower(n)] = true
		}
	}
	seen := map[int64]bool{}
	ids := []int64{}
	add := func(name string) {
		if id, ok := byName[strings.ToLower(name)]; ok && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, tag := range decodeTags(t.Tags) {
		if !columnLabels[strings.ToLower(tag)] {
			add(tag)
		}
	}
	if link.Mode == "labels" {
		add(cm[t.ColumnID])
	}
	return ids, nil
}

// CreateIssue opens an issue for a new task of a linked list (or one moved
// into it), as actor. When it fails the background sync tries again.
func (e *Engine) CreateIssue(ctx context.Context, actor string, t *models.Task) error {
	var link models.GitLink
	if e.db.Where("list_id = ?", t.ListID).First(&link).Error != nil || t.ParentID != nil || t.GitIssueID != 0 {
		return nil
	}
	defer e.lock(link.ListID)()
	return e.dirtyOn(t, e.createIssue(ctx, actor, link, t))
}

func (e *Engine) createIssue(ctx context.Context, actor string, link models.GitLink, t *models.Task) error {
	c, err := e.clientAs(actor, &link, link.Host)
	if err != nil {
		return err
	}
	in := IssueInput{Title: &t.Title}
	if t.Notes != "" {
		in.Body = &t.Notes
	}
	if t.Due != nil {
		d := DueTime(*t.Due)
		in.DueDate = &d
	}
	if ids := t.AssigneeList(); len(ids) > 0 {
		_, byUser := e.workspaceLogins(t.WorkspaceID, link.Host)
		var logins []string
		for _, uid := range ids {
			if l := byUser[uid]; l != "" {
				logins = append(logins, l)
			}
		}
		if len(logins) > 0 {
			in.Assignees = &logins
		}
	}
	if ids, err := e.labelIDs(ctx, c, link, t); err == nil && len(ids) > 0 {
		in.Labels = ids
	}
	is, err := c.CreateIssue(ctx, link.Owner, link.Repo, in)
	if err != nil {
		return err
	}
	if link.Mode == "project" {
		cm := DecodeColumnMap(link.ColumnMap)
		if pc, ok := cm[t.ColumnID]; ok && pc != "" {
			id, _ := strconv.ParseInt(pc, 10, 64)
			if err := c.MoveIssue(ctx, link.Owner, link.Repo, link.ProjectID, is.ID, id); err != nil {
				e.log.Warn("couldn't put the new issue on the board", zap.Error(err))
			}
		}
	}
	if t.CompletedAt != nil {
		closed := "closed"
		if upd, err := c.EditIssue(ctx, link.Owner, link.Repo, is.Number, IssueInput{State: &closed}); err == nil {
			is = upd
		}
	}
	synced := is.UpdatedAt
	t.GitHost, t.GitRepo, t.GitNumber, t.GitIssueID, t.GitURL, t.GitState, t.GitSyncedAt = link.Host, link.Owner+"/"+link.Repo, is.Number, is.ID, is.HTMLURL, is.GitState(), &synced
	return e.db.Model(&models.Task{}).Where("id = ?", t.ID).Updates(map[string]any{
		"git_host": t.GitHost, "git_repo": t.GitRepo, "git_number": t.GitNumber, "git_issue_id": t.GitIssueID,
		"git_url": t.GitURL, "git_state": t.GitState, "git_synced_at": &synced,
	}).Error
}

// ---- Linking a task by hand -------------------------------------------------------

var issueURL = regexp.MustCompile(`^(https?://[^/]+(?:/[^/]+)*?)/([^/]+)/([^/]+)/(issues|pulls)/(\d+)(?:/[^?#]*)?(?:[?#].*)?$`)

// ParseIssueURL splits an issue or pull request address: the server's base
// URL, owner, repository, number, and whether it's a pull request.
func ParseIssueURL(raw string) (base, owner, repo string, number int64, pull bool, err error) {
	m := issueURL.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return "", "", "", 0, false, errors.New("paste the address of an issue or pull request, like https://gitea.example.org/owner/repo/issues/12")
	}
	number, _ = strconv.ParseInt(m[5], 10, 64)
	base, err = normalize(m[1])
	return base, m[2], m[3], number, m[4] == "pulls", err
}

// LinkTask points a task at an issue or pull request (by its address), as
// the user, and takes its state.
func (e *Engine) LinkTask(ctx context.Context, userID string, t *models.Task, raw string) error {
	base, owner, repo, number, _, err := ParseIssueURL(raw)
	if err != nil {
		return err
	}
	c, err := e.ClientFor(userID, base)
	if err != nil {
		return err
	}
	is, err := c.Issue(ctx, owner, repo, number)
	if err != nil {
		if isStatus(err, http.StatusNotFound) {
			return errors.New("there's no such issue, or your account can't see it")
		}
		return err
	}
	synced := is.UpdatedAt
	t.GitHost, t.GitRepo, t.GitNumber, t.GitIssueID, t.GitURL = base, owner+"/"+repo, is.Number, is.ID, is.HTMLURL
	t.GitState, t.GitPull, t.GitSyncedAt = is.GitState(), is.PullRequest != nil, &synced
	return nil
}

// refreshLinked pulls the state of tasks linked by hand (in lists that
// aren't linked themselves): a closed issue or merged pull request completes
// its task, a reopened one reopens it.
func (e *Engine) refreshLinked(ctx context.Context) {
	var tasks []models.Task
	e.db.Where("git_issue_id <> 0 AND list_id NOT IN (?)", e.db.Model(&models.GitLink{}).Select("list_id")).
		Order("git_synced_at").Limit(200).Find(&tasks)
	touched := map[string]bool{}
	for i := range tasks {
		t := &tasks[i]
		owner, repo, ok := strings.Cut(t.GitRepo, "/")
		if !ok {
			continue
		}
		// Whoever can: the task's creator while still a member, else any
		// member with an account there.
		c, err := e.ClientFor(t.CreatedBy, t.GitHost)
		if err == nil && !e.member(t.WorkspaceID, t.CreatedBy) {
			err = errLinkerLeft
		}
		if err != nil {
			_, byUser := e.workspaceLogins(t.WorkspaceID, t.GitHost)
			for uid := range byUser {
				if c, err = e.ClientFor(uid, t.GitHost); err == nil {
					break
				}
			}
		}
		if err != nil {
			continue
		}
		if t.GitDirty {
			if e.member(t.WorkspaceID, t.CreatedBy) {
				_ = e.dirtyOn(t, e.push(ctx, t.CreatedBy, nil, t, Change{State: true}))
			}
			continue
		}
		is, err := c.Issue(ctx, owner, repo, t.GitNumber)
		if err != nil || (t.GitSyncedAt != nil && !is.UpdatedAt.After(*t.GitSyncedAt) && is.GitState() == t.GitState) {
			continue
		}
		up := map[string]any{"git_state": is.GitState(), "git_synced_at": is.UpdatedAt, "git_url": is.HTMLURL}
		done := is.State == "closed"
		if done != (t.CompletedAt != nil) {
			var list models.TaskList
			if e.db.First(&list, "id = ?", t.ListID).Error == nil {
				cols := planner.DecodeColumns(list.Columns)
				col := planner.FirstOpen(cols)
				if done {
					col = planner.FirstDone(cols)
					up["completed_at"] = time.Now()
				} else {
					up["completed_at"] = nil
				}
				if t.ParentID == nil {
					up["column_id"] = col.ID
				}
			}
		}
		e.db.Model(&models.Task{}).Where("id = ?", t.ID).Updates(up)
		touched[t.WorkspaceID] = true
	}
	if e.Changed != nil {
		for ws := range touched {
			e.Changed(ws)
		}
	}
}

// SyncAll pulls every linked list, then the tasks linked by hand.
func (e *Engine) SyncAll(ctx context.Context) {
	var links []models.GitLink
	e.db.Find(&links)
	for _, l := range links {
		if err := e.SyncLink(ctx, l); err != nil {
			e.log.Debug("git sync failed", zap.String("repo", l.Owner+"/"+l.Repo), zap.Error(err))
		}
	}
	e.refreshLinked(ctx)
}

// NotifyHub tells the members of a workspace whose tasks a sync changed, so
// their open views re-fetch.
func (e *Engine) NotifyHub(hub *events.Hub) {
	if hub == nil {
		return
	}
	e.Changed = func(wsID string) {
		b, _ := json.Marshal(map[string]string{"type": "ws", "ws": wsID, "what": "tasks"})
		var ids []string
		e.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ?", wsID).Pluck("user_id", &ids)
		for _, id := range ids {
			hub.Publish(id, string(b))
		}
	}
}

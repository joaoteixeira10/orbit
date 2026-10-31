package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/gitsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Git: each person connects their own account on a Gitea / Forgejo server
// (a personal access token); a task list can then hold a repository's
// issues, or a project board, kept in step both ways; and any task can be
// linked to one issue or pull request. See internal/gitsync.

// gitTimeout bounds a request's calls to the git server.
const gitTimeout = 20 * time.Second

func (h *Handlers) gitOn() bool {
	return h.git != nil && feature.Enabled(h.db, feature.GitIntegration)
}

func (h *Handlers) gitEnabled(c *gin.Context) bool {
	if !h.gitOn() {
		c.JSON(http.StatusNotFound, gin.H{"error": "git integration is turned off on this instance"})
		return false
	}
	return true
}

func gitCtx(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), gitTimeout)
}

// gitFail answers an error of the git server (or of a check) as 502 / 400.
func gitFail(c *gin.Context, err error) {
	var ae *gitsync.APIError
	if errors.As(err, &ae) {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

// ---- Accounts ----------------------------------------------------------------------

type gitAccountResponse struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	BaseURL   string     `json:"base_url"`
	Login     string     `json:"login"`
	LastError string     `json:"last_error,omitempty"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	Via       string     `json:"via"` // token (pasted) | oauth (signed in)
}

func toGitAccount(a models.GitAccount) gitAccountResponse {
	via := a.Via
	if via == "" {
		via = "token"
	}
	return gitAccountResponse{ID: a.ID, Kind: a.Kind, BaseURL: a.BaseURL, Login: a.Login, LastError: a.LastError, CheckedAt: a.CheckedAt, Via: via}
}

type gitAccountsResponse struct {
	Accounts []gitAccountResponse `json:"accounts"`
	Servers  []string             `json:"servers"` // suggested (GIT_SERVERS)
	SignIn   []string             `json:"sign_in"` // servers with "Sign in with …"
}

// listGitAccounts returns the caller's git accounts.
//
//	@Summary	My git accounts
//	@Tags		git
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	gitAccountsResponse
//	@Router		/me/git [get]
func (h *Handlers) listGitAccounts(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	var accs []models.GitAccount
	h.db.Where("user_id = ?", auth.UserID(c)).Order("base_url").Find(&accs)
	out := gitAccountsResponse{Accounts: []gitAccountResponse{}, Servers: h.git.Allowed(), SignIn: []string{}}
	for _, a := range h.git.OAuthApps() {
		out.SignIn = append(out.SignIn, a.BaseURL)
	}
	for _, a := range accs {
		out.Accounts = append(out.Accounts, toGitAccount(a))
	}
	c.JSON(http.StatusOK, out)
}

type gitConnectRequest struct {
	BaseURL string `json:"base_url" binding:"required,max=255"`
	Token   string `json:"token" binding:"required,max=255"`
}

// connectGit checks a personal access token against a server and saves it
// (sealed) as the caller's account there; again for the same server
// replaces the token.
//
//	@Summary	Connect a git account
//	@Tags		git
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		gitConnectRequest	true	"Server and token"
//	@Success	200		{object}	gitAccountResponse
//	@Router		/me/git [post]
func (h *Handlers) connectGit(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	var req gitConnectRequest
	if !bindJSON(c, &req) {
		return
	}
	ctx, cancel := gitCtx(c)
	defer cancel()
	acc, err := h.git.Connect(ctx, auth.UserID(c), req.BaseURL, req.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.recordCtx(c, "git.connect", "git_account", acc.ID, gin.H{"server": acc.BaseURL, "login": acc.Login})
	c.JSON(http.StatusOK, toGitAccount(acc))
}

func (h *Handlers) gitAccount(c *gin.Context) (models.GitAccount, bool) {
	var a models.GitAccount
	if h.db.First(&a, "id = ? AND user_id = ?", c.Param("id"), auth.UserID(c)).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return a, false
	}
	return a, true
}

// disconnectGit forgets one of the caller's git accounts (lists they linked
// keep syncing only while another member's account can).
//
//	@Summary	Disconnect a git account
//	@Tags		git
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Account id"
//	@Success	204
//	@Router		/me/git/{id} [delete]
func (h *Handlers) disconnectGit(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	a, ok := h.gitAccount(c)
	if !ok {
		return
	}
	h.db.Delete(&a)
	h.recordCtx(c, "git.disconnect", "git_account", a.ID, gin.H{"server": a.BaseURL})
	c.Status(http.StatusNoContent)
}

func (h *Handlers) gitClientOf(c *gin.Context) (*gitsync.Client, bool) {
	if !h.gitEnabled(c) {
		return nil, false
	}
	a, ok := h.gitAccount(c)
	if !ok {
		return nil, false
	}
	cl, err := h.git.ClientFor(a.UserID, a.BaseURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, false
	}
	return cl, true
}

// gitRepos finds repositories the account can see.
//
//	@Summary	Search repositories
//	@Tags		git
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Account id"
//	@Param		q	query	string	false	"Search"
//	@Success	200	{array}	gitsync.Repo
//	@Router		/me/git/{id}/repos [get]
func (h *Handlers) gitRepos(c *gin.Context) {
	cl, ok := h.gitClientOf(c)
	if !ok {
		return
	}
	ctx, cancel := gitCtx(c)
	defer cancel()
	repos, err := cl.SearchRepos(ctx, c.Query("q"))
	if err != nil {
		gitFail(c, err)
		return
	}
	if repos == nil {
		repos = []gitsync.Repo{}
	}
	c.JSON(http.StatusOK, repos)
}

type gitBoardsResponse struct {
	Supported bool              `json:"supported"` // the server has the projects API
	Projects  []gitsync.Project `json:"projects"`
	Labels    []gitsync.Label   `json:"labels"`
}

// gitBoards lists what a repository's issues can be laid out by: its
// labels, and its project boards when the server's API has them.
//
//	@Summary	A repository's labels and projects
//	@Tags		git
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string	true	"Account id"
//	@Param		owner	query		string	true	"Owner"
//	@Param		repo	query		string	true	"Repository"
//	@Success	200		{object}	gitBoardsResponse
//	@Router		/me/git/{id}/boards [get]
func (h *Handlers) gitBoards(c *gin.Context) {
	cl, ok := h.gitClientOf(c)
	if !ok {
		return
	}
	owner, repo := c.Query("owner"), c.Query("repo")
	if owner == "" || repo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "give the repository"})
		return
	}
	ctx, cancel := gitCtx(c)
	defer cancel()
	out := gitBoardsResponse{Supported: true, Projects: []gitsync.Project{}, Labels: []gitsync.Label{}}
	labels, err := cl.Labels(ctx, owner, repo)
	if err != nil {
		gitFail(c, err)
		return
	}
	if labels != nil {
		out.Labels = labels
	}
	projects, err := cl.Projects(ctx, owner, repo)
	switch {
	case errors.Is(err, gitsync.ErrNoProjects):
		out.Supported = false
	case err != nil:
		gitFail(c, err)
		return
	case projects != nil:
		out.Projects = projects
	}
	c.JSON(http.StatusOK, out)
}

// ---- Linked lists ------------------------------------------------------------------

type gitListRequest struct {
	AccountID string   `json:"account_id" binding:"required"`
	Owner     string   `json:"owner" binding:"required,max=100"`
	Repo      string   `json:"repo" binding:"required,max=100"`
	Mode      string   `json:"mode" binding:"required,oneof=issues labels project"`
	ProjectID int64    `json:"project_id"`     // mode project
	Labels    []string `json:"labels"`         // mode labels: one column each, in order
	Name      string   `json:"name,omitempty"` // default: the repository's name
}

// gitLinkResponse is a list's link to a repository.
type gitLinkResponse struct {
	Host      string     `json:"host"`
	Owner     string     `json:"owner"`
	Repo      string     `json:"repo"`
	Mode      string     `json:"mode"`
	URL       string     `json:"url"`
	SyncedAt  *time.Time `json:"synced_at,omitempty"`
	LastError string     `json:"last_error,omitempty"`
	LinkedBy  string     `json:"linked_by"`
	// BotLogin: the bot account members without their own account act as
	// ("" = they can only read). Write: how the caller's changes reach git —
	// own (their account), bot, or none (read only).
	BotLogin string `json:"bot_login,omitempty"`
	Write    string `json:"write"`
	// Instant: a webhook tells Orbit of changes (HookAt: its last post).
	Instant bool       `json:"instant"`
	HookAt  *time.Time `json:"hook_at,omitempty"`
}

func (h *Handlers) gitLinkOf(listID, uid string) *gitLinkResponse {
	var l models.GitLink
	if h.db.Where("list_id = ?", listID).First(&l).Error != nil {
		return nil
	}
	url := l.Host + "/" + l.Owner + "/" + l.Repo
	if l.Mode == "project" {
		url += "/projects/" + strconv.FormatInt(l.ProjectID, 10)
	} else {
		url += "/issues"
	}
	write := "own"
	if h.git != nil {
		write = h.git.WriteMode(uid, &l, l.Host)
	}
	return &gitLinkResponse{Host: l.Host, Owner: l.Owner, Repo: l.Repo, Mode: l.Mode, URL: url, SyncedAt: l.SyncedAt, LastError: l.LastError, LinkedBy: l.LinkedBy, BotLogin: l.BotLogin, Write: write,
		Instant: len(l.HookSecretSealed) > 0, HookAt: l.HookAt}
}

// createGitList makes a task list that holds a repository's issues (as the
// caller, with their account there): as open / closed, with labels as
// columns, or as a project board's columns. Its issues come in at once.
//
//	@Summary	New list from a git repository
//	@Tags		git
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		gitListRequest	true	"Repository and layout"
//	@Success	201		{object}	listResponse
//	@Router		/workspaces/{ws}/lists/git [post]
func (h *Handlers) createGitList(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req gitListRequest
	if !bindJSON(c, &req) {
		return
	}
	var acc models.GitAccount
	if h.db.First(&acc, "id = ? AND user_id = ?", req.AccountID, auth.UserID(c)).Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connect your git account first"})
		return
	}
	cl, err := h.git.ClientFor(acc.UserID, acc.BaseURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := gitCtx(c)
	defer cancel()

	// The columns, and what each stands for on git.
	var names, keys []string
	switch req.Mode {
	case "issues":
		names, keys = []string{"Open"}, []string{""}
	case "labels":
		labels, err := cl.Labels(ctx, req.Owner, req.Repo)
		if err != nil {
			gitFail(c, err)
			return
		}
		// Issues with none of the labels sit in Open.
		names, keys = []string{"Open"}, []string{""}
		have := map[string]string{}
		for _, l := range labels {
			have[strings.ToLower(l.Name)] = l.Name
		}
		for _, n := range req.Labels {
			if name, ok := have[strings.ToLower(strings.TrimSpace(n))]; ok {
				names, keys = append(names, name), append(keys, name)
			}
		}
		if len(names) == 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pick the labels that are the columns"})
			return
		}
	case "project":
		cols, err := cl.Columns(ctx, req.Owner, req.Repo, req.ProjectID)
		if err != nil {
			gitFail(c, err)
			return
		}
		for _, pc := range cols {
			names, keys = append(names, pc.Title), append(keys, strconv.FormatInt(pc.ID, 10))
		}
		if len(names) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "that project has no columns"})
			return
		}
	}
	if len(names) > 11 {
		names, keys = names[:11], keys[:11]
	}
	cols, err := planner.CleanColumns(planner.Columns(append(names, "Closed")...))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cm := gitsync.ColumnMap{}
	for i, k := range keys {
		if k != "" {
			cm[cols[i].ID] = k
		}
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = req.Repo
	}
	l := models.TaskList{WorkspaceID: ws.ID, Name: truncateRunes(name, 100), Color: ws.Color, Icon: "git-branch", Columns: planner.EncodeColumns(cols)}
	var last struct{ Max *float64 }
	h.db.Model(&models.TaskList{}).Select("MAX(rank) AS max").Where("workspace_id = ?", ws.ID).Scan(&last)
	l.Rank = planner.Rank(last.Max, nil)
	link := models.GitLink{WorkspaceID: ws.ID, Host: acc.BaseURL, Owner: req.Owner, Repo: req.Repo, Mode: req.Mode,
		ProjectID: req.ProjectID, ColumnMap: cm.Encode(), IgnoreIDs: "[]", LinkedBy: acc.UserID}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&l).Error; err != nil {
			return err
		}
		if err := planner.ListFolder(tx, l, auth.UserID(c)); err != nil {
			return err
		}
		link.ListID = l.ID
		return tx.Create(&link).Error
	}); err != nil {
		h.internalError(c, err, "could not create the list")
		return
	}
	// The first pull; a failure shows on the list (and the sync tries again).
	_ = h.git.SyncLink(ctx, link)
	h.recordCtx(c, "git.link", "list", l.ID, gin.H{"repo": req.Owner + "/" + req.Repo, "mode": req.Mode})
	h.publishWS(c, ws.ID, "lists")
	h.publishWS(c, ws.ID, "tasks")
	c.JSON(http.StatusCreated, h.toListResponse(c, l))
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// unlinkGitList stops a list syncing with its repository (its tasks stay,
// each still linked to its issue).
//
//	@Summary	Unlink a list from git
//	@Tags		git
//	@Security	BearerAuth
//	@Param		id	path	string	true	"List id"
//	@Success	204
//	@Router		/lists/{id}/git [delete]
func (h *Handlers) unlinkGitList(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	l, ok := h.listAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if link := h.git.ListLink(l.ID); link != nil && link.HookID != 0 {
		ctx, cancel := gitCtx(c)
		h.git.DisableHook(ctx, auth.UserID(c), link)
		cancel()
	}
	h.db.Where("list_id = ?", l.ID).Delete(&models.GitLink{})
	h.recordCtx(c, "git.unlink", "list", l.ID, nil)
	h.publishWS(c, l.WorkspaceID, "lists")
	c.Status(http.StatusNoContent)
}

// syncGitList pulls a linked list's issues now.
//
//	@Summary	Sync a list with git now
//	@Tags		git
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"List id"
//	@Success	200	{object}	listResponse
//	@Router		/lists/{id}/git/sync [post]
func (h *Handlers) syncGitList(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	l, ok := h.listAccess(c, models.WSViewer)
	if !ok {
		return
	}
	var link models.GitLink
	if h.db.Where("list_id = ?", l.ID).First(&link).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "this list isn't linked to git"})
		return
	}
	ctx, cancel := gitCtx(c)
	defer cancel()
	_ = h.git.SyncLink(ctx, link)
	h.publishWS(c, l.WorkspaceID, "lists")
	c.JSON(http.StatusOK, h.toListResponse(c, l))
}

// ---- A task linked by hand -------------------------------------------------------

type gitTaskRequest struct {
	URL string `json:"url" binding:"required,max=500"`
}

// linkGitTask links a task to an issue or pull request by its address (with
// the caller's account on that server). Closing the issue, or merging the
// pull request, then completes the task; completing it closes the issue.
//
//	@Summary	Link a task to an issue
//	@Tags		git
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Task id"
//	@Param		body	body		gitTaskRequest	true	"Issue address"
//	@Success	200		{object}	taskResponse
//	@Router		/tasks/{id}/git [post]
func (h *Handlers) linkGitTask(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	t, ok := h.taskAccess(c)
	if !ok {
		return
	}
	var req gitTaskRequest
	if !bindJSON(c, &req) {
		return
	}
	ctx, cancel := gitCtx(c)
	defer cancel()
	if err := h.git.LinkTask(ctx, auth.UserID(c), &t, req.URL); err != nil {
		gitFail(c, err)
		return
	}
	t.GitDirty = false
	if err := h.db.Select("GitHost", "GitRepo", "GitNumber", "GitIssueID", "GitURL", "GitState", "GitPull", "GitSyncedAt", "GitDirty").Save(&t).Error; err != nil {
		h.internalError(c, err, "could not link the task")
		return
	}
	h.publishWS(c, t.WorkspaceID, "tasks")
	c.JSON(http.StatusOK, h.countedOne(toTaskResponse(t)))
}

// unlinkGitTask forgets a task's issue (the issue itself stays as it is).
//
//	@Summary	Unlink a task from its issue
//	@Tags		git
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Task id"
//	@Success	200	{object}	taskResponse
//	@Router		/tasks/{id}/git [delete]
func (h *Handlers) unlinkGitTask(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	t, ok := h.taskAccess(c)
	if !ok {
		return
	}
	// In a linked list the issue mustn't come back as a new task.
	h.git.Ignore(t.ListID, t.GitIssueID)
	t.GitHost, t.GitRepo, t.GitNumber, t.GitIssueID, t.GitURL, t.GitState, t.GitPull, t.GitSyncedAt, t.GitDirty = "", "", 0, 0, "", "", false, nil, false
	if err := h.db.Select("GitHost", "GitRepo", "GitNumber", "GitIssueID", "GitURL", "GitState", "GitPull", "GitSyncedAt", "GitDirty").Save(&t).Error; err != nil {
		h.internalError(c, err, "could not unlink the task")
		return
	}
	h.publishWS(c, t.WorkspaceID, "tasks")
	c.JSON(http.StatusOK, h.countedOne(toTaskResponse(t)))
}

// ---- Hooks from the task handlers ----------------------------------------------

// gitChange is what of a task changed that git should hear about.
func gitChange(before, after models.Task) gitsync.Change {
	eqs := func(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	return gitsync.Change{
		Title:     before.Title != after.Title,
		Notes:     before.Notes != after.Notes,
		Due:       !eqs(before.Due, after.Due),
		State:     (before.CompletedAt == nil) != (after.CompletedAt == nil),
		Assignee:  before.Assignees != after.Assignees,
		Tags:      before.Tags != after.Tags,
		Column:    before.ColumnID != after.ColumnID || before.ListID != after.ListID,
		Milestone: before.GitMilestoneID != after.GitMilestoneID,
	}
}

// gitGuard refuses a change to a git-linked task (or a new task in a linked
// list) that would have to go to git but can't: the caller has no account
// of their own there and the list has no bot account. Nobody's change goes
// to git as someone else. It answers 403 and reports false.
func (h *Handlers) gitGuard(c *gin.Context, before *models.Task, after *models.Task) bool {
	if !h.gitOn() || after.ParentID != nil {
		return true
	}
	var link *models.GitLink
	host := after.GitHost
	needs := false
	switch {
	case before == nil: // a new task: an issue, in a linked list
		if link = h.git.ListLink(after.ListID); link != nil {
			host, needs = link.Host, true
		}
	case after.GitIssueID != 0:
		link = h.git.LinkOf(after)
		ch := gitChange(*before, *after)
		if link == nil {
			needs = ch.State // linked by hand: only open / closed goes
		} else {
			needs = ch.Any()
		}
	}
	if !needs || h.git.WriteMode(auth.UserID(c), link, host) != "none" {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": gitsync.CantWrite{Host: host}.Error(), "git_read_only": true})
	return false
}

// gitAfter tells git what happened to a task (a new issue for a new task of
// a linked list, else its changes), as the caller. A failure doesn't undo
// the change: it is answered as git_error, and the sync tries again.
func (h *Handlers) gitAfter(c *gin.Context, before *models.Task, t *models.Task) string {
	if !h.gitOn() || t.ParentID != nil {
		return ""
	}
	ctx, cancel := gitCtx(c)
	defer cancel()
	var err error
	if t.GitIssueID == 0 {
		err = h.git.CreateIssue(ctx, auth.UserID(c), t)
	} else if before != nil {
		err = h.git.Push(ctx, auth.UserID(c), t, gitChange(*before, *t))
	}
	if err != nil {
		h.log.Debug("git push failed", zap.Error(err))
		return err.Error()
	}
	return ""
}

type gitBotRequest struct {
	Token string `json:"token" binding:"max=500"` // "" removes it
}

// setGitBot sets (or with an empty token removes) the bot account members
// without their own account act as on a linked list — so their changes
// never go to git as whoever linked it. Whoever linked the list, or an
// owner of the workspace.
//
//	@Summary	The bot account of a linked list
//	@Tags		git
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"List id"
//	@Param		body	body		gitBotRequest	true	"The bot's token (empty: none)"
//	@Success	200		{object}	listResponse
//	@Router		/lists/{id}/git/bot [put]
func (h *Handlers) setGitBot(c *gin.Context) {
	if !h.gitEnabled(c) {
		return
	}
	l, ok := h.listAccess(c, models.WSEditor)
	if !ok {
		return
	}
	link := h.git.ListLink(l.ID)
	if link == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "this list isn't linked to a repository"})
		return
	}
	uid := auth.UserID(c)
	if link.LinkedBy != uid {
		var m models.WorkspaceMember
		if h.db.Where("workspace_id = ? AND user_id = ?", l.WorkspaceID, uid).First(&m).Error != nil || m.Role != models.WSOwner {
			c.JSON(http.StatusForbidden, gin.H{"error": "only whoever linked the list, or an owner of the workspace, chooses its bot account"})
			return
		}
	}
	var req gitBotRequest
	if !bindJSON(c, &req) {
		return
	}
	ctx, cancel := gitCtx(c)
	defer cancel()
	if err := h.git.SetBot(ctx, link, req.Token); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.recordCtx(c, "git.bot_set", "list", l.ID, gin.H{"bot": link.BotLogin})
	h.publishWS(c, l.WorkspaceID, "lists")
	c.JSON(http.StatusOK, h.toListResponse(c, l))
}

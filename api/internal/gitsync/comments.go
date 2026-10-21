package gitsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

// Comments on a git-linked task follow the issue's comments both ways. One
// written in Orbit goes to the issue as its author when they have an account
// on that server, else as the list's bot account, signed "**Name** (in
// Orbit):" — never as someone else; without either it stays in Orbit. The issue's comments come back with the sync: from a member with
// an account there, as that member; from anyone else, under their git login.
// GitHash is the issue comment's text as last seen or sent, so an edit on
// either side is told from an echo.

// Comment is an issue comment on the server.
type Comment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (c *Client) Comments(ctx context.Context, owner, repo string, number int64) ([]Comment, error) {
	var out []Comment
	path := fmt.Sprintf("%s/issues/%d/comments", repoPath(owner, repo), number)
	if !c.GitHub() {
		// Gitea answers them all at once (it has no paging here).
		err := c.do(ctx, "GET", path, nil, nil, &out)
		return out, err
	}
	for page := 1; page <= 20; page++ {
		var batch []Comment
		err := c.do(ctx, "GET", path, url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}}, nil, &batch)
		if err != nil {
			return nil, err
		}
		out = append(out, batch...)
		if len(batch) < 100 {
			break
		}
	}
	return out, nil
}

func (c *Client) CreateComment(ctx context.Context, owner, repo string, number int64, body string) (Comment, error) {
	var out Comment
	err := c.do(ctx, "POST", fmt.Sprintf("%s/issues/%d/comments", repoPath(owner, repo), number), nil, map[string]string{"body": body}, &out)
	return out, err
}

func (c *Client) EditComment(ctx context.Context, owner, repo string, id int64, body string) (Comment, error) {
	var out Comment
	err := c.do(ctx, "PATCH", fmt.Sprintf("%s/issues/comments/%d", repoPath(owner, repo), id), nil, map[string]string{"body": body}, &out)
	return out, err
}

func (c *Client) DeleteComment(ctx context.Context, owner, repo string, id int64) error {
	err := c.do(ctx, "DELETE", fmt.Sprintf("%s/issues/comments/%d", repoPath(owner, repo), id), nil, nil, nil)
	if isStatus(err, 404) {
		return nil
	}
	return err
}

func hashBody(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

const viaMark = " (in Orbit):**\n\n"

// signed is a comment as sent through someone else's token; unsigned strips
// that back off.
func signed(name, body string) string { return "**" + name + viaMark + body }
func unsigned(body string) string {
	if strings.HasPrefix(body, "**") {
		if i := strings.Index(body, viaMark); i > 0 && !strings.Contains(body[2:i], "\n") {
			return body[i+len(viaMark):]
		}
	}
	return body
}

// pullComments brings an issue's comments into its task: new ones added,
// edited ones changed, deleted ones removed.
func (e *Engine) pullComments(ctx context.Context, c *Client, t *models.Task, byLogin map[string]string) {
	owner, repo, ok := strings.Cut(t.GitRepo, "/")
	if !ok || t.GitNumber == 0 {
		return
	}
	remote, err := c.Comments(ctx, owner, repo, t.GitNumber)
	if err != nil {
		return
	}
	var local []models.TaskComment
	e.db.Where("task_id = ? AND git_comment_id <> 0", t.ID).Find(&local)
	have := map[int64]*models.TaskComment{}
	for i := range local {
		have[local[i].GitCommentID] = &local[i]
	}
	seen := map[int64]bool{}
	for _, rc := range remote {
		seen[rc.ID] = true
		h := hashBody(rc.Body)
		if lc := have[rc.ID]; lc != nil {
			if lc.GitHash != h {
				now := time.Now()
				e.db.Model(lc).Updates(map[string]any{"body": unsigned(rc.Body), "git_hash": h, "edited_at": &now})
			}
			continue
		}
		cm := models.TaskComment{TaskID: t.ID, WorkspaceID: t.WorkspaceID, Body: unsigned(rc.Body), GitCommentID: rc.ID, GitHash: h}
		if uid, ok := byLogin[strings.ToLower(rc.User.Login)]; ok {
			cm.AuthorID = uid
		} else {
			cm.GitAuthor = rc.User.Login
		}
		cm.CreatedAt = rc.CreatedAt
		e.db.Create(&cm)
	}
	for id, lc := range have {
		if !seen[id] {
			e.db.Delete(lc)
		}
	}
}

// PushComment sends a new or changed Orbit comment to the task's issue (a
// no-op for tasks without one). deleted removes it there.
func (e *Engine) PushComment(ctx context.Context, actor, actorName string, t *models.Task, cm *models.TaskComment, deleted bool) error {
	if t.GitIssueID == 0 || t.GitNumber == 0 {
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
	body := cm.Body
	if viaBot {
		body = signed(actorName, body)
	}
	switch {
	case deleted:
		if cm.GitCommentID == 0 {
			return nil
		}
		return c.DeleteComment(ctx, owner, repo, cm.GitCommentID)
	case cm.GitCommentID == 0:
		rc, err := c.CreateComment(ctx, owner, repo, t.GitNumber, body)
		if err != nil {
			return err
		}
		cm.GitCommentID, cm.GitHash = rc.ID, hashBody(rc.Body)
	default:
		rc, err := c.EditComment(ctx, owner, repo, cm.GitCommentID, body)
		if err != nil {
			return err
		}
		cm.GitHash = hashBody(rc.Body)
	}
	return e.db.Model(cm).Updates(map[string]any{"git_comment_id": cm.GitCommentID, "git_hash": cm.GitHash}).Error
}

package gitsync

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"go.uber.org/zap"
)

// ---- Milestones -------------------------------------------------------------

// Milestone is a repository milestone.
type Milestone struct {
	ID     int64      `json:"id"`
	Number int64      `json:"number"` // GitHub names it by this
	Title  string     `json:"title"`
	State  string     `json:"state"`
	DueOn  *time.Time `json:"due_on"`
}

// msRef is how an issue names a milestone on this server: GitHub by its
// number, Gitea by its id.
func (c *Client) msRef(m Milestone) int64 {
	if c.GitHub() {
		return m.Number
	}
	return m.ID
}

// Milestones lists a repository's open milestones.
func (c *Client) Milestones(ctx context.Context, owner, repo string) ([]Milestone, error) {
	q := url.Values{"state": {"open"}, "limit": {"50"}}
	if c.GitHub() {
		q = url.Values{"state": {"open"}, "per_page": {"100"}}
	}
	var ms []Milestone
	err := c.do(ctx, "GET", repoPath(owner, repo)+"/milestones", q, nil, &ms)
	return ms, err
}

// MilestoneChoice is a milestone as a task picks it: Ref is what the task
// keeps (GitMilestoneID).
type MilestoneChoice struct {
	Ref   int64      `json:"ref"`
	Title string     `json:"title"`
	DueOn *time.Time `json:"due_on,omitempty"`
}

// Milestones lists a linked list's repository's open milestones (read as
// whoever linked it).
func (e *Engine) Milestones(ctx context.Context, link models.GitLink) ([]MilestoneChoice, error) {
	c, err := e.linkerClient(link)
	if err != nil && link.BotLogin != "" {
		c, err = e.botClient(link)
	}
	if err != nil {
		return nil, err
	}
	ms, err := c.Milestones(ctx, link.Owner, link.Repo)
	if err != nil {
		return nil, err
	}
	out := make([]MilestoneChoice, 0, len(ms))
	for _, m := range ms {
		out = append(out, MilestoneChoice{Ref: c.msRef(m), Title: m.Title, DueOn: m.DueOn})
	}
	return out, nil
}

// ---- Webhooks: instant updates ----------------------------------------------
//
// A webhook on the repository tells Orbit an issue changed; Orbit then syncs
// the list at once (instead of within the 3-minute poll). Its posts are
// signed with HMAC-SHA256 under a secret only Orbit and the server know
// (sealed on ours); anything unsigned or signed otherwise is refused.
// Whoever turns it on makes the hook with their own account when it may
// (repository admin); otherwise they add it by hand with the address and
// secret Orbit shows once.

// CreateHook adds a webhook for issue events; it answers the hook's id.
func (c *Client) CreateHook(ctx context.Context, owner, repo, target, secret string) (int64, error) {
	var body any
	if c.GitHub() {
		body = map[string]any{
			"name": "web", "active": true, "events": []string{"issues", "issue_comment"},
			"config": map[string]string{"url": target, "content_type": "json", "secret": secret, "insecure_ssl": "0"},
		}
	} else {
		body = map[string]any{
			"type": "gitea", "active": true, "branch_filter": "*",
			"events": []string{"issues", "issue_assign", "issue_label", "issue_milestone", "issue_comment"},
			"config": map[string]string{"url": target, "content_type": "json", "secret": secret},
		}
	}
	var out struct {
		ID int64 `json:"id"`
	}
	err := c.do(ctx, "POST", repoPath(owner, repo)+"/hooks", nil, body, &out)
	return out.ID, err
}

// DeleteHook removes a webhook (gone already is fine).
func (c *Client) DeleteHook(ctx context.Context, owner, repo string, id int64) error {
	err := c.do(ctx, "DELETE", repoPath(owner, repo)+"/hooks/"+strconv.FormatInt(id, 10), nil, nil, nil)
	if isStatus(err, http.StatusNotFound) {
		return nil
	}
	return err
}

// HookSetup is what turning instant updates on did: Auto when Orbit made
// the hook itself; otherwise the secret to paste with the address (shown
// once).
type HookSetup struct {
	Auto   bool   `json:"auto"`
	URL    string `json:"url"`
	Secret string `json:"secret,omitempty"`
}

// EnableHook turns on instant updates for a link, target being Orbit's
// address for it. It replaces a hook Orbit made before.
func (e *Engine) EnableHook(ctx context.Context, actor string, link *models.GitLink, target string) (HookSetup, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return HookSetup{}, err
	}
	secret := hex.EncodeToString(raw)
	sealed, err := e.sealer.Seal([]byte(secret))
	if err != nil {
		return HookSetup{}, err
	}
	setup := HookSetup{URL: target}
	var id int64
	if c, err := e.ClientFor(actor, link.Host); err == nil {
		if link.HookID != 0 {
			_ = c.DeleteHook(ctx, link.Owner, link.Repo, link.HookID)
		}
		if id, err = c.CreateHook(ctx, link.Owner, link.Repo, target, secret); err == nil {
			setup.Auto = true
		} else {
			id = 0
		}
	}
	if !setup.Auto {
		setup.Secret = secret
	}
	link.HookSecretSealed, link.HookID, link.HookAt = sealed, id, nil
	err = e.db.Model(link).Updates(map[string]any{"hook_secret_sealed": sealed, "hook_id": id, "hook_at": nil}).Error
	return setup, err
}

// DisableHook turns instant updates off (removing the hook Orbit made, if
// someone's account may).
func (e *Engine) DisableHook(ctx context.Context, actor string, link *models.GitLink) {
	if link.HookID != 0 {
		for _, who := range []string{actor, link.LinkedBy} {
			if c, err := e.ClientFor(who, link.Host); err == nil && c.DeleteHook(ctx, link.Owner, link.Repo, link.HookID) == nil {
				break
			}
		}
	}
	link.HookSecretSealed, link.HookID, link.HookAt = nil, 0, nil
	e.db.Model(link).Updates(map[string]any{"hook_secret_sealed": nil, "hook_id": 0, "hook_at": nil})
}

// ErrHookSignature: a webhook post that isn't signed with the link's secret.
var ErrHookSignature = errors.New("the webhook's signature doesn't match")

// VerifyHook checks a webhook post's signature: GitHub's X-Hub-Signature-256
// ("sha256=<hex>"), Gitea's X-Gitea-Signature, Forgejo's
// X-Forgejo-Signature (hex), all HMAC-SHA256 of the body.
func (e *Engine) VerifyHook(link models.GitLink, body []byte, h http.Header) error {
	if len(link.HookSecretSealed) == 0 {
		return errors.New("instant updates are off for this list")
	}
	secret, err := e.sealer.Unseal(link.HookSecretSealed)
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	want := mac.Sum(nil)
	for _, name := range []string{"X-Hub-Signature-256", "X-Gitea-Signature", "X-Forgejo-Signature"} {
		got := strings.TrimPrefix(strings.TrimSpace(h.Get(name)), "sha256=")
		if got == "" {
			continue
		}
		if sig, err := hex.DecodeString(got); err == nil && hmac.Equal(sig, want) {
			return nil
		}
	}
	return ErrHookSignature
}

// Kick syncs a link soon, in the background: webhooks come in bursts (a
// label, an assignee and a comment at once), so they're gathered for a
// moment into one sync.
func (e *Engine) Kick(linkID string) {
	e.mu.Lock()
	if e.kicked[linkID] {
		e.mu.Unlock()
		return
	}
	e.kicked[linkID] = true
	e.mu.Unlock()
	go func() {
		time.Sleep(e.kickDelay)
		e.mu.Lock()
		delete(e.kicked, linkID)
		e.mu.Unlock()
		var link models.GitLink
		if e.db.First(&link, "id = ?", linkID).Error != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := e.SyncLink(ctx, link); err != nil {
			e.log.Debug("webhook sync failed", zap.String("link", linkID), zap.Error(err))
		}
	}()
}

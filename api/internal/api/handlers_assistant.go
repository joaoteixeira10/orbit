package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/assistant"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/safedial"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The built-in assistant (package assistant): the owner sets up a model
// (Owner → Assistant); each person asks it, in a panel, about their Orbit.
// Its tools are the MCP server's, run as the person asking — in Ask mode only
// the ones that read.

// How many earlier lines of a conversation go with a new message.
const assistantHistory = 20

// assistantHTTP makes the calls to the model: guarded (the owner's address
// may be on the local network, never loopback or the cloud's metadata).
// A model sends nothing until it has answered, so it gets the whole Timeout
// to start replying, not the guarded client's few seconds.
// Tests swap it for one that reaches their stand-in server.
var assistantHTTP = func() *http.Client {
	c := safedial.GuardedClient(assistant.Timeout)
	c.Transport.(*http.Transport).ResponseHeaderTimeout = assistant.Timeout
	return c
}

type assistantStatusResponse struct {
	Enabled    bool   `json:"enabled"` // there's a model for them: the instance's or their own
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Own        bool   `json:"own"`         // the model is their own
	OwnAllowed bool   `json:"own_allowed"` // they may set up their own (Settings)
	DailyLimit int    `json:"daily_limit"` // 0: none
	UsedToday  int    `json:"used_today"`
}

func today() string { return time.Now().Format("2006-01-02") }

func (h *Handlers) assistantUsed(uid string) int {
	var u models.AssistantUsage
	h.db.Where("user_id = ? AND day = ?", uid, today()).First(&u)
	return u.Count
}

// assistantStatus says whether the assistant is there, which model, and how
// much of today's limit is used.
//
//	@Summary	The assistant: on, which model, today's use
//	@Tags		assistant
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	assistantStatusResponse
//	@Router		/assistant [get]
func (h *Handlers) assistantStatus(c *gin.Context) {
	uid := auth.UserID(c)
	cfg, own, ok := h.assistantFor(uid)
	out := assistantStatusResponse{OwnAllowed: assistant.Load(h.db, h.sealer).AllowOwn, UsedToday: h.assistantUsed(uid)}
	if ok {
		out.Enabled, out.Provider, out.Model, out.Own, out.DailyLimit = true, cfg.Provider, cfg.Model, own, cfg.DailyLimit
	}
	c.JSON(http.StatusOK, out)
}

type assistantConfigResponse struct {
	Enabled    bool   `json:"enabled"`
	Provider   string `json:"provider"` // anthropic | openai | ollama
	Model      string `json:"model"`
	URL        string `json:"url"`
	HasKey     bool   `json:"has_key"` // the key itself is never sent back
	DailyLimit int    `json:"daily_limit"`
	Context    int    `json:"context"`   // Ollama: how much the model reads, in tokens
	AllowOwn   bool   `json:"allow_own"` // people may use their own model instead
}

type assistantConfigRequest struct {
	Enabled    bool   `json:"enabled"`
	Provider   string `json:"provider" binding:"omitempty,oneof=anthropic gemini openai ollama"`
	Model      string `json:"model" binding:"max=120"`
	URL        string `json:"url" binding:"max=300"`
	Key        string `json:"key" binding:"max=500"` // "" keeps the one there is
	ClearKey   bool   `json:"clear_key"`
	DailyLimit int    `json:"daily_limit" binding:"min=0,max=100000"`
	// Context: for Ollama, how much the model reads of a request (tokens;
	// 0: 16384).
	Context int `json:"context" binding:"omitempty,min=2048,max=1048576"`
	// AllowOwn lets people use their own model (Settings → Assistant).
	AllowOwn bool `json:"allow_own"`
}

// getAssistantConfig returns the instance's assistant (owner).
//
//	@Summary	Get the assistant's set-up
//	@Tags		assistant
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	assistantConfigResponse
//	@Router		/assistant/config [get]
func (h *Handlers) getAssistantConfig(c *gin.Context) {
	cfg := assistant.Load(h.db, h.sealer)
	c.JSON(http.StatusOK, assistantConfigResponse{
		Enabled: cfg.Enabled, Provider: cfg.Provider, Model: cfg.Model, URL: cfg.URL,
		HasKey: cfg.Key != "", DailyLimit: cfg.DailyLimit, Context: cfg.Context, AllowOwn: cfg.AllowOwn,
	})
}

// setAssistantConfig sets up the instance's assistant (owner): Claude with an
// API key, or an OpenAI-style server (OpenAI, Ollama…) with its address.
//
//	@Summary	Set up the assistant
//	@Tags		assistant
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		assistantConfigRequest	true	"The assistant"
//	@Success	200		{object}	assistantConfigResponse
//	@Router		/assistant/config [put]
func (h *Handlers) setAssistantConfig(c *gin.Context) {
	var req assistantConfigRequest
	if !bindJSON(c, &req) {
		return
	}
	old := assistant.Load(h.db, h.sealer)
	cfg := assistant.Config{Enabled: req.Enabled, Provider: req.Provider, Model: strings.TrimSpace(req.Model), URL: strings.TrimSpace(req.URL), Key: req.Key, DailyLimit: req.DailyLimit, Context: req.Context, AllowOwn: req.AllowOwn}
	if cfg.Provider == "" {
		cfg.Provider = assistant.Anthropic
	}
	if cfg.URL == "" && cfg.Provider == assistant.OpenAI {
		cfg.URL = "https://api.openai.com/v1"
	}
	if !assistant.HasURL(cfg.Provider) {
		cfg.URL = ""
	}
	if cfg.URL != "" {
		if u, err := url.Parse(cfg.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the address must be an http(s) address, with no query"})
			return
		}
		if cfg.Provider == assistant.Ollama {
			cfg.URL = assistant.OllamaRoot(cfg.URL)
		}
	}
	if cfg.Model == "" {
		cfg.Model = assistant.DefaultModel(cfg.Provider)
	}
	if cfg.Enabled {
		hasKey := strings.TrimSpace(req.Key) != "" || (!req.ClearKey && old.Key != "")
		switch {
		case cfg.Provider == assistant.Anthropic && !hasKey:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Claude needs an API key"})
			return
		case cfg.Provider == assistant.Gemini && !hasKey:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Gemini needs an API key (from Google AI Studio)"})
			return
		case assistant.HasURL(cfg.Provider) && cfg.URL == "":
			c.JSON(http.StatusBadRequest, gin.H{"error": "give the server's address (…/v1)"})
			return
		case cfg.Model == "":
			c.JSON(http.StatusBadRequest, gin.H{"error": "name the model"})
			return
		}
	}
	if err := assistant.Save(h.db, h.sealer, cfg, req.ClearKey); err != nil {
		h.internalError(c, err, "could not save it")
		return
	}
	h.recordCtx(c, "assistant.config", "settings", "assistant", gin.H{"enabled": cfg.Enabled, "allow_own": cfg.AllowOwn, "provider": cfg.Provider, "model": cfg.Model, "key_changed": strings.TrimSpace(req.Key) != "" || req.ClearKey})
	h.getAssistantConfig(c)
}

// ---- Conversations ----

type assistantChatSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
}

type assistantChatResponse struct {
	ID    string           `json:"id"`
	Title string           `json:"title"`
	Turns []assistant.Turn `json:"turns"`
}

func chatTurns(ch models.AssistantChat) []assistant.Turn {
	var turns []assistant.Turn
	if ch.Turns != "" {
		_ = json.Unmarshal([]byte(ch.Turns), &turns)
	}
	return turns
}

// listAssistantChats lists my conversations, the latest first.
//
//	@Summary	My conversations with the assistant
//	@Tags		assistant
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	assistantChatSummary
//	@Router		/assistant/chats [get]
func (h *Handlers) listAssistantChats(c *gin.Context) {
	var rows []models.AssistantChat
	h.db.Select("id", "title", "updated_at").Where("user_id = ?", auth.UserID(c)).Order("updated_at DESC").Limit(100).Find(&rows)
	out := make([]assistantChatSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, assistantChatSummary{ID: r.ID, Title: r.Title, UpdatedAt: r.UpdatedAt})
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) myChat(c *gin.Context) (models.AssistantChat, bool) {
	var ch models.AssistantChat
	if err := h.db.Where("id = ? AND user_id = ?", c.Param("id"), auth.UserID(c)).First(&ch).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such conversation"})
		return ch, false
	}
	return ch, true
}

// getAssistantChat returns one of my conversations.
//
//	@Summary	A conversation with the assistant
//	@Tags		assistant
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Conversation id"
//	@Success	200	{object}	assistantChatResponse
//	@Router		/assistant/chats/{id} [get]
func (h *Handlers) getAssistantChat(c *gin.Context) {
	ch, ok := h.myChat(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, assistantChatResponse{ID: ch.ID, Title: ch.Title, Turns: chatTurns(ch)})
}

// deleteAssistantChat deletes one of my conversations.
//
//	@Summary	Delete a conversation with the assistant
//	@Tags		assistant
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Conversation id"
//	@Success	204
//	@Router		/assistant/chats/{id} [delete]
func (h *Handlers) deleteAssistantChat(c *gin.Context) {
	ch, ok := h.myChat(c)
	if !ok {
		return
	}
	h.db.Delete(&ch)
	_ = uploads.Delete(h.db, "target = ? AND target_id = ? AND owner_id = ?", uploads.Chat, ch.ID, ch.UserID)
	c.Status(http.StatusNoContent)
}

// deleteAssistantChats deletes all my conversations.
//
//	@Summary	Delete all my conversations with the assistant
//	@Tags		assistant
//	@Security	BearerAuth
//	@Success	204
//	@Router		/assistant/chats [delete]
func (h *Handlers) deleteAssistantChats(c *gin.Context) {
	h.db.Where("user_id = ?", auth.UserID(c)).Delete(&models.AssistantChat{})
	_ = uploads.Delete(h.db, "target = ? AND owner_id = ?", uploads.Chat, auth.UserID(c))
	c.Status(http.StatusNoContent)
}

// ---- Asking ----

type assistantContext struct {
	Path        string `json:"path" binding:"max=500"`  // the page on screen
	Title       string `json:"title" binding:"max=300"` // its title, as shown
	WorkspaceID string `json:"workspace_id" binding:"max=36"`
	Now         string `json:"now" binding:"max=60"` // the person's local time, as they see it
	TimeZone    string `json:"time_zone" binding:"max=64"`
}

type assistantChatRequest struct {
	ChatID  string           `json:"chat_id" binding:"max=36"`
	Message string           `json:"message" binding:"required,max=8000"`
	Context assistantContext `json:"context"`
	// Mode: ask (reads only, the default) or agent (also lines up changes
	// for the person to approve).
	Mode string `json:"mode" binding:"omitempty,oneof=ask agent"`
	// Files: ids of files attached for this message (POST /assistant/files).
	Files []string `json:"files" binding:"max=10,dive,max=36"`
}

// assistantEvent is a line of the answer's stream: a step (a tool it used),
// a piece of the answer as it's written (delta; restart: what was written so
// far isn't the answer), the answer — with, in Agent
// mode, the changes it lined up — done (with the conversation's id) or an
// error.
type assistantEvent struct {
	Type    string             `json:"type"` // step | delta | restart | answer | done | error
	Tool    string             `json:"tool,omitempty"`
	Hint    string             `json:"hint,omitempty"`
	Text    string             `json:"text,omitempty"`
	Actions []assistant.Action `json:"actions,omitempty"`
	TookMs  int64              `json:"took_ms,omitempty"` // answer: how long it took
	ChatID  string             `json:"chat_id,omitempty"`
	Error   string             `json:"error,omitempty"`
}

const assistantRules = `You are the assistant built into Orbit, a planner, talking with %s (@%s) in a chat window over what they're looking at. You work with their Orbit through the tools below; they act as this person, so you see exactly what they can see.
%s
Answer in the language they write in. Keep answers short and to the point; use Markdown (short lists, bold, small tables) where it helps. When you mention a page, task list, sheet, diagram or event, link it the way Orbit writes links (below) so they can open it. Don't invent: if the tools don't show something, say so.
What's on their screen is described at the end, with its ids (and a page's, sheet's or diagram's content): "this", "here" and "this page" mean that. Use it; never ask them for an id or a title.

`

// assistantChat answers a message as a stream of events (text/event-stream):
// each tool it uses as it goes, then the answer. The conversation is kept.
//
//	@Summary	Ask the assistant
//	@Tags		assistant
//	@Accept		json
//	@Produce	text/event-stream
//	@Security	BearerAuth
//	@Param		body	body	assistantChatRequest	true	"The message, the conversation, what's on screen"
//	@Success	200
//	@Router		/assistant/chat [post]
func (h *Handlers) assistantChat(api http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req assistantChatRequest
		if !bindJSON(c, &req) {
			return
		}
		req.Message = strings.TrimSpace(req.Message)
		if req.Message == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "say something"})
			return
		}
		uid := auth.UserID(c)
		cfg, own, ok := h.assistantFor(uid)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "the assistant isn't on"})
			return
		}
		model, err := cfg.New(h.assistantClient(cfg, own))
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "the assistant isn't on"})
			return
		}
		if cfg.DailyLimit > 0 && h.assistantUsed(uid) >= cfg.DailyLimit {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": fmt.Sprintf("that's today's %d messages; the limit starts again tomorrow", cfg.DailyLimit)})
			return
		}
		var ch models.AssistantChat
		if req.ChatID != "" {
			if h.db.Where("id = ? AND user_id = ?", req.ChatID, uid).First(&ch).Error != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "no such conversation"})
				return
			}
		} else {
			ch = models.AssistantChat{UserID: uid, Title: clip(strings.Join(strings.Fields(req.Message), " "), 80)}
		}
		var user models.User
		h.db.Select("username", "display_name").First(&user, "id = ?", uid)
		h.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "day"}},
			DoUpdates: clause.Assignments(map[string]any{"count": gorm.Expr("assistant_usages.count + 1")}),
		}).Create(&models.AssistantUsage{UserID: uid, Day: today(), Count: 1})

		asked := time.Now()
		files, ok := h.attachTo(uid, ch.ID, req.Files)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no such attached file"})
			return
		}
		turns := append(chatTurns(ch), assistant.Turn{Role: "user", Text: req.Message, At: asked.UTC().Format(time.RFC3339), Mode: req.Mode, Files: files})
		history := turns
		if len(history) > assistantHistory {
			history = history[len(history)-assistantHistory:]
		}
		name := user.DisplayName
		if name == "" {
			name = user.Username
		}

		// Ask mode: the tools that read. Agent mode: those that change things
		// too, which only line the change up (propose).
		agent := req.Mode == "agent"
		call := &mcpCall{c: c, api: api, client: "assistant"}
		allowed := map[string]*mcpTool{}
		var tools []assistant.Tool
		// The tools for a module only when one of their workspaces has it on
		// (fewer for the model to read through).
		var theirs []struct {
			Modules []string `json:"modules"`
		}
		_ = call.do("GET", "/workspaces", nil, nil, &theirs)
		on := map[string]bool{}
		for _, w := range theirs {
			for _, mod := range w.Modules {
				on[mod] = true
			}
		}
		offered := make([]*mcpTool, 0, len(mcpTools)+len(attachmentTools))
		for i := range mcpTools {
			if mcpTools[i].Module == "" || on[mcpTools[i].Module] {
				offered = append(offered, &mcpTools[i])
			}
		}
		// The tools for attached files, once there are some.
		for _, t := range turns {
			if len(t.Files) > 0 {
				for i := range attachmentTools {
					offered = append(offered, &attachmentTools[i])
				}
				break
			}
		}
		for _, t := range offered {
			switch {
			case t.ReadOnly:
				tools = append(tools, assistant.Tool{Name: t.Name, Description: t.Description, Schema: t.schema()})
			case agent:
				tools = append(tools, assistant.Tool{Name: t.Name, Description: t.Description, Schema: agentSchema(t)})
			default:
				continue
			}
			allowed[t.Name] = t
		}
		var actions []assistant.Action
		// Their workspaces go along by name, so "on Escola" needs no looking
		// up (and a change can't name one they don't have).
		var mine []myWorkspace
		_ = call.do("GET", "/workspaces", nil, nil, &mine)
		modeRules := assistantAskRules
		if agent {
			modeRules = assistantAgentRules
		}

		run := func(tool string, args map[string]any) (string, bool) {
			t := allowed[tool]
			if t == nil {
				return "there's no such tool here: " + tool, true
			}
			for _, k := range t.Required {
				if v, ok := args[k]; !ok || v == nil || v == "" {
					return "missing argument: " + k, true
				}
			}
			if !t.ReadOnly {
				return propose(&actions, t, args, mine, func(kind, id string) (assistantItem, bool) { return h.lookup(c, mine, kind, id) })
			}
			out, err := t.Run(call, mcpArgs(args))
			if err != nil {
				return err.Error(), true
			}
			return toolText(out), false
		}

		events := make(chan assistantEvent, 64)
		var steps []assistant.Step
		go func() {
			defer close(events)
			ask := assistant.Request{
				System:  fmt.Sprintf(assistantRules, name, user.Username, modeRules) + mcpInstructions,
				Context: assistantSituation(req.Context, run) + h.keptOut(uid, req.Context.WorkspaceID) + theirWorkspaces(mine) + h.theirFiles(turns) + earlierChanges(history),
				History: history,
				Tools:   tools,
				Step: func(s assistant.Step) {
					steps = append(steps, s)
					events <- assistantEvent{Type: "step", Tool: s.Tool, Hint: s.Hint}
				},
				Delta: func(s string) { events <- assistantEvent{Type: "delta", Text: s} },
				Call:  func(_ context.Context, tool string, args map[string]any) (string, bool) { return run(tool, args) },
			}
			text, err := model.Answer(c.Request.Context(), ask)
			// Agent mode: an answer that says it lined changes up while none
			// came through (a model writing it rather than calling the tool)
			// is sent back once, for it to call them.
			if err == nil && agent && len(actions) == 0 && claimsChanges(text) {
				events <- assistantEvent{Type: "restart"}
				ask.History = append(append([]assistant.Turn{}, history...),
					assistant.Turn{Role: "assistant", Text: text},
					assistant.Turn{Role: "user", Text: notLinedUp})
				text, err = model.Answer(c.Request.Context(), ask)
			}
			if err != nil {
				events <- assistantEvent{Type: "error", Error: h.assistantError(err)}
				return
			}
			events <- assistantEvent{Type: "answer", Text: text, Actions: actions, TookMs: time.Since(asked).Milliseconds()}
		}()

		hdr := c.Writer.Header()
		hdr.Set("Content-Type", "text/event-stream")
		hdr.Set("Cache-Control", "no-cache")
		hdr.Set("X-Accel-Buffering", "no")
		c.Writer.WriteHeader(http.StatusOK)
		send := func(e assistantEvent) {
			b, _ := json.Marshal(e)
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", b)
			c.Writer.Flush()
		}
		beat := time.NewTicker(sseHeartbeat)
		defer beat.Stop()
		var answer string
		var took int64
		failed := false
	loop:
		for {
			select {
			case e, open := <-events:
				if !open {
					break loop
				}
				switch e.Type {
				case "answer":
					answer, took = e.Text, e.TookMs
				case "error":
					failed = true
				}
				send(e)
			case <-beat.C:
				_, _ = fmt.Fprint(c.Writer, ": ping\n\n")
				c.Writer.Flush()
			}
		}
		// The person's line is kept even when the model failed, so it can be
		// asked again; the answer only when there is one.
		if !failed {
			turns = append(turns, assistant.Turn{Role: "assistant", Text: answer, Steps: steps, At: time.Now().UTC().Format(time.RFC3339), TookMs: took, Mode: req.Mode, Actions: actions})
		}
		b, _ := json.Marshal(turns)
		ch.Turns = models.JSONText(b)
		if err := h.db.Save(&ch).Error; err != nil {
			send(assistantEvent{Type: "error", Error: "the conversation couldn't be kept"})
			return
		}
		if len(files) > 0 { // the files are the conversation's now
			h.db.Model(&models.Upload{}).Where("id IN ? AND target = ? AND owner_id = ?", req.Files, uploads.Chat, uid).Update("target_id", ch.ID)
		}
		send(assistantEvent{Type: "done", ChatID: ch.ID})
	}
}

// keptOut tells the model when the workspace on screen is one its owners
// keep the assistant out of.
func (h *Handlers) keptOut(uid, wsID string) string {
	if wsID == "" {
		return ""
	}
	var n int64
	h.db.Model(&models.Workspace{}).Joins("JOIN workspace_members ON workspace_members.workspace_id = workspaces.id").
		Where("workspaces.id = ? AND workspace_members.user_id = ? AND workspaces.assistant_off = ?", wsID, uid, true).Count(&n)
	if n == 0 {
		return ""
	}
	return "The workspace on their screen has the assistant turned off by its owners: you can't see or change anything in it. If they ask about it, say so.\n"
}

// assistantScreens: the workspace screens that show one thing, by the
// address's segment — what it is, its id's name in the tools, and the tool
// that reads it (whose content then goes along, so "sum up this page" needs
// no looking up, even for a small model).
var assistantScreens = map[string]struct{ kind, idArg, read string }{
	"p":           {"page", "page_id", "read_page"},
	"s":           {"sheet", "sheet_id", "read_sheet"},
	"d":           {"diagram", "diagram_id", "read_diagram"},
	"tasks":       {"task list", "list_id", ""},
	"board":       {"task list (as a board)", "list_id", ""},
	"f":           {"file", "file_id", ""},
	"whiteboards": {"whiteboard", "whiteboard_id", ""},
	"forms":       {"form", "form_id", ""},
}

// assistantShown caps what goes along of the thing on screen.
const assistantShown = 16000

// assistantSituation is what changes with each message: when it is, and
// what's on the person's screen — read with the tools, as them.
func assistantSituation(x assistantContext, read func(tool string, args map[string]any) (string, bool)) string {
	var b strings.Builder
	if x.Now != "" {
		fmt.Fprintf(&b, "Now, for them: %s", x.Now)
		if x.TimeZone != "" {
			fmt.Fprintf(&b, " (%s)", x.TimeZone)
		}
		b.WriteString(".\n")
	} else {
		fmt.Fprintf(&b, "Now: %s.\n", time.Now().UTC().Format("Monday 2006-01-02 15:04 UTC"))
	}
	if x.Path == "" {
		return b.String()
	}
	path, _, _ := strings.Cut(x.Path, "?")
	seg := strings.Split(strings.Trim(path, "/"), "/")
	if len(seg) >= 4 && seg[0] == "w" {
		if sc, ok := assistantScreens[seg[2]]; ok && seg[3] != "" {
			fmt.Fprintf(&b, "On their screen: the %s", sc.kind)
			if x.Title != "" {
				fmt.Fprintf(&b, " %q", x.Title)
			}
			fmt.Fprintf(&b, " (%s %s, workspace_id %s).\n", sc.idArg, seg[3], seg[1])
			if sc.read != "" {
				if text, bad := read(sc.read, map[string]any{sc.idArg: seg[3]}); !bad {
					if r := []rune(text); len(r) > assistantShown {
						text = string(r[:assistantShown]) + "\n…(cut short)"
					}
					fmt.Fprintf(&b, "Its content, as they see it:\n<%s>\n%s\n</%s>\n", seg[2], text, seg[2])
				}
			}
			return b.String()
		}
	}
	fmt.Fprintf(&b, "On their screen: %q at %s", x.Title, x.Path)
	if len(seg) >= 2 && seg[0] == "w" {
		fmt.Fprintf(&b, " (workspace_id %s)", seg[1])
	} else if x.WorkspaceID != "" {
		fmt.Fprintf(&b, " (workspace_id %s)", x.WorkspaceID)
	}
	b.WriteString(".\n")
	return b.String()
}

// assistantError says what went wrong in words for the panel (no request
// details from the model's service).
func (h *Handlers) assistantError(err error) string {
	switch {
	case errors.Is(err, assistant.ErrRefused):
		return "The model declined to answer this."
	case errors.Is(err, assistant.ErrTooLong), errors.Is(err, assistant.ErrTooMany):
		return err.Error()
	case errors.Is(err, context.Canceled):
		return "Stopped."
	case errors.Is(err, context.DeadlineExceeded):
		return "The model took too long to answer."
	}
	var ae *anthropic.Error
	if errors.As(err, &ae) {
		switch {
		case ae.StatusCode == http.StatusUnauthorized || ae.StatusCode == http.StatusForbidden:
			return "The model's service refused the API key: ask the owner to check it."
		case ae.StatusCode == http.StatusTooManyRequests:
			return "The model's service is over its limit right now; try again in a moment."
		case ae.StatusCode >= 500:
			return "The model's service is having trouble; try again in a moment."
		case ae.StatusCode == http.StatusNotFound:
			return "The model's service doesn't know that model: ask the owner to check its name."
		}
		return fmt.Sprintf("The model's service said no (HTTP %d).", ae.StatusCode)
	}
	var he *assistant.HTTPError
	if errors.As(err, &he) {
		h.log.Warn("assistant: the model's service said no", zap.Int("status", he.Status), zap.String("message", he.Msg))
		switch {
		case he.BadKey():
			return "The model's service refused the API key: ask the owner to check it."
		case he.Busy():
			return "The model's service is too busy right now; try again in a moment, or ask the owner for another model."
		case he.Status >= 500:
			return "The model's service is having trouble; try again in a moment."
		case he.Status == http.StatusNotFound:
			return "The model's service doesn't know that model: ask the owner to check its name."
		}
		return he.Error()
	}
	h.log.Debug("assistant: " + err.Error())
	msg := err.Error()
	if strings.HasPrefix(msg, "the model's server said") {
		return msg
	}
	return "The model couldn't be reached."
}

package api

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/assistant"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"
)

// Agent mode: the assistant may change things, but nothing is changed as it
// asks. Each change it calls for is lined up (an Action); once it has
// answered, the person sees them all in one card, unticks what they don't
// want and approves once. The changes are then made as them, through the
// same tools, marked "via assistant".

const assistantAskRules = `This is Ask mode: you can look things up, sum up and explain, but you cannot change anything. If asked to change something, say what you would change, and that they can switch to Agent mode (below the box they type in) for you to do it.`

const assistantAgentRules = `This is Agent mode: besides looking things up, you can change their Orbit with the tools that change things. Those changes aren't made as you call them: each is lined up, and once you've answered they see the whole list, tick what they want and approve it in one go. So:
- Look up what you need first (their workspaces are listed at the end, by name and id), then line up every change the request needs, then answer in a sentence or two saying what you've lined up for them to approve. Never say it's done, and don't ask them to confirm: the approval is how they confirm.
- Give each change a "summary" in their language that names the thing, e.g. Move "Book the room" to Friday.
- A change that makes something (a task list, a page, a task…) gives back an id like new:1; use it in later changes that need it (tasks in the new list, say). It can't be read before it's made.
- A change is lined up only by calling its tool. Writing that you've lined something up, made it or queued it does nothing: they'd see no changes to approve.
- Line up only what they asked for.
- A task, note or event can't move to another workspace. To move one, line up making it anew there (with its details) and deleting the old one, and say that's how.`

// claimed: an answer saying it lined changes up or made them ("I've lined
// up…", "I created…") — not "couldn't line it up".
var claimed = regexp.MustCompile(`(?i)\b(i('ve| have)( just)?|i'll|i will|i)\s+(lined up|queued|created|added|moved|deleted|made|changed|updated|scheduled|set up)\b|\b(has|have) been (lined up|queued)\b`)

// claimsChanges tells whether an answer says it lined changes up.
func claimsChanges(text string) bool { return claimed.MatchString(text) }

// notLinedUp is what the model is told when it only said so.
const notLinedUp = "(From Orbit, not the person:) Nothing was lined up — you wrote about the changes but didn't call the tools, so they'd see nothing to approve. Call the tools for each change now, then answer again."

// agentSummary is the argument each change carries in Agent mode: what it
// does, in words, for the card.
const agentSummary = "summary"

// agentSchema is a changing tool's schema with the summary added.
func agentSchema(t *mcpTool) map[string]any {
	s := t.schema()
	props := s["properties"].(map[string]any)
	props[agentSummary] = map[string]any{"type": "string", "description": "For the person approving it: what this change does, in a few words naming the thing (in their language)."}
	s["required"] = append(append([]string{}, t.Required...), agentSummary)
	return s
}

// myWorkspace is one of the person's workspaces, as the model is told them.
type myWorkspace struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	ArchivedAt *string `json:"archived_at"`
	Notes      string  `json:"assistant_notes"`
}

// theirWorkspaces lists the person's workspaces for the model, with what
// each one's owners ask of it there.
func theirWorkspaces(ws []myWorkspace) string {
	var b strings.Builder
	for _, w := range ws {
		if w.ArchivedAt != nil {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("Their workspaces (name: workspace_id):")
		}
		fmt.Fprintf(&b, "\n- %s: %s", w.Name, w.ID)
		if n := strings.TrimSpace(w.Notes); n != "" {
			fmt.Fprintf(&b, "\n  Its owners' instructions for you, for what's in it: %s", strings.Join(strings.Fields(n), " "))
		}
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

// assistantItem is something a change names, as Orbit has it.
type assistantItem struct{ Kind, Title, WorkspaceID string }

// idKind is what a tool's argument names (page_id a page…); "" not one
// that's checked.
func idKind(tool, arg string) string {
	switch arg {
	case "page_id":
		return "page"
	case "parent_id":
		if tool == "create_task" {
			return "task"
		}
		return "page"
	case "task_id":
		return "task"
	case "list_id":
		return "list"
	case "event_id":
		return "event"
	case "diagram_id":
		return "diagram"
	case "file_id":
		return "file"
	case "habit_id":
		return "habit"
	case "goal_id":
		return "goal"
	case "bookmark_id":
		return "bookmark"
	case "note_id":
		return "note"
	}
	return ""
}

// lookup finds what an id is among what the assistant may see in the
// person's workspaces (kind "": whatever it is).
func (h *Handlers) lookup(c *gin.Context, mine []myWorkspace, kind, id string) (assistantItem, bool) {
	in := func(ws string) bool {
		for _, w := range mine {
			if w.ID == ws {
				return true
			}
		}
		return false
	}
	if kind == "" {
		for _, k := range []string{"page", "task", "list", "event", "diagram", "goal", "bookmark", "note", "habit"} {
			if it, ok := h.lookup(c, mine, k, id); ok {
				return it, true
			}
		}
		return assistantItem{}, false
	}
	it := assistantItem{Kind: kind}
	switch kind {
	case "page":
		var p models.Page
		if h.db.First(&p, "id = ?", id).Error != nil || p.TrashedAt != nil || !in(p.WorkspaceID) || !h.pageVisible(c, p) {
			return it, false
		}
		it.Title, it.WorkspaceID = p.Title, p.WorkspaceID
	case "task":
		var t models.Task
		if h.db.Select("id", "workspace_id", "title").First(&t, "id = ?", id).Error != nil || !in(t.WorkspaceID) {
			return it, false
		}
		it.Title, it.WorkspaceID = t.Title, t.WorkspaceID
	case "list":
		var l models.TaskList
		if h.db.Select("id", "workspace_id", "name").First(&l, "id = ?", id).Error != nil || !in(l.WorkspaceID) {
			return it, false
		}
		it.Title, it.WorkspaceID = l.Name, l.WorkspaceID
	case "event":
		var e models.Event
		if h.db.Select("id", "workspace_id", "title").First(&e, "id = ?", id).Error != nil || !in(e.WorkspaceID) {
			return it, false
		}
		it.Title, it.WorkspaceID = e.Title, e.WorkspaceID
	case "diagram":
		var d models.Diagram
		if h.db.Select("id", "workspace_id", "title").First(&d, "id = ?", id).Error != nil || !in(d.WorkspaceID) {
			return it, false
		}
		it.Title, it.WorkspaceID = d.Title, d.WorkspaceID
	case "habit": // their own
		var hb models.Habit
		if h.db.Select("id", "workspace_id", "title").First(&hb, "id = ? AND owner_id = ?", id, auth.UserID(c)).Error != nil || !in(hb.WorkspaceID) {
			return it, false
		}
		it.Title, it.WorkspaceID = hb.Title, hb.WorkspaceID
	case "goal":
		var g models.Goal
		if h.db.Select("id", "workspace_id", "title").First(&g, "id = ?", id).Error != nil || !in(g.WorkspaceID) {
			return it, false
		}
		it.Title, it.WorkspaceID = g.Title, g.WorkspaceID
	case "bookmark":
		var b models.Bookmark
		if h.db.Select("id", "workspace_id", "title", "url").First(&b, "id = ?", id).Error != nil || !in(b.WorkspaceID) {
			return it, false
		}
		it.Title, it.WorkspaceID = cmp.Or(b.Title, b.URL), b.WorkspaceID
	case "note": // a workspace's, or their own Quick Note
		var n models.Note
		if h.db.Select("id", "workspace_id", "owner_id", "title", "plain_text").First(&n, "id = ?", id).Error != nil ||
			(n.WorkspaceID == "" && n.OwnerID != auth.UserID(c)) || (n.WorkspaceID != "" && !in(n.WorkspaceID)) {
			return it, false
		}
		it.Title, it.WorkspaceID = cmp.Or(n.Title, clip(strings.TrimSpace(n.PlainText), 60)), n.WorkspaceID
	case "file":
		var u models.Upload
		if h.db.Select("id", "name").First(&u, "id = ? AND target = ? AND owner_id = ?", id, uploads.Chat, auth.UserID(c)).Error != nil {
			return it, false
		}
		it.Title = u.Name
	default:
		return it, false
	}
	return it, true
}

// propose lines a change up and tells the model so — or, when it names a
// workspace they don't have, or something that isn't what the tool takes
// (a task's id as a page's), says so at once, for the model to put right.
// The card shows what the change is on, as Orbit has it, beside the model's
// summary of it.
func propose(actions *[]assistant.Action, t *mcpTool, args map[string]any, mine []myWorkspace, look func(kind, id string) (assistantItem, bool)) (string, bool) {
	wsName := func(id string) string {
		for _, w := range mine {
			if w.ID == id {
				return w.Name
			}
		}
		return ""
	}
	wsArg, _ := args["workspace_id"].(string)
	if wsArg != "" && !newRef.MatchString(wsArg) && wsName(wsArg) == "" {
		return "not lined up: there's no workspace with id " + wsArg + ". " + strings.TrimSpace(theirWorkspaces(mine)), true
	}
	var target, where string
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		kind := idKind(t.Name, k)
		id, _ := args[k].(string)
		if kind == "" || id == "" || newRef.MatchString(id) {
			continue
		}
		it, ok := look(kind, id)
		if !ok {
			msg := fmt.Sprintf("not lined up: %s %s isn't a %s they can see", k, id, kind)
			if other, found := look("", id); found {
				msg += fmt.Sprintf(" — it's the %s %q; a %s is changed with the tools for a %s", other.Kind, other.Title, other.Kind, other.Kind)
			}
			return msg + ".", true
		}
		if wsArg != "" && !newRef.MatchString(wsArg) && it.WorkspaceID != "" && it.WorkspaceID != wsArg {
			return fmt.Sprintf("not lined up: the %s %q is in the workspace %s, not in workspace_id %s (%s).", kind, it.Title, wsName(it.WorkspaceID), wsArg, wsName(wsArg)), true
		}
		if target == "" && k != "parent_id" && k != "list_id" {
			target, where = it.Title, it.WorkspaceID
		}
	}
	if where == "" && !newRef.MatchString(wsArg) {
		where = wsArg
	}
	n := len(*actions) + 1
	summary, _ := args[agentSummary].(string)
	rest := make(map[string]any, len(args))
	for k, v := range args {
		if k != agentSummary {
			rest[k] = v
		}
	}
	*actions = append(*actions, assistant.Action{
		N: n, Tool: t.Name, Title: t.Title, Summary: strings.TrimSpace(summary), Target: target, Where: wsName(where),
		Destructive: t.Destructive, Args: rest, State: assistant.Proposed,
	})
	// Short, and not worded as an instruction: small models copy it out.
	return fmt.Sprintf(`{"status":"lined up","change":%d,"id":"new:%d","waits_for":"their approval"}`, n, n), false
}

// earlierChanges tells the model what became of the changes lined up
// earlier in the conversation (made, not approved…), apart from its answers
// — told within them, a model takes it for a way to answer and copies it.
func earlierChanges(turns []assistant.Turn) string {
	var b strings.Builder
	for i, t := range turns {
		if len(t.Actions) == 0 {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("For you, not to repeat to them — the changes you lined up earlier in this conversation, and what became of them:")
		}
		fmt.Fprintf(&b, "\nIn your answer %d:", i/2+1)
		for _, a := range t.Actions {
			fmt.Fprintf(&b, "\n- %s — %s: ", a.Title, a.Summary)
			switch a.State {
			case assistant.Done:
				b.WriteString("made")
				if a.MadeID != "" {
					fmt.Fprintf(&b, " (id %s)", a.MadeID)
				}
			case assistant.Failed:
				b.WriteString("failed: " + a.Result)
			case assistant.Skipped:
				b.WriteString("not made (" + a.Result + ")")
			default:
				b.WriteString("still waiting for their approval")
			}
		}
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

var newRef = regexp.MustCompile(`new:(\d+)`)

// fillRefs puts what earlier changes made in place of their new:N; it says
// which change a value waits on when that one wasn't made.
func fillRefs(v any, made map[int]string) (any, int) {
	switch x := v.(type) {
	case string:
		missing := 0
		s := newRef.ReplaceAllStringFunc(x, func(m string) string {
			n, _ := strconv.Atoi(m[4:])
			if id, ok := made[n]; ok {
				return id
			}
			missing = n
			return m
		})
		return s, missing
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			var miss int
			if out[i], miss = fillRefs(e, made); miss != 0 {
				return nil, miss
			}
		}
		return out, 0
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			var miss int
			if out[k], miss = fillRefs(e, made); miss != 0 {
				return nil, miss
			}
		}
		return out, 0
	}
	return v, 0
}

type assistantApplyRequest struct {
	Turn    int   `json:"turn" binding:"min=0"`     // the answer's place in the conversation
	Approve []int `json:"approve" binding:"max=50"` // the changes ticked (N); the rest aren't made
}

// applyAssistantPlan makes the changes an answer lined up that the person
// ticked, in order and as them; the others are left unmade. Once decided,
// a list can't be approved again.
//
//	@Summary	Approve the assistant's changes
//	@Tags		assistant
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string					true	"Conversation id"
//	@Param		body	body		assistantApplyRequest	true	"Which answer, and the changes ticked"
//	@Success	200		{object}	assistant.Turn
//	@Router		/assistant/chats/{id}/apply [post]
func (h *Handlers) applyAssistantPlan(api http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req assistantApplyRequest
		if !bindJSON(c, &req) {
			return
		}
		ch, ok := h.myChat(c)
		if !ok {
			return
		}
		turns := chatTurns(ch)
		if req.Turn >= len(turns) || len(turns[req.Turn].Actions) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "that answer has no changes to approve"})
			return
		}
		turn := &turns[req.Turn]
		for _, a := range turn.Actions {
			if a.State != assistant.Proposed {
				c.JSON(http.StatusConflict, gin.H{"error": "these changes were already decided"})
				return
			}
		}
		ticked := map[int]bool{}
		for _, n := range req.Approve {
			ticked[n] = true
		}

		call := &mcpCall{c: c, api: api, client: "assistant"}
		made := map[int]string{}
		var log []gin.H
		for i := range turn.Actions {
			a := &turn.Actions[i]
			t := toolByName(a.Tool)
			switch {
			case !ticked[a.N]:
				a.State, a.Result = assistant.Skipped, "not approved"
				continue
			case t == nil || t.ReadOnly:
				a.State, a.Result = assistant.Failed, "there's no such change"
				continue
			}
			args, miss := fillRefs(a.Args, made)
			if miss != 0 {
				a.State, a.Result = assistant.Skipped, fmt.Sprintf("needs change %d, which wasn't made", miss)
				continue
			}
			out, err := t.Run(call, mcpArgs(args.(map[string]any)))
			if err != nil {
				a.State, a.Result = assistant.Failed, err.Error()
				log = append(log, gin.H{"tool": a.Tool, "failed": true})
				continue
			}
			a.State = assistant.Done
			var got struct {
				ID string `json:"id"`
			}
			if json.Unmarshal([]byte(toolText(out)), &got) == nil && got.ID != "" {
				a.MadeID = got.ID
				made[a.N] = got.ID
			} else {
				made[a.N] = ""
			}
			log = append(log, gin.H{"tool": a.Tool, "id": a.MadeID})
		}

		b, _ := json.Marshal(turns)
		ch.Turns = models.JSONText(b)
		if err := h.db.Save(&ch).Error; err != nil {
			h.internalError(c, err, "could not keep the outcome")
			return
		}
		if len(log) > 0 {
			h.recordCtx(c, "assistant.apply", "assistant_chat", ch.ID, gin.H{"changes": log})
		}
		c.JSON(http.StatusOK, turn)
	}
}

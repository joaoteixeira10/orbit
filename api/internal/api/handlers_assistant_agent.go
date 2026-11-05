package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/assistant"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
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

// propose lines a change up and tells the model so — or, when it names a
// workspace they don't have, says so at once, for the model to put right.
func propose(actions *[]assistant.Action, t *mcpTool, args map[string]any, mine []myWorkspace) (string, bool) {
	if id, ok := args["workspace_id"].(string); ok && !newRef.MatchString(id) {
		known := false
		for _, w := range mine {
			known = known || w.ID == id
		}
		if !known {
			return "not lined up: there's no workspace with id " + id + ". " + strings.TrimSpace(theirWorkspaces(mine)), true
		}
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
		N: n, Tool: t.Name, Title: t.Title, Summary: strings.TrimSpace(summary),
		Destructive: t.Destructive, Args: rest, State: assistant.Proposed,
	})
	return fmt.Sprintf(`{"status":"lined up","change":%d,"id":"new:%d","next":"Nothing more to do for this one: they approve all the changes together after your answer. Line up any other changes the request needs, then tell them in a sentence what you've lined up."}`, n, n), false
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
			t := mcpToolByName(a.Tool)
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

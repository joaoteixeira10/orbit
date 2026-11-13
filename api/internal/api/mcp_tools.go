package api

import (
	"cmp"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/planner"
)

// The MCP tools: each a call (or two) to the REST API, its answer trimmed to
// what an assistant needs.

type mcpProp struct {
	Type        string // string | integer | number | boolean | object | array
	Items       string // an array's: string (the default) or object
	Description string
	Enum        []string
}

type mcpTool struct {
	Name        string
	Title       string
	Description string
	Props       map[string]mcpProp
	Required    []string
	ReadOnly    bool
	Destructive bool
	// Module: the workspace module it's for ("" any); the built-in assistant
	// is offered it only when one of the person's workspaces has it on.
	Module string
	Run    func(m *mcpCall, a mcpArgs) (any, error)
}

func (t mcpTool) schema() map[string]any {
	props := map[string]any{}
	for k, p := range t.Props {
		s := map[string]any{"type": p.Type, "description": p.Description}
		if p.Type == "array" {
			s["items"] = map[string]any{"type": cmp.Or(p.Items, "string")}
		}
		if len(p.Enum) > 0 {
			s["enum"] = p.Enum
		}
		if p.Type == "string|null" {
			s["type"] = []string{"string", "null"}
		}
		props[k] = s
	}
	out := map[string]any{"type": "object", "properties": props}
	if len(t.Required) > 0 {
		out["required"] = t.Required
	}
	return out
}

func mcpToolByName(name string) *mcpTool {
	for i := range mcpTools {
		if mcpTools[i].Name == name {
			return &mcpTools[i]
		}
	}
	return nil
}

var (
	pWS   = mcpProp{Type: "string", Description: "The workspace's id (from list_workspaces)"}
	pPage = mcpProp{Type: "string", Description: "The page's id"}
	pTask = mcpProp{Type: "string", Description: "The task's id"}
	pDate = func(what string) mcpProp {
		return mcpProp{Type: "string", Description: what + " (YYYY-MM-DD; empty clears it)"}
	}
	pInstant = func(what string) mcpProp {
		return mcpProp{Type: "string", Description: what + " (RFC 3339, e.g. 2026-05-04T09:30:00+01:00)"}
	}
	pPriority = mcpProp{Type: "integer", Description: "0 none, 1 low, 2 medium, 3 high"}
)

// oneOccurrence is the query naming one occurrence of a repeating event.
func oneOccurrence(a mcpArgs) url.Values {
	o := a.str("occurrence")
	if o == "" {
		return nil
	}
	return url.Values{"occurrence": {o}, "scope": {cmp.Or(a.str("scope"), "this")}}
}

// items wraps a list so it can be structured content.
func items(list any) map[string]any { return map[string]any{"items": list} }

var mcpTools = []mcpTool{
	// ---- Orbit's own guides ----
	{
		Name: "orbit_help", Title: "Orbit's guides", ReadOnly: true,
		Description: "How Orbit works and how to set it up, from its own guides (user guide, configuration, reverse proxies, SSO with Authentik or Entra, troubleshooting, security, API, what's new): the sections that answer a question, with where they are. Use it before answering how to do something in Orbit.",
		Props:       map[string]mcpProp{"query": {Type: "string", Description: "The question or its key words, e.g. \"SSO Authentik\" or \"move files to S3\""}},
		Required:    []string{"query"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var hits []map[string]any
			if err := m.do("GET", "/docs", url.Values{"q": {a.str("query")}}, nil, &hits); err != nil {
				return nil, err
			}
			if len(hits) > 4 {
				hits = hits[:4]
			}
			for _, h := range hits {
				if t, _ := h["text"].(string); len(t) > 6000 {
					h["text"] = t[:6000] + "\n…"
				}
				slug, _ := h["slug"].(string)
				anchor, _ := h["anchor"].(string)
				h["link"] = "/docs/" + slug + "#" + anchor
			}
			return items(hits), nil
		},
	},
	// ---- Overview ----
	{
		Name: "list_workspaces", Title: "List workspaces", ReadOnly: true,
		Description: "The workspaces you belong to, with the modules each has on and your role.",
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var ws []map[string]any
			if err := m.do("GET", "/workspaces", nil, nil, &ws); err != nil {
				return nil, err
			}
			return items(keep(ws, "id", "name", "kind", "role", "modules", "archived_at")), nil
		},
	},
	{
		Name: "get_today", Title: "Today", ReadOnly: true,
		Description: "Your day across workspaces: overdue tasks, today's and upcoming tasks, the week's events.",
		Props:       map[string]mcpProp{"time_zone": {Type: "string", Description: "Your time zone (IANA, e.g. Europe/Lisbon); UTC if not given"}},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			loc, err := time.LoadLocation(a.str("time_zone"))
			if err != nil {
				loc = time.UTC
			}
			now := time.Now().In(loc)
			from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
			q := url.Values{"today": {from.Format("2006-01-02")}, "from": {from.Format(time.RFC3339)}, "to": {from.AddDate(0, 0, 8).Format(time.RFC3339)}}
			var home map[string]any
			if err := m.do("GET", "/home", q, nil, &home); err != nil {
				return nil, err
			}
			tasks := func(k string) []map[string]any {
				list, _ := home[k].([]any)
				out := make([]map[string]any, 0, len(list))
				for _, x := range list {
					if t, ok := x.(map[string]any); ok {
						out = append(out, t)
					}
				}
				return keep(out, "id", "title", "workspace_id", "list_id", "due", "due_time", "priority")
			}
			events, _ := home["events"].([]any)
			return map[string]any{"overdue": tasks("overdue"), "today": tasks("today"), "upcoming": tasks("upcoming"), "events": events}, nil
		},
	},
	{
		Name: "search", Title: "Search", ReadOnly: true,
		Description: "Find pages, tasks, notes, events, sheets, diagrams and more by their words.",
		Props:       map[string]mcpProp{"query": {Type: "string", Description: "Words to find (2 characters or more)"}, "workspace_id": {Type: "string", Description: "Only this workspace"}},
		Required:    []string{"query"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			q := url.Values{"q": {a.str("query")}}
			if w := a.str("workspace_id"); w != "" {
				q.Set("ws", w)
			}
			var hits []map[string]any
			if err := m.do("GET", "/search", q, nil, &hits); err != nil {
				return nil, err
			}
			return items(keep(hits, "kind", "id", "workspace_id", "title", "snippet", "list_id", "due", "done")), nil
		},
	},
	{
		Name: "list_members", Title: "List members", ReadOnly: true,
		Description: "A workspace's people (their user ids are what tasks are assigned to).",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var ms []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/members", nil, nil, &ms); err != nil {
				return nil, err
			}
			return items(keep(ms, "user_id", "username", "display_name", "role")), nil
		},
	},

	// ---- Pages ----
	{
		Name: "list_pages", Title: "List pages", ReadOnly: true,
		Description: "A workspace's pages: their tree (parent_id) and folders, without their text.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var ps []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/pages", nil, nil, &ps); err != nil {
				return nil, err
			}
			return items(keep(ps, "id", "title", "icon", "parent_id", "folder_id", "updated_at")), nil
		},
	},
	{
		Name: "read_page", Title: "Read a page", ReadOnly: true,
		Description: "A page's title and its text as Markdown.",
		Props:       map[string]mcpProp{"page_id": pPage}, Required: []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var p map[string]any
			if err := m.do("GET", "/pages/"+a.str("page_id"), nil, nil, &p); err != nil {
				return nil, err
			}
			title, _ := p["title"].(string)
			content, _ := p["content"].(string)
			if title == "" {
				title = "Untitled"
			}
			if sealed, _ := p["sealed"].(bool); sealed {
				content = "(Locked with its author's PIN: its text can't be read here, or changed.)"
			}
			return "# " + title + "\n\n" + content, nil
		},
	},
	{
		Name: "create_page", Title: "Create a page",
		Description: "A new page, with its text in Markdown (the title isn't repeated in it); inside another page (parent_id) or filed in a folder (folder_id) if given; with tags if given.",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "title": {Type: "string", Description: "Its title"}, "content": {Type: "string", Description: "Its text, in Markdown"},
			"parent_id": {Type: "string", Description: "The page it goes inside"}, "folder_id": {Type: "string", Description: "A folder to file it in"}, "icon": {Type: "string", Description: "An emoji"},
			"tags": {Type: "array", Description: "Its tags"},
		},
		Required: []string{"workspace_id", "title"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("title", "content", "parent_id", "folder_id", "icon")
			text, tags, fields := pageText(a.str("title"), a.str("content"))
			if a.has("content") {
				body["content"] = text
			}
			var p map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/pages", nil, body, &p); err != nil {
				return nil, err
			}
			if err := tagPage(m, p, append(a.strs("tags"), tags...), fields); err != nil {
				return nil, err
			}
			return keepOne(p, "id", "title", "workspace_id", "parent_id", "folder_id", "version"), nil
		},
	},
	{
		Name: "update_page", Title: "Change a page",
		Description: "Change a page's title, replace its text (content), add text at its end (append), or set its tags. Its history keeps what it was.",
		Props: map[string]mcpProp{
			"page_id": pPage, "title": {Type: "string", Description: "A new title"}, "content": {Type: "string", Description: "Its whole new text, in Markdown"},
			"append": {Type: "string", Description: "Markdown to add at the end"}, "icon": {Type: "string", Description: "An emoji (empty removes it)"},
			"tags": {Type: "array", Description: "Its tags (the whole list)"},
		},
		Required: []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			id := a.str("page_id")
			var cur map[string]any
			if err := m.do("GET", "/pages/"+id, nil, nil, &cur); err != nil {
				return nil, err
			}
			body := a.pick("title", "icon", "tags")
			if a.has("content") || a.has("append") {
				text, _ := cur["content"].(string)
				if a.has("content") {
					title, _ := cur["title"].(string)
					if a.has("title") {
						title = a.str("title")
					}
					var tags []string
					var fields []planner.Field
					text, tags, fields = pageText(title, a.str("content"))
					if len(tags) > 0 && !a.has("tags") {
						body["tags"] = tags
					}
					if len(fields) > 0 {
						body["fields"] = withFields(cur["fields"], fields)
					}
				}
				if add, _ := a["append"].(string); add != "" {
					text = strings.TrimRight(text, "\n") + "\n\n" + add
				}
				body["content"] = strings.TrimLeft(text, "\n")
				body["version"] = cur["version"]
			}
			if len(body) == 0 {
				return nil, errors.New("nothing to change: give title, content, append, icon or tags")
			}
			var p map[string]any
			if err := m.do("PUT", "/pages/"+id, nil, body, &p); err != nil {
				return nil, err
			}
			return keepOne(p, "id", "title", "version", "updated_at"), nil
		},
	},
	{
		Name: "trash_page", Title: "Move a page to the trash", Destructive: true,
		Description: "Moves a page (and the pages inside it) to the trash, where it can be restored for a while.",
		Props:       map[string]mcpProp{"page_id": pPage}, Required: []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/pages/"+a.str("page_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "In the trash.", nil
		},
	},
	{
		Name: "move_page", Title: "Move a page",
		Description: "Puts a page inside another page, or at the top (parent_id empty).",
		Props:       map[string]mcpProp{"page_id": pPage, "parent_id": {Type: "string", Description: "The page it goes inside; empty: the top"}},
		Required:    []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var parent any
			if p := a.str("parent_id"); p != "" {
				parent = p
			}
			var p map[string]any
			if err := m.do("POST", "/pages/"+a.str("page_id")+"/move", nil, map[string]any{"parent_id": parent, "after_id": ""}, &p); err != nil {
				return nil, err
			}
			return keepOne(p, "id", "title", "parent_id"), nil
		},
	},
	{
		Name: "page_links", Title: "What links to a page", ReadOnly: true,
		Description: "The pages and tasks that link to a page.",
		Props:       map[string]mcpProp{"page_id": pPage}, Required: []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var b map[string]any
			if err := m.do("GET", "/pages/"+a.str("page_id")+"/backlinks", nil, nil, &b); err != nil {
				return nil, err
			}
			return b, nil
		},
	},
	{
		Name: "page_history", Title: "A page's history", ReadOnly: true,
		Description: "A page's saved versions, newest first, with who saved each (read_page_version reads one).",
		Props:       map[string]mcpProp{"page_id": pPage}, Required: []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var vs []map[string]any
			if err := m.do("GET", "/pages/"+a.str("page_id")+"/versions", nil, nil, &vs); err != nil {
				return nil, err
			}
			return items(keep(vs, "id", "version", "title", "user_id", "updated_at", "restored", "chars")), nil
		},
	},
	{
		Name: "read_page_version", Title: "Read an old version", ReadOnly: true,
		Description: "One version of a page (from page_history), as Markdown.",
		Props:       map[string]mcpProp{"version_id": {Type: "string", Description: "The version's id"}}, Required: []string{"version_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var v map[string]any
			if err := m.do("GET", "/page-versions/"+a.str("version_id"), nil, nil, &v); err != nil {
				return nil, err
			}
			title, _ := v["title"].(string)
			content, _ := v["content"].(string)
			return "# " + title + "\n\n" + content, nil
		},
	},

	// ---- Tasks ----
	{
		Name: "list_task_lists", Title: "List task lists", ReadOnly: true,
		Description: "A workspace's task lists, with how many tasks are open and done, and the folder each is the project of.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var ls []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/lists", nil, nil, &ls); err != nil {
				return nil, err
			}
			return items(keep(ls, "id", "name", "open_count", "done_count", "folder_id", "columns")), nil
		},
	},
	{
		Name: "create_task_list", Title: "Create a task list",
		Description: "A new task list in a workspace.",
		Props:       map[string]mcpProp{"workspace_id": pWS, "name": {Type: "string", Description: "Its name"}, "color": {Type: "string", Description: "A #rrggbb colour"}},
		Required:    []string{"workspace_id", "name"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var l map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/lists", nil, a.pick("name", "color"), &l); err != nil {
				return nil, err
			}
			return keepOne(l, "id", "name"), nil
		},
	},
	{
		Name: "list_tasks", Title: "List tasks", ReadOnly: true,
		Description: "A workspace's tasks (or one list's): open ones by default.",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "list_id": {Type: "string", Description: "Only this list"},
			"state": {Type: "string", Description: "Which: active (default), done or all", Enum: []string{"active", "done", "all"}},
		},
		Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			q := url.Values{}
			if l := a.str("list_id"); l != "" {
				q.Set("list", l)
			}
			if s := a.str("state"); s != "" {
				q.Set("state", s)
			}
			var ts []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/tasks", q, nil, &ts); err != nil {
				return nil, err
			}
			for _, t := range ts {
				if n, ok := t["notes"].(string); ok {
					t["notes"] = clip(n, 300)
				}
			}
			return items(keep(ts, "id", "title", "list_id", "parent_id", "due", "due_time", "start", "priority", "estimate", "tags", "assignee_ids", "done", "notes", "comment_count")), nil
		},
	},
	{
		Name: "create_task", Title: "Create a task",
		Description: "A new task. Without list_id it goes in the workspace's first list (make one with create_task_list if there's none).",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "title": {Type: "string", Description: "What to do"}, "list_id": {Type: "string", Description: "Its list"},
			"due": pDate("When it's due"), "due_time": {Type: "string", Description: "Its time (HH:MM)"}, "start": pDate("When work starts"),
			"notes": {Type: "string", Description: "Notes, in Markdown"}, "priority": pPriority,
			"estimate":     {Type: "integer", Description: "Minutes it takes"},
			"assignee_ids": {Type: "array", Description: "Who does it (user ids, from list_members)"},
			"parent_id":    {Type: "string", Description: "The task it's a sub-task of"},
			"tags":         {Type: "array", Description: "Tags"},
			"goal_id":      {Type: "string", Description: "A goal it counts towards (from list_goals)"},
		},
		Required: []string{"workspace_id", "title"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var t map[string]any
			body := a.pick("title", "list_id", "due", "due_time", "start", "notes", "priority", "estimate", "assignee_ids", "parent_id", "tags", "goal_id")
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/tasks", nil, body, &t); err != nil {
				return nil, err
			}
			return keepOne(t, "id", "title", "list_id", "due", "due_time", "priority"), nil
		},
	},
	{
		Name: "update_task", Title: "Change a task",
		Description: "Change a task's title, dates, notes, priority, people, list, tags or goal; done: true completes it (false reopens).",
		Props: map[string]mcpProp{
			"task_id": pTask, "title": {Type: "string", Description: "What to do"}, "list_id": {Type: "string", Description: "Move it to this list"},
			"due": pDate("When it's due"), "due_time": {Type: "string", Description: "Its time (HH:MM; empty clears)"}, "start": pDate("When work starts"),
			"notes": {Type: "string", Description: "Its notes (the whole text), in Markdown"}, "priority": pPriority,
			"estimate":     {Type: "integer", Description: "Minutes it takes (0 clears)"},
			"assignee_ids": {Type: "array", Description: "Who does it (the whole list of user ids)"},
			"tags":         {Type: "array", Description: "Its tags (the whole list)"},
			"done":         {Type: "boolean", Description: "Done (true) or open again (false)"},
			"goal_id":      {Type: "string", Description: "A goal it counts towards (from list_goals; empty: none)"},
		},
		Required: []string{"task_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("title", "list_id", "due", "due_time", "start", "notes", "priority", "estimate", "assignee_ids", "tags", "goal_id")
			if a.has("done") {
				body["completed"] = a["done"]
			}
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var t map[string]any
			if err := m.do("PUT", "/tasks/"+a.str("task_id"), nil, body, &t); err != nil {
				return nil, err
			}
			return keepOne(unwrapTask(t), "id", "title", "list_id", "due", "due_time", "priority", "done"), nil
		},
	},
	{
		Name: "complete_task", Title: "Complete a task",
		Description: "Marks a task done (a repeating one moves to its next date).",
		Props:       map[string]mcpProp{"task_id": pTask}, Required: []string{"task_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var t map[string]any
			if err := m.do("PUT", "/tasks/"+a.str("task_id"), nil, map[string]any{"completed": true}, &t); err != nil {
				return nil, err
			}
			return keepOne(unwrapTask(t), "id", "title", "done", "due"), nil
		},
	},
	{
		Name: "delete_task", Title: "Delete a task", Destructive: true,
		Description: "Deletes a task for good (with its sub-tasks and comments).",
		Props:       map[string]mcpProp{"task_id": pTask}, Required: []string{"task_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/tasks/"+a.str("task_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},
	{
		Name: "list_task_comments", Title: "A task's comments", ReadOnly: true,
		Description: "The comments on a task, oldest first.",
		Props:       map[string]mcpProp{"task_id": pTask}, Required: []string{"task_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var cs []map[string]any
			if err := m.do("GET", "/tasks/"+a.str("task_id")+"/comments", nil, nil, &cs); err != nil {
				return nil, err
			}
			return items(keep(cs, "id", "author_id", "author_name", "body", "created_at")), nil
		},
	},
	{
		Name: "comment_on_task", Title: "Comment on a task",
		Description: "Adds a comment to a task (@username tells that member).",
		Props:       map[string]mcpProp{"task_id": pTask, "body": {Type: "string", Description: "The comment, in Markdown"}},
		Required:    []string{"task_id", "body"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var cm map[string]any
			if err := m.do("POST", "/tasks/"+a.str("task_id")+"/comments", nil, a.pick("body"), &cm); err != nil {
				return nil, err
			}
			return keepOne(cm, "id", "created_at"), nil
		},
	},

	// ---- Calendar ----
	{
		Name: "list_events", Title: "List events", ReadOnly: true,
		Description: "Calendar events between two instants (at most 120 days apart): one workspace's, or all of yours. Repeating ones come as their occurrences.",
		Props:       map[string]mcpProp{"from": pInstant("From"), "to": pInstant("Until"), "workspace_id": {Type: "string", Description: "Only this workspace"}},
		Required:    []string{"from", "to"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			q := url.Values{"from": {a.str("from")}, "to": {a.str("to")}}
			path := "/calendar"
			if w := a.str("workspace_id"); w != "" {
				path = "/workspaces/" + w + "/calendar"
			}
			var out map[string]any
			if err := m.do("GET", path, q, nil, &out); err != nil {
				return nil, err
			}
			return map[string]any{"events": out["events"]}, nil
		},
	},
	{
		Name: "create_event", Title: "Create an event",
		Description: "A new calendar event (all day: starts_at and ends_at at midnight UTC, the end exclusive).",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "title": {Type: "string", Description: "Its title"}, "starts_at": pInstant("When it starts"), "ends_at": pInstant("When it ends"),
			"all_day": {Type: "boolean", Description: "All day"}, "location": {Type: "string", Description: "Where"}, "notes": {Type: "string", Description: "Notes"},
			"rrule": {Type: "string", Description: "How it repeats (an iCalendar RRULE, e.g. FREQ=WEEKLY;BYDAY=MO)"}, "tz": {Type: "string", Description: "Its time zone (IANA, e.g. Europe/Lisbon)"},
			"remind_minutes": {Type: "integer", Description: "A reminder this many minutes before"},
		},
		Required: []string{"workspace_id", "title", "starts_at", "ends_at"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var e map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/events", nil, a.pick("title", "starts_at", "ends_at", "all_day", "location", "notes", "rrule", "tz", "remind_minutes"), &e); err != nil {
				return nil, err
			}
			return keepOne(e, "id", "title", "starts_at", "ends_at", "all_day", "rrule"), nil
		},
	},
	{
		Name: "update_event", Title: "Change an event",
		Description: "Change an event. A repeating one: the whole series, or with occurrence (that day's start, from list_events) just that one (scope this) or it and the ones after (scope following).",
		Props: map[string]mcpProp{
			"occurrence": pInstant("One occurrence's start"), "scope": {Type: "string", Description: "With occurrence: this (default) or following", Enum: []string{"this", "following"}},
			"event_id": {Type: "string", Description: "The event's id"}, "title": {Type: "string", Description: "Its title"},
			"starts_at": pInstant("When it starts"), "ends_at": pInstant("When it ends"), "all_day": {Type: "boolean", Description: "All day"},
			"location": {Type: "string", Description: "Where"}, "notes": {Type: "string", Description: "Notes"}, "rrule": {Type: "string", Description: "How it repeats (empty: it doesn't)"},
		},
		Required: []string{"event_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var e map[string]any
			if err := m.do("PUT", "/events/"+a.str("event_id"), oneOccurrence(a), a.pick("title", "starts_at", "ends_at", "all_day", "location", "notes", "rrule"), &e); err != nil {
				return nil, err
			}
			return keepOne(e, "id", "title", "starts_at", "ends_at", "all_day", "rrule"), nil
		},
	},
	{
		Name: "delete_event", Title: "Delete an event", Destructive: true,
		Description: "Deletes an event. A repeating one: the whole series, or with occurrence (that day's start, from list_events) just that one (scope this) or it and the ones after (scope following).",
		Props: map[string]mcpProp{
			"event_id": {Type: "string", Description: "The event's id"}, "occurrence": pInstant("One occurrence's start"),
			"scope": {Type: "string", Description: "With occurrence: this (default) or following", Enum: []string{"this", "following"}},
		},
		Required: []string{"event_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/events/"+a.str("event_id"), oneOccurrence(a), nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},

	// ---- Notes ----
	{
		Name: "list_notes", Title: "List notes", ReadOnly: true,
		Description: "A workspace's notes, or your own Quick Notes (no workspace_id).",
		Props:       map[string]mcpProp{"workspace_id": {Type: "string", Description: "The workspace; none: your Quick Notes"}},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			path := "/me/notes"
			if w := a.str("workspace_id"); w != "" {
				path = "/workspaces/" + w + "/notes"
			}
			var ns []map[string]any
			if err := m.do("GET", path, nil, nil, &ns); err != nil {
				return nil, err
			}
			return items(keep(ns, "id", "title", "body", "items", "checklist", "pinned", "sealed", "updated_at")), nil
		},
	},
	{
		Name: "create_note", Title: "Create a note",
		Description: "A new note in a workspace, or in your Quick Notes (no workspace_id).",
		Props:       map[string]mcpProp{"workspace_id": {Type: "string", Description: "The workspace; none: your Quick Notes"}, "title": {Type: "string", Description: "Its title"}, "body": {Type: "string", Description: "Its text"}},
		Required:    []string{"body"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			path := "/me/notes"
			if w := a.str("workspace_id"); w != "" {
				path = "/workspaces/" + w + "/notes"
			}
			var n map[string]any
			if err := m.do("POST", path, nil, a.pick("title", "body"), &n); err != nil {
				return nil, err
			}
			return keepOne(n, "id", "title"), nil
		},
	},

	// ---- Files: sheets, diagrams, whiteboards, folders ----
	{
		Name: "list_files", Title: "List files", ReadOnly: true,
		Description: "A workspace's folders (with the task list a folder is the project of), sheets, diagrams, whiteboards and files (uploaded ones, and those attached to pages, tasks and notes; read_file reads one).",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			ws := "/workspaces/" + a.str("workspace_id")
			out := map[string]any{}
			for _, k := range []struct{ key, path string }{{"folders", "/folders"}, {"sheets", "/sheets"}, {"diagrams", "/diagrams"}, {"whiteboards", "/drawings"}, {"files", "/files"}} {
				var xs []map[string]any
				if err := m.do("GET", ws+k.path, nil, nil, &xs); err != nil {
					continue // a module that's off
				}
				out[k.key] = keep(xs, "id", "name", "title", "kind", "type", "size", "folder_id", "parent_id", "list_id", "on_kind", "on_title", "updated_at")
			}
			return out, nil
		},
	},
	{
		Name: "read_sheet", Title: "Read a sheet", ReadOnly: true,
		Description: "A sheet's cells, tab by tab: what was typed in each (formulas as =…).",
		Props:       map[string]mcpProp{"sheet_id": {Type: "string", Description: "The sheet's id"}}, Required: []string{"sheet_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var s struct {
				Title string `json:"title"`
				Data  struct {
					Tabs []struct {
						Name  string                    `json:"name"`
						Cells map[string]map[string]any `json:"cells"`
					} `json:"tabs"`
				} `json:"data"`
			}
			if err := m.do("GET", "/sheets/"+a.str("sheet_id"), nil, nil, &s); err != nil {
				return nil, err
			}
			var b strings.Builder
			b.WriteString("# " + s.Title + "\n")
			for _, tab := range s.Data.Tabs {
				b.WriteString("\n## " + tab.Name + "\n")
				keys := make([]string, 0, len(tab.Cells))
				for k, c := range tab.Cells {
					if v, ok := c["v"]; ok && v != "" {
						keys = append(keys, k)
					}
				}
				sort.Slice(keys, func(i, j int) bool { return cellLess(keys[i], keys[j]) })
				for _, k := range keys {
					b.WriteString(k + ": " + strings.ReplaceAll(toString(tab.Cells[k]["v"]), "\n", " ") + "\n")
				}
			}
			return b.String(), nil
		},
	},
	{
		Name: "read_diagram", Title: "Read a diagram", ReadOnly: true,
		Description: "A diagram: a Mermaid one's text, or a draw.io one's labels.",
		Props:       map[string]mcpProp{"diagram_id": {Type: "string", Description: "The diagram's id"}}, Required: []string{"diagram_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var d map[string]any
			if err := m.do("GET", "/diagrams/"+a.str("diagram_id"), nil, nil, &d); err != nil {
				return nil, err
			}
			if d["kind"] == "mermaid" {
				return map[string]any{"title": d["title"], "kind": "mermaid", "mermaid": d["data"]}, nil
			}
			return map[string]any{"title": d["title"], "kind": "drawio", "drawio_xml": clip(toString(d["data"]), 20000)}, nil
		},
	},
	{
		Name: "create_mermaid_diagram", Title: "Create a Mermaid diagram",
		Description: "A new diagram written in Mermaid (flowchart, sequence, class, state, ER, Gantt, timeline, pie, mind map); its picture is drawn when it's first opened. Put [Title](/w/{workspace_id}/d/{id}) alone on a line in a page to show it there.",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "title": {Type: "string", Description: "Its title"}, "mermaid": {Type: "string", Description: "The Mermaid text"},
			"folder_id": {Type: "string", Description: "A folder to put it in"},
		},
		Required: []string{"workspace_id", "title", "mermaid"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := map[string]any{"kind": "mermaid", "title": a["title"], "data": a["mermaid"], "text": a["mermaid"], "folder_id": a.str("folder_id")}
			var d map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/diagrams", nil, body, &d); err != nil {
				return nil, err
			}
			return keepOne(d, "id", "title", "kind"), nil
		},
	},
	{
		Name: "update_mermaid_diagram", Title: "Change a Mermaid diagram",
		Description: "Replaces a Mermaid diagram's text, or renames it (title; any diagram, draw.io too).",
		Props: map[string]mcpProp{
			"diagram_id": {Type: "string", Description: "The diagram's id"}, "mermaid": {Type: "string", Description: "Its new Mermaid text"}, "title": {Type: "string", Description: "A new title"},
		},
		Required: []string{"diagram_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var cur map[string]any
			if err := m.do("GET", "/diagrams/"+a.str("diagram_id"), nil, nil, &cur); err != nil {
				return nil, err
			}
			if cur["kind"] != "mermaid" && a.has("mermaid") {
				return nil, errors.New("it's a draw.io diagram: its drawing is changed in Orbit (its title can be, here)")
			}
			body := a.pick("title")
			if a.has("mermaid") {
				body["data"], body["text"], body["preview"] = a["mermaid"], a["mermaid"], "" // drawn again when opened
			}
			var d map[string]any
			if err := m.do("PUT", "/diagrams/"+a.str("diagram_id"), nil, body, &d); err != nil {
				return nil, err
			}
			return keepOne(d, "id", "title", "version"), nil
		},
	},
	{
		Name: "workspace_graph", Title: "How a workspace connects", ReadOnly: true,
		Description: "A workspace's things (pages, sheets, diagrams, whiteboards, lists, folders, people) and the links between them.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var g map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/graph", nil, nil, &g); err != nil {
				return nil, err
			}
			return g, nil
		},
	},
}

// unwrapTask: a task's answer, or the task inside one ({"task": …} when
// completing moved a repeating one on).
func unwrapTask(x map[string]any) map[string]any {
	if t, ok := x["task"].(map[string]any); ok {
		return t
	}
	return x
}

func keepOne(x map[string]any, keys ...string) map[string]any {
	return keep([]map[string]any{x}, keys...)[0]
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// cellLess orders cell addresses as read: row by row, then column (A1, B1, A2).
func cellLess(a, b string) bool {
	ca, ra := splitAddr(a)
	cb, rb := splitAddr(b)
	if ra != rb {
		return ra < rb
	}
	if len(ca) != len(cb) {
		return len(ca) < len(cb)
	}
	return ca < cb
}

func splitAddr(s string) (string, int) {
	i := 0
	for i < len(s) && s[i] >= 'A' && s[i] <= 'Z' {
		i++
	}
	n := 0
	for _, r := range s[i:] {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return s[:i], n
}

// firstHeading is a "# heading" on the first line of Markdown text.
var firstHeading = regexp.MustCompile(`^\s*#[ \t]+(.+?)[ \t]*#*[ \t]*(?:\n|$)`)

// pageText takes off the top of a page's text what belongs elsewhere — a
// YAML header (its tags and properties) and a first "# heading" repeating
// its title — as a model writes it (it sees pages so in read_page), or a
// Markdown file has it.
func pageText(title, text string) (rest string, tags []string, fields []planner.Field) {
	text = strings.TrimLeft(strings.ReplaceAll(text, "\r\n", "\n"), "\ufeff\n")
	tags, fields, rest = planner.ReadFrontmatter(text)
	if h := firstHeading.FindStringSubmatch(rest); h != nil && strings.EqualFold(strings.TrimSpace(h[1]), strings.TrimSpace(title)) {
		rest = rest[len(h[0]):]
	}
	return strings.TrimLeft(rest, "\n"), tags, fields
}

// tagPage gives a page just made its tags and properties, if it has any.
func tagPage(m *mcpCall, p map[string]any, tags []string, fields []planner.Field) error {
	body := map[string]any{}
	if len(tags) > 0 {
		body["tags"] = tags
	}
	if len(fields) > 0 {
		body["fields"] = fields
	}
	if len(body) == 0 {
		return nil
	}
	id, _ := p["id"].(string)
	return m.do("PUT", "/pages/"+id, nil, body, nil)
}

// withFields is a page's properties with these set (by name).
func withFields(cur any, set []planner.Field) []planner.Field {
	var out []planner.Field
	if b, err := json.Marshal(cur); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	for _, f := range set {
		i := slices.IndexFunc(out, func(g planner.Field) bool { return strings.EqualFold(g.Key, f.Key) })
		if i < 0 {
			out = append(out, f)
		} else {
			out[i] = f
		}
	}
	return out
}

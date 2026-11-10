package api

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The MCP tools for the rest of what people work with in Orbit: workspaces
// and their members, a page's trash, copies and favourites, task lists,
// boards, comments and smart lists, calendar subscriptions, notes' moves and
// checklists, files and folders, writing sheets, whiteboards, courses and
// grades, the inbox. Not here, on purpose: the instance's and an account's
// settings (users, roles, sign-in, tokens, backups) — an assistant doesn't
// change those.

func init() { mcpTools = append(mcpTools, moreTools...) }

var (
	pList    = mcpProp{Type: "string", Description: "The task list's id (from list_task_lists)"}
	pFolder  = mcpProp{Type: "string", Description: "The folder's id (from list_files)"}
	pSheet   = mcpProp{Type: "string", Description: "The sheet's id (from list_files)"}
	pBoard   = mcpProp{Type: "string", Description: "The whiteboard's id (from list_files)"}
	pFile    = mcpProp{Type: "string", Description: "The file's id (from list_files)"}
	pCourse  = mcpProp{Type: "string", Description: "The course's id (from list_courses)"}
	pGrade   = mcpProp{Type: "string", Description: "The grade's id (from list_grades)"}
	pComment = mcpProp{Type: "string", Description: "The comment's id (from list_task_comments)"}
	pSmart   = mcpProp{Type: "string", Description: "The smart list's id (from list_smart_lists)"}
	pColor   = mcpProp{Type: "string", Description: "A colour, #rrggbb"}
	pModules = mcpProp{Type: "array", Description: "Its modules (the whole list), of: home, pages, tasks, board, calendar, notes, timeline, charts, whiteboards, habits, focus, goals, journal, bookmarks, sheets, diagrams, files, courses, grades"}
)

// mcpObject is a JSON object given as an argument (or as its JSON text).
func (a mcpArgs) object(k string) map[string]any {
	switch v := a[k].(type) {
	case map[string]any:
		return v
	case string:
		var out map[string]any
		if json.Unmarshal([]byte(v), &out) == nil {
			return out
		}
	}
	return nil
}

var moreTools = []mcpTool{
	// ---- Workspaces ----
	{
		Name: "create_workspace", Title: "Create a workspace",
		Description: "A new workspace, yours: school, work, personal, shared or custom (its modules from that kind, unless given). Starter lists and pages come with it unless starter is false.",
		Props: map[string]mcpProp{
			"name": {Type: "string", Description: "Its name"}, "kind": {Type: "string", Description: "What it's for", Enum: []string{"school", "work", "personal", "shared", "custom"}},
			"icon": {Type: "string", Description: "An emoji"}, "color": pColor, "description": {Type: "string", Description: "What it's about"},
			"modules": pModules, "starter": {Type: "boolean", Description: "With the kind's starter lists and pages (default true)"},
		},
		Required: []string{"name"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("name", "kind", "icon", "color", "description", "starter")
			if a.has("modules") {
				body["modules"] = a.strs("modules")
			}
			var w map[string]any
			if err := m.do("POST", "/workspaces", nil, body, &w); err != nil {
				return nil, err
			}
			return keepOne(w, "id", "name", "kind", "modules"), nil
		},
	},
	{
		Name: "update_workspace", Title: "Change a workspace",
		Description: "Change a workspace's name, icon, colour, description or modules (its owners); archived: true puts it away (false brings it back); holidays: a country code to show its public holidays (empty: none).",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "name": {Type: "string", Description: "Its name"}, "icon": {Type: "string", Description: "An emoji"}, "color": pColor,
			"description": {Type: "string", Description: "What it's about"}, "modules": pModules,
			"archived": {Type: "boolean", Description: "Archived (true) or back (false)"}, "holidays": {Type: "string", Description: "A country code (PT, GB, US…), empty for none"},
		},
		Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("name", "icon", "color", "description", "archived", "holidays")
			if a.has("modules") {
				body["modules"] = a.strs("modules")
			}
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var w map[string]any
			if err := m.do("PUT", "/workspaces/"+a.str("workspace_id"), nil, body, &w); err != nil {
				return nil, err
			}
			return keepOne(w, "id", "name", "modules", "archived_at"), nil
		},
	},
	{
		Name: "add_member", Title: "Add someone to a workspace",
		Description: "Adds a person to a workspace (its owners), by their exact username or email: owner, editor or viewer.",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "user": {Type: "string", Description: "Their username or email, exactly"},
			"role": {Type: "string", Description: "What they may do", Enum: []string{"owner", "editor", "viewer"}},
		},
		Required: []string{"workspace_id", "user", "role"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var ms []map[string]any // the members, now
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/members", nil, a.pick("user", "role"), &ms); err != nil {
				return nil, err
			}
			return items(keep(ms, "user_id", "username", "display_name", "role")), nil
		},
	},
	{
		Name: "set_member_role", Title: "Change someone's role",
		Description: "Makes a member of a workspace an owner, editor or viewer (its owners).",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "user_id": {Type: "string", Description: "Their user id (from list_members)"},
			"role": {Type: "string", Description: "What they may do", Enum: []string{"owner", "editor", "viewer"}},
		},
		Required: []string{"workspace_id", "user_id", "role"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("PUT", "/workspaces/"+a.str("workspace_id")+"/members/"+a.str("user_id"), nil, a.pick("role"), nil); err != nil {
				return nil, err
			}
			return "Done.", nil
		},
	},
	{
		Name: "remove_member", Title: "Remove someone from a workspace", Destructive: true,
		Description: "Takes a member out of a workspace (its owners), or — with your own user id — leaves it.",
		Props:       map[string]mcpProp{"workspace_id": pWS, "user_id": {Type: "string", Description: "Their user id (from list_members)"}},
		Required:    []string{"workspace_id", "user_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/workspaces/"+a.str("workspace_id")+"/members/"+a.str("user_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Removed.", nil
		},
	},
	{
		Name: "workspace_activity", Title: "What happened in a workspace", ReadOnly: true,
		Description: "A workspace's recent activity, newest first: who made, changed, finished or commented on what. before: older than this instant (RFC 3339), for the page after.",
		Props:       map[string]mcpProp{"workspace_id": pWS, "before": pInstant("Older than")},
		Required:    []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			q := url.Values{}
			if b := a.str("before"); b != "" {
				q.Set("before", b)
			}
			var out any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/activity", q, nil, &out); err != nil {
				return nil, err
			}
			return out, nil
		},
	},
	{
		Name: "list_tags", Title: "List tags", ReadOnly: true,
		Description: "The tags used in a workspace's pages and tasks.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var out any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/tags", nil, nil, &out); err != nil {
				return nil, err
			}
			return map[string]any{"tags": out}, nil
		},
	},

	// ---- Pages: the trash, copies, favourites, old versions ----
	{
		Name: "list_trash", Title: "The pages in the trash", ReadOnly: true,
		Description: "A workspace's trashed pages, which restore_page brings back.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var ps []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/trash", nil, nil, &ps); err != nil {
				return nil, err
			}
			return items(keep(ps, "id", "title", "parent_id", "trashed_at")), nil
		},
	},
	{
		Name: "restore_page", Title: "Take a page out of the trash",
		Description: "Brings a trashed page back (with the pages that were inside it).",
		Props:       map[string]mcpProp{"page_id": pPage}, Required: []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var p map[string]any
			if err := m.do("POST", "/pages/"+a.str("page_id")+"/restore", nil, map[string]any{}, &p); err != nil {
				return nil, err
			}
			return keepOne(p, "id", "title", "parent_id"), nil
		},
	},
	{
		Name: "delete_page_forever", Title: "Delete a trashed page for good", Destructive: true,
		Description: "Deletes a page that's already in the trash, for good (it can't be brought back).",
		Props:       map[string]mcpProp{"page_id": pPage}, Required: []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/pages/"+a.str("page_id"), url.Values{"permanent": {"1"}}, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted for good.", nil
		},
	},
	{
		Name: "duplicate_page", Title: "Copy a page",
		Description: "Copies a page (not the pages inside it), right after itself.",
		Props:       map[string]mcpProp{"page_id": pPage}, Required: []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var p map[string]any
			if err := m.do("POST", "/pages/"+a.str("page_id")+"/duplicate", nil, map[string]any{}, &p); err != nil {
				return nil, err
			}
			return keepOne(p, "id", "title", "parent_id"), nil
		},
	},
	{
		Name: "favorite_page", Title: "Favourite a page",
		Description: "Puts a page in your favourites, or takes it out (on: false).",
		Props:       map[string]mcpProp{"page_id": pPage, "on": {Type: "boolean", Description: "In (default) or out"}},
		Required:    []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			method := "POST"
			if v, ok := a["on"].(bool); ok && !v {
				method = "DELETE"
			}
			if err := m.do(method, "/pages/"+a.str("page_id")+"/favorite", nil, nil, nil); err != nil {
				return nil, err
			}
			return "Done.", nil
		},
	},
	{
		Name: "list_favorites", Title: "Your favourite pages", ReadOnly: true,
		Description: "The pages you've favourited, across workspaces.",
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var ps []map[string]any
			if err := m.do("GET", "/me/favorites", nil, nil, &ps); err != nil {
				return nil, err
			}
			return items(keep(ps, "id", "title", "workspace_id")), nil
		},
	},
	{
		Name: "restore_page_version", Title: "Put an old version back",
		Description: "Makes an earlier version of a page (from page_history) its text again; the one it replaces stays in the history.",
		Props:       map[string]mcpProp{"version_id": {Type: "string", Description: "The version's id (from page_history)"}},
		Required:    []string{"version_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var p map[string]any
			if err := m.do("POST", "/page-versions/"+a.str("version_id")+"/restore", nil, map[string]any{}, &p); err != nil {
				return nil, err
			}
			return keepOne(p, "id", "title", "version"), nil
		},
	},
	{
		Name: "list_forms", Title: "A database's forms", ReadOnly: true,
		Description: "The forms that add rows to a database page: each one's questions, and whether it's open.",
		Props:       map[string]mcpProp{"page_id": pPage}, Required: []string{"page_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var fs []map[string]any
			if err := m.do("GET", "/pages/"+a.str("page_id")+"/forms", nil, nil, &fs); err != nil {
				return nil, err
			}
			return items(keep(fs, "id", "title", "intro", "fields", "open", "public")), nil
		},
	},
	{
		Name: "answer_form", Title: "Answer a form",
		Description: "Answers a database's form (a row in the database): values by question key (from list_forms).",
		Props: map[string]mcpProp{
			"form_id": {Type: "string", Description: "The form's id (from list_forms)"},
			"values":  {Type: "object", Description: "The answers, by question key: {\"name\": \"Ana\", \"score\": 4}"},
		},
		Required: []string{"form_id", "values"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			vals := a.object("values")
			if vals == nil {
				return nil, errors.New("values must be an object of answers")
			}
			if err := m.do("POST", "/forms/"+a.str("form_id")+"/answers", nil, map[string]any{"values": vals}, nil); err != nil {
				return nil, err
			}
			return "Answered.", nil
		},
	},

	// ---- Task lists, boards, comments, smart lists ----
	{
		Name: "update_task_list", Title: "Change a task list",
		Description: "Rename a task list, change its colour or icon, archive it (archived: true) or bring it back.",
		Props: map[string]mcpProp{
			"list_id": pList, "name": {Type: "string", Description: "Its name"}, "color": pColor, "icon": {Type: "string", Description: "An emoji"},
			"archived": {Type: "boolean", Description: "Archived (true) or back (false)"},
		},
		Required: []string{"list_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("name", "color", "icon", "archived")
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var l map[string]any
			if err := m.do("PUT", "/lists/"+a.str("list_id"), nil, body, &l); err != nil {
				return nil, err
			}
			return keepOne(l, "id", "name", "color", "archived_at"), nil
		},
	},
	{
		Name: "delete_task_list", Title: "Delete a task list", Destructive: true,
		Description: "Deletes a task list with all its tasks.",
		Props:       map[string]mcpProp{"list_id": pList}, Required: []string{"list_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/lists/"+a.str("list_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},
	{
		Name: "set_task_status", Title: "Move a task on the board",
		Description: "Puts a task in a column of its list's board (its status: To do, Doing, Done… — the list's columns are in list_task_lists), at the end or after another task; list_id moves it to another list's column.",
		Props: map[string]mcpProp{
			"task_id": pTask, "column_id": {Type: "string", Description: "The column's id (from list_task_lists' columns)"},
			"list_id": {Type: "string", Description: "Another list it goes to"}, "after_id": {Type: "string", Description: "The task it goes after (\"^\" first; empty: last)"},
		},
		Required: []string{"task_id", "column_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var t map[string]any
			if err := m.do("POST", "/tasks/"+a.str("task_id")+"/move", nil, map[string]any{"column_id": a.str("column_id"), "list_id": a.str("list_id"), "after_id": a.str("after_id")}, &t); err != nil {
				return nil, err
			}
			return keepOne(unwrapTask(t), "id", "title", "list_id", "column_id", "done"), nil
		},
	},
	{
		Name: "task_activity", Title: "A task's history", ReadOnly: true,
		Description: "What happened to a task: who made it, changed its dates, status or people, and when.",
		Props:       map[string]mcpProp{"task_id": pTask}, Required: []string{"task_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var out any
			if err := m.do("GET", "/tasks/"+a.str("task_id")+"/activity", nil, nil, &out); err != nil {
				return nil, err
			}
			return map[string]any{"activity": out}, nil
		},
	},
	{
		Name: "update_comment", Title: "Change a comment",
		Description: "Rewrites one of your comments on a task (Markdown).",
		Props:       map[string]mcpProp{"comment_id": pComment, "body": {Type: "string", Description: "Its new text"}},
		Required:    []string{"comment_id", "body"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var cm map[string]any
			if err := m.do("PUT", "/comments/"+a.str("comment_id"), nil, a.pick("body"), &cm); err != nil {
				return nil, err
			}
			return keepOne(cm, "id", "updated_at"), nil
		},
	},
	{
		Name: "delete_comment", Title: "Delete a comment", Destructive: true,
		Description: "Deletes one of your comments on a task.",
		Props:       map[string]mcpProp{"comment_id": pComment}, Required: []string{"comment_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/comments/"+a.str("comment_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},
	{
		Name: "list_smart_lists", Title: "List smart lists", ReadOnly: true,
		Description: "A workspace's smart lists: saved task filters (words, due, priority, progress, people, tags, lists). list_tasks shows the tasks.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var ls []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/smart-lists", nil, nil, &ls); err != nil {
				return nil, err
			}
			return items(keep(ls, "id", "name", "icon", "shared", "filter")), nil
		},
	},
	{
		Name: "create_smart_list", Title: "Create a smart list",
		Description: `A saved task filter in a workspace. filter: {"q": words, "due": [overdue|today|week|next|later|none], "priority": [0-3], "progress": [notStarted|inProgress|done], "assignee": [user ids, "me", "none"], "tags": [...], "lists": [list ids]}; shared: everyone in the workspace sees it.`,
		Props: map[string]mcpProp{
			"workspace_id": pWS, "name": {Type: "string", Description: "Its name"}, "icon": {Type: "string", Description: "An emoji"}, "color": pColor,
			"filter": {Type: "object", Description: "What it shows (see above)"}, "shared": {Type: "boolean", Description: "For everyone in the workspace"},
		},
		Required: []string{"workspace_id", "name", "filter"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("name", "icon", "color", "shared")
			body["filter"] = a.object("filter")
			var l map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/smart-lists", nil, body, &l); err != nil {
				return nil, err
			}
			return keepOne(l, "id", "name", "filter"), nil
		},
	},
	{
		Name: "update_smart_list", Title: "Change a smart list",
		Description: "Change a smart list's name, icon, colour, filter (the whole of it, as in create_smart_list) or whether it's shared.",
		Props: map[string]mcpProp{
			"smart_list_id": pSmart, "name": {Type: "string", Description: "Its name"}, "icon": {Type: "string", Description: "An emoji"}, "color": pColor,
			"filter": {Type: "object", Description: "What it shows"}, "shared": {Type: "boolean", Description: "For everyone in the workspace"},
		},
		Required: []string{"smart_list_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("name", "icon", "color", "shared")
			if a.has("filter") {
				body["filter"] = a.object("filter")
			}
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var l map[string]any
			if err := m.do("PUT", "/smart-lists/"+a.str("smart_list_id"), nil, body, &l); err != nil {
				return nil, err
			}
			return keepOne(l, "id", "name", "filter"), nil
		},
	},
	{
		Name: "delete_smart_list", Title: "Delete a smart list", Destructive: true,
		Description: "Deletes a smart list (the tasks it showed stay).",
		Props:       map[string]mcpProp{"smart_list_id": pSmart}, Required: []string{"smart_list_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/smart-lists/"+a.str("smart_list_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},

	// ---- Calendar subscriptions ----
	{
		Name: "list_calendar_subscriptions", Title: "Subscribed calendars", ReadOnly: true,
		Description: "The calendars a workspace follows by link (iCal), shown with its own events.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var ss []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/subscriptions", nil, nil, &ss); err != nil {
				return nil, err
			}
			return items(keep(ss, "id", "name", "url", "color", "refresh_minutes", "last_error", "refreshed_at")), nil
		},
	},
	{
		Name: "subscribe_calendar", Title: "Subscribe to a calendar",
		Description: "Follows a calendar by its link (an iCal / .ics address) in a workspace.",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "url": {Type: "string", Description: "The calendar's address (https, webcal)"}, "name": {Type: "string", Description: "Its name"}, "color": pColor,
		},
		Required: []string{"workspace_id", "url"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var s map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/subscriptions", nil, a.pick("url", "name", "color"), &s); err != nil {
				return nil, err
			}
			return keepOne(s, "id", "name", "url"), nil
		},
	},
	{
		Name: "unsubscribe_calendar", Title: "Unsubscribe from a calendar", Destructive: true,
		Description: "Stops following a subscribed calendar (its events go from the calendar).",
		Props:       map[string]mcpProp{"subscription_id": {Type: "string", Description: "The subscription's id"}}, Required: []string{"subscription_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/subscriptions/"+a.str("subscription_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Unsubscribed.", nil
		},
	},

	// ---- Notes: checklists, moves ----
	{
		Name: "set_note_checklist", Title: "Write a note's checklist",
		Description: "Makes a note a checklist with these items (the whole list, in order), each ticked or not.",
		Props: map[string]mcpProp{
			"note_id": pNote, "items": {Type: "array", Description: "The items' text, in order"}, "done": {Type: "array", Description: "Those of them already ticked (their text)"},
		},
		Required: []string{"note_id", "items"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			done := map[string]bool{}
			for _, d := range a.strs("done") {
				done[d] = true
			}
			list := []map[string]any{}
			for _, t := range a.strs("items") {
				list = append(list, map[string]any{"id": shortID(), "text": t, "done": done[t]})
			}
			var n map[string]any
			if err := m.do("PUT", "/notes/"+a.str("note_id"), nil, map[string]any{"checklist": true, "items": list}, &n); err != nil {
				return nil, err
			}
			return keepOne(n, "id", "title", "items"), nil
		},
	},
	{
		Name: "move_note", Title: "Move a note",
		Description: "Moves a note into a workspace, between workspaces, or back to your Quick Notes (no workspace_id).",
		Props:       map[string]mcpProp{"note_id": pNote, "workspace_id": {Type: "string", Description: "Where to; none: your Quick Notes"}},
		Required:    []string{"note_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var n map[string]any
			if err := m.do("POST", "/notes/"+a.str("note_id")+"/move", nil, map[string]any{"workspace_id": a.str("workspace_id")}, &n); err != nil {
				return nil, err
			}
			return keepOne(n, "id", "title", "workspace_id"), nil
		},
	},

	// ---- Files and folders ----
	{
		Name: "read_file", Title: "Read a file", ReadOnly: true,
		Description: "What a file in a workspace's Files (or attached to a page, task or note) says: a text, Markdown, CSV or code file as it is, a PDF's text, a Word or Excel file's lines.",
		Props:       map[string]mcpProp{"file_id": pFile}, Required: []string{"file_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var f map[string]any
			if err := m.do("GET", "/files/"+a.str("file_id")+"/text", nil, nil, &f); err != nil {
				return nil, err
			}
			if t, _ := f["text"].(string); t == "" {
				f["text"] = "(nothing to read in it: it may be a picture, or a PDF not opened in Orbit yet)"
			} else {
				f["text"] = clip(t, 60000)
			}
			return f, nil
		},
	},
	{
		Name: "update_file", Title: "Rename or move a file",
		Description: "Renames a file of a workspace's Files, or puts it in a folder (folder_id; empty: the top).",
		Props:       map[string]mcpProp{"file_id": pFile, "name": {Type: "string", Description: "Its new name"}, "folder_id": {Type: "string", Description: "A folder (empty: the top)"}},
		Required:    []string{"file_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("name", "folder_id")
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var f map[string]any
			if err := m.do("PUT", "/files/"+a.str("file_id"), nil, body, &f); err != nil {
				return nil, err
			}
			return keepOne(f, "id", "name", "folder_id"), nil
		},
	},
	{
		Name: "delete_file", Title: "Delete a file", Destructive: true,
		Description: "Moves a file to the trash (emptied after 30 days).",
		Props:       map[string]mcpProp{"file_id": pFile}, Required: []string{"file_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/uploads/"+a.str("file_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "In the trash.", nil
		},
	},
	{
		Name: "create_folder", Title: "Create a folder",
		Description: "A new folder in a workspace's Files, at the top or inside another (parent_id).",
		Props:       map[string]mcpProp{"workspace_id": pWS, "name": {Type: "string", Description: "Its name"}, "parent_id": {Type: "string", Description: "The folder it goes inside"}, "color": pColor},
		Required:    []string{"workspace_id", "name"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var f map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/folders", nil, a.pick("name", "parent_id", "color"), &f); err != nil {
				return nil, err
			}
			return keepOne(f, "id", "name", "parent_id"), nil
		},
	},
	{
		Name: "update_folder", Title: "Change a folder",
		Description: "Renames a folder, changes its colour, or moves it inside another (parent_id; empty: the top).",
		Props:       map[string]mcpProp{"folder_id": pFolder, "name": {Type: "string", Description: "Its name"}, "parent_id": {Type: "string", Description: "The folder it goes inside (empty: the top)"}, "color": pColor},
		Required:    []string{"folder_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("name", "parent_id", "color")
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var f map[string]any
			if err := m.do("PUT", "/folders/"+a.str("folder_id"), nil, body, &f); err != nil {
				return nil, err
			}
			return keepOne(f, "id", "name", "parent_id"), nil
		},
	},
	{
		Name: "delete_folder", Title: "Delete a folder", Destructive: true,
		Description: "Deletes a folder; what it held moves up to where the folder was.",
		Props:       map[string]mcpProp{"folder_id": pFolder}, Required: []string{"folder_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/folders/"+a.str("folder_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted; what it held moved up.", nil
		},
	},
	{
		Name: "move_to_folder", Title: "Put things in a folder",
		Description: "Puts pages, sheets, diagrams, whiteboards, files and folders of a workspace into a folder (folder_id; empty: the top).",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "folder_id": {Type: "string", Description: "Where to (empty: the top)"},
			"pages": {Type: "array", Description: "Pages' ids"}, "sheets": {Type: "array", Description: "Sheets' ids"}, "diagrams": {Type: "array", Description: "Diagrams' ids"},
			"whiteboards": {Type: "array", Description: "Whiteboards' ids"}, "files": {Type: "array", Description: "Files' ids"}, "folders": {Type: "array", Description: "Folders' ids"},
		},
		Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := map[string]any{
				"folder_id": a.str("folder_id"), "pages": a.strs("pages"), "sheets": a.strs("sheets"), "diagrams": a.strs("diagrams"),
				"drawings": a.strs("whiteboards"), "files": a.strs("files"), "folders": a.strs("folders"),
			}
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/files/move", nil, body, nil); err != nil {
				return nil, err
			}
			return "Moved.", nil
		},
	},

	// ---- Sheets ----
	{
		Name: "create_sheet", Title: "Create a sheet",
		Description: "A new sheet (spreadsheet) in a workspace, empty or with cells: cells by address ({\"A1\": \"Item\", \"B2\": \"=SUM(B3:B9)\"}) or rows as tab-separated text (tsv) from A1. Formulas are Excel's.",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "title": {Type: "string", Description: "Its title"}, "folder_id": {Type: "string", Description: "A folder to put it in"},
			"cells": {Type: "object", Description: "Cells by address: what's typed in each"}, "tsv": {Type: "string", Description: "Rows, cells split by tabs, from A1"},
		},
		Required: []string{"workspace_id", "title"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			data := sheetData{Tabs: []map[string]any{newSheetTab("Sheet 1")}}
			if err := data.write(0, a.object("cells"), a.str("tsv"), "A1"); err != nil {
				return nil, err
			}
			raw, text := data.out()
			var s map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/sheets", nil, map[string]any{"title": a.str("title"), "folder_id": a.str("folder_id"), "data": raw, "text": text}, &s); err != nil {
				return nil, err
			}
			return keepOne(s, "id", "title"), nil
		},
	},
	{
		Name: "write_sheet", Title: "Write in a sheet",
		Description: "Writes cells of a sheet: by address (cells; an empty value clears the cell) or rows of tab-separated text (tsv) from a cell (at, default A1). In a tab by name (the first if not given; a new one if there's none of that name). Read it first with read_sheet. Formulas are Excel's.",
		Props: map[string]mcpProp{
			"sheet_id": pSheet, "tab": {Type: "string", Description: "The tab's name"},
			"cells": {Type: "object", Description: "Cells by address: what's typed in each"}, "tsv": {Type: "string", Description: "Rows, cells split by tabs"},
			"at": {Type: "string", Description: "Where the tsv starts (default A1)"}, "title": {Type: "string", Description: "A new title for the sheet"},
		},
		Required: []string{"sheet_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var cur struct {
				Title   string          `json:"title"`
				Version int             `json:"version"`
				Data    json.RawMessage `json:"data"`
			}
			if err := m.do("GET", "/sheets/"+a.str("sheet_id"), nil, nil, &cur); err != nil {
				return nil, err
			}
			var data sheetData
			_ = json.Unmarshal(cur.Data, &data)
			if len(data.Tabs) == 0 {
				data.Tabs = []map[string]any{newSheetTab("Sheet 1")}
			}
			i := 0
			if name := a.str("tab"); name != "" {
				i = -1
				for j, t := range data.Tabs {
					if n, _ := t["name"].(string); strings.EqualFold(n, name) {
						i = j
					}
				}
				if i < 0 {
					data.Tabs = append(data.Tabs, newSheetTab(name))
					i = len(data.Tabs) - 1
				}
			}
			cells, tsv := a.object("cells"), a.str("tsv")
			body := a.pick("title")
			if cells != nil || tsv != "" {
				if err := data.write(i, cells, tsv, a.str("at")); err != nil {
					return nil, err
				}
				body["data"], body["text"] = data.out()
			}
			if len(body) == 0 {
				return nil, errors.New("nothing to write")
			}
			body["version"] = cur.Version
			var s map[string]any
			if err := m.do("PUT", "/sheets/"+a.str("sheet_id"), nil, body, &s); err != nil {
				return nil, err
			}
			return keepOne(s, "id", "title", "version"), nil
		},
	},
	{
		Name: "delete_sheet", Title: "Delete a sheet", Destructive: true,
		Description: "Deletes a sheet.",
		Props:       map[string]mcpProp{"sheet_id": pSheet}, Required: []string{"sheet_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/sheets/"+a.str("sheet_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},

	// ---- Diagrams ----
	{
		Name: "delete_diagram", Title: "Delete a diagram", Destructive: true,
		Description: "Deletes a diagram (draw.io or Mermaid).",
		Props:       map[string]mcpProp{"diagram_id": {Type: "string", Description: "The diagram's id"}}, Required: []string{"diagram_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/diagrams/"+a.str("diagram_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},

	// ---- Whiteboards ----
	{
		Name: "read_whiteboard", Title: "Read a whiteboard", ReadOnly: true,
		Description: "What's on a whiteboard: its words (notes, labels, text), with how many shapes, arrows, drawings and pictures it has.",
		Props:       map[string]mcpProp{"whiteboard_id": pBoard}, Required: []string{"whiteboard_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var d struct {
				Title string `json:"title"`
				Scene string `json:"scene"`
			}
			if err := m.do("GET", "/drawings/"+a.str("whiteboard_id"), nil, nil, &d); err != nil {
				return nil, err
			}
			var scene struct {
				Type     string           `json:"type"`
				Elements []map[string]any `json:"elements"`
			}
			_ = json.Unmarshal([]byte(d.Scene), &scene)
			if scene.Type == "orbit-sketch" {
				return map[string]any{"title": d.Title, "kind": "sketch", "note": "a hand-drawn sketch: no words to read"}, nil
			}
			var texts []string
			kinds := map[string]int{}
			for _, el := range scene.Elements {
				if del, _ := el["isDeleted"].(bool); del {
					continue
				}
				t, _ := el["type"].(string)
				if t == "text" {
					if s, _ := el["text"].(string); strings.TrimSpace(s) != "" {
						texts = append(texts, strings.TrimSpace(s))
					}
					continue
				}
				kinds[t]++
			}
			return map[string]any{"title": d.Title, "texts": texts, "shapes": kinds}, nil
		},
	},
	{
		Name: "create_whiteboard", Title: "Create a whiteboard",
		Description: "A new whiteboard in a workspace, empty or with sticky notes on it (notes: their text, laid out in rows).",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "title": {Type: "string", Description: "Its title"}, "folder_id": {Type: "string", Description: "A folder to put it in"},
			"notes": {Type: "array", Description: "Sticky notes' text"},
		},
		Required: []string{"workspace_id", "title"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := map[string]any{"title": a.str("title"), "folder_id": a.str("folder_id")}
			if notes := a.strs("notes"); len(notes) > 0 {
				scene, text := boardWithNotes(nil, notes)
				body["scene"], body["text"] = scene, text
			}
			var d map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/drawings", nil, body, &d); err != nil {
				return nil, err
			}
			return keepOne(d, "id", "title"), nil
		},
	},
	{
		Name: "add_to_whiteboard", Title: "Add notes to a whiteboard",
		Description: "Puts sticky notes on a whiteboard, in rows below what's there; or renames it (title).",
		Props:       map[string]mcpProp{"whiteboard_id": pBoard, "notes": {Type: "array", Description: "Sticky notes' text"}, "title": {Type: "string", Description: "A new title"}},
		Required:    []string{"whiteboard_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var d struct {
				Version int    `json:"version"`
				Scene   string `json:"scene"`
			}
			if err := m.do("GET", "/drawings/"+a.str("whiteboard_id"), nil, nil, &d); err != nil {
				return nil, err
			}
			body := a.pick("title")
			if notes := a.strs("notes"); len(notes) > 0 {
				var scene map[string]any
				_ = json.Unmarshal([]byte(d.Scene), &scene)
				if t, _ := scene["type"].(string); t == "orbit-sketch" {
					return nil, errors.New("it's a hand-drawn sketch: notes go on whiteboards")
				}
				body["scene"], body["text"] = boardWithNotes(scene, notes)
				body["preview"] = "" // drawn again when it's opened
			}
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			body["version"] = d.Version
			var out map[string]any
			if err := m.do("PUT", "/drawings/"+a.str("whiteboard_id"), nil, body, &out); err != nil {
				return nil, err
			}
			return keepOne(out, "id", "title", "version"), nil
		},
	},
	{
		Name: "delete_whiteboard", Title: "Delete a whiteboard", Destructive: true,
		Description: "Deletes a whiteboard.",
		Props:       map[string]mcpProp{"whiteboard_id": pBoard}, Required: []string{"whiteboard_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/drawings/"+a.str("whiteboard_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},

	// ---- School: courses and grades ----
	{
		Name: "list_courses", Title: "List courses", ReadOnly: true,
		Description: "A school workspace's courses: code, teacher, room, term, weekly classes (weekday 1 Monday … 7 Sunday, HH:MM), and your average and goal in each.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var cs []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/courses", nil, nil, &cs); err != nil {
				return nil, err
			}
			return items(cs), nil
		},
	},
	{
		Name: "create_course", Title: "Create a course",
		Description: "A new course in a school workspace, with its weekly classes (slots: [{\"weekday\": 1, \"start\": \"09:00\", \"end\": \"10:30\", \"room\": \"B2\"}]) shown in the calendar.",
		Props:       courseProps(map[string]mcpProp{"workspace_id": pWS}),
		Required:    []string{"workspace_id", "name"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body, err := courseBody(a)
			if err != nil {
				return nil, err
			}
			var cs map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/courses", nil, body, &cs); err != nil {
				return nil, err
			}
			return keepOne(cs, "id", "name", "code"), nil
		},
	},
	{
		Name: "update_course", Title: "Change a course",
		Description: "Change a course: its name, code, teacher, room, term, weight, notes, or weekly classes (slots, the whole list).",
		Props:       courseProps(map[string]mcpProp{"course_id": pCourse}),
		Required:    []string{"course_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body, err := courseBody(a)
			if err != nil {
				return nil, err
			}
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var cs map[string]any
			if err := m.do("PUT", "/courses/"+a.str("course_id"), nil, body, &cs); err != nil {
				return nil, err
			}
			return keepOne(cs, "id", "name", "code"), nil
		},
	},
	{
		Name: "delete_course", Title: "Delete a course", Destructive: true,
		Description: "Deletes a course with its classes and grades.",
		Props:       map[string]mcpProp{"course_id": pCourse}, Required: []string{"course_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/courses/"+a.str("course_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},
	{
		Name: "set_course_goal", Title: "Set a course goal",
		Description: "The average you aim at in a course, on the workspace's grade scale (no target: clears it).",
		Props:       map[string]mcpProp{"course_id": pCourse, "target": {Type: "number", Description: "The average aimed at"}},
		Required:    []string{"course_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("PUT", "/courses/"+a.str("course_id")+"/goal", nil, map[string]any{"target": a["target"]}, nil); err != nil {
				return nil, err
			}
			return "Done.", nil
		},
	},
	{
		Name: "list_grades", Title: "List grades", ReadOnly: true,
		Description: "Your grades in a school workspace, by course: each test's score, out of, weight and date.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var out any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/grades", nil, nil, &out); err != nil {
				return nil, err
			}
			return map[string]any{"grades": out}, nil
		},
	},
	{
		Name: "add_grade", Title: "Add a grade",
		Description: "A grade of yours in a course: a test's score, out of how much, its weight (default 1) and date.",
		Props: map[string]mcpProp{
			"course_id": pCourse, "title": {Type: "string", Description: "What it was (Test 1, Exam…)"}, "score": {Type: "number", Description: "The score"},
			"out_of": {Type: "number", Description: "Out of (default the workspace's scale)"}, "weight": {Type: "number", Description: "Its weight"}, "date": pDate("When"),
		},
		Required: []string{"course_id", "title", "score"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var g map[string]any
			if err := m.do("POST", "/courses/"+a.str("course_id")+"/grades", nil, a.pick("title", "score", "out_of", "weight", "date"), &g); err != nil {
				return nil, err
			}
			return keepOne(g, "id", "title", "score", "out_of"), nil
		},
	},
	{
		Name: "update_grade", Title: "Change a grade",
		Description: "Change a grade's name, score, out of, weight or date.",
		Props: map[string]mcpProp{
			"grade_id": pGrade, "title": {Type: "string", Description: "What it was"}, "score": {Type: "number", Description: "The score"},
			"out_of": {Type: "number", Description: "Out of"}, "weight": {Type: "number", Description: "Its weight"}, "date": pDate("When"),
		},
		Required: []string{"grade_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("title", "score", "out_of", "weight", "date")
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var g map[string]any
			if err := m.do("PUT", "/grades/"+a.str("grade_id"), nil, body, &g); err != nil {
				return nil, err
			}
			return keepOne(g, "id", "title", "score", "out_of"), nil
		},
	},
	{
		Name: "delete_grade", Title: "Delete a grade", Destructive: true,
		Description: "Deletes a grade.",
		Props:       map[string]mcpProp{"grade_id": pGrade}, Required: []string{"grade_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/grades/"+a.str("grade_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},

	// ---- The inbox ----
	{
		Name: "list_notifications", Title: "Your notifications", ReadOnly: true,
		Description: "Your inbox, newest first: mentions, assignments, comments, reminders, with whether each is read.",
		Props:       map[string]mcpProp{"limit": {Type: "integer", Description: "At most (default 30, max 100)"}},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			q := url.Values{}
			if n := a.str("limit"); n != "" {
				q.Set("limit", n)
			}
			var out any
			if err := m.do("GET", "/me/notifications", q, nil, &out); err != nil {
				return nil, err
			}
			return out, nil
		},
	},
	{
		Name: "mark_notifications_read", Title: "Mark notifications read",
		Description: "Marks notifications read: these ids, or all of them (all: true).",
		Props:       map[string]mcpProp{"ids": {Type: "array", Description: "The notifications' ids"}, "all": {Type: "boolean", Description: "All of them"}},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			all, _ := a["all"].(bool)
			ids := a.strs("ids")
			if !all && len(ids) == 0 {
				return nil, errors.New("give ids, or all: true")
			}
			if err := m.do("POST", "/me/notifications/read", nil, map[string]any{"ids": ids, "all": all}, nil); err != nil {
				return nil, err
			}
			return "Marked read.", nil
		},
	},
}

// ---- Sheets' cells ----

// sheetData is a sheet's stored data, its tabs kept as they are (widths,
// frozen rows…) but for the cells written.
type sheetData struct {
	Tabs []map[string]any `json:"tabs"`
}

func shortID() string { return strings.ToLower(rand.Text()[:8]) }

// seed is a random number for an Excalidraw element's look.
func seed() int {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return int(binary.BigEndian.Uint32(b[:]) >> 2)
}

func newSheetTab(name string) map[string]any {
	return map[string]any{"id": shortID(), "name": name, "cells": map[string]any{}, "rows": 100, "cols": 26}
}

var cellAddr = regexp.MustCompile(`^([A-Z]{1,2})([1-9][0-9]{0,4})$`)

// write puts cells (by address) and tsv rows (from at) into tab i.
func (d *sheetData) write(i int, cells map[string]any, tsv, at string) error {
	tab := d.Tabs[i]
	cs, _ := tab["cells"].(map[string]any)
	if cs == nil {
		cs = map[string]any{}
	}
	maxR, maxC := 0, 0
	set := func(addr string, v string) error {
		addr = strings.ToUpper(strings.TrimSpace(addr))
		mm := cellAddr.FindStringSubmatch(addr)
		if mm == nil {
			return fmt.Errorf("%q isn't a cell (A1, B12…)", addr)
		}
		r, _ := strconv.Atoi(mm[2])
		c := colNumber(mm[1]) // A is 1
		if r > 20000 || c > 702 {
			return fmt.Errorf("%s is past the sheet's edge (ZZ20000)", addr)
		}
		maxR, maxC = max(maxR, r), max(maxC, c)
		if v == "" {
			delete(cs, addr)
			return nil
		}
		cell, _ := cs[addr].(map[string]any)
		if cell == nil {
			cell = map[string]any{}
		}
		cell["v"] = v // its look (bold, colours…) kept
		cs[addr] = cell
		return nil
	}
	keys := make([]string, 0, len(cells))
	for k := range cells {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := set(k, toString(cells[k])); err != nil {
			return err
		}
	}
	if tsv != "" {
		if at == "" {
			at = "A1"
		}
		mm := cellAddr.FindStringSubmatch(strings.ToUpper(at))
		if mm == nil {
			return fmt.Errorf("at: %q isn't a cell (A1, B12…)", at)
		}
		r0, _ := strconv.Atoi(mm[2])
		c0 := colNumber(mm[1])
		for dr, line := range strings.Split(strings.TrimRight(strings.ReplaceAll(tsv, "\r\n", "\n"), "\n"), "\n") {
			for dc, v := range strings.Split(line, "\t") {
				if err := set(colLetters(c0+dc)+strconv.Itoa(r0+dr), v); err != nil {
					return err
				}
			}
		}
	}
	tab["cells"] = cs
	if n, _ := tab["rows"].(float64); int(n) < maxR {
		tab["rows"] = maxR
	}
	if n, _ := tab["cols"].(float64); int(n) < maxC {
		tab["cols"] = maxC
	}
	return nil
}

// out is the data to store, and its words for search (as the app keeps them).
func (d *sheetData) out() (map[string]any, string) {
	var words []string
	for _, t := range d.Tabs {
		if n, _ := t["name"].(string); n != "" {
			words = append(words, n)
		}
		cs, _ := t["cells"].(map[string]any)
		for _, c := range cs {
			if cm, ok := c.(map[string]any); ok {
				if v, _ := cm["v"].(string); v != "" && !strings.HasPrefix(v, "=") {
					words = append(words, v)
				}
			}
		}
	}
	text := strings.Join(words, " ")
	if len(text) > 250_000 {
		text = text[:250_000]
	}
	return map[string]any{"tabs": d.Tabs}, text
}

// ---- Whiteboards' sticky notes ----

// boardWithNotes adds sticky notes (a yellow box with its text in it) to an
// Excalidraw scene, in rows of four below what's on it: the scene's JSON,
// and the board's words for search. Excalidraw fills in what an element
// leaves out when the board is opened.
func boardWithNotes(scene map[string]any, notes []string) (string, string) {
	if scene == nil {
		scene = map[string]any{}
	}
	els, _ := scene["elements"].([]any)
	top := 0.0
	var words []string
	for _, e := range els {
		el, _ := e.(map[string]any)
		if el == nil {
			continue
		}
		y, _ := el["y"].(float64)
		h, _ := el["height"].(float64)
		top = max(top, y+h+40)
		if t, _ := el["text"].(string); t != "" {
			words = append(words, t)
		}
	}
	const w, h, gap = 220.0, 160.0, 30.0
	th := func(s string) float64 { return 25 * float64(strings.Count(s, "\n")+1) } // 20px text, 1.25 lines
	for i, n := range notes {
		x := float64(i%4) * (w + gap)
		y := top + float64(i/4)*(h+gap)
		box, txt := shortID(), shortID()
		els = append(els,
			map[string]any{
				"id": box, "type": "rectangle", "x": x, "y": y, "width": w, "height": h,
				"strokeColor": "#1e1e1e", "backgroundColor": "#ffec99", "fillStyle": "solid", "strokeWidth": 1, "roughness": 0,
				"roundness": map[string]any{"type": 3}, "seed": seed(), "version": 1, "versionNonce": seed(),
				"boundElements": []any{map[string]any{"type": "text", "id": txt}},
			},
			map[string]any{
				// Its own height (its lines), in the middle of the note.
				"id": txt, "type": "text", "x": x + 10, "y": y + (h-th(n))/2, "width": w - 20, "height": th(n),
				"text": n, "originalText": n, "fontSize": 20, "fontFamily": 5, "textAlign": "center", "verticalAlign": "middle",
				"containerId": box, "autoResize": true, "lineHeight": 1.25, "strokeColor": "#1e1e1e",
				"seed": seed(), "version": 1, "versionNonce": seed(),
			})
		words = append(words, n)
	}
	scene["type"], scene["version"], scene["source"] = "excalidraw", 2, "orbit"
	scene["elements"] = els
	if _, ok := scene["appState"]; !ok {
		scene["appState"] = map[string]any{}
	}
	if _, ok := scene["files"]; !ok {
		scene["files"] = map[string]any{}
	}
	b, _ := json.Marshal(scene)
	return string(b), strings.Join(words, " ")
}

// ---- Courses ----

func courseProps(more map[string]mcpProp) map[string]mcpProp {
	return withProps(map[string]mcpProp{
		"name": {Type: "string", Description: "Its name"}, "code": {Type: "string", Description: "A short code (MAT, BIO…)"},
		"teacher": {Type: "string", Description: "Who teaches it"}, "room": {Type: "string", Description: "Where"}, "color": pColor,
		"term_start": pDate("When the term starts"), "term_end": pDate("When it ends"), "tz": {Type: "string", Description: "The classes' time zone (IANA)"},
		"weight": {Type: "number", Description: "Its weight in the overall average"}, "notes": {Type: "string", Description: "Notes"},
		"slots": {Type: "array", Items: "object", Description: "Its weekly classes: {weekday 1–7, start HH:MM, end HH:MM, room, kind} each"},
	}, more)
}

func courseBody(a mcpArgs) (map[string]any, error) {
	body := a.pick("name", "code", "teacher", "room", "color", "term_start", "term_end", "tz", "weight", "notes")
	if v, ok := a["slots"]; ok {
		switch s := v.(type) {
		case []any:
			body["slots"] = s
		case string:
			var list []any
			if json.Unmarshal([]byte(s), &list) != nil {
				return nil, errors.New("slots: a list of {weekday, start, end, room}")
			}
			body["slots"] = list
		default:
			return nil, errors.New("slots: a list of {weekday, start, end, room}")
		}
	}
	return body, nil
}

// toolModules: the tools that are for one module (the rest are for any).
var toolModules = map[string][]string{
	"habits":      {"list_habits", "check_habit", "create_habit", "update_habit", "delete_habit"},
	"goals":       {"list_goals", "create_goal", "update_goal", "delete_goal"},
	"journal":     {"read_journal", "write_journal"},
	"bookmarks":   {"list_bookmarks", "create_bookmark", "update_bookmark", "delete_bookmark"},
	"focus":       {"get_focus", "start_focus", "stop_focus"},
	"sheets":      {"read_sheet", "create_sheet", "write_sheet", "delete_sheet"},
	"diagrams":    {"read_diagram", "create_mermaid_diagram", "update_mermaid_diagram", "delete_diagram"},
	"whiteboards": {"read_whiteboard", "create_whiteboard", "add_to_whiteboard", "delete_whiteboard"},
	"courses":     {"list_courses", "create_course", "update_course", "delete_course", "set_course_goal"},
	"grades":      {"list_grades", "add_grade", "update_grade", "delete_grade"},
	"calendar":    {"list_calendar_subscriptions", "subscribe_calendar", "unsubscribe_calendar"},
}

func init() {
	for mod, names := range toolModules {
		for _, n := range names {
			t := mcpToolByName(n)
			if t == nil {
				panic("toolModules: no tool " + n)
			}
			t.Module = mod
		}
	}
}

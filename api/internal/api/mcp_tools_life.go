package api

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// The MCP tools for the rest of a workspace's modules: habits, goals, the
// journal, bookmarks, focus sessions, and changing or deleting notes. Each a
// call (or two) to the REST API, as the others.

func init() { mcpTools = append(mcpTools, lifeTools...) }

var (
	pHabit    = mcpProp{Type: "string", Description: "The habit's id (from list_habits)"}
	pGoal     = mcpProp{Type: "string", Description: "The goal's id (from list_goals)"}
	pBookmark = mcpProp{Type: "string", Description: "The bookmark's id (from list_bookmarks)"}
	pNote     = mcpProp{Type: "string", Description: "The note's id (from list_notes)"}
	pMood     = mcpProp{Type: "integer", Description: "How the day went: 1 awful, 2 bad, 3 okay, 4 good, 5 great (0 none)"}
)

// habitDaysShown: how many days back list_habits shows a habit's checks.
const habitDaysShown = 35

var habitProps = map[string]mcpProp{
	"title":     {Type: "string", Description: "Its name"},
	"icon":      {Type: "string", Description: "An emoji"},
	"schedule":  {Type: "string", Description: "daily (on the days in days) or weekly (target times a week, any days)", Enum: []string{"daily", "weekly"}},
	"days":      {Type: "string", Description: "Daily: the days it's for, seven 0 or 1, Monday first (1111100: weekdays)"},
	"target":    {Type: "integer", Description: "Weekly: how many times a week (1–7)"},
	"remind_at": {Type: "string", Description: "A daily reminder at this time (HH:MM; empty: none)"},
}

func withProps(base map[string]mcpProp, more map[string]mcpProp) map[string]mcpProp {
	out := make(map[string]mcpProp, len(base)+len(more))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range more {
		out[k] = v
	}
	return out
}

// habitView is a habit as the tools show it: its days done lately only.
func habitView(hb map[string]any) map[string]any {
	since := time.Now().AddDate(0, 0, -habitDaysShown).Format("2006-01-02")
	recent := []string{}
	if cs, ok := hb["checks"].([]any); ok {
		for _, c := range cs {
			if d, _ := c.(string); d >= since {
				recent = append(recent, d)
			}
		}
	}
	out := keepOne(hb, "id", "title", "icon", "schedule", "days", "target", "remind_at", "archived_at")
	out["done_on"] = recent
	return out
}

var lifeTools = []mcpTool{
	// ---- Habits (each person's own) ----
	{
		Name: "list_habits", Title: "List habits", ReadOnly: true,
		Description: "Your habits in a workspace, with the days each was done in the last 5 weeks (done_on, YYYY-MM-DD).",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var hs []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/habits", nil, nil, &hs); err != nil {
				return nil, err
			}
			out := make([]map[string]any, len(hs))
			for i, hb := range hs {
				out[i] = habitView(hb)
			}
			return items(out), nil
		},
	},
	{
		Name: "check_habit", Title: "Tick a habit",
		Description: "Marks a habit done on a day (today or earlier), or not done (done: false).",
		Props: map[string]mcpProp{
			"habit_id": pHabit, "date": {Type: "string", Description: "The day (YYYY-MM-DD), in the person's own time zone"},
			"done": {Type: "boolean", Description: "Done (the default) or not done"},
		},
		Required: []string{"habit_id", "date"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			done := true
			if v, ok := a["done"].(bool); ok {
				done = v
			}
			var hb map[string]any
			if err := m.do("PUT", "/habits/"+a.str("habit_id")+"/checks/"+url.PathEscape(a.str("date")), nil, map[string]any{"done": done}, &hb); err != nil {
				return nil, err
			}
			return habitView(hb), nil
		},
	},
	{
		Name: "create_habit", Title: "Create a habit",
		Description: "A new habit of yours in a workspace: daily on some days (every day if not said), or a number of times a week.",
		Props:       withProps(habitProps, map[string]mcpProp{"workspace_id": pWS}),
		Required:    []string{"workspace_id", "title"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var hb map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/habits", nil, a.pick("title", "icon", "schedule", "days", "target", "remind_at"), &hb); err != nil {
				return nil, err
			}
			return habitView(hb), nil
		},
	},
	{
		Name: "update_habit", Title: "Change a habit",
		Description: "Change a habit's name, schedule or reminder; archived: true puts it away (its history kept), false brings it back.",
		Props:       withProps(habitProps, map[string]mcpProp{"habit_id": pHabit, "archived": {Type: "boolean", Description: "Put away (true) or back (false)"}}),
		Required:    []string{"habit_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("title", "icon", "schedule", "days", "target", "remind_at", "archived")
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var hb map[string]any
			if err := m.do("PUT", "/habits/"+a.str("habit_id"), nil, body, &hb); err != nil {
				return nil, err
			}
			return habitView(hb), nil
		},
	},
	{
		Name: "delete_habit", Title: "Delete a habit", Destructive: true,
		Description: "Deletes a habit with all its history (update_habit with archived: true keeps it).",
		Props:       map[string]mcpProp{"habit_id": pHabit}, Required: []string{"habit_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/habits/"+a.str("habit_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},

	// ---- Goals ----
	{
		Name: "list_goals", Title: "List goals", ReadOnly: true,
		Description: "A workspace's goals: due date, progress (0–100, by hand) or tasks done of tasks (a task counts towards a goal through update_task's goal_id), and whether reached.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var gs []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/goals", nil, nil, &gs); err != nil {
				return nil, err
			}
			return items(keep(gs, "id", "title", "icon", "notes", "due", "progress", "tasks", "done", "achieved_at")), nil
		},
	},
	{
		Name: "create_goal", Title: "Create a goal",
		Description: "A new goal in a workspace, something worked towards by a date.",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "title": {Type: "string", Description: "What it is"}, "notes": {Type: "string", Description: "Notes"},
			"icon": {Type: "string", Description: "An emoji"}, "due": pDate("By when"),
		},
		Required: []string{"workspace_id", "title"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var g map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/goals", nil, a.pick("title", "notes", "icon", "due"), &g); err != nil {
				return nil, err
			}
			return keepOne(g, "id", "title", "due"), nil
		},
	},
	{
		Name: "update_goal", Title: "Change a goal",
		Description: "Change a goal: its name, notes, date, progress by hand (when it has no tasks), or achieved: true when it's reached (false: not yet).",
		Props: map[string]mcpProp{
			"goal_id": pGoal, "title": {Type: "string", Description: "What it is"}, "notes": {Type: "string", Description: "Notes (the whole text)"},
			"icon": {Type: "string", Description: "An emoji"}, "due": pDate("By when"),
			"progress": {Type: "integer", Description: "How far along, 0–100"}, "achieved": {Type: "boolean", Description: "Reached (true) or not yet (false)"},
		},
		Required: []string{"goal_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("title", "notes", "icon", "due", "progress", "achieved")
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var g map[string]any
			if err := m.do("PUT", "/goals/"+a.str("goal_id"), nil, body, &g); err != nil {
				return nil, err
			}
			return keepOne(g, "id", "title", "due", "progress", "achieved_at"), nil
		},
	},
	{
		Name: "delete_goal", Title: "Delete a goal", Destructive: true,
		Description: "Deletes a goal (its tasks stay, no longer counting towards it).",
		Props:       map[string]mcpProp{"goal_id": pGoal}, Required: []string{"goal_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/goals/"+a.str("goal_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},

	// ---- Journal (each person's own) ----
	{
		Name: "read_journal", Title: "Read the journal", ReadOnly: true,
		Description: "Your journal in a workspace: one day's entry in full (date), with the same day in earlier years; or a month's days with their mood and first words (month).",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "date": {Type: "string", Description: "A day (YYYY-MM-DD)"}, "month": {Type: "string", Description: "A month (YYYY-MM)"},
		},
		Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			ws := "/workspaces/" + a.str("workspace_id") + "/journal"
			if d := a.str("date"); d != "" {
				var day map[string]any
				if err := m.do("GET", ws+"/"+url.PathEscape(d), nil, nil, &day); err != nil {
					return nil, err
				}
				out := map[string]any{"date": d, "content": "", "mood": 0, "on_this_day": day["on_this_day"]}
				if e, ok := day["entry"].(map[string]any); ok {
					out["content"], out["mood"] = e["content"], e["mood"]
				}
				return out, nil
			}
			month := a.str("month")
			if month == "" {
				return nil, errors.New("give a date (YYYY-MM-DD) or a month (YYYY-MM)")
			}
			var days []map[string]any
			if err := m.do("GET", ws, url.Values{"month": {month}}, nil, &days); err != nil {
				return nil, err
			}
			return items(days), nil
		},
	},
	{
		Name: "write_journal", Title: "Write in the journal",
		Description: "Writes a day of your journal (Markdown): content replaces the day's text, append adds to its end; mood sets how the day went.",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "date": {Type: "string", Description: "The day (YYYY-MM-DD)"},
			"content": {Type: "string", Description: "The day's whole text"}, "append": {Type: "string", Description: "Text to add at the end"}, "mood": pMood,
		},
		Required: []string{"workspace_id", "date"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			path := "/workspaces/" + a.str("workspace_id") + "/journal/" + url.PathEscape(a.str("date"))
			body := a.pick("content", "mood")
			if add := a.str("append"); add != "" {
				var day struct {
					Entry *struct{ Content string } `json:"entry"`
				}
				if err := m.do("GET", path, nil, nil, &day); err != nil {
					return nil, err
				}
				text := ""
				if day.Entry != nil {
					text = strings.TrimRight(day.Entry.Content, "\n")
				}
				if c, ok := body["content"].(string); ok {
					text = strings.TrimRight(c, "\n")
				}
				if text != "" {
					text += "\n\n"
				}
				body["content"] = text + add
			}
			if len(body) == 0 {
				return nil, errors.New("nothing to write")
			}
			var day map[string]any
			if err := m.do("PUT", path, nil, body, &day); err != nil {
				return nil, err
			}
			if e, ok := day["entry"].(map[string]any); ok {
				return keepOne(e, "date", "mood"), nil
			}
			return "The day is empty now.", nil
		},
	},

	// ---- Bookmarks ----
	{
		Name: "list_bookmarks", Title: "List bookmarks", ReadOnly: true,
		Description: "A workspace's saved links: title, site, tags, notes, whether kept to read later and whether read.",
		Props:       map[string]mcpProp{"workspace_id": pWS}, Required: []string{"workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var bs []map[string]any
			if err := m.do("GET", "/workspaces/"+a.str("workspace_id")+"/bookmarks", nil, nil, &bs); err != nil {
				return nil, err
			}
			return items(keep(bs, "id", "url", "title", "description", "site_name", "tags", "notes", "read_later", "read_at", "created_at")), nil
		},
	},
	{
		Name: "create_bookmark", Title: "Save a link",
		Description: "Saves a link in a workspace's bookmarks (its title and picture are fetched from the page when not given).",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "url": {Type: "string", Description: "The link (http or https)"}, "title": {Type: "string", Description: "Its title"},
			"tags": {Type: "array", Description: "Tags"}, "notes": {Type: "string", Description: "Notes"}, "read_later": {Type: "boolean", Description: "Kept to read later"},
		},
		Required: []string{"workspace_id", "url"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var b map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/bookmarks", nil, a.pick("url", "title", "tags", "notes", "read_later"), &b); err != nil {
				return nil, err
			}
			return keepOne(b, "id", "url", "title", "tags"), nil
		},
	},
	{
		Name: "update_bookmark", Title: "Change a bookmark",
		Description: "Change a bookmark's title, description, tags (the whole list) or notes; read_later keeps it to read; read marks it read (false: unread).",
		Props: map[string]mcpProp{
			"bookmark_id": pBookmark, "title": {Type: "string", Description: "Its title"}, "description": {Type: "string", Description: "What it is"},
			"tags": {Type: "array", Description: "Its tags (the whole list)"}, "notes": {Type: "string", Description: "Notes (the whole text)"},
			"read_later": {Type: "boolean", Description: "Kept to read later"}, "read": {Type: "boolean", Description: "Read (true) or unread (false)"},
		},
		Required: []string{"bookmark_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("title", "description", "tags", "notes", "read_later", "read")
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var b map[string]any
			if err := m.do("PUT", "/bookmarks/"+a.str("bookmark_id"), nil, body, &b); err != nil {
				return nil, err
			}
			return keepOne(b, "id", "url", "title", "tags", "read_later", "read_at"), nil
		},
	},
	{
		Name: "delete_bookmark", Title: "Delete a bookmark", Destructive: true,
		Description: "Deletes a saved link.",
		Props:       map[string]mcpProp{"bookmark_id": pBookmark}, Required: []string{"bookmark_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/bookmarks/"+a.str("bookmark_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},

	// ---- Focus (each person's own) ----
	{
		Name: "get_focus", Title: "Focus sessions", ReadOnly: true,
		Description: "Your focus timer: the session running now (if any), the ones finished today and today's minutes.",
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var f map[string]any
			if err := m.do("GET", "/me/focus", nil, nil, &f); err != nil {
				return nil, err
			}
			return f, nil
		},
	},
	{
		Name: "start_focus", Title: "Start focusing",
		Description: "Starts a focus session of some minutes in a workspace, on a task or not (ending one still running).",
		Props: map[string]mcpProp{
			"workspace_id": pWS, "task_id": {Type: "string", Description: "The task it's on"}, "minutes": {Type: "integer", Description: "How long (1–180)"},
		},
		Required: []string{"workspace_id", "minutes"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var f map[string]any
			if err := m.do("POST", "/me/focus", nil, a.pick("workspace_id", "task_id", "minutes"), &f); err != nil {
				return nil, err
			}
			return map[string]any{"running": f["running"]}, nil
		},
	},
	{
		Name: "stop_focus", Title: "Stop focusing",
		Description: "Ends the focus session running now (the minutes so far are kept).",
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var f map[string]any
			if err := m.do("POST", "/me/focus/stop", nil, map[string]any{}, &f); err != nil {
				return nil, err
			}
			return map[string]any{"minutes_today": f["minutes"]}, nil
		},
	},

	// ---- Notes: change, delete ----
	{
		Name: "update_note", Title: "Change a note",
		Description: "Change a note (a workspace's or a Quick Note): its title, its text (the whole of it), or pin it.",
		Props: map[string]mcpProp{
			"note_id": pNote, "title": {Type: "string", Description: "Its title"}, "body": {Type: "string", Description: "Its text (the whole of it)"},
			"pinned": {Type: "boolean", Description: "Pinned at the top"},
		},
		Required: []string{"note_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			body := a.pick("title", "body", "pinned")
			if len(body) == 0 {
				return nil, errors.New("nothing to change")
			}
			var n map[string]any
			if err := m.do("PUT", "/notes/"+a.str("note_id"), nil, body, &n); err != nil {
				return nil, err
			}
			return keepOne(n, "id", "title", "pinned"), nil
		},
	},
	{
		Name: "delete_note", Title: "Delete a note", Destructive: true,
		Description: "Deletes a note (a workspace's or a Quick Note).",
		Props:       map[string]mcpProp{"note_id": pNote}, Required: []string{"note_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			if err := m.do("DELETE", "/notes/"+a.str("note_id"), nil, nil, nil); err != nil {
				return nil, err
			}
			return "Deleted.", nil
		},
	},
}

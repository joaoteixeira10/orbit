// Package planner holds Orbit's planner rules that don't need HTTP: the
// module catalog and workspace templates, board columns, ordering ranks,
// recurrence (events and repeating tasks), ICS import and export, and the
// plain-text extraction of page documents. The api package's handlers use it;
// it imports only models.
package planner

// Module keys. A workspace turns on any subset, in its own order.
const (
	ModHome     = "home"        // overview: today, upcoming, recent pages
	ModPages    = "pages"       // block-editor pages
	ModTasks    = "tasks"       // task lists (Reminders-style)
	ModBoard    = "board"       // the same tasks as kanban boards
	ModCalendar = "calendar"    // events, due tasks, subscribed calendars
	ModNotes    = "notes"       // quick notes (sticky notes)
	ModTimeline = "timeline"    // the tasks on a timeline (Gantt)
	ModCharts   = "charts"      // the tasks in charts (status, lists, priority, people)
	ModBoards   = "whiteboards" // Excalidraw whiteboards
)

// Module describes one entry of the catalog. Soon marks a module that is on
// the roadmap: listed (so people see what's coming) but not selectable.
type Module struct {
	Key  string `json:"key"`
	Icon string `json:"icon"` // lucide icon name
	Soon bool   `json:"soon"`
}

// Catalog is every module, in the default sidebar order.
var Catalog = []Module{
	{Key: ModHome, Icon: "house"},
	{Key: ModNotes, Icon: "sticky-note"},
	{Key: ModPages, Icon: "file-text"},
	{Key: ModTasks, Icon: "list-checks"},
	{Key: ModBoard, Icon: "columns-3"},
	{Key: ModTimeline, Icon: "chart-gantt"},
	{Key: ModCharts, Icon: "chart-pie"},
	{Key: ModCalendar, Icon: "calendar"},
	{Key: ModCourses, Icon: "graduation-cap"},
	{Key: ModGrades, Icon: "award"},
	{Key: ModBoards, Icon: "brush"},
	{Key: "habits", Icon: "repeat", Soon: true},
	{Key: "journal", Icon: "notebook-pen", Soon: true},
	{Key: "goals", Icon: "target", Soon: true},
	{Key: "bookmarks", Icon: "bookmark", Soon: true},
	{Key: "focus", Icon: "timer", Soon: true},
}

// Available reports whether key is a module that can be turned on now.
func Available(key string) bool {
	for _, m := range Catalog {
		if m.Key == key {
			return !m.Soon
		}
	}
	return false
}

// CleanModules keeps the available modules of list, in its order, without
// duplicates. ok is false when an unknown or unavailable key was given.
func CleanModules(list []string) (out []string, ok bool) {
	seen := map[string]bool{}
	ok = true
	out = []string{}
	for _, k := range list {
		if !Available(k) {
			ok = false
			continue
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out, ok
}

// Template is a starting point for a new workspace: its look, its modules
// and the content created with it (task lists with their columns, a first
// page). Everything can be changed afterwards.
type Template struct {
	Kind    string         `json:"kind"`
	Icon    string         `json:"icon"`
	Color   string         `json:"color"`
	Modules []string       `json:"modules"`
	Lists   []TemplateList `json:"lists"`
	Pages   []string       `json:"pages"` // titles of the first pages
}

// TemplateList is a task list a template creates, with its board columns.
type TemplateList struct {
	Name    string   `json:"name"`
	Color   string   `json:"color"`
	Icon    string   `json:"icon"`
	Columns []Column `json:"columns"`
}

// Templates is the catalog of workspace templates. Colors are Apple's
// system palette (light mode values; the UI adapts them to dark mode).
var Templates = []Template{
	{
		Kind: "school", Icon: "graduation-cap", Color: "#007AFF",
		Modules: []string{ModHome, ModCourses, ModCalendar, ModTasks, ModBoard, ModGrades, ModNotes, ModPages},
		Lists: []TemplateList{
			{Name: "Assignments", Color: "#007AFF", Icon: "pencil", Columns: Columns("To do", "Doing", "Handed in")},
			{Name: "Exams", Color: "#FF3B30", Icon: "file-check", Columns: Columns("Upcoming", "Studying", "Done")},
		},
		Pages: []string{"Class notes"},
	},
	{
		Kind: "work", Icon: "briefcase", Color: "#5856D6",
		Modules: []string{ModHome, ModBoard, ModTasks, ModTimeline, ModCharts, ModCalendar, ModNotes, ModPages},
		Lists: []TemplateList{
			{Name: "Projects", Color: "#5856D6", Icon: "folder-kanban", Columns: Columns("Backlog", "In progress", "Review", "Done")},
		},
		Pages: []string{"Meeting notes"},
	},
	{
		Kind: "personal", Icon: "heart", Color: "#FF2D55",
		Modules: []string{ModHome, ModTasks, ModCalendar, ModNotes, ModPages},
		Lists: []TemplateList{
			{Name: "Reminders", Color: "#FF9500", Icon: "bell", Columns: Columns("To do", "Done")},
		},
		Pages: []string{"Journal"},
	},
	{
		Kind: "shared", Icon: "users", Color: "#34C759",
		Modules: []string{ModHome, ModTasks, ModBoard, ModCalendar, ModNotes, ModPages},
		Lists: []TemplateList{
			{Name: "Shopping", Color: "#34C759", Icon: "shopping-cart", Columns: Columns("To buy", "Bought")},
			{Name: "Chores", Color: "#FF9500", Icon: "sparkles", Columns: Columns("To do", "Doing", "Done")},
		},
		Pages: []string{"House notes"},
	},
	{
		Kind: "custom", Icon: "sparkles", Color: "#AF52DE",
		Modules: []string{ModHome, ModNotes, ModPages},
	},
}

// TemplateFor returns the template of a kind (custom when unknown).
func TemplateFor(kind string) Template {
	for _, t := range Templates {
		if t.Kind == kind {
			return t
		}
	}
	return Templates[len(Templates)-1]
}

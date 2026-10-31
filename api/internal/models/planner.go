package models

import (
	"encoding/json"
	"time"
)

// ---- Workspaces -------------------------------------------------------------

// Workspace roles. Access to a workspace's content comes only from
// membership: instance admins manage accounts, not other people's planners.
const (
	WSOwner  = "owner"  // everything, including members, settings and deletion
	WSEditor = "editor" // create, change and delete content
	WSViewer = "viewer" // read only
)

// Workspace is one planner (School, Work, Personal, a shared household…)
// with its own set of modules. Kind records the template it started from;
// it doesn't limit what can be turned on later.
type Workspace struct {
	Base
	Name        string `gorm:"size:120;not null" json:"name"`
	Icon        string `gorm:"size:64" json:"icon"` // an icon key (lucide name) or an emoji
	Color       string `gorm:"size:9" json:"color"` // #rrggbb; tints the workspace
	Kind        string `gorm:"size:16" json:"kind"` // school | work | personal | shared | custom
	Description string `gorm:"size:500" json:"description"`
	// Modules is the ordered JSON list of the module keys turned on (see
	// package planner for the catalog). The order is the sidebar's.
	Modules    string     `gorm:"type:text" json:"-"`
	CreatedBy  string     `gorm:"size:36" json:"created_by"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	// GradeScale is what marks are out of in the Grades module (20 in
	// Portugal, 100 for percentages, 10, 5).
	GradeScale int `gorm:"not null;default:20" json:"grade_scale"`
	// Holidays: the country whose public holidays its calendar shows
	// (ISO code; "" none).
	Holidays string `gorm:"size:2" json:"holidays"`
}

// WorkspaceMember gives one user a role in one workspace. Position is that
// user's own order of their workspaces in the sidebar.
type WorkspaceMember struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;uniqueIndex:idx_ws_member" json:"workspace_id"`
	UserID      string  `gorm:"size:36;not null;uniqueIndex:idx_ws_member;index" json:"user_id"`
	Role        string  `gorm:"size:10;not null" json:"role"`
	Position    float64 `gorm:"not null;default:0" json:"position"`
	// Muted stops notifications about other members' activity in it.
	Muted bool `gorm:"not null;default:false" json:"muted"`
}

// ---- Pages ------------------------------------------------------------------

// Page is a block document (TipTap / ProseMirror JSON) in a workspace, nested
// under ParentID. Version counts saves, so a stale editor can't silently
// overwrite a newer save from someone else (409 instead). Trashed pages keep
// their place until emptied from the trash.
type Page struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;index" json:"workspace_id"`
	ParentID    *string `gorm:"size:36;index" json:"parent_id"`
	FolderID    string  `gorm:"size:36;not null;default:'';index" json:"folder_id"` // a folder it's filed in ("" none); its sub-pages go with it
	Title       string  `gorm:"size:300" json:"title"`
	Icon        string  `gorm:"size:64" json:"icon"`
	Cover       string  `gorm:"size:64" json:"cover"` // a gradient key or #rrggbb; empty = none
	// Content is the editor's JSON document; ContentText its plain text, kept
	// for search and previews.
	Content     string     `gorm:"type:text" json:"content,omitempty"`
	ContentText string     `gorm:"type:text" json:"-"`
	Rank        float64    `gorm:"not null;default:0" json:"rank"`
	Version     int        `gorm:"not null;default:1" json:"version"`
	CreatedBy   string     `gorm:"size:36" json:"created_by"`
	UpdatedBy   string     `gorm:"size:36" json:"updated_by"`
	TrashedAt   *time.Time `gorm:"index" json:"trashed_at,omitempty"`
	// Template: offered when a new page starts empty (its title and
	// document, its placeholders — the date and the like — filled in).
	Template bool `gorm:"not null;default:false" json:"template"`
	// DB makes the page a database of the pages inside it: its properties
	// and views (planner.Database, JSON). Props are a row's values, by
	// property id (JSON). The API answers both as JSON objects.
	DB    string `gorm:"type:text" json:"db,omitempty"`
	Props string `gorm:"type:text" json:"props,omitempty"`
	// Restricted: only AllowedUsers (JSON []user id) see it and what's
	// under it (api/page_access.go).
	Restricted bool `gorm:"not null;default:false;index" json:"restricted"`
	// Glossary: its terms ("**Term** — what it means") are explained where
	// the workspace's other pages use them.
	Glossary     bool   `gorm:"not null;default:false" json:"glossary"`
	AllowedUsers string `gorm:"type:text" json:"-"`
}

// PageVersion is a page as it was after a save: its title and Markdown, and
// who saved it. One person's saves close together make one version (a
// session of editing), not one per pause. Restored: the version this one
// put back (0: an edit).
type PageVersion struct {
	Base
	PageID      string `gorm:"size:36;not null;index" json:"page_id"`
	WorkspaceID string `gorm:"size:36;not null;index" json:"workspace_id"`
	Version     int    `gorm:"not null" json:"version"` // the page's version it is
	Title       string `gorm:"size:300" json:"title"`
	Content     string `gorm:"type:text" json:"content,omitempty"`
	UserID      string `gorm:"size:36" json:"user_id"`
	Restored    int    `gorm:"not null;default:0" json:"restored,omitempty"`
}

// ShareLink opens a page — and, with Subpages, the pages under it — to
// anyone with its link, without an account: to read, or also to edit
// (Mode). The link's secret is kept hashed (to find the link) and sealed
// (to show it again to the workspace's people).
type ShareLink struct {
	Base
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	PageID      string     `gorm:"size:36;not null;index" json:"page_id"`
	Mode        string     `gorm:"size:8;not null;default:'read'" json:"mode"` // read | edit
	Subpages    bool       `gorm:"not null;default:true" json:"subpages"`
	TokenHash   string     `gorm:"size:64;uniqueIndex" json:"-"`
	TokenSealed string     `gorm:"type:text" json:"-"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedBy   string     `gorm:"size:36" json:"created_by"`
}

// Form asks anyone with its link — or the workspace's people — for a row of
// a database (PageID): its fields are some of the database's properties
// (Fields, JSON []FormField; "title" is the row's name). Each answer is a
// new row. A public one is reached by its secret link (hashed and sealed,
// as share links).
type Form struct {
	Base
	WorkspaceID string `gorm:"size:36;not null;index" json:"workspace_id"`
	PageID      string `gorm:"size:36;not null;index" json:"page_id"`
	Title       string `gorm:"size:300" json:"title"`
	Intro       string `gorm:"type:text" json:"intro"`
	Fields      string `gorm:"type:text" json:"-"`
	Thanks      string `gorm:"size:500" json:"thanks"`
	Public      bool   `gorm:"not null;default:false" json:"public"`
	Open        bool   `gorm:"not null;default:true" json:"open"` // taking answers
	TokenHash   string `gorm:"size:64;index" json:"-"`
	TokenSealed string `gorm:"type:text" json:"-"`
	Answers     int    `gorm:"not null;default:0" json:"answers"`
	CreatedBy   string `gorm:"size:36" json:"created_by"`
}

// PageChange is one of a person's changes to a page's place in the tree —
// made, moved, renamed, copied, put in the trash — with what it was before
// and after (JSON), so they can undo it (api/page_changes.go).
type PageChange struct {
	Base
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	UserID      string     `gorm:"size:36;not null;index" json:"user_id"`
	PageID      string     `gorm:"size:36;not null;index" json:"page_id"`
	Kind        string     `gorm:"size:8;not null" json:"kind"` // create | move | rename | copy | trash
	Before      string     `gorm:"type:text" json:"-"`
	After       string     `gorm:"type:text" json:"-"`
	UndoneAt    *time.Time `json:"undone_at,omitempty"`
}

// PageFavorite pins a page to one user's Favorites.
type PageFavorite struct {
	Base
	UserID string  `gorm:"size:36;not null;uniqueIndex:idx_page_fav" json:"user_id"`
	PageID string  `gorm:"size:36;not null;uniqueIndex:idx_page_fav;index" json:"page_id"`
	Rank   float64 `gorm:"not null;default:0" json:"rank"`
}

// ---- Tasks and boards -------------------------------------------------------

// TaskList is a list of tasks (Apple Reminders' "list", a kanban "board").
// Columns is its JSON list of statuses ([{id, name, color, done}]): the
// board's columns, in order. A task in a column with done=true is complete.
type TaskList struct {
	Base
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	Name        string     `gorm:"size:120;not null" json:"name"`
	Color       string     `gorm:"size:9" json:"color"`
	Icon        string     `gorm:"size:64" json:"icon"`
	Columns     string     `gorm:"type:text" json:"-"`
	Rank        float64    `gorm:"not null;default:0" json:"rank"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
}

// Task is one to-do. Due and DueTime are floating (no time zone, like Apple
// Reminders): "2026-10-20" and optionally "14:30", read in the viewer's zone.
// Start (floating too, optional) is when work on it begins: the left end of
// its bar on the timeline.
// Repeat is an RRULE; completing a repeating task moves it to its next date
// instead of closing it. Rank orders tasks inside their column.
type Task struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;index" json:"workspace_id"`
	ListID      string  `gorm:"size:36;not null;index" json:"list_id"`
	ColumnID    string  `gorm:"size:36;not null" json:"column_id"`
	ParentID    *string `gorm:"size:36;index" json:"parent_id"`
	Title       string  `gorm:"size:500;not null" json:"title"`
	Notes       string  `gorm:"type:text" json:"notes"`
	Priority    int     `gorm:"not null;default:0" json:"priority"` // 0 none, 1 low, 2 medium, 3 high
	// Estimate is how long it should take, in minutes (0: not estimated).
	Estimate    int        `gorm:"not null;default:0" json:"estimate"`
	Kind        string     `gorm:"size:8;not null;default:''" json:"kind"` // "" a task | exam (counted down to)
	Tags        string     `gorm:"type:text" json:"-"`                     // JSON []string
	Start       *string    `gorm:"size:10" json:"start"`                   // YYYY-MM-DD, on or before Due
	Due         *string    `gorm:"size:10;index" json:"due"`               // YYYY-MM-DD
	DueTime     *string    `gorm:"size:5" json:"due_time"`                 // HH:MM
	Repeat      string     `gorm:"size:200" json:"repeat"`
	Rank        float64    `gorm:"not null;default:0" json:"rank"`
	CompletedAt *time.Time `gorm:"index" json:"completed_at"`
	// Assignees are who it's for in a shared workspace: a JSON list of user
	// ids (reminders go to each; none = to whoever created it). See
	// AssigneeList / SetAssignees.
	Assignees string `gorm:"type:text" json:"-"`
	CreatedBy string `gorm:"size:36" json:"created_by"`
	// The git issue (or pull request) this task is, when it is one: a task
	// of a git-linked list, or a task linked by hand. GitIssueID is the
	// server's global id; GitRepo is "owner/repo" on GitHost (a base URL).
	GitHost     string     `gorm:"size:255" json:"git_host,omitempty"`
	GitRepo     string     `gorm:"size:255" json:"git_repo,omitempty"`
	GitNumber   int64      `gorm:"not null;default:0" json:"git_number,omitempty"`
	GitIssueID  int64      `gorm:"not null;default:0;index" json:"-"`
	GitURL      string     `gorm:"size:500" json:"git_url,omitempty"`
	GitState    string     `gorm:"size:16" json:"git_state,omitempty"` // open | closed | merged
	GitPull     bool       `gorm:"not null;default:false" json:"git_pull,omitempty"`
	GitSyncedAt *time.Time `json:"-"`
	GitDirty    bool       `gorm:"not null;default:false" json:"git_dirty,omitempty"` // a change didn't reach git yet (retried by the sync)
	GitDirtyBy  string     `gorm:"size:36" json:"-"`                                  // whose change it was (retried as them)
	// The issue's milestone (a linked list's task): its title, and how the
	// server names it (Gitea: its id; GitHub: its number). 0: none.
	GitMilestone   string `gorm:"size:255" json:"git_milestone,omitempty"`
	GitMilestoneID int64  `gorm:"not null;default:0" json:"git_milestone_id,omitempty"`
	// FocusMinutes is the time focused on it (focus sessions).
	FocusMinutes int `gorm:"not null;default:0" json:"focus_minutes,omitempty"`
	// GoalID: the goal it counts towards ("" none).
	GoalID string `gorm:"size:36;index" json:"goal_id,omitempty"`
	// WaitsOn: tasks of the workspace it starts after (finish-to-start), a
	// JSON list of ids. See WaitsOnList / SetWaitsOn.
	WaitsOn string `gorm:"type:text" json:"-"`
}

// ---- Quick notes ------------------------------------------------------------

// Note is a quick note (a sticky note): a short text or a checklist on a
// coloured card. Items (JSON [{id, text, done}]) is set for a checklist note,
// which then has no Body. Pinned notes come first; Rank orders the rest.
// Longer writing belongs in a Page. A note is in a workspace, or — with no
// WorkspaceID — personal: OwnerID's own scratchpad, until it's moved into a
// workspace.
type Note struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;default:'';index" json:"workspace_id"`
	OwnerID     string  `gorm:"size:36;not null;default:'';index" json:"owner_id"` // personal notes only
	Title       string  `gorm:"size:300" json:"title"`
	Body        string  `gorm:"type:text" json:"body"`
	Items       string  `gorm:"type:text" json:"-"`
	Checklist   bool    `gorm:"not null;default:false" json:"checklist"`
	PlainText   string  `gorm:"type:text" json:"-"`   // body or items as text, for search
	Color       string  `gorm:"size:16" json:"color"` // a colour key (yellow, green, …); "" = plain
	Pinned      bool    `gorm:"not null;default:false" json:"pinned"`
	Rank        float64 `gorm:"not null;default:0" json:"rank"`
	CreatedBy   string  `gorm:"size:36" json:"created_by"`
	UpdatedBy   string  `gorm:"size:36" json:"updated_by"`
	// RemindAt: a one-off reminder (to RemindBy, who set it); cleared once sent.
	RemindAt *time.Time `gorm:"index" json:"remind_at,omitempty"`
	RemindBy string     `gorm:"size:36" json:"-"`
}

// ---- Habits -----------------------------------------------------------------

// Habit is something someone does again and again (read, run, practise
// piano), in a workspace with the Habits module. Habits are their owner's
// alone, like grades. Schedule: "daily" (Days: which weekdays count, as
// seven 0/1 for Monday…Sunday — "1111111" every day) or "weekly" (Target
// times a week, any days). RemindAt ("HH:MM", the owner's zone) reminds on
// the days it counts until it's done.
type Habit struct {
	Base
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	OwnerID     string     `gorm:"size:36;not null;index" json:"owner_id"`
	Title       string     `gorm:"size:200;not null" json:"title"`
	Icon        string     `gorm:"size:64" json:"icon"` // an emoji (or empty)
	Color       string     `gorm:"size:9" json:"color"`
	Schedule    string     `gorm:"size:8;not null;default:daily" json:"schedule"` // daily | weekly
	Days        string     `gorm:"size:7;not null;default:1111111" json:"days"`
	Target      int        `gorm:"not null;default:1" json:"target"` // weekly: times a week
	RemindAt    string     `gorm:"size:5" json:"remind_at"`
	Rank        float64    `gorm:"not null;default:0" json:"rank"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
}

// HabitCheck is one day a habit was done (a floating date).
type HabitCheck struct {
	Base
	HabitID string `gorm:"size:36;not null;uniqueIndex:idx_habit_day" json:"habit_id"`
	Date    string `gorm:"size:10;not null;uniqueIndex:idx_habit_day" json:"date"`
}

// ---- Bookmarks --------------------------------------------------------------

// Bookmark is a link worth keeping, in a workspace with the Bookmarks module:
// what the page says about itself (fetched once, through the public-only
// client), tags, a note, and "read later" (ReadAt set once opened). Its
// picture is kept in BookmarkImage, so viewers never call the site.
type Bookmark struct {
	Base
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	URL         string     `gorm:"size:2000;not null" json:"url"`
	Title       string     `gorm:"size:300" json:"title"`
	Description string     `gorm:"size:700" json:"description"`
	SiteName    string     `gorm:"size:120" json:"site_name"`
	Tags        string     `gorm:"type:text" json:"-"` // JSON []string
	Notes       string     `gorm:"type:text" json:"notes"`
	ReadLater   bool       `gorm:"not null;default:false" json:"read_later"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	HasImage    bool       `gorm:"not null;default:false" json:"has_image"`
	CreatedBy   string     `gorm:"size:36" json:"created_by"`
}

// BookmarkImage is a bookmark's picture (ID = the bookmark's).
type BookmarkImage struct {
	ID   string `gorm:"primaryKey;size:36"`
	Type string `gorm:"size:20;not null"`
	Data []byte `gorm:"not null"`
}

// ---- Journal ----------------------------------------------------------------

// JournalEntry is one day of someone's journal in a workspace with the
// Journal module: Markdown and a mood (1–5, 0 none). Entries are their
// owner's alone. ContentText is the plain text, for search and excerpts.
type JournalEntry struct {
	Base
	WorkspaceID string `gorm:"size:36;not null;uniqueIndex:idx_journal_day" json:"workspace_id"`
	OwnerID     string `gorm:"size:36;not null;uniqueIndex:idx_journal_day" json:"owner_id"`
	Date        string `gorm:"size:10;not null;uniqueIndex:idx_journal_day" json:"date"`
	Content     string `gorm:"type:text" json:"content"`
	ContentText string `gorm:"type:text" json:"-"`
	Mood        int    `gorm:"not null;default:0" json:"mood"`
}

// ---- Goals ------------------------------------------------------------------

// Goal is something a workspace works towards by a date (Due, floating):
// its progress is the share of its tasks (Task.GoalID) that are done, or —
// with no tasks — Progress (0–100) set by hand. AchievedAt marks it reached.
type Goal struct {
	Base
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	Title       string     `gorm:"size:200;not null" json:"title"`
	Notes       string     `gorm:"type:text" json:"notes"`
	Icon        string     `gorm:"size:64" json:"icon"`
	Color       string     `gorm:"size:9" json:"color"`
	Due         *string    `gorm:"size:10" json:"due"`
	Progress    int        `gorm:"not null;default:0" json:"progress"` // by hand, when it has no tasks
	AchievedAt  *time.Time `json:"achieved_at,omitempty"`
	Rank        float64    `gorm:"not null;default:0" json:"rank"`
	CreatedBy   string     `gorm:"size:36" json:"created_by"`
}

// ---- Smart lists ------------------------------------------------------------

// SmartList is a saved task filter, shown with Today and Scheduled in Tasks:
// its owner's alone, or (Shared) every member's. Filter is the filter as
// JSON (keyword, due buckets, priorities, progress, people, tags, lists).
type SmartList struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;index" json:"workspace_id"`
	OwnerID     string  `gorm:"size:36;not null;index" json:"owner_id"`
	Name        string  `gorm:"size:100;not null" json:"name"`
	Color       string  `gorm:"size:9" json:"color"`
	Icon        string  `gorm:"size:64" json:"icon"`
	Shared      bool    `gorm:"not null;default:false" json:"shared"`
	Filter      string  `gorm:"type:text" json:"-"`
	Rank        float64 `gorm:"not null;default:0" json:"rank"`
}

// ---- Focus ------------------------------------------------------------------

// FocusSession is a stretch of focused work (a Pomodoro): Minutes planned
// from StartedAt, on a task or not. EndedAt is set when it's stopped (early:
// the time so far counts) or its time is up (the scheduler ends it and tells
// its person). Its minutes are added to the task's FocusMinutes.
type FocusSession struct {
	Base
	UserID      string     `gorm:"size:36;not null;index" json:"user_id"`
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	TaskID      string     `gorm:"size:36;index" json:"task_id,omitempty"`
	Minutes     int        `gorm:"not null" json:"minutes"`
	StartedAt   time.Time  `gorm:"not null;index" json:"started_at"`
	EndedAt     *time.Time `gorm:"index" json:"ended_at,omitempty"`
	Spent       int        `gorm:"not null;default:0" json:"spent"` // minutes it lasted
}

// ---- Drawings ---------------------------------------------------------------

// Drawing is an Excalidraw scene: a whiteboard of a workspace, or — with
// NoteID — the drawing on a note (personal notes' drawings have no
// WorkspaceID and their note's OwnerID). Scene is Excalidraw's JSON
// (elements, app state, embedded files); Preview an SVG of it for cards
// (shown as an image, so it runs nothing); Text its words, for search.
// Version counts saves, as on pages (a stale save is refused).
type Drawing struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;default:'';index" json:"workspace_id"`
	OwnerID     string  `gorm:"size:36;not null;default:'';index" json:"owner_id,omitempty"`
	NoteID      string  `gorm:"size:36;not null;default:'';index" json:"note_id,omitempty"`
	FolderID    string  `gorm:"size:36;not null;default:'';index" json:"folder_id"` // its folder ("" at the top)
	Title       string  `gorm:"size:300" json:"title"`
	Scene       string  `gorm:"type:text" json:"scene,omitempty"`
	Preview     string  `gorm:"type:text" json:"preview,omitempty"`
	Text        string  `gorm:"type:text" json:"-"`
	Version     int     `gorm:"not null;default:1" json:"version"`
	Rank        float64 `gorm:"not null;default:0" json:"rank"`
	CreatedBy   string  `gorm:"size:36" json:"created_by"`
	UpdatedBy   string  `gorm:"size:36" json:"updated_by"`
}

// ---- Sheets -----------------------------------------------------------------

// Sheet is a spreadsheet: tabs of cells, as the app writes them (Data, JSON:
// each cell's input — a value or an "=" formula — and its look; the values
// are worked out in the app). Text is its words and numbers, for search.
// Version guards saves as for drawings.
type Sheet struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;index" json:"workspace_id"`
	FolderID    string  `gorm:"size:36;not null;default:'';index" json:"folder_id"` // its folder ("" at the top)
	Title       string  `gorm:"size:300" json:"title"`
	Data        string  `gorm:"type:text" json:"data,omitempty"`
	Text        string  `gorm:"type:text" json:"-"`
	Version     int     `gorm:"not null;default:1" json:"version"`
	Rank        float64 `gorm:"not null;default:0" json:"rank"`
	CreatedBy   string  `gorm:"size:36" json:"created_by"`
	UpdatedBy   string  `gorm:"size:36" json:"updated_by"`
}

// ---- Time blocks --------------------------------------------------------------

// TaskBlock is time a person set aside to work on a task (the week planner
// makes them): theirs alone, shown on their calendar. A task may have
// several (a long one split across the week).
type TaskBlock struct {
	Base
	WorkspaceID string    `gorm:"size:36;not null;index" json:"workspace_id"`
	TaskID      string    `gorm:"size:36;not null;index" json:"task_id"`
	UserID      string    `gorm:"size:36;not null;index" json:"-"`
	StartsAt    time.Time `gorm:"not null;index" json:"starts_at"`
	EndsAt      time.Time `gorm:"not null" json:"ends_at"`
}

// ---- Folders ----------------------------------------------------------------

// Folder holds a workspace's sheets, diagrams, whiteboards and pages (each
// says which, FolderID); folders go inside folders (ParentID, "" at the
// top). With a task list (ListID) it is a project: the list's tasks and the
// folder's files together.
type Folder struct {
	Base
	WorkspaceID string `gorm:"size:36;not null;index" json:"workspace_id"`
	ParentID    string `gorm:"size:36;not null;default:'';index" json:"parent_id"`
	Name        string `gorm:"size:200;not null" json:"name"`
	Color       string `gorm:"size:9" json:"color,omitempty"`
	ListID      string `gorm:"size:36;not null;default:'';index" json:"list_id"` // its task list ("" none): a folder and a list make a project; a list is in one folder at most
	CreatedBy   string `gorm:"size:36" json:"created_by"`
	OwnerID     string `gorm:"size:36;not null;default:'';index" json:"owner_id,omitempty"` // a personal folder's (My files: no workspace)
}

// ---- Diagrams ---------------------------------------------------------------

// Diagram is a draw.io or a Mermaid diagram (Kind): Data is its file
// (draw.io's XML, as the desktop app saves a .drawio; or Mermaid's text),
// Preview a picture of it (SVG, made by the editor) for lists and pages.
// Text is its labels, for search. Version guards saves as for drawings.
type Diagram struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;index" json:"workspace_id"`
	FolderID    string  `gorm:"size:36;not null;default:'';index" json:"folder_id"` // its folder ("" at the top)
	Kind        string  `gorm:"size:16;not null;default:'drawio'" json:"kind"`      // drawio | mermaid
	Title       string  `gorm:"size:300" json:"title"`
	Data        string  `gorm:"type:text" json:"data,omitempty"`
	Preview     string  `gorm:"type:text" json:"preview,omitempty"`
	Text        string  `gorm:"type:text" json:"-"`
	Version     int     `gorm:"not null;default:1" json:"version"`
	Rank        float64 `gorm:"not null;default:0" json:"rank"`
	CreatedBy   string  `gorm:"size:36" json:"created_by"`
	UpdatedBy   string  `gorm:"size:36" json:"updated_by"`
}

// The kinds of diagram.
const (
	DiagramDrawio  = "drawio"
	DiagramMermaid = "mermaid"
)

// ---- Calendar ---------------------------------------------------------------

// Event is a calendar entry. StartsAt / EndsAt are instants (UTC); an
// all-day event runs from the start date's midnight UTC to the end date's
// (exclusive). RRule repeats it in TZ (so "every Monday 09:00" stays 09:00
// across daylight saving); Until bounds the series for range queries (nil =
// forever) and ExDates (JSON []RFC3339) are occurrences removed from it.
type Event struct {
	Base
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	Title       string     `gorm:"size:300;not null" json:"title"`
	Notes       string     `gorm:"type:text" json:"notes"`
	Location    string     `gorm:"size:300" json:"location"`
	StartsAt    time.Time  `gorm:"not null;index" json:"starts_at"`
	EndsAt      time.Time  `gorm:"not null" json:"ends_at"`
	AllDay      bool       `gorm:"not null;default:false" json:"all_day"`
	TZ          string     `gorm:"size:64" json:"tz"`
	RRule       string     `gorm:"size:300" json:"rrule"`
	Until       *time.Time `gorm:"index" json:"until,omitempty"`
	ExDates     string     `gorm:"type:text" json:"-"`
	Color       string     `gorm:"size:9" json:"color"`
	// RemindMinutes reminds the workspace's members this many minutes before
	// each occurrence (nil = no reminder).
	RemindMinutes *int   `json:"remind_minutes"`
	CreatedBy     string `gorm:"size:36" json:"created_by"`
	// CourseID: a class of a course's timetable. The course owns it (its
	// slots make and change it); only single classes can be cancelled here.
	CourseID string `gorm:"size:36;index" json:"course_id,omitempty"`
}

// CalendarSubscription is a read-only ICS calendar shown in a workspace
// (a school timetable, a Google / iCloud / Outlook calendar's secret address).
// The URL is sealed: private calendar addresses are credentials. Its events
// are fetched into ExternalEvent, replaced on every refresh.
type CalendarSubscription struct {
	Base
	WorkspaceID    string     `gorm:"size:36;not null;index" json:"workspace_id"`
	Name           string     `gorm:"size:120;not null" json:"name"`
	URLSealed      []byte     `json:"-"`
	URLHost        string     `gorm:"size:255" json:"url_host"` // shown instead of the URL
	Color          string     `gorm:"size:9" json:"color"`
	RefreshMinutes int        `gorm:"not null;default:60" json:"refresh_minutes"`
	FetchedAt      *time.Time `json:"fetched_at,omitempty"`
	LastError      string     `gorm:"size:500" json:"last_error,omitempty"`
	EventCount     int        `gorm:"not null;default:0" json:"event_count"`
	CreatedBy      string     `gorm:"size:36" json:"created_by"`
}

// ExternalEvent is one occurrence fetched from a subscription (recurring
// series are expanded at fetch time over a fixed window).
type ExternalEvent struct {
	Base
	SubscriptionID string    `gorm:"size:36;not null;index" json:"subscription_id"`
	UID            string    `gorm:"size:255" json:"uid"`
	Title          string    `gorm:"size:300" json:"title"`
	Location       string    `gorm:"size:300" json:"location"`
	Notes          string    `gorm:"type:text" json:"notes"`
	StartsAt       time.Time `gorm:"not null;index" json:"starts_at"`
	EndsAt         time.Time `gorm:"not null" json:"ends_at"`
	AllDay         bool      `gorm:"not null;default:false" json:"all_day"`
}

// CalDAVLink keeps a workspace's calendar in step with one calendar on a
// CalDAV server (iCloud, Nextcloud, Fastmail…), both ways. The password —
// an app-specific one, for iCloud — is sealed.
type CalDAVLink struct {
	Base
	WorkspaceID string `gorm:"size:36;not null;uniqueIndex" json:"workspace_id"`
	// Provider: "" a CalDAV account (user name and password); "google" a
	// Google account (signed in; PasswordSealed holds its refresh token).
	Provider       string     `gorm:"size:12;not null;default:''" json:"provider"`
	CalendarURL    string     `gorm:"size:1000;not null" json:"calendar_url"`
	CalendarName   string     `gorm:"size:200" json:"calendar_name"`
	Host           string     `gorm:"size:255" json:"host"` // shown
	Username       string     `gorm:"size:255" json:"username"`
	PasswordSealed []byte     `json:"-"`
	CreatedBy      string     `gorm:"size:36" json:"created_by"`
	SyncedAt       *time.Time `json:"synced_at,omitempty"`
	LastError      string     `gorm:"size:500" json:"last_error,omitempty"`
	EventCount     int        `gorm:"not null;default:0" json:"event_count"`
}

// CalDAVItem pairs an event with its object on the server: its address, the
// version last seen (ETag), and the event's UpdatedAt when the two last
// agreed (Stamp) — later means it changed in Orbit since. An object that
// isn't an event (a to-do) has no EventID: it's only remembered, to skip.
type CalDAVItem struct {
	Base
	LinkID  string    `gorm:"size:36;not null;index"`
	EventID string    `gorm:"size:36;index"`
	URL     string    `gorm:"size:1000;not null"`
	ETag    string    `gorm:"size:200"`
	Stamp   time.Time `gorm:"not null"`
}

// CalendarFeed is a private ICS address a user adds to another calendar app
// (Apple Calendar, Google, Outlook) to see their Orbit events and due tasks.
// Only the sha256 of the token is stored, like API tokens.
type CalendarFeed struct {
	Base
	UserID       string     `gorm:"size:36;not null;index" json:"user_id"`
	Name         string     `gorm:"size:120" json:"name"`
	TokenHash    string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	Prefix       string     `gorm:"size:24" json:"prefix"`
	WorkspaceIDs string     `gorm:"type:text" json:"-"` // JSON []string; empty = all of the user's
	IncludeTasks bool       `gorm:"not null;default:true" json:"include_tasks"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}

// ---- Notifications ----------------------------------------------------------

// Notification is one entry of a user's inbox. It stores what happened, not
// the sentence (Kind, Verb and Params: who, which task, which workspace…), so
// the app and the server each word it in the reader's language. GroupKey lets
// a burst of the same activity (Ana added 5 tasks) update one entry.
type Notification struct {
	Base
	UserID      string     `gorm:"size:36;not null;index:idx_notif_user,priority:1" json:"-"`
	Kind        string     `gorm:"size:16;not null" json:"kind"` // reminder | assigned | workspace | activity | digest
	Verb        string     `gorm:"size:32;not null" json:"verb"` // e.g. task_assigned, added, task_done
	WorkspaceID string     `gorm:"size:36;index" json:"workspace_id"`
	ActorID     string     `gorm:"size:36" json:"actor_id"`
	Params      string     `gorm:"type:text" json:"-"`   // JSON object
	Link        string     `gorm:"size:300" json:"link"` // a path in the app
	GroupKey    string     `gorm:"size:120;index" json:"-"`
	Count       int        `gorm:"not null;default:1" json:"count"`
	ReadAt      *time.Time `gorm:"index:idx_notif_user,priority:2" json:"read_at"`
	// SnoozedUntil: put off until then (out of the list; it comes back
	// unread, and is sent again). Held: it came during quiet hours and
	// wasn't sent to devices or channels yet.
	SnoozedUntil *time.Time `gorm:"index" json:"snoozed_until,omitempty"`
	Held         bool       `gorm:"not null;default:false;index" json:"-"`
}

// PushSubscription is one browser (or installed app) that receives Web Push
// notifications for a user. Endpoint is the push service's address for it.
type PushSubscription struct {
	Base
	UserID     string     `gorm:"size:36;not null;index" json:"-"`
	Endpoint   string     `gorm:"size:1000;not null;uniqueIndex" json:"-"`
	P256dh     string     `gorm:"size:200;not null" json:"-"`
	Auth       string     `gorm:"size:100;not null" json:"-"`
	UserAgent  string     `gorm:"size:300" json:"user_agent"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// ---- Git ------------------------------------------------------------------------

// GitAccount is one user's access to a git server (Gitea / Forgejo): its
// base URL, their login there and a personal access token, sealed. Orbit
// acts on git as that user.
type GitAccount struct {
	Base
	UserID      string     `gorm:"size:36;not null;uniqueIndex:idx_git_account" json:"-"`
	Kind        string     `gorm:"size:16;not null;default:gitea" json:"kind"` // gitea (Gitea, Forgejo) | github | gitlab
	BaseURL     string     `gorm:"size:255;not null;uniqueIndex:idx_git_account" json:"base_url"`
	Login       string     `gorm:"size:255" json:"login"`
	TokenSealed []byte     `json:"-"`
	LastError   string     `gorm:"size:500" json:"last_error,omitempty"`
	CheckedAt   *time.Time `json:"checked_at,omitempty"`
	// Signed in with the server (OAuth): the access token expires and is
	// renewed with the refresh token. Via is "token" (pasted) or "oauth".
	Via           string     `gorm:"size:8" json:"via"`
	RefreshSealed []byte     `json:"-"`
	ExpiresAt     *time.Time `json:"-"`
}

// GitLink ties a task list to a repository: its issues are the list's
// tasks, both ways. Mode "issues" puts open issues in the first open column
// and closed ones in the done column; "labels" picks the column by a label
// (ColumnMap: column id → label name); "project" mirrors a project board
// (ColumnMap: column id → project column id). LinkedBy's account syncs it
// in the background; changes made in Orbit go out as whoever made them.
type GitLink struct {
	Base
	ListID      string `gorm:"size:36;not null;uniqueIndex" json:"list_id"`
	WorkspaceID string `gorm:"size:36;not null;index" json:"workspace_id"`
	Host        string `gorm:"size:255;not null" json:"host"`
	Owner       string `gorm:"size:255;not null" json:"owner"`
	Repo        string `gorm:"size:255;not null" json:"repo"`
	Mode        string `gorm:"size:16;not null" json:"mode"` // issues | labels | project
	ProjectID   int64  `gorm:"not null;default:0" json:"project_id,omitempty"`
	ColumnMap   string `gorm:"type:text" json:"-"`
	IgnoreIDs   string `gorm:"type:text" json:"-"` // JSON []int64: issues whose task was deleted here
	LinkedBy    string `gorm:"size:36;not null" json:"linked_by"`
	// A bot account (its token, sealed) for members without their own
	// account on the server: their changes go to git as the bot, never as
	// whoever linked the list. Without one, such members can only read.
	BotTokenSealed []byte `json:"-"`
	BotLogin       string `gorm:"size:255" json:"bot_login,omitempty"`
	// Instant updates: a webhook on the repository tells Orbit an issue
	// changed. HookSecretSealed signs its posts (HMAC-SHA256); HookID is the
	// hook Orbit made there (0: added by hand); HookAt the last post.
	HookSecretSealed []byte     `json:"-"`
	HookID           int64      `gorm:"not null;default:0" json:"-"`
	HookAt           *time.Time `json:"hook_at,omitempty"`
	SyncedAt         *time.Time `json:"synced_at,omitempty"`
	LastError        string     `gorm:"size:500" json:"last_error,omitempty"`
}

// ---- Uploads ---------------------------------------------------------------------

// Upload is a file someone added to a page, a task or a note: an image in a
// page, an attachment. It belongs to the workspace of what it's on ("" with
// OwnerID: a personal Quick Note). Its bytes are kept apart, by their SHA-256
// (once however many files share them), where the instance keeps files: the
// database (Blob), a directory or an S3 bucket (package uploads).
type Upload struct {
	Base
	WorkspaceID string `gorm:"size:36;index" json:"workspace_id"`
	OwnerID     string `gorm:"size:36;not null" json:"owner_id"`        // who uploaded it
	Target      string `gorm:"size:8;not null" json:"target"`           // page | task | note, or file (one of its own, in Files)
	TargetID    string `gorm:"size:36;not null;index" json:"target_id"` // the page, task or note (a file: its workspace, "me" a personal one)
	Name        string `gorm:"size:255;not null" json:"name"`
	Type        string `gorm:"size:100;not null" json:"type"` // sniffed, not the browser's word
	Size        int64  `gorm:"not null" json:"size"`
	SHA256      string `gorm:"size:64;index" json:"-"`
	// FolderID: a file of its own's folder ("" the top); an attachment shows
	// where what it's on is (a task: its list's folder; a page: its own).
	FolderID string `gorm:"size:36;not null;default:'';index" json:"folder_id"`
	// Lineage is the first version's id: the versions of one file (a file of
	// the same name added to the same place is its next version).
	Lineage    string     `gorm:"size:36;index" json:"lineage"`
	ReplacedAt *time.Time `gorm:"index" json:"replaced_at,omitempty"` // an older version
	TrashedAt  *time.Time `gorm:"index" json:"trashed_at,omitempty"`  // in the trash (emptied after 30 days)
	// What it was on, when that went for good (it became a file of its own).
	WasKind  string `gorm:"size:8" json:"was_kind,omitempty"`
	WasTitle string `gorm:"size:300" json:"was_title,omitempty"`
}

// Blob holds a file's bytes when they're kept in the database, by their
// SHA-256 (one row for every file with the same bytes).
type Blob struct {
	SHA256 string `gorm:"primaryKey;size:64"`
	Data   []byte `gorm:"not null"`
}

// UploadText is what a file says, for search: read from it by the server
// (Office documents, text) or the app (a PDF's, with pdf.js). ID = the
// upload's (each version its own).
type UploadText struct {
	ID   string `gorm:"primaryKey;size:36"`
	Text string `gorm:"type:text"`
}

// UploadBlob held one upload's bytes (ID = the upload's) before they were
// kept by their SHA-256; what's left is moved at start (uploads.Migrate).
type UploadBlob struct {
	ID   string `gorm:"primaryKey;size:36"`
	Data []byte `gorm:"not null"`
}

// WorkspaceInvite is a link that lets people join a workspace (with a role),
// creating an account if they have none. Only the token's SHA-256 is kept;
// it expires, can be used MaxUses times, and can be revoked.
type WorkspaceInvite struct {
	Base
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	TokenHash   string     `gorm:"size:64;not null;uniqueIndex" json:"-"`
	Prefix      string     `gorm:"size:12" json:"prefix"`        // the token's start, to tell links apart
	Role        string     `gorm:"size:10;not null" json:"role"` // editor | viewer
	CreatedBy   string     `gorm:"size:36;not null" json:"created_by"`
	ExpiresAt   time.Time  `json:"expires_at"`
	MaxUses     int        `gorm:"not null;default:1" json:"max_uses"`
	Uses        int        `gorm:"not null;default:0" json:"uses"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// ---- School: courses and grades ----------------------------------------------

// Course is a subject (Maths, Physics…) in a workspace with the Courses
// module: who teaches it, where, its weekly timetable (CourseSlot), the
// tasks tagged with Tag, and — with the Grades module — its marks. Weight
// is its share of the overall average (credits, a coefficient).
type Course struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;index" json:"workspace_id"`
	Name        string  `gorm:"size:120;not null" json:"name"`
	Code        string  `gorm:"size:16" json:"code"` // short name for the timetable (MAT)
	Teacher     string  `gorm:"size:120" json:"teacher"`
	Room        string  `gorm:"size:60" json:"room"`
	Color       string  `gorm:"size:9" json:"color"`
	Tag         string  `gorm:"size:40" json:"tag"`        // its tasks carry #tag
	TermStart   *string `gorm:"size:10" json:"term_start"` // YYYY-MM-DD: classes from
	TermEnd     *string `gorm:"size:10" json:"term_end"`   // YYYY-MM-DD: classes until (inclusive)
	TZ          string  `gorm:"size:64" json:"tz"`         // the zone its class times are in
	Weight      float64 `gorm:"not null;default:1" json:"weight"`
	Notes       string  `gorm:"type:text" json:"notes"`
	Rank        float64 `gorm:"not null;default:0" json:"rank"`
	CreatedBy   string  `gorm:"size:36" json:"created_by"`
	PageID      *string `gorm:"size:36" json:"page_id"` // its notes page
}

// CourseSlot is one weekly class: a weekday (1 Monday … 7 Sunday) and times
// (HH:MM, in the course's zone). EventID is the repeating event it keeps on
// the calendar.
type CourseSlot struct {
	Base
	CourseID    string `gorm:"size:36;not null;index" json:"course_id"`
	WorkspaceID string `gorm:"size:36;not null;index" json:"workspace_id"`
	Weekday     int    `gorm:"not null" json:"weekday"`
	Start       string `gorm:"size:5;not null" json:"start"`
	End         string `gorm:"size:5;not null" json:"end"`
	Room        string `gorm:"size:60" json:"room"` // when not the course's
	Kind        string `gorm:"size:40" json:"kind"` // Lab, Tutorial… (optional)
	EventID     string `gorm:"size:36" json:"event_id"`
}

// Grade is one mark someone got in a course: Score out of OutOf, counting
// Weight in the course's average. Grades are their owner's alone — in a
// shared workspace, each member sees only their own.
type Grade struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;index" json:"workspace_id"`
	CourseID    string  `gorm:"size:36;not null;index" json:"course_id"`
	OwnerID     string  `gorm:"size:36;not null;index" json:"owner_id"`
	Title       string  `gorm:"size:200;not null" json:"title"`
	Score       float64 `gorm:"not null" json:"score"`
	OutOf       float64 `gorm:"not null" json:"out_of"`
	Weight      float64 `gorm:"not null;default:1" json:"weight"`
	Date        *string `gorm:"size:10" json:"date"` // YYYY-MM-DD
}

// CourseGoal is the average someone aims at in a course (on the workspace's
// scale) — personal, like their grades.
type CourseGoal struct {
	CourseID string  `gorm:"primaryKey;size:36" json:"course_id"`
	UserID   string  `gorm:"primaryKey;size:36" json:"user_id"`
	Target   float64 `gorm:"not null" json:"target"`
}

// TaskComment is a comment on a task, in Markdown, by a member of its
// workspace (viewers too). EditedAt is set when its author changed it.
type TaskComment struct {
	Base
	TaskID      string     `gorm:"size:36;not null;index" json:"task_id"`
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	AuthorID    string     `gorm:"size:36;not null;index" json:"author_id"`
	Body        string     `gorm:"type:text;not null" json:"body"`
	EditedAt    *time.Time `json:"edited_at,omitempty"`
	// The issue comment it is, on a git-linked task; GitAuthor is the git
	// login of someone who isn't a member (AuthorID is then empty).
	GitCommentID int64  `gorm:"not null;default:0;index" json:"-"`
	GitHash      string `gorm:"size:16" json:"-"`
	GitAuthor    string `gorm:"size:100" json:"git_author,omitempty"`
	// AuthorLabel names an author with no account here (a comment that came
	// with an imported workspace); AuthorID is then empty.
	AuthorLabel string `gorm:"size:100" json:"-"`
}

// TaskEvent is one line of a task's activity: who did what, when (created,
// renamed, assigned, moved, completed…). Params holds what's shown (names at
// the time, the new date…) as JSON.
type TaskEvent struct {
	Base
	TaskID      string `gorm:"size:36;not null;index" json:"task_id"`
	WorkspaceID string `gorm:"size:36;not null;index" json:"workspace_id"`
	ActorID     string `gorm:"size:36" json:"actor_id"`
	Kind        string `gorm:"size:20;not null" json:"kind"`
	Params      string `gorm:"type:text" json:"-"`
}

// PersonalChannel is one of a person's own notification channels (their
// Discord webhook, ntfy topic, Telegram chat…), besides the owner's shared
// ones. Config is the settings, sealed; Hint is what the page shows instead
// (a host, a topic).
type PersonalChannel struct {
	Base
	UserID       string `gorm:"size:36;not null;index" json:"-"`
	Kind         string `gorm:"size:16;not null" json:"kind"`
	Name         string `gorm:"size:60" json:"name"`
	Hint         string `gorm:"size:200" json:"hint"`
	ConfigSealed []byte `json:"-"`
	LastError    string `gorm:"size:300" json:"last_error,omitempty"`
}

// Activity is one line of a workspace's feed: who did what ("Ana completed
// a task: Shopping"), kept 90 days. Verb is one of the activity verbs
// (task_added, task_done, page_added, note_added, event_added,
// member_joined, commented); Link opens what it's about.
type Activity struct {
	Base
	WorkspaceID string `gorm:"size:36;not null;index:idx_activity_ws_time,priority:1" json:"workspace_id"`
	ActorID     string `gorm:"size:36" json:"actor_id"`
	Verb        string `gorm:"size:20;not null" json:"verb"`
	Title       string `gorm:"size:300" json:"title"`
	Link        string `gorm:"size:300" json:"link"`
}

// AssigneeList is the task's assignees (user ids), never nil.
func (t Task) AssigneeList() []string {
	out := []string{}
	if t.Assignees != "" {
		_ = json.Unmarshal([]byte(t.Assignees), &out)
	}
	return out
}

// SetAssignees stores ids (in order, without duplicates or blanks).
func (t *Task) SetAssignees(ids []string) {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	b, _ := json.Marshal(out)
	t.Assignees = string(b)
}

// WaitsOnList is the tasks it waits on (ids), never nil.
func (t Task) WaitsOnList() []string {
	out := []string{}
	if t.WaitsOn != "" {
		_ = json.Unmarshal([]byte(t.WaitsOn), &out)
	}
	return out
}

// SetWaitsOn stores ids (in order, without duplicates, blanks or itself).
func (t *Task) SetWaitsOn(ids []string) {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		if id != "" && id != t.ID && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		t.WaitsOn = ""
		return
	}
	b, _ := json.Marshal(out)
	t.WaitsOn = string(b)
}

// AssignedTo reports whether uid is one of the task's assignees.
func (t Task) AssignedTo(uid string) bool {
	for _, id := range t.AssigneeList() {
		if id == uid {
			return true
		}
	}
	return false
}

// MineOrUnassigned is a where clause for tasks assigned to a user (the one
// argument, AssigneeArg(uid)) or to nobody.
const MineOrUnassigned = "(assignees IS NULL OR assignees = '' OR assignees = '[]' OR assignees LIKE ?)"

// AssigneeArg is MineOrUnassigned's argument for uid.
func AssigneeArg(uid string) string { return `%"` + uid + `"%` }

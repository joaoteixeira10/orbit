package models

import "time"

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
	WorkspaceID string     `gorm:"size:36;not null;index" json:"workspace_id"`
	ListID      string     `gorm:"size:36;not null;index" json:"list_id"`
	ColumnID    string     `gorm:"size:36;not null" json:"column_id"`
	ParentID    *string    `gorm:"size:36;index" json:"parent_id"`
	Title       string     `gorm:"size:500;not null" json:"title"`
	Notes       string     `gorm:"type:text" json:"notes"`
	Priority    int        `gorm:"not null;default:0" json:"priority"` // 0 none, 1 low, 2 medium, 3 high
	Tags        string     `gorm:"type:text" json:"-"`                 // JSON []string
	Start       *string    `gorm:"size:10" json:"start"`               // YYYY-MM-DD, on or before Due
	Due         *string    `gorm:"size:10;index" json:"due"`           // YYYY-MM-DD
	DueTime     *string    `gorm:"size:5" json:"due_time"`             // HH:MM
	Repeat      string     `gorm:"size:200" json:"repeat"`
	Rank        float64    `gorm:"not null;default:0" json:"rank"`
	CompletedAt *time.Time `gorm:"index" json:"completed_at"`
	// AssigneeID is who it's for in a shared workspace (reminders go to them;
	// nil = to whoever created it).
	AssigneeID *string `gorm:"size:36;index" json:"assignee_id"`
	CreatedBy  string  `gorm:"size:36" json:"created_by"`
}

// ---- Quick notes ------------------------------------------------------------

// Note is a quick note (a sticky note): a short text or a checklist on a
// coloured card. Items (JSON [{id, text, done}]) is set for a checklist note,
// which then has no Body. Pinned notes come first; Rank orders the rest.
// Longer writing belongs in a Page.
type Note struct {
	Base
	WorkspaceID string  `gorm:"size:36;not null;index" json:"workspace_id"`
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
}

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

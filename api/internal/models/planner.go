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

// ---- Git ------------------------------------------------------------------------

// GitAccount is one user's access to a git server (Gitea / Forgejo): its
// base URL, their login there and a personal access token, sealed. Orbit
// acts on git as that user.
type GitAccount struct {
	Base
	UserID      string     `gorm:"size:36;not null;uniqueIndex:idx_git_account" json:"-"`
	Kind        string     `gorm:"size:16;not null;default:gitea" json:"kind"` // gitea (github later)
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
	BotTokenSealed []byte     `json:"-"`
	BotLogin       string     `gorm:"size:255" json:"bot_login,omitempty"`
	SyncedAt       *time.Time `json:"synced_at,omitempty"`
	LastError      string     `gorm:"size:500" json:"last_error,omitempty"`
}

// ---- Uploads ---------------------------------------------------------------------

// Upload is a file someone added to a page, a task or a note: an image in a
// page, an attachment. It belongs to the workspace of what it's on ("" with
// OwnerID: a personal Quick Note). Its bytes are in UploadBlob (apart, so
// listings stay light), in the database, so backups carry them.
type Upload struct {
	Base
	WorkspaceID string `gorm:"size:36;index" json:"workspace_id"`
	OwnerID     string `gorm:"size:36;not null" json:"owner_id"`        // who uploaded it
	Target      string `gorm:"size:8;not null" json:"target"`           // page | task | note
	TargetID    string `gorm:"size:36;not null;index" json:"target_id"` // the page, task or note
	Name        string `gorm:"size:255;not null" json:"name"`
	Type        string `gorm:"size:100;not null" json:"type"` // sniffed, not the browser's word
	Size        int64  `gorm:"not null" json:"size"`
	SHA256      string `gorm:"size:64" json:"-"`
}

// UploadBlob holds an upload's bytes (ID = the upload's).
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

// Package transfer moves a workspace from one Orbit to another: Export writes
// it as a zip — workspace.json (lists, tasks, pages, notes, events, courses,
// comments, who's who) and files/<id> (its uploads) — and Import makes a new
// workspace from one, with new ids throughout (links between its pages and to
// its files follow). People are matched by username, then email, to accounts
// on this instance; who isn't found is left out (a comment keeps their name).
//
// Not carried: calendar subscriptions (their addresses are secrets), git
// links (tokens), invites, the trash, the activity history, notifications,
// and other people's grades (grades are personal: the exporter's go with it).
package transfer

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/holidays"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// Format names the archive; Version its layout (Import reads up to it).
	Format  = "orbit-workspace"
	Version = 1

	manifest = "workspace.json"
	filesDir = "files/"
)

// Person is someone the workspace mentions: a member (with a role) or the
// author of something in it.
type Person struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role,omitempty"` // owner | editor | viewer; "" not a member
}

// The models, with the fields their JSON hides (kept as JSON in the database).
type (
	xWorkspace struct {
		models.Workspace
		Modules []string `json:"modules"`
	}
	xList struct {
		models.TaskList
		Columns json.RawMessage `json:"columns"`
	}
	xTask struct {
		models.Task
		Tags      json.RawMessage `json:"tags"`
		Assignees []string        `json:"assignees"`
		WaitsOn   []string        `json:"waits_on,omitempty"`
	}
	xNote struct {
		models.Note
		Items json.RawMessage `json:"items"`
	}
	xEvent struct {
		models.Event
		ExDates json.RawMessage `json:"exdates"`
	}
)

// Archive is workspace.json.
type Archive struct {
	Format         string               `json:"format"`
	Version        int                  `json:"version"`
	ExportedAt     time.Time            `json:"exported_at"`
	App            string               `json:"app_version"`
	Workspace      xWorkspace           `json:"workspace"`
	People         []Person             `json:"people"`
	Lists          []xList              `json:"lists"`
	Tasks          []xTask              `json:"tasks"`
	Pages          []models.Page        `json:"pages"`
	Notes          []xNote              `json:"notes"`
	Events         []xEvent             `json:"events"`
	Courses        []models.Course      `json:"courses"`
	Slots          []models.CourseSlot  `json:"course_slots"`
	Comments       []models.TaskComment `json:"comments"`
	Grades         []models.Grade       `json:"grades"`
	Goals          []models.CourseGoal  `json:"goals"`
	Files          []models.Upload      `json:"files"`
	Drawings       []models.Drawing     `json:"drawings,omitempty"` // whiteboards and notes' drawings
	WorkspaceGoals []models.Goal        `json:"workspace_goals,omitempty"`
	Bookmarks      []models.Bookmark    `json:"bookmarks,omitempty"` // their pictures are read again
	// The exporter's own habits and the days they were done.
	Habits      []models.Habit      `json:"habits,omitempty"`
	HabitChecks []models.HabitCheck `json:"habit_checks,omitempty"`
	// The exporter's own journal.
	Journal []models.JournalEntry `json:"journal,omitempty"`
	// The shared smart lists and the exporter's own.
	SmartLists []xSmartList `json:"smart_lists,omitempty"`
}

type xSmartList struct {
	models.SmartList
	Filter json.RawMessage `json:"filter"`
}

// smartFilter rewrites a smart list's filter for its new home: lists by
// their new ids, people by who they are here (those not here are dropped).
func smartFilter(old string, list func(string) string, person map[string]string) string {
	var f map[string]any
	if json.Unmarshal([]byte(old), &f) != nil {
		return ""
	}
	remap := func(key string, to func(string) string) {
		arr, _ := f[key].([]any)
		out := []string{}
		for _, v := range arr {
			if s, ok := v.(string); ok {
				if n := to(s); n != "" {
					out = append(out, n)
				}
			}
		}
		f[key] = out
	}
	remap("lists", list)
	remap("assignee", func(s string) string {
		if s == "me" || s == "none" {
			return s
		}
		return person[s]
	})
	b, _ := json.Marshal(f)
	return string(b)
}

func raw(s string) json.RawMessage {
	if strings.TrimSpace(s) == "" {
		return json.RawMessage("null")
	}
	return json.RawMessage(s)
}

func unraw(r json.RawMessage) string {
	if s := strings.TrimSpace(string(r)); s != "" && s != "null" {
		return s
	}
	return ""
}

// Export writes workspace wsID as a zip to w. uid is who exports it (their
// own grades go with it).
func Export(db *gorm.DB, wsID, uid, app string, w io.Writer, now time.Time) error {
	var ws models.Workspace
	if err := db.First(&ws, "id = ?", wsID).Error; err != nil {
		return err
	}
	a := Archive{Format: Format, Version: Version, ExportedAt: now.UTC(), App: app}
	var mods []string
	_ = json.Unmarshal([]byte(ws.Modules), &mods)
	a.Workspace = xWorkspace{Workspace: ws, Modules: mods}

	var lists []models.TaskList
	db.Where("workspace_id = ?", wsID).Order("rank").Find(&lists)
	for _, l := range lists {
		a.Lists = append(a.Lists, xList{TaskList: l, Columns: raw(l.Columns)})
	}
	var tasks []models.Task
	db.Where("workspace_id = ?", wsID).Order("rank").Find(&tasks)
	for _, t := range tasks {
		var who []string
		_ = json.Unmarshal([]byte(t.Assignees), &who)
		a.Tasks = append(a.Tasks, xTask{Task: t, Tags: raw(t.Tags), Assignees: who, WaitsOn: t.WaitsOnList()})
	}
	db.Where("workspace_id = ? AND trashed_at IS NULL", wsID).Order("rank").Find(&a.Pages)
	var notes []models.Note
	db.Where("workspace_id = ?", wsID).Order("rank").Find(&notes)
	for _, n := range notes {
		a.Notes = append(a.Notes, xNote{Note: n, Items: raw(n.Items)})
	}
	var events []models.Event
	db.Where("workspace_id = ?", wsID).Order("starts_at").Find(&events)
	for _, e := range events {
		a.Events = append(a.Events, xEvent{Event: e, ExDates: raw(e.ExDates)})
	}
	db.Where("workspace_id = ?", wsID).Order("rank").Find(&a.Courses)
	db.Where("workspace_id = ?", wsID).Find(&a.Slots)
	db.Where("workspace_id = ?", wsID).Order("created_at").Find(&a.Comments)
	db.Where("workspace_id = ? AND owner_id = ?", wsID, uid).Order("date, created_at").Find(&a.Grades)
	courseIDs := []string{""}
	for _, c := range a.Courses {
		courseIDs = append(courseIDs, c.ID)
	}
	db.Where("course_id IN ? AND user_id = ?", courseIDs, uid).Find(&a.Goals)
	db.Where("workspace_id = ?", wsID).Order("created_at").Find(&a.Files)
	db.Where("workspace_id = ?", wsID).Order("created_at").Find(&a.Drawings)
	db.Where("workspace_id = ?", wsID).Order("rank").Find(&a.WorkspaceGoals)
	db.Where("workspace_id = ?", wsID).Order("created_at").Find(&a.Bookmarks)
	db.Where("workspace_id = ? AND owner_id = ?", wsID, uid).Order("rank").Find(&a.Habits)
	habitIDs := []string{""}
	for _, hb := range a.Habits {
		habitIDs = append(habitIDs, hb.ID)
	}
	db.Where("habit_id IN ?", habitIDs).Order("date").Find(&a.HabitChecks)
	db.Where("workspace_id = ? AND owner_id = ?", wsID, uid).Order("date").Find(&a.Journal)
	var smart []models.SmartList
	db.Where("workspace_id = ? AND (shared = ? OR owner_id = ?)", wsID, true, uid).Order("rank").Find(&smart)
	for _, l := range smart {
		a.SmartLists = append(a.SmartLists, xSmartList{SmartList: l, Filter: raw(l.Filter)})
	}
	a.People = people(db, wsID, &a)

	z := zip.NewWriter(w)
	f, err := z.Create(manifest)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", " ")
	if err := enc.Encode(a); err != nil {
		return err
	}
	for _, u := range a.Files {
		var blob models.UploadBlob
		if db.First(&blob, "id = ?", u.ID).Error != nil {
			continue
		}
		f, err := z.CreateHeader(&zip.FileHeader{Name: filesDir + u.ID, Method: zip.Deflate, Modified: u.CreatedAt})
		if err != nil {
			return err
		}
		if _, err := f.Write(blob.Data); err != nil {
			return err
		}
	}
	return z.Close()
}

// people lists the members and everyone else the archive mentions.
func people(db *gorm.DB, wsID string, a *Archive) []Person {
	role := map[string]string{}
	var members []models.WorkspaceMember
	db.Where("workspace_id = ?", wsID).Find(&members)
	ids := []string{}
	add := func(id string) {
		if id != "" {
			ids = append(ids, id)
		}
	}
	for _, m := range members {
		role[m.UserID] = m.Role
		add(m.UserID)
	}
	add(a.Workspace.CreatedBy)
	for _, t := range a.Tasks {
		add(t.CreatedBy)
		for _, id := range t.Assignees {
			add(id)
		}
	}
	for _, p := range a.Pages {
		add(p.CreatedBy)
		add(p.UpdatedBy)
	}
	for _, n := range a.Notes {
		add(n.CreatedBy)
		add(n.UpdatedBy)
	}
	for _, e := range a.Events {
		add(e.CreatedBy)
	}
	for _, c := range a.Courses {
		add(c.CreatedBy)
	}
	for _, c := range a.Comments {
		add(c.AuthorID)
	}
	for _, u := range a.Files {
		add(u.OwnerID)
	}
	for _, d := range a.Drawings {
		add(d.CreatedBy)
		add(d.UpdatedBy)
	}
	var users []models.User
	db.Where("id IN ?", append(ids, "")).Order("username").Find(&users)
	out := make([]Person, 0, len(users))
	for _, u := range users {
		out = append(out, Person{ID: u.ID, Username: u.Username, Email: u.Email, Name: u.DisplayName, Role: role[u.ID]})
	}
	return out
}

// ---- Import ----

// ErrFormat: not a workspace archive Orbit can read.
var ErrFormat = errors.New("this isn't an Orbit workspace export")

// Read opens an archive and reads its workspace.json (at most maxJSON bytes).
func Read(r io.ReaderAt, size, maxJSON int64) (*Archive, *zip.Reader, error) {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return nil, nil, ErrFormat
	}
	f, err := z.Open(manifest)
	if err != nil {
		return nil, nil, ErrFormat
	}
	defer func() { _ = f.Close() }()
	var a Archive
	dec := json.NewDecoder(io.LimitReader(f, maxJSON))
	if err := dec.Decode(&a); err != nil || a.Format != Format {
		return nil, nil, ErrFormat
	}
	if a.Version < 1 || a.Version > Version {
		return nil, nil, fmt.Errorf("this export is from a newer Orbit (format %d); update this one first", a.Version)
	}
	return &a, z, nil
}

// Options say who imports and the limits.
type Options struct {
	UserID    string // the importer: the new workspace's owner
	AddPeople bool   // add the members found here, with their roles
	Quota     int64  // the instance's upload quota (0 none)
	MaxFile   int64  // the largest file taken
}

// Report says what came in.
type Report struct {
	Lists    int      `json:"lists"`
	Tasks    int      `json:"tasks"`
	Pages    int      `json:"pages"`
	Notes    int      `json:"notes"`
	Events   int      `json:"events"`
	Courses  int      `json:"courses"`
	Comments int      `json:"comments"`
	Files    int      `json:"files"`
	Drawings int      `json:"drawings"`
	Added    []string `json:"added"`   // members added (usernames)
	Missing  []string `json:"missing"` // people with no account here (names)
	Skipped  []string `json:"skipped"` // files not taken (too large, quota)
}

// Import makes a new workspace from an archive read by Read.
func Import(db *gorm.DB, a *Archive, z *zip.Reader, o Options) (models.Workspace, Report, error) {
	var rep Report
	newID := map[string]string{}
	fresh := func(old string) string {
		if old == "" {
			return ""
		}
		if id, ok := newID[old]; ok {
			return id
		}
		id := uuid.Must(uuid.NewV7()).String()
		newID[old] = id
		return id
	}
	// Every id first, so the text rewrite knows them all.
	wsID := fresh(a.Workspace.ID)
	for _, x := range a.Lists {
		fresh(x.ID)
	}
	for _, x := range a.Tasks {
		fresh(x.ID)
	}
	for _, x := range a.Pages {
		fresh(x.ID)
	}
	for _, x := range a.Notes {
		fresh(x.ID)
	}
	for _, x := range a.Events {
		fresh(x.ID)
	}
	for _, x := range a.Courses {
		fresh(x.ID)
	}
	for _, x := range a.Files {
		fresh(x.ID)
	}
	for _, x := range a.Drawings {
		fresh(x.ID)
	}
	for _, x := range a.Habits {
		fresh(x.ID)
	}
	for _, x := range a.WorkspaceGoals {
		fresh(x.ID)
	}
	pairs := make([]string, 0, 2*len(newID))
	for o, n := range newID {
		if len(o) >= 8 { // ids, not stray short strings
			pairs = append(pairs, o, n)
		}
	}
	rewrite := strings.NewReplacer(pairs...).Replace
	ref := func(old string) string { return newID[old] } // "" when not in the archive
	refp := func(old *string) *string {
		if old == nil || newID[*old] == "" {
			return nil
		}
		s := newID[*old]
		return &s
	}

	// People: matched by username, then email; the importer is always them.
	user := map[string]string{} // old user id → user id here
	label := map[string]string{}
	var me models.User
	if err := db.First(&me, "id = ?", o.UserID).Error; err != nil {
		return models.Workspace{}, rep, err
	}
	members := map[string]string{} // user id here → role
	for _, p := range a.People {
		label[p.ID] = p.Name
		if label[p.ID] == "" {
			label[p.ID] = p.Username
		}
		var u models.User
		found := false
		for _, q := range []string{p.Username, p.Email} {
			if q = strings.ToLower(strings.TrimSpace(q)); q != "" && db.Where("disabled = ? AND (LOWER(username) = ? OR LOWER(email) = ?)", false, q, q).First(&u).Error == nil {
				found = true
				break
			}
		}
		if !found {
			rep.Missing = append(rep.Missing, label[p.ID])
			continue
		}
		user[p.ID] = u.ID
		if u.ID != me.ID && o.AddPeople && (p.Role == models.WSOwner || p.Role == models.WSEditor || p.Role == models.WSViewer) {
			members[u.ID] = p.Role
		}
	}
	members[me.ID] = models.WSOwner
	who := func(old string) string { // an author: found here, else the importer
		if id := user[old]; id != "" {
			return id
		}
		return me.ID
	}

	src := a.Workspace
	ws := models.Workspace{
		Name: trim(strings.TrimSpace(src.Name), 120), Icon: trim(src.Icon, 64), Color: src.Color, Kind: src.Kind,
		Description: trim(src.Description, 500), CreatedBy: me.ID, GradeScale: src.GradeScale, Holidays: src.Holidays,
	}
	ws.ID = wsID
	if ws.Name == "" {
		ws.Name = "Imported"
	}
	tpl := planner.TemplateFor(ws.Kind)
	ws.Kind = tpl.Kind
	if !planner.ValidColor(ws.Color) {
		ws.Color = tpl.Color
	}
	if !holidays.Known(ws.Holidays) {
		ws.Holidays = ""
	}
	if !planner.ValidScale(ws.GradeScale) {
		ws.GradeScale = 20
	}
	mods, _ := planner.CleanModules(src.Modules)
	ws.Modules = encode(mods)

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&ws).Error; err != nil {
			return err
		}
		for uid, role := range members {
			var last struct{ Max *float64 }
			tx.Model(&models.WorkspaceMember{}).Select("MAX(position) AS max").Where("user_id = ?", uid).Scan(&last)
			if err := tx.Create(&models.WorkspaceMember{WorkspaceID: ws.ID, UserID: uid, Role: role, Position: planner.Rank(last.Max, nil)}).Error; err != nil {
				return err
			}
			if uid != me.ID {
				var u models.User
				tx.Select("username").First(&u, "id = ?", uid)
				rep.Added = append(rep.Added, u.Username)
			}
		}

		for _, x := range a.Bookmarks {
			b := x
			b.ID, b.WorkspaceID, b.CreatedBy, b.HasImage = "", ws.ID, who(x.CreatedBy), false
			if err := tx.Create(&b).Error; err != nil {
				return err
			}
		}
		for _, x := range a.WorkspaceGoals {
			g := x
			g.ID, g.WorkspaceID, g.CreatedBy = ref(x.ID), ws.ID, who(x.CreatedBy)
			if err := tx.Create(&g).Error; err != nil {
				return err
			}
		}
		for _, x := range a.SmartLists {
			l := x.SmartList
			l.ID, l.WorkspaceID, l.Name, l.OwnerID = "", ws.ID, trim(strings.TrimSpace(l.Name), 100), me.ID
			if l.Shared {
				l.OwnerID = who(x.OwnerID)
			}
			if l.Name == "" {
				continue
			}
			if !validColor(l.Color) {
				l.Color = ""
			}
			l.Filter = smartFilter(unraw(x.Filter), ref, user)
			if err := tx.Create(&l).Error; err != nil {
				return err
			}
		}
		for _, x := range a.Lists {
			l := x.TaskList
			l.ID, l.WorkspaceID, l.Columns = ref(x.ID), ws.ID, unraw(x.Columns)
			if !validColor(l.Color) {
				l.Color = ""
			}
			if err := tx.Create(&l).Error; err != nil {
				return err
			}
			rep.Lists++
		}
		for _, x := range a.Pages {
			p := x
			p.ID, p.WorkspaceID, p.ParentID = ref(x.ID), ws.ID, refp(x.ParentID)
			p.Content = rewrite(p.Content)
			p.ContentText = planner.DocText(p.Content)
			p.CreatedBy, p.UpdatedBy, p.TrashedAt = who(x.CreatedBy), who(x.UpdatedBy), nil
			if err := tx.Create(&p).Error; err != nil {
				return err
			}
			rep.Pages++
		}
		for _, x := range a.Tasks {
			t := x.Task
			if ref(t.ListID) == "" {
				continue
			}
			t.ID, t.WorkspaceID, t.ListID, t.ParentID = ref(x.ID), ws.ID, ref(x.ListID), refp(x.ParentID)
			t.Notes, t.Tags, t.CreatedBy = rewrite(t.Notes), unraw(x.Tags), who(x.CreatedBy)
			var as []string
			for _, old := range x.Assignees {
				if id := user[old]; id != "" && members[id] != "" {
					as = append(as, id)
				}
			}
			t.Assignees = encode(as)
			t.GitDirty, t.GitDirtyBy = false, ""
			t.GoalID = ref(x.GoalID)
			var waits []string
			for _, old := range x.WaitsOn {
				if id := ref(old); id != "" {
					waits = append(waits, id)
				}
			}
			t.SetWaitsOn(waits)
			if err := tx.Create(&t).Error; err != nil {
				return err
			}
			rep.Tasks++
		}
		for _, x := range a.Notes {
			n := x.Note
			n.ID, n.WorkspaceID, n.OwnerID = ref(x.ID), ws.ID, ""
			n.Body, n.Items = rewrite(n.Body), rewrite(unraw(x.Items))
			n.PlainText = n.Body
			if n.Checklist {
				n.PlainText = itemsText(n.Items)
			}
			n.CreatedBy, n.UpdatedBy = who(x.CreatedBy), who(x.UpdatedBy)
			if err := tx.Create(&n).Error; err != nil {
				return err
			}
			rep.Notes++
		}
		for _, x := range a.Courses {
			c := x
			c.ID, c.WorkspaceID, c.PageID, c.CreatedBy = ref(x.ID), ws.ID, refp(x.PageID), who(x.CreatedBy)
			c.Notes = rewrite(c.Notes)
			if !validColor(c.Color) {
				c.Color = ""
			}
			if err := tx.Create(&c).Error; err != nil {
				return err
			}
			rep.Courses++
		}
		for _, x := range a.Events {
			e := x.Event
			e.ID, e.WorkspaceID, e.CourseID, e.CreatedBy = ref(x.ID), ws.ID, ref(x.CourseID), who(x.CreatedBy)
			e.Notes, e.ExDates = rewrite(e.Notes), unraw(x.ExDates)
			if !validColor(e.Color) {
				e.Color = ""
			}
			if err := tx.Create(&e).Error; err != nil {
				return err
			}
			rep.Events++
		}
		for _, x := range a.Slots {
			s := x
			if ref(s.CourseID) == "" {
				continue
			}
			s.ID, s.WorkspaceID, s.CourseID, s.EventID = "", ws.ID, ref(x.CourseID), ref(x.EventID)
			if err := tx.Create(&s).Error; err != nil {
				return err
			}
		}
		for _, x := range a.Comments {
			c := x
			if ref(c.TaskID) == "" {
				continue
			}
			c.ID, c.WorkspaceID, c.TaskID, c.Body = "", ws.ID, ref(x.TaskID), rewrite(x.Body)
			c.AuthorID, c.GitCommentID, c.GitHash = user[x.AuthorID], 0, ""
			if c.AuthorID == "" && c.GitAuthor == "" {
				c.AuthorLabel = trim(label[x.AuthorID], 100)
			}
			if err := tx.Create(&c).Error; err != nil {
				return err
			}
			rep.Comments++
		}
		for _, x := range a.Grades {
			g := x
			if ref(g.CourseID) == "" {
				continue
			}
			g.ID, g.WorkspaceID, g.CourseID, g.OwnerID = "", ws.ID, ref(x.CourseID), me.ID
			if err := tx.Create(&g).Error; err != nil {
				return err
			}
		}
		for _, x := range a.Goals {
			if ref(x.CourseID) == "" {
				continue
			}
			if err := tx.Create(&models.CourseGoal{CourseID: ref(x.CourseID), UserID: me.ID, Target: x.Target}).Error; err != nil {
				return err
			}
		}
		for _, x := range a.Drawings {
			d := x
			if x.NoteID != "" && ref(x.NoteID) == "" {
				continue
			}
			d.ID, d.WorkspaceID, d.OwnerID, d.NoteID = ref(x.ID), ws.ID, "", ref(x.NoteID)
			d.CreatedBy, d.UpdatedBy = who(x.CreatedBy), who(x.UpdatedBy)
			if err := tx.Create(&d).Error; err != nil {
				return err
			}
			rep.Drawings++
		}
		for _, x := range a.Habits {
			hb := x
			hb.ID, hb.WorkspaceID, hb.OwnerID = ref(x.ID), ws.ID, me.ID
			if err := tx.Create(&hb).Error; err != nil {
				return err
			}
		}
		for _, x := range a.Journal {
			e := models.JournalEntry{WorkspaceID: ws.ID, OwnerID: me.ID, Date: x.Date, Content: rewrite(x.Content), Mood: x.Mood}
			e.ContentText = planner.MarkdownText(e.Content)
			if err := tx.Create(&e).Error; err != nil {
				return err
			}
		}
		for _, x := range a.HabitChecks {
			if ref(x.HabitID) == "" {
				continue
			}
			if err := tx.Create(&models.HabitCheck{HabitID: ref(x.HabitID), Date: x.Date}).Error; err != nil {
				return err
			}
		}
		return importFiles(tx, a, z, o, ws.ID, ref, who, &rep)
	})
	if err != nil {
		return models.Workspace{}, rep, err
	}
	return ws, rep, nil
}

// importFiles stores the uploads (within the quota and the file size cap).
func importFiles(tx *gorm.DB, a *Archive, z *zip.Reader, o Options, wsID string, ref, who func(string) string, rep *Report) error {
	byName := map[string]*zip.File{}
	for _, f := range z.File {
		byName[f.Name] = f
	}
	for _, x := range a.Files {
		target := ref(x.TargetID)
		f := byName[filesDir+x.ID]
		if target == "" || f == nil || (x.Target != uploads.Page && x.Target != uploads.Task && x.Target != uploads.Note) {
			continue
		}
		if o.MaxFile > 0 && f.UncompressedSize64 > uint64(o.MaxFile) {
			rep.Skipped = append(rep.Skipped, x.Name)
			continue
		}
		r, err := f.Open()
		if err != nil {
			rep.Skipped = append(rep.Skipped, x.Name)
			continue
		}
		limit := o.MaxFile
		if limit <= 0 {
			limit = 1 << 40
		}
		data, err := io.ReadAll(io.LimitReader(r, limit+1))
		_ = r.Close()
		if err != nil || len(data) == 0 || int64(len(data)) > limit {
			rep.Skipped = append(rep.Skipped, x.Name)
			continue
		}
		u := models.Upload{WorkspaceID: wsID, OwnerID: who(x.OwnerID), Target: x.Target, TargetID: target, Name: x.Name}
		u.ID, u.CreatedAt = ref(x.ID), x.CreatedAt
		if err := uploads.Create(tx, &u, data, o.Quota); err != nil {
			if errors.Is(err, uploads.ErrQuota) {
				rep.Skipped = append(rep.Skipped, x.Name)
				continue
			}
			return err
		}
		rep.Files++
	}
	return nil
}

func trim(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func validColor(s string) bool { return s == "" || planner.ValidColor(s) }

func encode(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// itemsText is a checklist's text (one item a line), for search.
func itemsText(items string) string {
	var list []struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal([]byte(items), &list)
	lines := make([]string, len(list))
	for i, it := range list {
		lines[i] = it.Text
	}
	return strings.Join(lines, "\n")
}

package gitsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// fakeGitea is one repository (o/r) on a Gitea, enough for the sync.
type fakeGitea struct {
	mu     sync.Mutex
	clock  time.Time
	issues map[int64]*Issue // by number
	labels []Label
	fail   bool // writes answer 500
	writes int
}

func newFake() *fakeGitea {
	return &fakeGitea{clock: time.Now().UTC(), issues: map[int64]*Issue{},
		labels: []Label{{ID: 1, Name: "Doing"}, {ID: 2, Name: "Review"}, {ID: 3, Name: "bug"}}}
}

func (f *fakeGitea) tick() time.Time {
	f.clock = f.clock.Add(time.Second)
	return f.clock
}

func (f *fakeGitea) label(name string) Label {
	for _, l := range f.labels {
		if l.Name == name {
			return l
		}
	}
	panic(name)
}

// add puts an issue on the server (as if made there).
func (f *fakeGitea) add(title string, labels ...string) *Issue {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := int64(len(f.issues) + 1)
	is := &Issue{ID: 1000 + n, Number: n, Title: title, State: "open", HTMLURL: "x/o/r/issues/" + strconv.FormatInt(n, 10), UpdatedAt: f.tick()}
	for _, l := range labels {
		is.Labels = append(is.Labels, f.label(l))
	}
	f.issues[n] = is
	return is
}

// edit changes an issue on the server.
func (f *fakeGitea) edit(n int64, fn func(*Issue)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.issues[n])
	f.issues[n].UpdatedAt = f.tick()
}

func (f *fakeGitea) get(n int64) Issue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *f.issues[n]
}

func (f *fakeGitea) handler() http.Handler {
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	write := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			fail := f.fail
			f.writes++
			f.mu.Unlock()
			if fail {
				http.Error(w, `{"message":"down"}`, http.StatusInternalServerError)
				return
			}
			next(w, r)
		}
	}
	num := func(r *http.Request) int64 {
		n, _ := strconv.ParseInt(r.PathValue("n"), 10, 64)
		return n
	}
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token good" {
			http.Error(w, `{"message":"bad token"}`, http.StatusUnauthorized)
			return
		}
		writeJSON(w, User{ID: 1, Login: "ana"})
	})
	mux.HandleFunc("GET /api/v1/repos/o/r/labels", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, f.labels)
	})
	mux.HandleFunc("GET /api/v1/repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		since, _ := time.Parse(time.RFC3339, r.URL.Query().Get("since"))
		out := []Issue{}
		if r.URL.Query().Get("page") == "1" {
			for n := int64(1); n <= int64(len(f.issues)); n++ {
				if is := f.issues[n]; !is.UpdatedAt.Before(since) {
					out = append(out, *is)
				}
			}
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("GET /api/v1/repos/o/r/issues/{n}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, f.get(num(r)))
	})
	mux.HandleFunc("POST /api/v1/repos/o/r/issues", write(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Title     string   `json:"title"`
			Body      string   `json:"body"`
			Assignees []string `json:"assignees"`
			Labels    []int64  `json:"labels"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		is := f.add(in.Title)
		f.edit(is.Number, func(is *Issue) {
			is.Body = in.Body
			for _, a := range in.Assignees {
				is.Assignees = append(is.Assignees, User{Login: a})
			}
			for _, id := range in.Labels {
				for _, l := range f.labels {
					if l.ID == id {
						is.Labels = append(is.Labels, l)
					}
				}
			}
		})
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, f.get(is.Number))
	}))
	mux.HandleFunc("PATCH /api/v1/repos/o/r/issues/{n}", write(func(w http.ResponseWriter, r *http.Request) {
		var in IssueInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.edit(num(r), func(is *Issue) {
			if in.Title != nil {
				is.Title = *in.Title
			}
			if in.Body != nil {
				is.Body = *in.Body
			}
			if in.State != nil {
				is.State = *in.State
			}
			if in.DueDate != nil {
				is.DueDate = in.DueDate
			}
			if in.UnsetDue {
				is.DueDate = nil
			}
			if in.Assignees != nil {
				is.Assignees = nil
				for _, a := range *in.Assignees {
					is.Assignees = append(is.Assignees, User{Login: a})
				}
			}
		})
		writeJSON(w, f.get(num(r)))
	}))
	mux.HandleFunc("PUT /api/v1/repos/o/r/issues/{n}/labels", write(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Labels []int64 `json:"labels"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.edit(num(r), func(is *Issue) {
			is.Labels = nil
			for _, id := range in.Labels {
				for _, l := range f.labels {
					if l.ID == id {
						is.Labels = append(is.Labels, l)
					}
				}
			}
		})
		writeJSON(w, f.get(num(r)).Labels)
	}))
	return mux
}

type fixture struct {
	db   *gorm.DB
	e    *Engine
	fake *fakeGitea
	srv  *httptest.Server
	link models.GitLink
	cols []planner.Column // Open, Doing, Review, Closed
	ana  string           // user id, login "ana" on the server
	bo   string           // a member with no account there
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	sealer, _ := keystore.New("git-test-master-key-0123456789abcdef", []byte("0123456789abcdef"))
	fake := newFake()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	f := &fixture{db: db, fake: fake, srv: srv, e: New(db, sealer, []string{srv.URL}, zap.NewNop())}

	ws := models.Workspace{Name: "Dev"}
	db.Create(&ws)
	ana, bo := models.User{Username: "ana"}, models.User{Username: "bo"}
	db.Create(&ana)
	db.Create(&bo)
	f.ana, f.bo = ana.ID, bo.ID
	db.Create(&models.WorkspaceMember{WorkspaceID: ws.ID, UserID: ana.ID, Role: models.WSEditor})
	db.Create(&models.WorkspaceMember{WorkspaceID: ws.ID, UserID: bo.ID, Role: models.WSEditor})
	if _, err := f.e.Connect(context.Background(), ana.ID, srv.URL, "good"); err != nil {
		t.Fatal(err)
	}

	cols, _ := planner.CleanColumns(planner.Columns("Open", "Doing", "Review", "Closed"))
	f.cols = cols
	list := models.TaskList{WorkspaceID: ws.ID, Name: "r", Columns: planner.EncodeColumns(cols)}
	db.Create(&list)
	f.link = models.GitLink{ListID: list.ID, WorkspaceID: ws.ID, Host: srv.URL, Owner: "o", Repo: "r", Mode: "labels",
		ColumnMap: ColumnMap{cols[1].ID: "Doing", cols[2].ID: "Review"}.Encode(), IgnoreIDs: "[]", LinkedBy: ana.ID}
	db.Create(&f.link)
	return f
}

func (f *fixture) sync(t *testing.T) {
	t.Helper()
	var link models.GitLink
	f.db.First(&link, "id = ?", f.link.ID)
	if err := f.e.SyncLink(context.Background(), link); err != nil {
		t.Fatal(err)
	}
}

// task is the list's task for issue #1.
func (f *fixture) task(t *testing.T) models.Task {
	const number = 1
	t.Helper()
	var task models.Task
	if err := f.db.First(&task, "list_id = ? AND git_number = ?", f.link.ListID, number).Error; err != nil {
		t.Fatalf("no task for #%d: %v", number, err)
	}
	return task
}

func TestConnect(t *testing.T) {
	f := setup(t)
	if _, err := f.e.Connect(context.Background(), f.bo, f.srv.URL, "bad"); err == nil {
		t.Fatal("a refused token connected")
	}
	var acc models.GitAccount
	f.db.First(&acc, "user_id = ?", f.ana)
	if acc.Login != "ana" || string(acc.TokenSealed) == "good" {
		t.Fatalf("account %+v", acc)
	}
	// Plain http only for the servers named in GIT_SERVERS.
	if _, err := f.e.NormalizeBase("http://gitea.example.org/"); err == nil {
		t.Fatal("http server allowed")
	}
	if b, err := f.e.NormalizeBase("https://Gitea.Example.org/sub/"); err != nil || b != "https://gitea.example.org/sub" {
		t.Fatalf("normalize: %q %v", b, err)
	}
}

// TestPull: issues come in as tasks — labels that are columns place them,
// other labels are tags, the assignee is the member with that login, a
// closed one is done — and later changes on git follow.
func TestPull(t *testing.T) {
	f := setup(t)
	f.fake.add("Fix login", "Review", "bug")
	f.fake.edit(1, func(is *Issue) {
		is.Body = "steps"
		is.Assignees = []User{{Login: "ana"}}
		d := "2026-03-01T00:00:00Z"
		is.DueDate = &d
	})
	f.fake.add("Old")
	f.fake.edit(2, func(is *Issue) {
		is.State = "closed"
		at := time.Now().Add(-30 * 24 * time.Hour)
		is.ClosedAt = &at
	})
	f.sync(t)

	task := f.task(t)
	if task.Title != "Fix login" || task.Notes != "steps" || task.ColumnID != f.cols[2].ID || task.Tags != `["bug"]` ||
		task.Assignees != `["`+f.ana+`"]` || task.Due == nil || *task.Due != "2026-03-01" {
		t.Fatalf("task %+v", task)
	}
	var n int64
	f.db.Model(&models.Task{}).Where("list_id = ?", f.link.ListID).Count(&n)
	if n != 1 {
		t.Fatalf("an old closed issue came in (%d tasks)", n)
	}

	// Its column label taken off on git → back to Open.
	f.fake.edit(1, func(is *Issue) { is.Labels = []Label{f.fake.label("bug")} })
	f.sync(t)
	if task = f.task(t); task.ColumnID != f.cols[0].ID {
		t.Fatalf("not back in Open: %+v", task)
	}

	// Closed on git → done; a local tag that isn't a label stays.
	f.db.Model(&models.Task{}).Where("id = ?", task.ID).Update("tags", `["bug","mine"]`)
	f.fake.edit(1, func(is *Issue) { is.State = "closed"; now := time.Now(); is.ClosedAt = &now })
	f.sync(t)
	task = f.task(t)
	if task.CompletedAt == nil || task.ColumnID != f.cols[3].ID || task.Tags != `["bug","mine"]` {
		t.Fatalf("closed: %+v", task)
	}
}

// TestPush: Orbit's changes reach the issue, the column as its label; the
// pull doesn't bring them back as news.
func TestPush(t *testing.T) {
	f := setup(t)
	f.fake.add("Fix login", "Doing")
	f.sync(t)
	task := f.task(t)
	before := task

	task.Title, task.ColumnID, task.Tags = "Fix the login", f.cols[2].ID, `["bug","mine"]`
	task.SetAssignees([]string{f.bo})
	f.db.Save(&task)
	ch := Change{Title: true, Column: true, Tags: true, Assignee: before.Assignees != task.Assignees}
	if err := f.e.Push(context.Background(), f.bo, &task, ch); err != nil {
		t.Fatal(err)
	}
	is := f.fake.get(1)
	names := []string{}
	for _, l := range is.Labels {
		names = append(names, l.Name)
	}
	if is.Title != "Fix the login" || len(names) != 2 || names[0] != "bug" || names[1] != "Review" {
		t.Fatalf("issue %+v %v", is, names)
	}
	// bo has no account there: pushed as ana (who linked the list), and his
	// assignment stays in Orbit.
	if len(is.Assignees) != 0 {
		t.Fatalf("assignees %v", is.Assignees)
	}
	f.sync(t)
	if got := f.task(t); !got.AssignedTo(f.bo) || got.Tags != `["bug","mine"]` {
		t.Fatalf("after pull %+v", got)
	}

	// Completing it closes the issue.
	now := time.Now()
	task.CompletedAt, task.ColumnID = &now, f.cols[3].ID
	f.db.Save(&task)
	if err := f.e.Push(context.Background(), f.ana, &task, Change{State: true, Column: true}); err != nil {
		t.Fatal(err)
	}
	if is := f.fake.get(1); is.State != "closed" {
		t.Fatalf("not closed: %+v", is)
	}
}

// TestCreateAndRetry: a new task opens an issue; when git is down the change
// is kept and sent by the next sync, before any pull.
func TestCreateAndRetry(t *testing.T) {
	f := setup(t)
	task := models.Task{WorkspaceID: f.link.WorkspaceID, ListID: f.link.ListID, ColumnID: f.cols[2].ID, Title: "New", Notes: "body", Tags: `["bug"]`, CreatedBy: f.ana}
	f.db.Create(&task)
	if err := f.e.CreateIssue(context.Background(), f.ana, &task); err != nil {
		t.Fatal(err)
	}
	is := f.fake.get(1)
	if task.GitNumber != 1 || task.GitIssueID != is.ID || is.Body != "body" || len(is.Labels) != 2 {
		t.Fatalf("task %+v issue %+v", task, is)
	}

	f.fake.fail = true
	task.Title = "Renamed"
	f.db.Save(&task)
	if err := f.e.Push(context.Background(), f.ana, &task, Change{Title: true}); err == nil {
		t.Fatal("no error while git is down")
	}
	if got := f.task(t); !got.GitDirty || got.Title != "Renamed" {
		t.Fatalf("not kept for later: %+v", got)
	}
	f.fake.fail = false
	f.sync(t)
	if got := f.task(t); got.GitDirty || f.fake.get(1).Title != "Renamed" {
		t.Fatalf("not retried: %+v / %+v", got, f.fake.get(1))
	}
}

// TestIgnore: an issue whose task was deleted in Orbit doesn't come back.
func TestIgnore(t *testing.T) {
	f := setup(t)
	f.fake.add("One")
	f.sync(t)
	task := f.task(t)
	f.db.Delete(&task)
	f.e.Ignore(task.ListID, task.GitIssueID)
	f.fake.edit(1, func(is *Issue) { is.Title = "One!" })
	f.sync(t)
	var n int64
	f.db.Model(&models.Task{}).Where("list_id = ?", f.link.ListID).Count(&n)
	if n != 0 {
		t.Fatal("the deleted issue came back")
	}
}

// TestLinkByHand: a task of an ordinary list linked to an issue completes
// when it closes, and closes it when completed.
func TestLinkByHand(t *testing.T) {
	f := setup(t)
	f.fake.add("Elsewhere")
	cols := planner.DefaultColumns()
	cols, _ = planner.CleanColumns(cols)
	other := models.TaskList{WorkspaceID: f.link.WorkspaceID, Name: "Mine", Columns: planner.EncodeColumns(cols)}
	f.db.Create(&other)
	task := models.Task{WorkspaceID: other.WorkspaceID, ListID: other.ID, ColumnID: cols[0].ID, Title: "Do it", CreatedBy: f.ana, Tags: "[]"}
	f.db.Create(&task)

	if err := f.e.LinkTask(context.Background(), f.ana, &task, f.srv.URL+"/o/r/issues/1"); err != nil {
		t.Fatal(err)
	}
	f.db.Save(&task)
	f.fake.edit(1, func(is *Issue) { is.State = "closed" })
	f.e.SyncAll(context.Background())
	var got models.Task
	f.db.First(&got, "id = ?", task.ID)
	if got.CompletedAt == nil || got.ColumnID != cols[2].ID || got.GitState != "closed" {
		t.Fatalf("not completed: %+v", got)
	}

	// Reopened in Orbit → reopened on git; only the state goes.
	got.CompletedAt, got.Title = nil, "Local title"
	f.db.Save(&got)
	if err := f.e.Push(context.Background(), f.ana, &got, Change{State: true, Title: true}); err != nil {
		t.Fatal(err)
	}
	if is := f.fake.get(1); is.State != "open" || is.Title != "Elsewhere" {
		t.Fatalf("issue %+v", is)
	}
}

func TestParseIssueURL(t *testing.T) {
	for _, c := range []struct {
		in, base, owner, repo string
		n                     int64
		pull                  bool
	}{
		{"https://git.example.org/o/r/issues/12", "https://git.example.org", "o", "r", 12, false},
		{"https://Git.example.org/sub/o/r/pulls/3/files", "https://git.example.org/sub", "o", "r", 3, true},
		{"https://git.example.org/o/r", "", "", "", 0, false},
		{"https://git.example.org/sub/o/r/pulls/3#issuecomment-1", "https://git.example.org/sub", "o", "r", 3, true},
	} {
		base, owner, repo, n, pull, err := ParseIssueURL(c.in)
		if c.base == "" {
			if err == nil {
				t.Errorf("%s: parsed", c.in)
			}
			continue
		}
		if err != nil || base != c.base || owner != c.owner || repo != c.repo || n != c.n || pull != c.pull {
			t.Errorf("%s: %s %s %s %d %v %v", c.in, base, owner, repo, n, pull, err)
		}
	}
}

// TestLinkerLeft: once whoever linked a list leaves its workspace, their
// token isn't used for it any more.
func TestLinkerLeft(t *testing.T) {
	f := setup(t)
	f.fake.add("One")
	f.sync(t)
	f.db.Where("workspace_id = ? AND user_id = ?", f.link.WorkspaceID, f.ana).Delete(&models.WorkspaceMember{})
	f.fake.edit(1, func(is *Issue) { is.Title = "Changed on git" })
	var link models.GitLink
	f.db.First(&link, "id = ?", f.link.ID)
	if err := f.e.SyncLink(context.Background(), link); err == nil {
		t.Fatal("synced with a departed member's token")
	}
	if got := f.task(t); got.Title != "One" {
		t.Fatalf("pulled with a departed member's token: %q", got.Title)
	}
	f.db.First(&link, "id = ?", f.link.ID)
	if link.LastError == "" {
		t.Fatal("the list doesn't say why it stopped")
	}
}

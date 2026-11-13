package gitsync

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/keystore"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"

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
	// Comments by issue number; nextComment numbers them.
	comments    map[int64][]Comment
	nextComment int64
	refreshes   int // OAuth refresh-token grants answered
	// As GitHub (githubHandler): a pull request listed among the issues, and
	// whether a due date was sent (GitHub has none).
	pr      *Issue
	dueSent bool
	// Milestones, and the webhooks made (hookDenied: the token may not).
	milestones []Milestone
	hooks      map[int64]map[string]any
	hookDenied bool
}

func newFake() *fakeGitea {
	return &fakeGitea{clock: time.Now().UTC(), issues: map[int64]*Issue{}, comments: map[int64][]Comment{}, hooks: map[int64]map[string]any{},
		milestones: []Milestone{{ID: 501, Number: 1, Title: "v1.0"}, {ID: 502, Number: 2, Title: "v2.0"}},
		labels:     []Label{{ID: 1, Name: "Doing"}, {ID: 2, Name: "Review"}, {ID: 3, Name: "bug"}}}
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

// comment adds a comment to issue n (bumping the issue, as Gitea does).
func (f *fakeGitea) comment(n int64, login, body string) Comment {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextComment++
	at := f.tick()
	c := Comment{ID: f.nextComment, Body: body, User: User{Login: login}, CreatedAt: at, UpdatedAt: at}
	f.comments[n] = append(f.comments[n], c)
	f.issues[n].UpdatedAt = at
	return c
}

// editComment changes a comment's text (body "" deletes it).
func (f *fakeGitea) editComment(id int64, body string) Comment {
	f.mu.Lock()
	defer f.mu.Unlock()
	for n, cs := range f.comments {
		for i := range cs {
			if cs[i].ID != id {
				continue
			}
			f.issues[n].UpdatedAt = f.tick()
			if body == "" {
				f.comments[n] = append(cs[:i], cs[i+1:]...)
				return Comment{}
			}
			cs[i].Body, cs[i].UpdatedAt = body, f.clock
			return cs[i]
		}
	}
	return Comment{}
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
			if in.Milestone != nil {
				is.Milestone = nil
				for i := range f.milestones {
					if f.milestones[i].ID == *in.Milestone {
						m := f.milestones[i]
						is.Milestone = &m
					}
				}
			}
		})
		writeJSON(w, f.get(num(r)))
	}))
	mux.HandleFunc("GET /api/v1/repos/o/r/milestones", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, f.milestones)
	})
	mux.HandleFunc("POST /api/v1/repos/o/r/hooks", write(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.hookDenied {
			http.Error(w, `{"message":"must be repo admin"}`, http.StatusForbidden)
			return
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		id := int64(76 + len(f.hooks) + 1)
		f.hooks[id] = in
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{"id": id})
	}))
	mux.HandleFunc("DELETE /api/v1/repos/o/r/hooks/{id}", write(func(w http.ResponseWriter, r *http.Request) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		f.mu.Lock()
		delete(f.hooks, id)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	// OAuth2: code c1 (with a PKCE verifier) → a token that expires at once;
	// its refresh token r1 → one that lasts.
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["client_id"] != "orbit" || in["client_secret"] != "s3cret" {
			http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
			return
		}
		switch {
		case in["grant_type"] == "authorization_code" && in["code"] == "c1" && in["code_verifier"] != "":
			writeJSON(w, map[string]any{"access_token": "good", "refresh_token": "r1", "expires_in": 1})
		case in["grant_type"] == "refresh_token" && in["refresh_token"] == "r1":
			f.mu.Lock()
			f.refreshes++
			f.mu.Unlock()
			writeJSON(w, map[string]any{"access_token": "good", "refresh_token": "r2", "expires_in": 3600})
		default:
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		}
	})
	mux.HandleFunc("GET /api/v1/repos/o/r/issues/{n}/comments", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		out := append([]Comment{}, f.comments[num(r)]...)
		f.mu.Unlock()
		writeJSON(w, out)
	})
	mux.HandleFunc("POST /api/v1/repos/o/r/issues/{n}/comments", write(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		c := f.comment(num(r), "ana", in.Body) // the only token is ana's
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, c)
	}))
	mux.HandleFunc("PATCH /api/v1/repos/o/r/issues/comments/{id}", write(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		writeJSON(w, f.editComment(id, in.Body))
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

func setup(t *testing.T) *fixture { return setupAs(t, false) }

// setupAs builds the fixture against the fake as Gitea, or as GitHub
// Enterprise (its API at /api/v3: Connect finds it there).
func setupAs(t *testing.T, github bool) *fixture {
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
	h := fake.handler()
	if github {
		h = fake.githubHandler()
	}
	srv := httptest.NewServer(h)
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
	// Bo has no account there and the list no bot: nothing goes as Ana.
	var cw CantWrite
	if err := f.e.Push(context.Background(), f.bo, &task, ch); !errors.As(err, &cw) {
		t.Fatalf("bo pushed without an account: %v", err)
	}
	if f.fake.get(1).Title != "Fix login" || f.task(t).GitDirty {
		t.Fatal("the change went to git, or waits to be retried")
	}
	if err := f.e.Push(context.Background(), f.ana, &task, ch); err != nil {
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

// TestComments: an issue's comments come into its task (a member with an
// account there as that member, anyone else under their login); an Orbit
// comment by someone without an account goes to the issue signed with their
// name; edits and deletions on git follow; nothing comes back twice.
func TestComments(t *testing.T) {
	f := setup(t)
	f.fake.add("Fix login")
	f.sync(t)
	f.fake.comment(1, "zed", "Seen on Safari too")
	f.fake.comment(1, "ana", "On it")
	f.sync(t)
	task := f.task(t)
	var cs []models.TaskComment
	f.db.Where("task_id = ?", task.ID).Order("created_at").Find(&cs)
	if len(cs) != 2 || cs[0].GitAuthor != "zed" || cs[0].AuthorID != "" || cs[1].AuthorID != f.ana || cs[1].Body != "On it" {
		t.Fatalf("pulled: %+v", cs)
	}

	// Bo has no account on the server: without a bot it stays here; with
	// the list's bot it goes as the bot, signed with his name.
	mine := models.TaskComment{TaskID: task.ID, WorkspaceID: task.WorkspaceID, AuthorID: f.bo, Body: "Thanks, **both**"}
	f.db.Create(&mine)
	var cw CantWrite
	if err := f.e.PushComment(context.Background(), f.bo, "Bo", &task, &mine, false); !errors.As(err, &cw) {
		t.Fatalf("no bot: %v", err)
	}
	link := f.e.ListLink(task.ListID)
	if err := f.e.SetBot(context.Background(), link, "good"); err != nil || link.BotLogin != "ana" {
		t.Fatalf("bot: %v %+v", err, link)
	}
	if mode := f.e.WriteMode(f.bo, link, link.Host); mode != "bot" {
		t.Fatalf("bo's mode %q", mode)
	}
	if err := f.e.PushComment(context.Background(), f.bo, "Bo", &task, &mine, false); err != nil {
		t.Fatal(err)
	}
	remote := f.fake.comments[1]
	if len(remote) != 3 || remote[2].Body != "**Bo (in Orbit):**\n\nThanks, **both**" || mine.GitCommentID != remote[2].ID {
		t.Fatalf("pushed: %+v, %+v", remote, mine)
	}

	// Back from git: no duplicate, and the signature stays off.
	f.sync(t)
	f.db.Where("task_id = ?", task.ID).Order("git_comment_id").Find(&cs)
	if len(cs) != 3 || cs[2].Body != "Thanks, **both**" {
		t.Fatalf("after the echo: %+v", cs)
	}

	// Edited and deleted on git.
	f.fake.editComment(cs[0].GitCommentID, "Seen on Safari and Firefox")
	f.fake.editComment(cs[1].GitCommentID, "")
	f.sync(t)
	f.db.Where("task_id = ?", task.ID).Order("git_comment_id").Find(&cs)
	if len(cs) != 2 || cs[0].Body != "Seen on Safari and Firefox" || cs[0].EditedAt == nil {
		t.Fatalf("after edits on git: %+v", cs)
	}
}

// TestOAuthSignIn: an owner's registration (sealed, secret kept when left
// blank), a sign-in that turns a code into the person's account, and an
// access token renewed with its refresh token when it's about to expire.
func TestOAuthSignIn(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.e.SetOAuthApps([]OAuthApp{{BaseURL: f.srv.URL, ClientID: "orbit"}}); err == nil {
		t.Fatal("a registration without a secret was kept")
	}
	if _, err := f.e.SetOAuthApps([]OAuthApp{{BaseURL: f.srv.URL + "/", ClientID: "orbit", ClientSecret: "s3cret"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.SetOAuthApps([]OAuthApp{{BaseURL: f.srv.URL, ClientID: "orbit"}}); err != nil {
		t.Fatalf("keeping the secret: %v", err)
	}
	app, ok := f.e.OAuthAppFor(f.srv.URL)
	if !ok || app.ClientSecret != "s3cret" {
		t.Fatalf("app: %+v", app)
	}
	if u := AuthorizeURL(app, "https://orbit.example/cb", "st", "verifier"); !strings.Contains(u, "code_challenge="+Challenge("verifier")) || !strings.Contains(u, "state=st") {
		t.Fatalf("authorize url %s", u)
	}
	if _, err := f.e.ConnectOAuth(ctx, f.bo, app, "https://orbit.example/cb", "wrong", "v"); err == nil {
		t.Fatal("a wrong code connected")
	}
	acc, err := f.e.ConnectOAuth(ctx, f.bo, app, "https://orbit.example/cb", "c1", "v")
	if err != nil || acc.Login != "ana" || acc.Via != "oauth" || acc.ExpiresAt == nil {
		t.Fatalf("connected: %+v %v", acc, err)
	}
	// About to expire: using it renews it first.
	if _, err := f.e.ClientFor(f.bo, f.srv.URL); err != nil {
		t.Fatal(err)
	}
	var again models.GitAccount
	f.db.First(&again, "id = ?", acc.ID)
	if f.fake.refreshes != 1 || again.ExpiresAt == nil || time.Until(*again.ExpiresAt) < 30*time.Minute {
		t.Fatalf("refresh: %d, %v", f.fake.refreshes, again.ExpiresAt)
	}
	// A pasted token makes it a plain token account again.
	if acc, err = f.e.Connect(ctx, f.bo, f.srv.URL, "good"); err != nil || acc.Via != "token" || acc.ExpiresAt != nil {
		t.Fatalf("pasted: %+v %v", acc, err)
	}
}

// githubHandler serves the same repository as GitHub (Enterprise, /api/v3)
// does: Bearer tokens and the API version header, labels set by name, a
// pull request among the issues, no /api/v1. A due date sent is noted.
func (f *fakeGitea) githubHandler() http.Handler {
	inner := f.handler()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v3/") {
			http.Error(w, "<html>Not Found</html>", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-GitHub-Api-Version") == "" || r.URL.Query().Get("limit") != "" {
			http.Error(w, `{"message":"not GitHub's way"}`, http.StatusBadRequest)
			return
		}
		path := "/api/v1" + strings.TrimPrefix(r.URL.Path, "/api/v3")
		body, _ := io.ReadAll(r.Body)
		byName := func(names []string) []int64 {
			var ids []int64
			for _, n := range names {
				ids = append(ids, f.label(n).ID)
			}
			return ids
		}
		switch {
		case r.Method == "GET" && path == "/api/v1/user/repos":
			writeJSON(w, []Repo{{ID: 7, FullName: "o/r", Name: "r", Owner: User{Login: "o"}}, {ID: 8, FullName: "o/other", Name: "other"}})
			return
		case r.Method == "GET" && path == "/api/v1/repos/o/r/issues":
			rec := httptest.NewRecorder()
			inner.ServeHTTP(rec, forward(r, path, body))
			var out []Issue
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			f.mu.Lock()
			if f.pr != nil && r.URL.Query().Get("page") == "1" {
				out = append(out, *f.pr)
			}
			f.mu.Unlock()
			writeJSON(w, out)
			return
		case r.Method == "PUT" && strings.HasSuffix(path, "/labels"):
			var in struct {
				Labels []string `json:"labels"`
			}
			if json.Unmarshal(body, &in) != nil {
				http.Error(w, `{"message":"labels are names"}`, http.StatusUnprocessableEntity)
				return
			}
			body, _ = json.Marshal(map[string]any{"labels": byName(in.Labels)})
		case r.Method == "POST" && path == "/api/v1/repos/o/r/issues":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			if raw, ok := in["labels"].([]any); ok {
				var names []string
				for _, v := range raw {
					s, ok := v.(string)
					if !ok {
						http.Error(w, `{"message":"labels are names"}`, http.StatusUnprocessableEntity)
						return
					}
					names = append(names, s)
				}
				in["labels"] = byName(names)
			}
			if _, ok := in["due_date"]; ok {
				f.mu.Lock()
				f.dueSent = true
				f.mu.Unlock()
			}
			body, _ = json.Marshal(in)
		case r.Method == "PATCH":
			var in map[string]any
			_ = json.Unmarshal(body, &in)
			if _, ok := in["due_date"]; ok {
				f.mu.Lock()
				f.dueSent = true
				f.mu.Unlock()
			}
			if v, ok := in["milestone"]; ok {
				// GitHub: a number, or null to take it off; Gitea's fake: an id.
				id := int64(0)
				if n, ok := v.(float64); ok {
					for _, m := range f.milestones {
						if m.Number == int64(n) {
							id = m.ID
						}
					}
				} else if v != nil {
					http.Error(w, `{"message":"milestone is a number"}`, http.StatusUnprocessableEntity)
					return
				}
				in["milestone"] = id
			}
			body, _ = json.Marshal(in)
		}
		inner.ServeHTTP(w, forward(r, path, body))
	})
}

// forward is r on the Gitea fake's path, with its token.
func forward(r *http.Request, path string, body []byte) *http.Request {
	r2 := r.Clone(r.Context())
	r2.URL.Path = path
	r2.Header.Set("Authorization", "token good")
	r2.Body = io.NopCloser(bytes.NewReader(body))
	r2.ContentLength = int64(len(body))
	return r2
}

// TestGitHub: the same sync on GitHub — found at /api/v3 (Enterprise),
// labels set by name, pull requests left out of the list, due dates
// staying Orbit's (GitHub's issues have none), /pull/ addresses, and the
// OAuth scope asked for on github.com.
func TestGitHub(t *testing.T) {
	f := setupAs(t, true)
	var acc models.GitAccount
	f.db.First(&acc, "user_id = ?", f.ana)
	if acc.Kind != KindGitHub || acc.Login != "ana" {
		t.Fatalf("account %+v", acc)
	}
	f.fake.add("Fix login", "Review", "bug")
	f.fake.pr = &Issue{ID: 9999, Number: 99, Title: "A pull request", State: "open", UpdatedAt: time.Now()}
	f.fake.pr.PullRequest = &struct {
		Merged   bool       `json:"merged"`
		MergedAt *time.Time `json:"merged_at"`
	}{}
	f.sync(t)
	var n int64
	f.db.Model(&models.Task{}).Where("list_id = ?", f.link.ListID).Count(&n)
	task := f.task(t)
	if n != 1 || task.ColumnID != f.cols[2].ID || task.Tags != `["bug"]` {
		t.Fatalf("pull: %d tasks, %+v", n, task)
	}

	// A due date and a new title: the title goes up, the date stays here.
	due := "2026-12-01"
	task.Due, task.Title = &due, "Fix the login"
	f.db.Save(&task)
	if err := f.e.Push(context.Background(), f.ana, &task, Change{Title: true, Due: true}); err != nil {
		t.Fatal(err)
	}
	if f.fake.get(1).Title != "Fix the login" || f.fake.dueSent {
		t.Fatalf("push: %+v (due sent: %v)", f.fake.get(1), f.fake.dueSent)
	}
	f.fake.edit(1, func(is *Issue) { is.Body = "steps" })
	f.sync(t)
	if task = f.task(t); task.Due == nil || *task.Due != due || task.Notes != "steps" {
		t.Fatalf("the date was lost on pull: %+v", task)
	}

	// Moved to Doing with another tag: labels by name.
	task.ColumnID, task.Tags = f.cols[1].ID, `["bug"]`
	f.db.Save(&task)
	if err := f.e.Push(context.Background(), f.ana, &task, Change{Column: true, Tags: true}); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, l := range f.fake.get(1).Labels {
		names = append(names, l.Name)
	}
	if strings.Join(names, ",") != "bug,Doing" {
		t.Fatalf("labels: %v", names)
	}

	// A new task opens an issue, its tag a label by name.
	nt := models.Task{WorkspaceID: f.link.WorkspaceID, ListID: f.link.ListID, ColumnID: f.cols[0].ID, Title: "New one", Tags: `["bug"]`, Due: &due, CreatedBy: f.ana}
	f.db.Create(&nt)
	if err := f.e.CreateIssue(context.Background(), f.ana, &nt); err != nil {
		t.Fatal(err)
	}
	f.db.First(&nt, "id = ?", nt.ID)
	if is := f.fake.get(nt.GitNumber); is.Title != "New one" || len(is.Labels) != 1 || is.Labels[0].Name != "bug" || f.fake.dueSent {
		t.Fatalf("created: %+v (due sent: %v)", is, f.fake.dueSent)
	}

	// Milestones by number on GitHub; null takes one off.
	task = f.task(t)
	task.GitMilestoneID = 2
	if err := f.e.Push(context.Background(), f.ana, &task, Change{Milestone: true}); err != nil {
		t.Fatal(err)
	}
	if task = f.task(t); f.fake.get(1).Milestone == nil || task.GitMilestone != "v2.0" || task.GitMilestoneID != 2 {
		t.Fatalf("milestone: %+v %q %d", f.fake.get(1).Milestone, task.GitMilestone, task.GitMilestoneID)
	}
	task.GitMilestoneID = 0
	if err := f.e.Push(context.Background(), f.ana, &task, Change{Milestone: true}); err != nil {
		t.Fatal(err)
	}
	if f.fake.get(1).Milestone != nil {
		t.Fatal("milestone not taken off")
	}

	// Comments go both ways as on Gitea.
	f.fake.comment(1, "ana", "On it")
	f.sync(t)
	var cs []models.TaskComment
	f.db.Where("task_id = ?", task.ID).Find(&cs)
	if len(cs) != 1 || cs[0].Body != "On it" {
		t.Fatalf("comments: %+v", cs)
	}

	c, _ := f.e.ClientFor(f.ana, f.srv.URL)
	if repos, err := c.SearchRepos(context.Background(), "o/r"); err != nil || len(repos) != 1 || repos[0].FullName != "o/r" {
		t.Fatalf("repos: %+v %v", repos, err)
	}
	if _, err := c.Projects(context.Background(), "o", "r"); !errors.Is(err, ErrNoProjects) {
		t.Errorf("projects on GitHub: %v", err)
	}

	base, owner, repo, num, pull, err := ParseIssueURL("https://www.github.com/o/r/pull/5#discussion")
	if err != nil || base != "https://github.com" || owner != "o" || repo != "r" || num != 5 || !pull {
		t.Errorf("parse: %q %q %q %d %v %v", base, owner, repo, num, pull, err)
	}
	if f.e.kindOf("https://github.com") != KindGitHub || apiRoot("https://github.com", KindGitHub) != "https://api.github.com" {
		t.Error("github.com isn't GitHub")
	}
	if u := AuthorizeURL(OAuthApp{BaseURL: "https://github.com", ClientID: "x"}, "https://orbit/cb", "s", "v"); !strings.Contains(u, "scope=repo+read%3Auser") {
		t.Errorf("authorize: %s", u)
	}
}

// TestMilestones: an issue's milestone comes in, a task's goes out (and off).
func TestMilestones(t *testing.T) {
	f := setup(t)
	f.fake.add("Ship it")
	f.fake.edit(1, func(is *Issue) { m := f.fake.milestones[0]; is.Milestone = &m })
	f.sync(t)
	task := f.task(t)
	if task.GitMilestone != "v1.0" || task.GitMilestoneID != 501 {
		t.Fatalf("pulled: %q %d", task.GitMilestone, task.GitMilestoneID)
	}
	ms, err := f.e.Milestones(context.Background(), f.link)
	if err != nil || len(ms) != 2 || ms[1].Ref != 502 || ms[1].Title != "v2.0" {
		t.Fatalf("milestones: %+v %v", ms, err)
	}
	task.GitMilestoneID = 502
	if err := f.e.Push(context.Background(), f.ana, &task, Change{Milestone: true}); err != nil {
		t.Fatal(err)
	}
	if m := f.fake.get(1).Milestone; m == nil || m.Title != "v2.0" {
		t.Fatalf("pushed: %+v", m)
	}
	if task = f.task(t); task.GitMilestone != "v2.0" {
		t.Fatalf("the task's title of it: %q", task.GitMilestone)
	}
	task.GitMilestoneID = 0
	if err := f.e.Push(context.Background(), f.ana, &task, Change{Milestone: true}); err != nil {
		t.Fatal(err)
	}
	if m := f.fake.get(1).Milestone; m != nil {
		t.Fatalf("not taken off: %+v", m)
	}
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// TestHooks: Orbit makes the webhook when the account may (else it hands
// over the address and secret), only posts signed with the secret count,
// and a post syncs the list at once.
func TestHooks(t *testing.T) {
	f := setup(t)
	f.e.kickDelay = 0
	target := "https://orbit.example.org/api/v1/git/hook/" + f.link.ID
	setup, err := f.e.EnableHook(context.Background(), f.ana, &f.link, target)
	if err != nil || !setup.Auto || setup.Secret != "" || f.link.HookID == 0 {
		t.Fatalf("auto: %+v %v (hook %d)", setup, err, f.link.HookID)
	}
	f.fake.mu.Lock()
	cfg := f.fake.hooks[f.link.HookID]["config"].(map[string]any)
	f.fake.mu.Unlock()
	secret := cfg["secret"].(string)
	if cfg["url"] != target || len(secret) < 32 {
		t.Fatalf("hook config: %+v", cfg)
	}
	body := []byte(`{"action":"edited"}`)
	h := http.Header{}
	h.Set("X-Gitea-Signature", sign(secret, body))
	if err := f.e.VerifyHook(f.link, body, h); err != nil {
		t.Fatalf("Gitea signature: %v", err)
	}
	h = http.Header{}
	h.Set("X-Hub-Signature-256", "sha256="+sign(secret, body))
	if err := f.e.VerifyHook(f.link, body, h); err != nil {
		t.Fatalf("GitHub signature: %v", err)
	}
	h.Set("X-Hub-Signature-256", "sha256="+sign("guess", body))
	if err := f.e.VerifyHook(f.link, body, h); !errors.Is(err, ErrHookSignature) {
		t.Fatalf("a wrong signature: %v", err)
	}
	if err := f.e.VerifyHook(f.link, body, http.Header{}); err == nil {
		t.Fatal("an unsigned post passed")
	}

	// A post syncs at once: the new issue is there without waiting for the poll.
	f.fake.add("Hot fix")
	f.e.Kick(f.link.ID)
	f.e.Kick(f.link.ID)
	deadline := time.Now().Add(3 * time.Second)
	for {
		var n int64
		f.db.Model(&models.Task{}).Where("list_id = ? AND title = ?", f.link.ListID, "Hot fix").Count(&n)
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the kick didn't sync")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Not allowed to make hooks: the address and secret to add by hand; the
	// old hook is gone.
	old := f.link.HookID
	f.fake.hookDenied = true
	setup, err = f.e.EnableHook(context.Background(), f.ana, &f.link, target)
	if err != nil || setup.Auto || setup.Secret == "" || setup.URL != target || f.link.HookID != 0 {
		t.Fatalf("by hand: %+v %v", setup, err)
	}
	f.fake.mu.Lock()
	_, still := f.fake.hooks[old]
	f.fake.mu.Unlock()
	if still {
		t.Error("the old hook stayed")
	}
	body = []byte(`{}`)
	h = http.Header{}
	h.Set("X-Forgejo-Signature", sign(setup.Secret, body))
	if err := f.e.VerifyHook(f.link, body, h); err != nil {
		t.Fatalf("the shown secret: %v", err)
	}
	f.e.DisableHook(context.Background(), f.ana, &f.link)
	if err := f.e.VerifyHook(f.link, body, h); err == nil {
		t.Fatal("still verifying after turning off")
	}
}

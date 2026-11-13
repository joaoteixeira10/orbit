package gitsync

import (
	"context"
	"encoding/json"
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

// fakeGitLab is one project (grp/sub/proj) on a GitLab, as the sync uses it.
type fakeGitLab struct {
	mu        sync.Mutex
	issues    map[int64]*glIssue
	mrs       map[int64]*glIssue
	notes     map[int64][]glNote
	labels    []glLabel
	bodies    []map[string]any // what PUT / POST issues got
	hookToken string
}

func (f *fakeGitLab) handler() http.Handler {
	const project = "/api/v4/projects/grp%2Fsub%2Fproj"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !strings.HasPrefix(r.URL.Path, "/api/v4/") {
			http.NotFound(w, r) // no Gitea or GitHub API here
			return
		}
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
			return
		}
		path := r.URL.EscapedPath()
		out := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		in := func() map[string]any {
			m := map[string]any{}
			_ = json.NewDecoder(r.Body).Decode(&m)
			return m
		}
		switch {
		case path == "/api/v4/user":
			out(glUser{ID: 7, Username: "ana"})
		case path == "/api/v4/users":
			if r.URL.Query().Get("username") == "ana" {
				out([]glUser{{ID: 7, Username: "ana"}})
			} else {
				out([]glUser{})
			}
		case path == "/api/v4/projects" && r.Method == "GET":
			out([]map[string]any{{"id": 3, "name": "My Project", "path": "proj", "path_with_namespace": "grp/sub/proj", "visibility": "private", "web_url": "https://gl/grp/sub/proj"}})
		case path == project+"/labels":
			out(f.labels)
		case path == project+"/milestones":
			out([]glMilestone{{ID: 501, IID: 1, Title: "v1", State: "active"}})
		case path == project+"/issues" && r.Method == "GET":
			var list []glIssue
			for _, is := range f.issues {
				list = append(list, *is)
			}
			out(list)
		case path == project+"/issues" && r.Method == "POST":
			b := in()
			f.bodies = append(f.bodies, b)
			n := int64(len(f.issues) + 1)
			is := &glIssue{ID: 1000 + n, IID: n, Title: b["title"].(string), State: "opened", UpdatedAt: time.Now(), WebURL: "https://gl/grp/sub/proj/-/issues/" + strconv.FormatInt(n, 10)}
			f.issues[n] = is
			out(is)
		case path == project+"/hooks" && r.Method == "POST":
			b := in()
			f.hookToken, _ = b["token"].(string)
			out(map[string]any{"id": 77})
		case strings.HasPrefix(path, project+"/merge_requests/"):
			n, _ := strconv.ParseInt(strings.TrimPrefix(path, project+"/merge_requests/"), 10, 64)
			if mr, ok := f.mrs[n]; ok {
				out(mr)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case strings.HasPrefix(path, project+"/issues/"):
			rest := strings.Split(strings.TrimPrefix(path, project+"/issues/"), "/")
			n, _ := strconv.ParseInt(rest[0], 10, 64)
			is, ok := f.issues[n]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if len(rest) > 1 && rest[1] == "notes" {
				if r.Method == "POST" {
					b := in()
					note := glNote{ID: int64(9000 + len(f.notes[n])), Body: b["body"].(string), Author: glUser{ID: 7, Username: "ana"}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
					f.notes[n] = append(f.notes[n], note)
					out(note)
					return
				}
				out(f.notes[n])
				return
			}
			if r.Method == "PUT" {
				b := in()
				f.bodies = append(f.bodies, b)
				if v, ok := b["title"].(string); ok {
					is.Title = v
				}
				switch b["state_event"] {
				case "close":
					is.State = "closed"
				case "reopen":
					is.State = "opened"
				}
				if v, ok := b["due_date"].(string); ok {
					if v == "" {
						is.DueDate = nil
					} else {
						is.DueDate = &v
					}
				}
				if v, ok := b["labels"].(string); ok {
					is.Labels = nil
					for _, name := range strings.Split(v, ",") {
						for _, l := range f.labels {
							if l.Name == name {
								is.Labels = append(is.Labels, l)
							}
						}
					}
				}
				is.UpdatedAt = time.Now()
			}
			out(is)
		default:
			http.NotFound(w, r)
		}
	})
}

// TestGitLab: a GitLab is found by its API (v4) when connecting; issues come
// in by iid with their labels and due date; a change goes back with
// state_event, labels by name and the date; a new task opens an issue
// (assigned by user id); comments are notes; a merge request linked by hand
// is read; webhooks carry a token.
func TestGitLab(t *testing.T) {
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
	due := "2026-11-20"
	fake := &fakeGitLab{
		issues: map[int64]*glIssue{1: {ID: 1001, IID: 1, Title: "Broken build", State: "opened", DueDate: &due, UpdatedAt: time.Now(),
			Labels: []glLabel{{ID: 11, Name: "Doing", Color: "#428bca"}, {ID: 12, Name: "ci"}}, Assignees: []glUser{{ID: 7, Username: "ana"}}}},
		mrs:    map[int64]*glIssue{4: {ID: 4004, IID: 4, Title: "Fix it", State: "merged", UpdatedAt: time.Now()}},
		notes:  map[int64][]glNote{1: {{ID: 1, Body: "set the label", System: true}, {ID: 2, Body: "Seen it", Author: glUser{ID: 7, Username: "ana"}}}},
		labels: []glLabel{{ID: 11, Name: "Doing"}, {ID: 12, Name: "ci"}, {ID: 13, Name: "bug"}},
	}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	e := New(db, sealer, []string{srv.URL}, zap.NewNop())
	ctx := context.Background()

	ws := models.Workspace{Name: "Dev"}
	db.Create(&ws)
	ana := models.User{Username: "ana"}
	db.Create(&ana)
	db.Create(&models.WorkspaceMember{WorkspaceID: ws.ID, UserID: ana.ID, Role: models.WSEditor})
	acc, err := e.Connect(ctx, ana.ID, srv.URL, "good")
	if err != nil || acc.Kind != KindGitLab || acc.Login != "ana" {
		t.Fatalf("connect: %+v %v", acc, err)
	}
	c, _ := e.ClientFor(ana.ID, srv.URL)
	if repos, err := c.SearchRepos(ctx, "proj"); err != nil || len(repos) != 1 || repos[0].Owner.Login != "grp/sub" || repos[0].Name != "proj" {
		t.Fatalf("repos: %+v %v", repos, err)
	}

	cols, _ := planner.CleanColumns(planner.Columns("Open", "Doing", "Closed"))
	list := models.TaskList{WorkspaceID: ws.ID, Name: "proj", Columns: planner.EncodeColumns(cols)}
	db.Create(&list)
	link := models.GitLink{ListID: list.ID, WorkspaceID: ws.ID, Host: srv.URL, Owner: "grp/sub", Repo: "proj", Mode: "labels",
		ColumnMap: ColumnMap{cols[1].ID: "Doing"}.Encode(), IgnoreIDs: "[]", LinkedBy: ana.ID}
	db.Create(&link)
	if err := e.SyncLink(ctx, link); err != nil {
		t.Fatal(err)
	}
	var task models.Task
	if err := db.First(&task, "list_id = ? AND git_number = ?", list.ID, 1).Error; err != nil {
		t.Fatal(err)
	}
	if task.ColumnID != cols[1].ID || task.Tags != `["ci"]` || task.Due == nil || *task.Due != due || task.AssigneeList()[0] != ana.ID {
		t.Fatalf("pulled: %+v", task)
	}
	var cms []models.TaskComment
	db.Where("task_id = ?", task.ID).Find(&cms)
	if len(cms) != 1 || cms[0].Body != "Seen it" {
		t.Fatalf("notes (system ones left out): %+v", cms)
	}

	// Done here: closed there, with state_event; a tag becomes a label by name.
	now := time.Now()
	task.CompletedAt, task.Tags, task.ColumnID = &now, `["bug"]`, cols[2].ID
	db.Save(&task)
	if err := e.Push(ctx, ana.ID, &task, Change{State: true, Tags: true, Column: true}); err != nil {
		t.Fatal(err)
	}
	if fake.issues[1].State != "closed" {
		t.Fatalf("not closed: %+v", fake.issues[1])
	}
	var labels []string
	for _, l := range fake.issues[1].Labels {
		labels = append(labels, l.Name)
	}
	if strings.Join(labels, ",") != "bug" {
		t.Errorf("labels: %v", labels)
	}

	// A new task opens an issue, assigned by user id, its date as a date.
	nt := models.Task{WorkspaceID: ws.ID, ListID: list.ID, ColumnID: cols[0].ID, Title: "New one", Due: &due, CreatedBy: ana.ID}
	nt.SetAssignees([]string{ana.ID})
	db.Create(&nt)
	if err := e.CreateIssue(ctx, ana.ID, &nt); err != nil {
		t.Fatal(err)
	}
	last := fake.bodies[len(fake.bodies)-1]
	if ids, _ := last["assignee_ids"].([]any); len(ids) != 1 || ids[0] != float64(7) || last["due_date"] != due {
		t.Errorf("create body: %v", last)
	}

	// A comment from Orbit becomes a note.
	cm := models.TaskComment{TaskID: task.ID, WorkspaceID: ws.ID, AuthorID: ana.ID, Body: "Fixed"}
	db.Create(&cm)
	if err := e.PushComment(ctx, ana.ID, "Ana", &task, &cm, false); err != nil {
		t.Fatal(err)
	}
	if n := fake.notes[1]; n[len(n)-1].Body != "Fixed" {
		t.Errorf("notes: %+v", n)
	}

	// A merge request linked by hand: read, merged.
	mt := models.Task{WorkspaceID: ws.ID, ListID: list.ID, Title: "x", CreatedBy: ana.ID}
	if err := e.LinkTask(ctx, ana.ID, &mt, srv.URL+"/grp/sub/proj/-/merge_requests/4"); err != nil {
		t.Fatal(err)
	}
	if !mt.GitPull || mt.GitState != "merged" || mt.GitRepo != "grp/sub/proj" || mt.GitNumber != 4 {
		t.Fatalf("linked: %+v", mt)
	}

	// Instant updates: the hook carries a token.
	setup, err := e.EnableHook(ctx, ana.ID, &link, "https://orbit.example/api/v1/git/hook/x")
	if err != nil || !setup.Auto || fake.hookToken == "" {
		t.Fatalf("hook: %+v %v", setup, err)
	}
	db.First(&link, "id = ?", link.ID)
	h := http.Header{}
	h.Set("X-Gitlab-Token", fake.hookToken)
	if err := e.VerifyHook(link, []byte(`{}`), h); err != nil {
		t.Errorf("good token refused: %v", err)
	}
	h.Set("X-Gitlab-Token", "nope")
	if err := e.VerifyHook(link, []byte(`{}`), h); err == nil {
		t.Error("bad token accepted")
	}

	if u := AuthorizeURL(OAuthApp{BaseURL: "https://gitlab.com", ClientID: "x"}, "https://orbit/cb", "s", "v"); !strings.HasPrefix(u, "https://gitlab.com/oauth/authorize?") || !strings.Contains(u, "scope=api+read_user") {
		t.Errorf("authorize: %s", u)
	}
	if e.kindOf("https://gitlab.com") != KindGitLab {
		t.Error("gitlab.com isn't GitLab")
	}
}

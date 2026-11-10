package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMCPMore: the rest of what people work with, through MCP — workspaces
// and members, a page's trash and copies, lists, boards, comments, smart
// lists, notes' checklists, files and folders, sheets written, whiteboards,
// courses and grades, the inbox.
func TestMCPMore(t *testing.T) {
	r, _, _, alice, bob := plannerSetupDB(t)
	call := func(tok, name string, args map[string]any) map[string]any {
		t.Helper()
		text, bad := mcpToolCall(t, r, tok, name, args)
		if bad {
			t.Fatalf("%s: %s", name, text)
		}
		var out map[string]any
		_ = json.Unmarshal([]byte(text), &out)
		return out
	}
	text := func(tok, name string, args map[string]any) string {
		t.Helper()
		s, bad := mcpToolCall(t, r, tok, name, args)
		if bad {
			t.Fatalf("%s: %s", name, s)
		}
		return s
	}
	id := func(x map[string]any) string { return x["id"].(string) }

	// A workspace made and changed; bob added, made an editor, taken out.
	ws := id(call(alice, "create_workspace", map[string]any{"name": "Lab", "kind": "school", "starter": false,
		"modules": []string{"home", "pages", "tasks", "notes", "files", "sheets", "whiteboards", "diagrams", "courses", "grades", "calendar"}}))
	if w := call(alice, "update_workspace", map[string]any{"workspace_id": ws, "name": "Lab 2"}); w["name"] != "Lab 2" {
		t.Fatalf("renamed: %v", w)
	}
	call(alice, "add_member", map[string]any{"workspace_id": ws, "user": "bob", "role": "viewer"})
	var bobID string
	for _, m := range call(alice, "list_members", map[string]any{"workspace_id": ws})["items"].([]any) {
		if mm := m.(map[string]any); mm["username"] == "bob" {
			bobID = mm["user_id"].(string)
		}
	}
	call(alice, "set_member_role", map[string]any{"workspace_id": ws, "user_id": bobID, "role": "editor"})
	if _, bad := mcpToolCall(t, r, bob, "update_workspace", map[string]any{"workspace_id": ws, "name": "Mine"}); !bad {
		t.Fatal("an editor renamed the workspace")
	}
	call(alice, "remove_member", map[string]any{"workspace_id": ws, "user_id": bobID})

	// Pages: trashed and back, copied, favourited, an old version put back, gone for good.
	pg := id(call(alice, "create_page", map[string]any{"workspace_id": ws, "title": "Method", "content": "First"}))
	call(alice, "update_page", map[string]any{"page_id": pg, "content": "Second"})
	call(alice, "trash_page", map[string]any{"page_id": pg})
	if !strings.Contains(text(alice, "list_trash", map[string]any{"workspace_id": ws}), "Method") {
		t.Fatal("not in the trash")
	}
	call(alice, "restore_page", map[string]any{"page_id": pg})
	cp := id(call(alice, "duplicate_page", map[string]any{"page_id": pg}))
	call(alice, "favorite_page", map[string]any{"page_id": pg})
	if !strings.Contains(text(alice, "list_favorites", nil), pg) {
		t.Fatal("not a favourite")
	}
	hist := text(alice, "page_history", map[string]any{"page_id": pg})
	var versions struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	_ = json.Unmarshal([]byte(hist), &versions)
	if len(versions.Items) < 2 {
		t.Fatalf("history: %s", hist)
	}
	call(alice, "restore_page_version", map[string]any{"version_id": versions.Items[len(versions.Items)-1].ID})
	if !strings.Contains(text(alice, "read_page", map[string]any{"page_id": pg}), "First") {
		t.Fatal("the old version isn't back")
	}
	call(alice, "trash_page", map[string]any{"page_id": cp})
	call(alice, "delete_page_forever", map[string]any{"page_id": cp})
	if strings.Contains(text(alice, "list_trash", map[string]any{"workspace_id": ws}), cp) {
		t.Fatal("still in the trash")
	}

	// A list renamed; a task moved to its second column; a comment changed
	// and deleted; a smart list.
	lst := call(alice, "create_task_list", map[string]any{"workspace_id": ws, "name": "Experiments"})
	call(alice, "update_task_list", map[string]any{"list_id": id(lst), "name": "Lab work"})
	tk := id(call(alice, "create_task", map[string]any{"workspace_id": ws, "title": "Titration", "list_id": id(lst)}))
	var lists struct {
		Items []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Columns []struct {
				ID string `json:"id"`
			} `json:"columns"`
		} `json:"items"`
	}
	_ = json.Unmarshal([]byte(text(alice, "list_task_lists", map[string]any{"workspace_id": ws})), &lists)
	if len(lists.Items) != 1 || lists.Items[0].Name != "Lab work" || len(lists.Items[0].Columns) < 2 {
		t.Fatalf("lists: %+v", lists)
	}
	col := lists.Items[0].Columns[1].ID
	if moved := call(alice, "set_task_status", map[string]any{"task_id": tk, "column_id": col}); moved["column_id"] != col {
		t.Fatalf("moved: %v", moved)
	}
	if !strings.Contains(text(alice, "task_activity", map[string]any{"task_id": tk}), "activity") {
		t.Fatal("no activity")
	}
	cm := id(call(alice, "comment_on_task", map[string]any{"task_id": tk, "body": "Use 0.1 M"}))
	call(alice, "update_comment", map[string]any{"comment_id": cm, "body": "Use 0.2 M"})
	if !strings.Contains(text(alice, "list_task_comments", map[string]any{"task_id": tk}), "0.2 M") {
		t.Fatal("comment not changed")
	}
	call(alice, "delete_comment", map[string]any{"comment_id": cm})
	sl := id(call(alice, "create_smart_list", map[string]any{"workspace_id": ws, "name": "Urgent", "filter": map[string]any{"priority": []int{3}}}))
	call(alice, "update_smart_list", map[string]any{"smart_list_id": sl, "name": "Very urgent"})
	if !strings.Contains(text(alice, "list_smart_lists", map[string]any{"workspace_id": ws}), "Very urgent") {
		t.Fatal("smart list")
	}
	call(alice, "delete_smart_list", map[string]any{"smart_list_id": sl})

	// A note made a checklist, moved to Quick Notes.
	n := id(call(alice, "create_note", map[string]any{"workspace_id": ws, "body": "x"}))
	got := call(alice, "set_note_checklist", map[string]any{"note_id": n, "items": []string{"beaker", "pipette"}, "done": []string{"beaker"}})
	if it := got["items"].([]any); len(it) != 2 || it[0].(map[string]any)["done"] != true || it[1].(map[string]any)["done"] != false {
		t.Fatalf("checklist: %v", got)
	}
	call(alice, "move_note", map[string]any{"note_id": n})
	if !strings.Contains(text(alice, "list_notes", nil), n) {
		t.Fatal("not in Quick Notes")
	}

	// Files and folders: a file read, renamed, put in a folder; the folder
	// moved, deleted (what it held moves up).
	var up uploaded
	decode(t, uploadTo(r, alice, "/api/v1/uploads?target=file&target_id="+ws, "results.csv", []byte("sample,ph\nA,7.1\n")), &up)
	if f := call(alice, "read_file", map[string]any{"file_id": up.ID}); !strings.Contains(f["text"].(string), "7.1") {
		t.Fatalf("read file: %v", f)
	}
	if !strings.Contains(text(alice, "list_files", map[string]any{"workspace_id": ws}), "results.csv") {
		t.Fatal("file not listed")
	}
	fo := id(call(alice, "create_folder", map[string]any{"workspace_id": ws, "name": "Data"}))
	call(alice, "update_file", map[string]any{"file_id": up.ID, "name": "ph.csv"})
	call(alice, "move_to_folder", map[string]any{"workspace_id": ws, "folder_id": fo, "files": []string{up.ID}, "pages": []string{pg}})
	if f := call(alice, "update_folder", map[string]any{"folder_id": fo, "name": "Raw data"}); f["name"] != "Raw data" {
		t.Fatalf("folder: %v", f)
	}
	if !strings.Contains(text(alice, "list_files", map[string]any{"workspace_id": ws}), `"folder_id": "`+fo) {
		t.Fatal("not in the folder")
	}
	call(alice, "delete_folder", map[string]any{"folder_id": fo})
	call(alice, "delete_file", map[string]any{"file_id": up.ID})

	// A sheet made from rows, written in (a new tab too), read back.
	sh := id(call(alice, "create_sheet", map[string]any{"workspace_id": ws, "title": "pH", "tsv": "Sample\tpH\nA\t7.1\nB\t6.8"}))
	call(alice, "write_sheet", map[string]any{"sheet_id": sh, "cells": map[string]any{"C1": "Mean", "C2": "=AVERAGE(B2:B3)", "A3": ""}})
	call(alice, "write_sheet", map[string]any{"sheet_id": sh, "tab": "Notes", "tsv": "ok", "at": "B2"})
	s := text(alice, "read_sheet", map[string]any{"sheet_id": sh})
	for _, want := range []string{"A1: Sample", "B2: 7.1", "C2: =AVERAGE(B2:B3)", "## Notes", "B2: ok"} {
		if !strings.Contains(s, want) {
			t.Errorf("sheet lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "A3:") {
		t.Errorf("A3 not cleared:\n%s", s)
	}
	if _, bad := mcpToolCall(t, r, alice, "write_sheet", map[string]any{"sheet_id": sh, "cells": map[string]any{"nope": "1"}}); !bad {
		t.Error("a bad address written")
	}
	call(alice, "delete_sheet", map[string]any{"sheet_id": sh})

	// A whiteboard with notes, more added, read back.
	wb := id(call(alice, "create_whiteboard", map[string]any{"workspace_id": ws, "title": "Ideas", "notes": []string{"Heat it", "Cool it"}}))
	call(alice, "add_to_whiteboard", map[string]any{"whiteboard_id": wb, "notes": []string{"Weigh it"}})
	board := call(alice, "read_whiteboard", map[string]any{"whiteboard_id": wb})
	if tx := board["texts"].([]any); len(tx) != 3 || tx[2] != "Weigh it" || board["shapes"].(map[string]any)["rectangle"].(float64) != 3 {
		t.Fatalf("whiteboard: %v", board)
	}
	call(alice, "delete_whiteboard", map[string]any{"whiteboard_id": wb})

	// A course with a class a week; a grade.
	co := id(call(alice, "create_course", map[string]any{"workspace_id": ws, "name": "Chemistry", "code": "CHE",
		"slots": []any{map[string]any{"weekday": 2, "start": "09:00", "end": "10:30", "room": "Lab 1"}}}))
	call(alice, "update_course", map[string]any{"course_id": co, "teacher": "Dr. Sousa"})
	if c := text(alice, "list_courses", map[string]any{"workspace_id": ws}); !strings.Contains(c, "Dr. Sousa") || !strings.Contains(c, "Lab 1") {
		t.Fatalf("courses: %s", c)
	}
	g := id(call(alice, "add_grade", map[string]any{"course_id": co, "title": "Test 1", "score": 16, "out_of": 20}))
	call(alice, "update_grade", map[string]any{"grade_id": g, "score": 17})
	call(alice, "set_course_goal", map[string]any{"course_id": co, "target": 15})
	if gr := text(alice, "list_grades", map[string]any{"workspace_id": ws}); !strings.Contains(gr, "Test 1") || !strings.Contains(gr, "17") {
		t.Fatalf("grades: %s", gr)
	}
	call(alice, "delete_grade", map[string]any{"grade_id": g})
	call(alice, "delete_course", map[string]any{"course_id": co})

	// The inbox; activity; tags.
	text(alice, "list_notifications", nil)
	call(alice, "mark_notifications_read", map[string]any{"all": true})
	if _, bad := mcpToolCall(t, r, alice, "mark_notifications_read", nil); !bad {
		t.Error("marked nothing read")
	}
	if !strings.Contains(text(alice, "workspace_activity", map[string]any{"workspace_id": ws}), "Titration") {
		t.Error("activity")
	}
	text(alice, "list_tags", map[string]any{"workspace_id": ws})
}

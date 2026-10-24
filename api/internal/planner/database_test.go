package planner

import (
	"strings"
	"testing"
)

const testDB = `{"props":[{"id":"st","name":"Status","type":"select","options":[{"id":"o1","name":"Done","color":"#34c759"},{"id":"o2","name":"Doing"}]},
{"id":"n","name":" Pages ","type":"number"},{"id":"tg","name":"Tags","type":"multi_select","options":[{"id":"a","name":"fun"},{"id":"b","name":"work"}]},
{"id":"ok","name":"Read","type":"checkbox","options":[{"id":"x","name":"dropped"}]},{"id":"who","name":"Owner","type":"person"},
{"id":"f","name":"Per day","type":"formula","formula":" =[@Pages]/30 "}],
"views":[{"id":"v1","name":"All","type":"table","filters":[{"prop":"st","op":"is","value":"o1"}],"sorts":[{"prop":"title"}],"totals":{"n":"sum","title":"count"}}]}`

func TestCleanDatabase(t *testing.T) {
	out, err := CleanDatabase(testDB)
	if err != nil {
		t.Fatal(err)
	}
	db := ParseDatabase(out)
	if db == nil || len(db.Props) != 6 || db.Props[1].Name != "Pages" || db.Props[3].Options != nil || db.Props[5].Formula != "=[@Pages]/30" || db.Views[0].Totals["n"] != "sum" {
		t.Fatalf("cleaned: %s", out)
	}
	for _, bad := range []string{
		`{"props":[{"id":"a","name":"A","type":"colour"}]}`,
		`{"props":[{"id":"a","name":"A","type":"text"},{"id":"a","name":"B","type":"text"}]}`,
		`{"props":[{"id":"title","name":"A","type":"text"}]}`,
		`{"props":[],"views":[{"id":"v","name":"V","type":"calendar"}]}`,
		`{"props":[],"views":[{"id":"v","name":"V","type":"table","filters":[{"prop":"nope","op":"is"}]}]}`,
		`{"props":[],"views":[{"id":"v","name":"V","type":"table","totals":{"title":"median"}}]}`,
	} {
		if _, err := CleanDatabase(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	if out, err := CleanDatabase("null"); err != nil || out != "" {
		t.Errorf("null: %q %v", out, err)
	}
}

func TestMergeProps(t *testing.T) {
	cur, err := MergeProps("", `{"st":"o1","n":12,"tg":["a","b"],"ok":true}`)
	if err != nil {
		t.Fatal(err)
	}
	cur, err = MergeProps(cur, `{"n":null,"who":"u1"}`)
	if err != nil {
		t.Fatal(err)
	}
	vals := PropsOf(cur)
	if _, ok := vals["n"]; ok || string(vals["who"]) != `"u1"` || string(vals["st"]) != `"o1"` {
		t.Errorf("merged: %s", cur)
	}
	if _, err := MergeProps(cur, `{"x":{"deep":1}}`); err == nil {
		t.Error("an object as a value")
	}
	if _, err := MergeProps(cur, `["a"]`); err == nil {
		t.Error("not an object")
	}
}

func TestDatabaseText(t *testing.T) {
	db := ParseDatabase(must(CleanDatabase(testDB)))
	names := map[string]string{"u1": "Ana"}
	rows := []Row{
		{Title: "Second", Props: `{"st":"o2","n":3}`, Rank: 2},
		{Title: "First | one", Props: `{"st":"o1","tg":["a","b"],"ok":true,"who":"u1"}`, Rank: 1},
	}
	table := DatabaseTable(*db, rows, names)
	want := "| Name | Status | Pages | Tags | Read | Owner |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| First \\| one | Done |   | fun, work | Yes | Ana |\n| Second | Doing | 3 |   |   |   |"
	if table != want {
		t.Errorf("table:\n%s\nwant:\n%s", table, want)
	}
	if lines := PropLines(*db, rows[1].Props, names); !strings.Contains(lines, "Status: Done") || !strings.Contains(lines, "Owner: Ana") {
		t.Errorf("lines: %s", lines)
	}
}

func must(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}

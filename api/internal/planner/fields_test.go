package planner

import (
	"reflect"
	"testing"
)

func TestReadFrontmatter(t *testing.T) {
	// What Frontmatter writes comes back.
	head := Frontmatter(`["Procedure","On call"]`, `[{"key":"owner","type":"text","value":"Ana"},{"key":"due","type":"date","value":"2026-11-06"},{"key":"done","type":"checkbox","value":true},{"key":"cost","type":"number","value":12.5}]`)
	tags, fields, rest := ReadFrontmatter(head + "# Site Fix\n")
	want := []Field{{"owner", "text", "Ana"}, {"due", "date", "2026-11-06"}, {"done", "checkbox", true}, {"cost", "number", 12.5}}
	if !reflect.DeepEqual(tags, []string{"Procedure", "On call"}) || !reflect.DeepEqual(fields, want) || rest != "# Site Fix\n" {
		t.Fatalf("round trip: %q %+v %q", tags, fields, rest)
	}
	for in, tags := range map[string][]string{
		"---\ntags:\n  - a\n  - \"b\"\n---\n": {"a", "b"},
		"---\ntags: [x, '#y']\n---\n":         {"x", "y"},
		"---\ntags: one\n---\n":               {"one"},
	} {
		if got, _, _ := ReadFrontmatter(in); !reflect.DeepEqual(got, tags) {
			t.Fatalf("%q: %q", in, got)
		}
	}
	if tags, fields, rest := ReadFrontmatter("---\nnot a header\n"); tags != nil || fields != nil || rest != "---\nnot a header\n" {
		t.Fatal("a rule isn't a header")
	}
}

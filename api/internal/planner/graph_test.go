package planner

import (
	"reflect"
	"testing"
)

func TestRefs(t *testing.T) {
	md := "See [Plan](/w/ws1/p/a1) and [Plan again](https://orbit.example/w/ws1/p/a1).\n\n" +
		"[Budget](/w/ws1/s/s1)\n\n" +
		"A sheet [inline](/w/ws1/s/s2) here, [elsewhere](/w/ws2/p/x9), [list](/w/ws1/tasks/l1), [board](/w/ws1/board/l1?view=x).\n" +
		"  [Board](/w/ws1/whiteboards/b1)  \n" +
		"[Flow](/w/ws1/d/d1) and [Flow](/w/ws1/d/d1)\n" +
		"[Flow](/w/ws1/d/d1)"
	got := Refs(md, "ws1")
	want := []Ref{
		{Kind: "page", ID: "a1"},
		{Kind: "sheet", ID: "s1", Embed: true},
		{Kind: "sheet", ID: "s2"},
		{Kind: "list", ID: "l1"},
		{Kind: "board", ID: "b1", Embed: true},
		{Kind: "diagram", ID: "d1", Embed: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Refs:\n got %+v\nwant %+v", got, want)
	}
	if len(Refs("no links", "ws1")) != 0 {
		t.Error("none")
	}
}

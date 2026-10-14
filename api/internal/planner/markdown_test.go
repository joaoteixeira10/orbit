package planner

import (
	"strings"
	"testing"
)

// TestDocToMarkdown: an old editor document becomes the Markdown the editor
// writes now — marks spanning nodes stay one mark, callouts become alerts,
// lists, tasks, tables and code keep their shape.
func TestDocToMarkdown(t *testing.T) {
	doc := `{"type":"doc","content":[
	 {"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Plan"}]},
	 {"type":"paragraph","content":[
	   {"type":"text","marks":[{"type":"bold"}],"text":"bold "},
	   {"type":"text","marks":[{"type":"bold"},{"type":"italic"}],"text":"both"},
	   {"type":"text","marks":[{"type":"bold"}],"text":" end"},
	   {"type":"text","text":", "},
	   {"type":"text","marks":[{"type":"highlight"}],"text":"hl"},
	   {"type":"text","text":", "},
	   {"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.org"}}],"text":"a link"},
	   {"type":"text","text":", 2 * 3 and snake_case"}]},
	 {"type":"bulletList","content":[
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]},
	     {"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"nested"}]}]}]}]}]},
	 {"type":"taskList","content":[
	   {"type":"taskItem","attrs":{"checked":true},"content":[{"type":"paragraph","content":[{"type":"text","text":"done"}]}]},
	   {"type":"taskItem","attrs":{"checked":false},"content":[{"type":"paragraph","content":[{"type":"text","text":"open"}]}]}]},
	 {"type":"orderedList","attrs":{"start":1},"content":[
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"first"}]}]},
	   {"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"second"}]}]}]},
	 {"type":"callout","attrs":{"emoji":"⚠️"},"content":[{"type":"paragraph","content":[{"type":"text","text":"Careful"}]}]},
	 {"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"x := 1 * 2"}]},
	 {"type":"horizontalRule"},
	 {"type":"table","content":[
	   {"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"A"}]}]},{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"B"}]}]}]},
	   {"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"1"}]}]},{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"2"}]}]}]}]}
	]}`
	want := strings.Join([]string{
		"## Plan",
		"",
		"**bold *both* end**, <mark>hl</mark>, [a link](https://example.org), 2 * 3 and snake_case",
		"",
		"- one",
		"  - nested",
		"",
		"- [x] done",
		"- [ ] open",
		"",
		"1. first",
		"2. second",
		"",
		"> [!WARNING]",
		"> Careful",
		"",
		"```go",
		"x := 1 * 2",
		"```",
		"",
		"---",
		"",
		"| A | B |",
		"| --- | --- |",
		"| 1 | 2 |",
	}, "\n")
	if got := DocToMarkdown(doc); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if !IsDocJSON(doc) || IsDocJSON("# Not JSON") || IsDocJSON(`{"type":"paragraph"}`) {
		t.Error("IsDocJSON")
	}
	if PageMarkdown("plain") != "plain" || !strings.HasPrefix(PageMarkdown(doc), "## Plan") {
		t.Error("PageMarkdown")
	}
}

// TestMarkdownText: the words, without the syntax.
func TestMarkdownText(t *testing.T) {
	md := "# Title\n\nSome **bold** and a [link](https://x.org) with <mark>hl</mark>.\n\n- [ ] Buy milk\n- one\n\n> [!TIP]\n> Drink water\n\n```\ncode * here\n```\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\n2 \\* 3 &amp; more"
	want := "Title\nSome bold and a link with hl.\nBuy milk\none\nDrink water\ncode * here\nA B\n1 2\n2 * 3 & more"
	if got := MarkdownText(md); got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

// TestEscape: only what would become formatting.
func TestEscape(t *testing.T) {
	cases := map[string]string{
		"2 * 3 = 6":      "2 * 3 = 6",
		"snake_case":     "snake_case",
		"*not italic*":   `\*not italic\*`,
		"_under_":        `\_under\_`,
		"[label](x)":     `\[label](x)`,
		"a < b":          "a < b",
		"<b>":            "&lt;b>",
		"fish & chips":   "fish & chips",
		"&amp;":          "&amp;amp;",
		"~~no~~":         `\~\~no\~\~`,
		"`tick`":         "\\`tick\\`",
		`back\*slash`:    `back\\\*slash`,
		"Ação, café":     "Ação, café",
		"price_€ and _x": `price\_€ and \_x`,
	}
	for in, want := range cases {
		if got := Escape(in); got != want {
			t.Errorf("Escape(%q) = %q, want %q", in, got, want)
		}
	}
}

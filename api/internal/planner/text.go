package planner

import (
	"encoding/json"
	"errors"
	"strings"
)

// MaxPageBytes bounds one page document (the editor's JSON).
const MaxPageBytes = 2 << 20

// docNode is the part of a ProseMirror / TipTap node that matters here.
type docNode struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Attrs   json.RawMessage `json:"attrs"`
	Content []docNode       `json:"content"`
}

// blockTypes end a line in the extracted text.
var blockTypes = map[string]bool{
	"paragraph": true, "heading": true, "blockquote": true, "codeBlock": true,
	"listItem": true, "taskItem": true, "tableRow": true, "callout": true,
	"horizontalRule": true, "hardBreak": true, "toggle": true,
}

// CheckDoc validates that content is a ProseMirror document ("" = empty page).
func CheckDoc(content string) error {
	if content == "" {
		return nil
	}
	if len(content) > MaxPageBytes {
		return errors.New("the page is too large (2 MiB at most)")
	}
	var root docNode
	if err := json.Unmarshal([]byte(content), &root); err != nil || root.Type != "doc" {
		return errors.New("content must be an editor document")
	}
	return nil
}

// DocText extracts the plain text of a document: one line per block, for
// search and previews. Invalid documents give "".
func DocText(content string) string {
	if content == "" {
		return ""
	}
	var root docNode
	if json.Unmarshal([]byte(content), &root) != nil {
		return ""
	}
	var b strings.Builder
	var walk func(n docNode)
	walk = func(n docNode) {
		if n.Text != "" {
			b.WriteString(n.Text)
		}
		for _, c := range n.Content {
			walk(c)
		}
		if blockTypes[n.Type] {
			b.WriteByte('\n')
		}
	}
	walk(root)
	lines := strings.Split(b.String(), "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// Snippet is the first n runes of s around the first match of q (case
// insensitive), for search results.
func Snippet(s, q string, n int) string {
	s = strings.ReplaceAll(s, "\n", " · ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	i := strings.Index(strings.ToLower(s), strings.ToLower(q))
	start := 0
	if i > 0 {
		start = len([]rune(s[:i])) - n/3
		if start < 0 {
			start = 0
		}
	}
	end := start + n
	if end > len(r) {
		end, start = len(r), max(0, len(r)-n)
	}
	out := string(r[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(r) {
		out += "…"
	}
	return out
}

// Rank returns an order key between prev and next (nil = no neighbour on
// that side). Keys are spaced 1024 apart, so many moves fit between two
// neighbours before the handler renumbers a list.
func Rank(prev, next *float64) float64 {
	switch {
	case prev == nil && next == nil:
		return 1024
	case prev == nil:
		return *next - 1024
	case next == nil:
		return *prev + 1024
	default:
		return (*prev + *next) / 2
	}
}

// TooClose reports two neighbouring keys that can't be split much further.
func TooClose(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-6
}

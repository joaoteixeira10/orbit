package planner

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
)

// Pages are stored as GitHub-flavoured Markdown. Before that they were the
// editor's ProseMirror JSON; DocToMarkdown converts those (once, at start,
// and for API clients that still send JSON), writing what the editor writes
// (web/src/components/editor/extensions.ts): highlight and underline as
// <mark> and <u>, callouts as GitHub alerts.

// mdNode is a ProseMirror / TipTap node, as much as the conversion needs.
type mdNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Attrs   map[string]any `json:"attrs"`
	Marks   []mdMark       `json:"marks"`
	Content []mdNode       `json:"content"`
}

type mdMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

// IsDocJSON reports whether content is an editor JSON document (the old
// format) rather than Markdown.
func IsDocJSON(content string) bool {
	s := strings.TrimSpace(content)
	if !strings.HasPrefix(s, "{") {
		return false
	}
	var n mdNode
	return json.Unmarshal([]byte(s), &n) == nil && n.Type == "doc"
}

// PageMarkdown is the Markdown of page content in either format.
func PageMarkdown(content string) string {
	if IsDocJSON(content) {
		return DocToMarkdown(content)
	}
	return content
}

// DocToMarkdown converts an editor JSON document to Markdown ("" if it
// isn't one).
func DocToMarkdown(content string) string {
	var doc mdNode
	if json.Unmarshal([]byte(content), &doc) != nil || doc.Type != "doc" {
		return ""
	}
	return strings.TrimSpace(blocks(doc.Content))
}

// alerts maps a callout's emoji to its GitHub alert.
var alerts = map[string]string{"ℹ️": "NOTE", "💡": "TIP", "❗": "IMPORTANT", "⚠️": "WARNING", "🛑": "CAUTION"}

func blocks(ns []mdNode) string {
	var out []string
	for _, n := range ns {
		if s := block(n); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "\n\n")
}

// indent prefixes every line but the first (list continuation).
func indent(s, pad string) string {
	return strings.ReplaceAll(s, "\n", "\n"+pad)
}

// quote prefixes every line with "> " (">" on blank lines).
func quote(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n")
}

func block(n mdNode) string {
	switch n.Type {
	case "paragraph":
		return inline(n.Content)
	case "heading":
		level := 1
		if l, ok := n.Attrs["level"].(float64); ok && l >= 1 && l <= 6 {
			level = int(l)
		}
		return strings.Repeat("#", level) + " " + inline(n.Content)
	case "bulletList", "taskList":
		var items []string
		for _, it := range n.Content {
			marker := "- "
			if it.Type == "taskItem" {
				if c, _ := it.Attrs["checked"].(bool); c {
					marker = "- [x] "
				} else {
					marker = "- [ ] "
				}
			}
			items = append(items, marker+indent(listItem(it), "  "))
		}
		return strings.Join(items, "\n")
	case "orderedList":
		start := 1
		if s, ok := n.Attrs["start"].(float64); ok && s > 0 {
			start = int(s)
		}
		var items []string
		for i, it := range n.Content {
			marker := fmt.Sprintf("%d. ", start+i)
			items = append(items, marker+indent(listItem(it), strings.Repeat(" ", len(marker))))
		}
		return strings.Join(items, "\n")
	case "blockquote":
		return quote(blocks(n.Content))
	case "callout":
		emoji, _ := n.Attrs["emoji"].(string)
		kind := alerts[emoji]
		if kind == "" {
			kind = "NOTE"
		}
		return "> [!" + kind + "]\n" + quote(blocks(n.Content))
	case "codeBlock":
		lang, _ := n.Attrs["language"].(string)
		var code strings.Builder
		for _, c := range n.Content {
			code.WriteString(c.Text)
		}
		return "```" + lang + "\n" + code.String() + "\n```"
	case "horizontalRule":
		return "---"
	case "table":
		return table(n)
	case "image":
		return image(n)
	default:
		// Unknown blocks: their content, so no text is lost.
		if len(n.Content) > 0 && n.Content[0].Type == "text" {
			return inline(n.Content)
		}
		return blocks(n.Content)
	}
}

// listItem renders an item's blocks: the first on the marker's line, the
// rest (nested lists, more paragraphs) below it.
func listItem(it mdNode) string {
	var parts []string
	for i, c := range it.Content {
		s := block(c)
		if s == "" {
			continue
		}
		if i > 0 && c.Type != "bulletList" && c.Type != "orderedList" && c.Type != "taskList" {
			s = "\n" + s // a second paragraph: a blank line before it
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}

func table(n mdNode) string {
	var rows [][]string
	for _, r := range n.Content {
		var cells []string
		for _, c := range r.Content {
			var texts []string
			for _, p := range c.Content {
				texts = append(texts, strings.ReplaceAll(block(p), "\n", " "))
			}
			cells = append(cells, strings.ReplaceAll(strings.Join(texts, " "), "|", "\\|"))
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	line := func(cells []string) string {
		for len(cells) < cols {
			cells = append(cells, "")
		}
		return "| " + strings.Join(cells, " | ") + " |"
	}
	out := []string{line(rows[0]), "|" + strings.Repeat(" --- |", cols)}
	for _, r := range rows[1:] {
		out = append(out, line(r))
	}
	return strings.Join(out, "\n")
}

func image(n mdNode) string {
	src, _ := n.Attrs["src"].(string)
	alt, _ := n.Attrs["alt"].(string)
	return "![" + alt + "](" + src + ")"
}

// markOrder: outer marks first.
var markOrder = []string{"link", "bold", "italic", "strike", "highlight", "underline", "code"}

func markRank(t string) int {
	for i, m := range markOrder {
		if m == t {
			return i
		}
	}
	return len(markOrder)
}

func open(m mdMark) string {
	switch m.Type {
	case "bold":
		return "**"
	case "italic":
		return "*"
	case "strike":
		return "~~"
	case "code":
		return "`"
	case "highlight":
		return "<mark>"
	case "underline":
		return "<u>"
	case "link":
		return "["
	}
	return ""
}

func closing(m mdMark) string {
	switch m.Type {
	case "highlight":
		return "</mark>"
	case "underline":
		return "</u>"
	case "link":
		href, _ := m.Attrs["href"].(string)
		return "](" + href + ")"
	}
	return open(m)
}

func sameMark(a, b mdMark) bool {
	if a.Type != b.Type {
		return false
	}
	if a.Type == "link" {
		return a.Attrs["href"] == b.Attrs["href"]
	}
	return true
}

func hasMark(ms []mdMark, m mdMark) bool {
	for _, x := range ms {
		if sameMark(x, m) {
			return true
		}
	}
	return false
}

// inline renders text with marks, keeping marks that span several text
// nodes open across them (so bold around an italic word stays one bold).
func inline(ns []mdNode) string {
	var b strings.Builder
	var stack []mdMark
	closeTo := func(keep int) {
		for len(stack) > keep {
			b.WriteString(closing(stack[len(stack)-1]))
			stack = stack[:len(stack)-1]
		}
	}
	for _, n := range ns {
		switch n.Type {
		case "hardBreak":
			closeTo(0)
			b.WriteString("  \n")
			continue
		case "image":
			closeTo(0)
			b.WriteString(image(n))
			continue
		case "text":
		default:
			continue
		}
		// Keep the longest bottom of the stack this node also has.
		keep := 0
		for keep < len(stack) && hasMark(n.Marks, stack[keep]) {
			keep++
		}
		closeTo(keep)
		var toOpen []mdMark
		for _, m := range n.Marks {
			if !hasMark(stack, m) {
				toOpen = append(toOpen, m)
			}
		}
		for i := 1; i < len(toOpen); i++ {
			for j := i; j > 0 && markRank(toOpen[j].Type) < markRank(toOpen[j-1].Type); j-- {
				toOpen[j], toOpen[j-1] = toOpen[j-1], toOpen[j]
			}
		}
		for _, m := range toOpen {
			b.WriteString(open(m))
			stack = append(stack, m)
		}
		if hasMark(n.Marks, mdMark{Type: "code"}) {
			b.WriteString(n.Text)
		} else {
			b.WriteString(Escape(n.Text))
		}
	}
	closeTo(0)
	return b.String()
}

var entityRe = regexp.MustCompile(`(?i)^&(#\d+|#x[\da-f]+|\w+);`)

// Escape backslash-escapes only what would otherwise read as Markdown (the
// editor's minimalEscape): * or _ that could open or close emphasis, ~~, `,
// a [ with a ] after it, a backslash before punctuation, < or & that would
// start a tag or an entity.
func Escape(text string) string {
	r := []rune(text)
	word := func(i int) bool { return i >= 0 && i < len(r) && (unicode.IsLetter(r[i]) || unicode.IsDigit(r[i])) }
	space := func(i int) bool { return i < 0 || i >= len(r) || unicode.IsSpace(r[i]) }
	at := func(i int) rune {
		if i < 0 || i >= len(r) {
			return 0
		}
		return r[i]
	}
	var b strings.Builder
	for i, c := range r {
		switch c {
		case '\\':
			if n := at(i + 1); n != 0 && n < 128 && unicode.IsPunct(n) || strings.ContainsRune("$+<=>^`|~", n) {
				b.WriteString(`\\`)
				continue
			}
		case '`':
			b.WriteString("\\`")
			continue
		case '*':
			if !space(i-1) || !space(i+1) {
				b.WriteString(`\*`)
				continue
			}
		case '_':
			if !word(i-1) || !word(i+1) {
				b.WriteString(`\_`)
				continue
			}
		case '~':
			if at(i-1) == '~' || at(i+1) == '~' {
				b.WriteString(`\~`)
				continue
			}
		case '[':
			if strings.ContainsRune(string(r[i+1:]), ']') {
				b.WriteString(`\[`)
				continue
			}
		case '<':
			if n := at(i + 1); n == '/' || n == '!' || n == '?' || (n < 128 && unicode.IsLetter(n)) {
				b.WriteString("&lt;")
				continue
			}
		case '&':
			if entityRe.MatchString(string(r[i:])) {
				b.WriteString("&amp;")
				continue
			}
		}
		b.WriteRune(c)
	}
	return b.String()
}

var (
	mdFence     = regexp.MustCompile("^\\s*(```|~~~)")
	mdLead      = regexp.MustCompile(`^\s*(#{1,6}\s+|>\s?|[-*+]\s+\[[ xX]\]\s+|[-*+]\s+|\d+[.)]\s+)`)
	mdAlert     = regexp.MustCompile(`^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$`)
	mdRule      = regexp.MustCompile(`^\s*([-*_]\s*){3,}$|^\s*\|?(\s*:?-{3,}:?\s*\|)+\s*:?-*:?\s*$`)
	mdImage     = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	mdLink      = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdFootRef   = regexp.MustCompile(`\[\^[^\]]+\]:?`)
	mdTag       = regexp.MustCompile(`</?[A-Za-z][^>]*>`)
	mdEmphasis  = regexp.MustCompile("(\\*\\*|__|~~|`)")
	mdStar      = regexp.MustCompile(`(^|[\s(])[*_]+|[*_]+([\s).,;:!?]|$)`)
	mdEscape    = regexp.MustCompile("\\\\([!-/:-@\\[-`{-~])")
	mdTablePipe = regexp.MustCompile(`\s*\|\s*`)
)

// MarkdownText is the plain text of a Markdown page, one line per block,
// for search and previews.
func MarkdownText(md string) string {
	var out []string
	inCode := false
	for _, l := range strings.Split(md, "\n") {
		if mdFence.MatchString(l) {
			inCode = !inCode
			continue
		}
		if !inCode {
			if mdRule.MatchString(l) {
				continue
			}
			for {
				m := mdLead.FindStringIndex(l)
				if m == nil || m[1] == 0 {
					break
				}
				l = l[m[1]:]
			}
			if mdAlert.MatchString(strings.TrimSpace(l)) {
				continue
			}
			// Escaped characters are set aside (as private-use runes) so the
			// syntax stripping below leaves them be.
			l = mdEscape.ReplaceAllStringFunc(l, func(m string) string { return string(rune(0xE000) + rune(m[1])) })
			l = mdImage.ReplaceAllString(l, "$1")
			l = mdLink.ReplaceAllString(l, "$1")
			l = mdFootRef.ReplaceAllString(l, "")
			l = mdTag.ReplaceAllString(l, "")
			l = mdEmphasis.ReplaceAllString(l, "")
			l = mdStar.ReplaceAllString(l, "$1$2")
			l = strings.Map(func(r rune) rune {
				if r >= 0xE000 && r < 0xE080 {
					return r - 0xE000
				}
				return r
			}, l)
			if strings.HasPrefix(strings.TrimSpace(l), "|") {
				l = strings.Trim(mdTablePipe.ReplaceAllString(l, " "), " ")
			}
			l = html.UnescapeString(l)
		}
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

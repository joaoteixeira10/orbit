package planner

import (
	"regexp"
	"strings"
)

// Ref is a link in Markdown to something in a workspace: a page, a sheet, a
// diagram, a whiteboard or a task list (Kind), shown in place when the link
// is alone on its line (Embed).
type Ref struct {
	Kind  string
	ID    string
	Embed bool
}

// A link's address within Orbit, written relative (/w/…) or in full.
var refRe = regexp.MustCompile(`\]\((?:https?://[^/\s)]+)?/w/([^/\s)]+)/(p|s|d|whiteboards|tasks|board)/([^/\s)?#]+)[^)]*\)`)

var refKind = map[string]string{"p": "page", "s": "sheet", "d": "diagram", "whiteboards": "board", "tasks": "list", "board": "list"}

// Refs lists the things in workspace wsID that md links to, each once (an
// embed if any of its links is alone on a line).
func Refs(md, wsID string) []Ref {
	var out []Ref
	at := map[string]int{}
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		for _, m := range refRe.FindAllStringSubmatchIndex(line, -1) {
			if line[m[2]:m[3]] != wsID {
				continue
			}
			kind, id := refKind[line[m[4]:m[5]]], line[m[6]:m[7]]
			// Alone on its line: "[" … the link's ")" is the whole line.
			embed := kind != "page" && kind != "list" && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, line[m[0]:m[1]]) && strings.Count(trimmed, "](") == 1
			if i, ok := at[kind+id]; ok {
				out[i].Embed = out[i].Embed || embed
				continue
			}
			at[kind+id] = len(out)
			out = append(out, Ref{Kind: kind, ID: id, Embed: embed})
		}
	}
	return out
}

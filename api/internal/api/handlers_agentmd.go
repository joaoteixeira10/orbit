package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// A shared page as clean Markdown, for AI agents and scripts: the guest
// link's secret opens it, as text/markdown — no app, no JSON. Its links
// between the shared pages lead to their Markdown; its files, through the
// link; a sheet shown in it comes as a Markdown table (its cells as typed),
// a Mermaid diagram as its code, a draw.io diagram or a whiteboard as its
// words.

var (
	mdLink    = regexp.MustCompile(`\]\((/[^)\s]+)\)`)
	mdEmbed   = regexp.MustCompile(`^\[([^\]]*)\]\((?:https?://[^/\s]+)?/w/([^/\s]+)/(s|d|whiteboards)/([^)\s/?#]+)\)\s*$`)
	cellRef   = regexp.MustCompile(`^([A-Z]+)([0-9]+)$`)
	pageLinkR = regexp.MustCompile(`^/w/[^/]+/p/([^/?#]+)`)
)

// guestMarkdown is a page a guest link opens (?page=, else the first), or
// with ?all=1 every page it opens, as one Markdown document.
//
//	@Summary	A shared page as Markdown (for agents)
//	@Tags		sharing
//	@Produce	plain
//	@Param		token	path	string	true	"The link's secret"
//	@Param		page	query	string	false	"One of the pages it opens (default: the first)"
//	@Param		all		query	bool	false	"Every page it opens, one after another"
//	@Success	200		{string}	string	"text/markdown"
//	@Router		/shared/{token}/markdown [get]
func (h *Handlers) guestMarkdown(c *gin.Context) {
	g, ok := h.guestAccess(c)
	if !ok {
		return
	}
	base := h.baseURL(c)
	self := base + "/api/v1/shared/" + c.Param("token")
	var pages []models.Page
	switch {
	case c.Query("all") == "1":
		ids := make([]string, 0, len(g.pages))
		for id := range g.pages {
			ids = append(ids, id)
		}
		h.db.Where("id IN ? AND trashed_at IS NULL", ids).Find(&pages)
		pages = treeOrder(pages, g.root.ID)
	default:
		id := c.DefaultQuery("page", g.root.ID)
		var p models.Page
		if !g.pages[id] || h.db.First(&p, "id = ? AND trashed_at IS NULL", id).Error != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		pages = []models.Page{p}
	}
	var b strings.Builder
	for i, p := range pages {
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		title := p.Title
		if title == "" {
			title = "Untitled"
		}
		fmt.Fprintf(&b, "# %s\n\n", title)
		b.WriteString(h.agentBody(g, p.Content, base, self))
		// Its sub-pages the link opens, to read next.
		var kids []models.Page
		h.db.Select("id", "title").Where("parent_id = ? AND trashed_at IS NULL", p.ID).Order("rank").Find(&kids)
		first := true
		for _, k := range kids {
			if !g.pages[k.ID] || c.Query("all") == "1" {
				continue
			}
			if first {
				b.WriteString("\n\n## Pages inside\n\n")
				first = false
			}
			fmt.Fprintf(&b, "- [%s](%s/markdown?page=%s)\n", mdEscape(orUntitled(k.Title)), self, k.ID)
		}
	}
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.String(http.StatusOK, strings.TrimSpace(b.String())+"\n")
}

func orUntitled(s string) string {
	if s == "" {
		return "Untitled"
	}
	return s
}

func mdEscape(s string) string { return strings.NewReplacer("[", `\[`, "]", `\]`).Replace(s) }

// treeOrder puts pages in reading order: the root, then each one's pages
// inside it (by rank), depth first.
func treeOrder(pages []models.Page, root string) []models.Page {
	kids := map[string][]models.Page{}
	byID := map[string]models.Page{}
	for _, p := range pages {
		byID[p.ID] = p
		if p.ParentID != nil {
			kids[*p.ParentID] = append(kids[*p.ParentID], p)
		}
	}
	var out []models.Page
	var walk func(id string)
	walk = func(id string) {
		p, ok := byID[id]
		if !ok {
			return
		}
		out = append(out, p)
		ks := kids[id]
		sort.Slice(ks, func(i, j int) bool { return ks[i].Rank < ks[j].Rank })
		for _, k := range ks {
			walk(k.ID)
		}
	}
	walk(root)
	return out
}

// agentBody is a page's Markdown for an agent: what it shows in place, in
// Markdown; its links made whole.
func (h *Handlers) agentBody(g guestShare, content, base, self string) string {
	lines := strings.Split(content, "\n")
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if inFence {
			continue
		}
		if m := mdEmbed.FindStringSubmatch(strings.TrimSpace(line)); m != nil && m[2] == g.ws.ID {
			if out := h.agentEmbed(g, m[1], m[3], m[4]); out != "" {
				lines[i] = out
				continue
			}
		}
		lines[i] = mdLink.ReplaceAllStringFunc(line, func(s string) string {
			path := mdLink.FindStringSubmatch(s)[1]
			return "](" + agentURL(g, path, base, self) + ")"
		})
	}
	return strings.Join(lines, "\n")
}

// agentURL makes a page's link whole: a shared page's Markdown, a file
// through the link, anything else Orbit's own address (it asks to sign in).
func agentURL(g guestShare, path, base, self string) string {
	if strings.HasPrefix(path, "/api/v1/uploads/") {
		return self + "/uploads/" + strings.TrimPrefix(path, "/api/v1/uploads/")
	}
	if m := pageLinkR.FindStringSubmatch(path); m != nil && g.pages[m[1]] {
		return self + "/markdown?page=" + m[1]
	}
	return base + path
}

// agentEmbed is what a page shows in place, as Markdown ("": just its link).
func (h *Handlers) agentEmbed(g guestShare, label, seg, id string) string {
	switch seg {
	case "s":
		var s models.Sheet
		if h.db.First(&s, "id = ? AND workspace_id = ?", id, g.ws.ID).Error != nil {
			return ""
		}
		return sheetMarkdown(orUntitled(firstOf(s.Title, label)), s.Data)
	case "d":
		var d models.Diagram
		if h.db.First(&d, "id = ? AND workspace_id = ?", id, g.ws.ID).Error != nil {
			return ""
		}
		title := orUntitled(firstOf(d.Title, label))
		if d.Kind == models.DiagramMermaid && strings.TrimSpace(d.Data) != "" {
			return fmt.Sprintf("**%s** (diagram)\n\n```mermaid\n%s\n```", title, strings.TrimSpace(d.Data))
		}
		return fmt.Sprintf("**%s** (draw.io diagram): %s", title, strings.Join(strings.Fields(d.Text), " "))
	case "whiteboards":
		var d models.Drawing
		if h.db.First(&d, "id = ? AND workspace_id = ?", id, g.ws.ID).Error != nil {
			return ""
		}
		words := strings.Join(strings.Fields(d.Text), " ")
		if words == "" {
			return fmt.Sprintf("**%s** (whiteboard)", orUntitled(firstOf(d.Title, label)))
		}
		return fmt.Sprintf("**%s** (whiteboard): %s", orUntitled(firstOf(d.Title, label)), words)
	}
	return ""
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// sheetMarkdown is a sheet as Markdown tables (each tab its own, with its
// column letters and row numbers; cells as typed — formulas as written).
func sheetMarkdown(title, raw string) string {
	var data struct {
		Tabs []struct {
			Name  string                     `json:"name"`
			Cells map[string]struct{ V any } `json:"cells"`
		} `json:"tabs"`
	}
	_ = json.Unmarshal([]byte(raw), &data)
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** (sheet)", title)
	for _, tab := range data.Tabs {
		rows := map[int]map[int]string{}
		cols := map[int]bool{}
		for ref, cell := range tab.Cells {
			m := cellRef.FindStringSubmatch(ref)
			v := strings.TrimSpace(fmt.Sprint(cell.V))
			if m == nil || cell.V == nil || v == "" {
				continue
			}
			r, _ := strconv.Atoi(m[2])
			col := colNumber(m[1])
			if r > 500 || col > 40 {
				continue
			}
			if rows[r] == nil {
				rows[r] = map[int]string{}
			}
			rows[r][col] = strings.NewReplacer("|", `\|`, "\n", " ").Replace(v)
			cols[col] = true
		}
		if len(rows) == 0 {
			continue
		}
		cs := keys(cols)
		rs := make([]int, 0, len(rows))
		for r := range rows {
			rs = append(rs, r)
		}
		sort.Ints(rs)
		if len(data.Tabs) > 1 {
			fmt.Fprintf(&b, "\n\n*%s*", tab.Name)
		}
		b.WriteString("\n\n| |")
		for _, c := range cs {
			b.WriteString(" " + colLetters(c) + " |")
		}
		b.WriteString("\n|---|" + strings.Repeat("---|", len(cs)))
		for _, r := range rs {
			fmt.Fprintf(&b, "\n| %d |", r)
			for _, c := range cs {
				b.WriteString(" " + rows[r][c] + " |")
			}
		}
	}
	return b.String()
}

func keys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// colNumber: A → 1, Z → 26, AA → 27.
func colNumber(s string) int {
	n := 0
	for _, r := range s {
		n = n*26 + int(r-'A'+1)
	}
	return n
}

func colLetters(n int) string {
	s := ""
	for n > 0 {
		n--
		s = string(rune('A'+n%26)) + s
		n /= 26
	}
	return s
}

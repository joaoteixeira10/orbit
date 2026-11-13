package api

import (
	"net/http"
	"strings"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
)

// The graph: how a workspace's things connect — pages, sheets, diagrams,
// whiteboards, task lists, folders and the people pages @-mention — joined
// by links in pages and in tasks' notes (a task's links count as its
// list's), embeds, @-mentions, sub-pages, what's in which folder and a
// folder's task list.

type graphNode struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"` // page | sheet | diagram | board | list | folder | person
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`  // a page's emoji
	Color string `json:"color,omitempty"` // a list's or a folder's
}

type graphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // link | embed | mention | parent | folder | list
}

type graphResponse struct {
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

// workspaceGraph returns a workspace's graph.
//
//	@Summary	How a workspace's things connect
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path		string	true	"Workspace id"
//	@Success	200	{object}	graphResponse
//	@Router		/workspaces/{ws}/graph [get]
func (h *Handlers) workspaceGraph(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	out := graphResponse{Nodes: []graphNode{}, Edges: []graphEdge{}}
	have := map[string]string{} // id → kind
	node := func(n graphNode) {
		if _, dup := have[n.ID]; !dup {
			have[n.ID] = n.Kind
			out.Nodes = append(out.Nodes, n)
		}
	}
	seen := map[string]bool{}
	edge := func(from, to, kind string) {
		if from == "" || to == "" || from == to || have[from] == "" || have[to] == "" {
			return
		}
		if k := from + ">" + to; !seen[k] && !seen[to+">"+from] {
			seen[k] = true
			out.Edges = append(out.Edges, graphEdge{From: from, To: to, Kind: kind})
		}
	}

	var pages []models.Page
	h.db.Select("id", "parent_id", "folder_id", "title", "icon", "content").Where("workspace_id = ? AND trashed_at IS NULL", ws.ID).Find(&pages)
	var sheets []models.Sheet
	h.db.Select("id", "folder_id", "title").Where("workspace_id = ?", ws.ID).Find(&sheets)
	var diagrams []models.Diagram
	h.db.Select("id", "folder_id", "title").Where("workspace_id = ?", ws.ID).Find(&diagrams)
	var boards []models.Drawing
	h.db.Select("id", "folder_id", "title").Where("workspace_id = ? AND note_id = ''", ws.ID).Find(&boards)
	var lists []models.TaskList
	h.db.Select("id", "name", "color").Where("workspace_id = ? AND archived_at IS NULL", ws.ID).Find(&lists)
	var folders []models.Folder
	h.db.Where("workspace_id = ?", ws.ID).Find(&folders)

	// Pages the caller may not see aren't in it at all.
	g := h.gate(auth.UserID(c), ws.ID)
	visible := pages[:0]
	for _, p := range pages {
		if g.sees(p.ID) {
			visible = append(visible, p)
		}
	}
	pages = visible
	for _, p := range pages {
		node(graphNode{ID: p.ID, Kind: "page", Title: p.Title, Icon: p.Icon})
	}
	for _, x := range sheets {
		node(graphNode{ID: x.ID, Kind: "sheet", Title: x.Title})
	}
	for _, x := range diagrams {
		node(graphNode{ID: x.ID, Kind: "diagram", Title: x.Title})
	}
	for _, x := range boards {
		node(graphNode{ID: x.ID, Kind: "board", Title: x.Title})
	}
	for _, x := range lists {
		node(graphNode{ID: x.ID, Kind: "list", Title: x.Name, Color: x.Color})
	}
	for _, f := range folders {
		node(graphNode{ID: f.ID, Kind: "folder", Title: f.Name, Color: f.Color})
	}

	// The people pages mention (members only).
	type member struct{ ID, Username, DisplayName string }
	var members []member
	h.db.Table("workspace_members").Select("users.id, users.username, users.display_name").
		Joins("JOIN users ON users.id = workspace_members.user_id").Where("workspace_members.workspace_id = ?", ws.ID).Scan(&members)
	byName := map[string]member{}
	for _, m := range members {
		byName[strings.ToLower(m.Username)] = m
	}

	// What's in which folder, and its list.
	for _, f := range folders {
		edge(f.ParentID, f.ID, "folder")
		edge(f.ID, f.ListID, "list")
	}
	for _, x := range sheets {
		edge(x.FolderID, x.ID, "folder")
	}
	for _, x := range diagrams {
		edge(x.FolderID, x.ID, "folder")
	}
	for _, x := range boards {
		edge(x.FolderID, x.ID, "folder")
	}
	// Pages: sub-pages, folders, links, embeds, @-mentions.
	for _, p := range pages {
		if p.ParentID != nil {
			edge(*p.ParentID, p.ID, "parent")
		}
		edge(p.FolderID, p.ID, "folder")
		for _, r := range planner.Refs(p.Content, ws.ID) {
			kind := "link"
			if r.Embed {
				kind = "embed"
			}
			edge(p.ID, r.ID, kind)
		}
		for _, m := range mentionRe.FindAllStringSubmatch(strings.ToLower(p.Content), 50) {
			who, ok := byName[strings.TrimRight(m[1], ".")]
			if !ok {
				continue
			}
			name := who.DisplayName
			if name == "" {
				name = who.Username
			}
			node(graphNode{ID: who.ID, Kind: "person", Title: name})
			edge(p.ID, who.ID, "mention")
		}
	}
	// Tasks' notes: their links are their list's.
	var tasks []models.Task
	h.db.Select("list_id", "notes").Where("workspace_id = ? AND notes LIKE ?", ws.ID, "%/w/"+ws.ID+"/%").Find(&tasks)
	for _, t := range tasks {
		for _, r := range planner.Refs(t.Notes, ws.ID) {
			edge(t.ListID, r.ID, "link")
		}
	}
	c.JSON(http.StatusOK, out)
}

package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// listGuides lists Orbit's guides (the docs page), or with q the sections
// of them that answer it best.
//
//	@Summary	Orbit's guides, or a search in them
//	@Tags		docs
//	@Produce	json
//	@Security	BearerAuth
//	@Param		q	query		string	false	"Words to find; without it, the guides"
//	@Success	200	{array}		helpdocs.Guide
//	@Router		/docs [get]
func (h *Handlers) listGuides(c *gin.Context) {
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		c.JSON(http.StatusOK, h.guides.Search(q, 12))
		return
	}
	c.JSON(http.StatusOK, h.guides.List())
}

// getGuide returns one guide's Markdown, as the docs page shows it (headings
// with their anchors, links to other guides going to their pages).
//
//	@Summary	One of Orbit's guides
//	@Tags		docs
//	@Produce	json
//	@Security	BearerAuth
//	@Param		slug	path		string	true	"The guide (user-guide, configuration, …)"
//	@Success	200		{object}	guideResponse
//	@Router		/docs/{slug} [get]
func (h *Handlers) getGuide(c *gin.Context) {
	g, md, ok := h.guides.Page(c.Param("slug"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such guide"})
		return
	}
	c.JSON(http.StatusOK, guideResponse{Slug: g.Slug, Title: g.Title, File: g.File, Markdown: md})
}

type guideResponse struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	File     string `json:"file"`
	Markdown string `json:"markdown"`
}

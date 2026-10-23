package api

import (
	"net/http"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
)

// The journal: a page a day, each person's own, with a mood.

type journalDay struct {
	Date    string `json:"date"`
	Mood    int    `json:"mood"`
	Excerpt string `json:"excerpt"`
}

type journalResponse struct {
	Entry *models.JournalEntry `json:"entry"` // null: nothing written that day
	// The same day in earlier years.
	OnThisDay []journalDay `json:"on_this_day"`
}

func journalExcerpt(text string, n int) string {
	text = strings.Join(strings.Fields(text), " ")
	if r := []rune(text); len(r) > n {
		return string(r[:n]) + "…"
	}
	return text
}

// listJournal lists the caller's days of a month (?month=YYYY-MM): mood and
// a line of each.
//
//	@Summary	My journal's days of a month
//	@Tags		journal
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path	string	true	"Workspace id"
//	@Param		month	query	string	true	"YYYY-MM"
//	@Success	200		{array}	journalDay
//	@Router		/workspaces/{ws}/journal [get]
func (h *Handlers) listJournal(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	month := c.Query("month")
	if len(month) != 7 || !planner.ValidDate(month+"-01") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "month is YYYY-MM"})
		return
	}
	var rows []models.JournalEntry
	h.db.Select("date", "mood", "content_text").Where("workspace_id = ? AND owner_id = ? AND date LIKE ?", ws.ID, auth.UserID(c), month+"-%").Order("date").Find(&rows)
	out := make([]journalDay, 0, len(rows))
	for _, e := range rows {
		out = append(out, journalDay{Date: e.Date, Mood: e.Mood, Excerpt: journalExcerpt(e.ContentText, 90)})
	}
	c.JSON(http.StatusOK, out)
}

// getJournalDay returns the caller's entry of a day, and the same day in
// earlier years.
//
//	@Summary	A day of my journal
//	@Tags		journal
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string	true	"Workspace id"
//	@Param		date	path		string	true	"YYYY-MM-DD"
//	@Success	200		{object}	journalResponse
//	@Router		/workspaces/{ws}/journal/{date} [get]
func (h *Handlers) getJournalDay(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	date := c.Param("date")
	if !planner.ValidDate(date) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date is YYYY-MM-DD"})
		return
	}
	uid := auth.UserID(c)
	resp := journalResponse{OnThisDay: []journalDay{}}
	var e models.JournalEntry
	if h.db.Where("workspace_id = ? AND owner_id = ? AND date = ?", ws.ID, uid, date).First(&e).Error == nil {
		resp.Entry = &e
	}
	var past []models.JournalEntry
	h.db.Select("date", "mood", "content_text").
		Where("workspace_id = ? AND owner_id = ? AND date LIKE ? AND date < ?", ws.ID, uid, "%"+date[4:], date).
		Order("date DESC").Limit(10).Find(&past)
	for _, p := range past {
		resp.OnThisDay = append(resp.OnThisDay, journalDay{Date: p.Date, Mood: p.Mood, Excerpt: journalExcerpt(p.ContentText, 200)})
	}
	c.JSON(http.StatusOK, resp)
}

type journalRequest struct {
	Content *string `json:"content" binding:"omitempty,max=200000"`
	Mood    *int    `json:"mood" binding:"omitempty,min=0,max=5"`
}

// putJournalDay writes the caller's entry of a day (made the first time; an
// entry with nothing in it is removed).
//
//	@Summary	Write a day of my journal
//	@Tags		journal
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		date	path		string			true	"YYYY-MM-DD"
//	@Param		body	body		journalRequest	true	"Text and / or mood"
//	@Success	200		{object}	journalResponse
//	@Router		/workspaces/{ws}/journal/{date} [put]
func (h *Handlers) putJournalDay(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	date := c.Param("date")
	if !planner.ValidDate(date) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date is YYYY-MM-DD"})
		return
	}
	var req journalRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	var e models.JournalEntry
	found := h.db.Where("workspace_id = ? AND owner_id = ? AND date = ?", ws.ID, uid, date).First(&e).Error == nil
	if !found {
		e = models.JournalEntry{WorkspaceID: ws.ID, OwnerID: uid, Date: date}
	}
	if req.Content != nil {
		e.Content = strings.TrimRight(*req.Content, " \n\t")
		e.ContentText = planner.MarkdownText(e.Content)
	}
	if req.Mood != nil {
		e.Mood = *req.Mood
	}
	switch {
	case strings.TrimSpace(e.Content) == "" && e.Mood == 0:
		if found {
			h.db.Delete(&e)
		}
		c.JSON(http.StatusOK, journalResponse{OnThisDay: []journalDay{}})
		return
	case found:
		h.db.Save(&e)
	default:
		if err := h.db.Create(&e).Error; err != nil {
			h.internalError(c, err, "could not save the day")
			return
		}
	}
	c.JSON(http.StatusOK, journalResponse{Entry: &e, OnThisDay: []journalDay{}})
}

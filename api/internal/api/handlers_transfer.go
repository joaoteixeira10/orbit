package api

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/transfer"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/version"

	"github.com/gin-gonic/gin"
)

// exportWorkspace downloads all of a workspace as a zip, to import on
// another Orbit (owners). (/export is its pages as Markdown.)
//
//	@Summary	Export a workspace
//	@Tags		workspaces
//	@Produce	application/zip
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{file}	binary
//	@Router		/workspaces/{ws}/transfer [get]
func (h *Handlers) exportWorkspace(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	// Built in memory first, so a failure is an error, not a broken download.
	var buf bytes.Buffer
	g := h.gate(auth.UserID(c), ws.ID)
	hide := func(id string) bool { return !g.sees(id) }
	if err := transfer.Export(h.db, ws.ID, auth.UserID(c), version.Current().Version, &buf, time.Now(), hide); err != nil {
		h.internalError(c, err, "could not export the workspace")
		return
	}
	h.recordCtx(c, "workspace.export", "workspace", ws.ID, gin.H{"name": ws.Name, "bytes": buf.Len()})
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.orbit.zip"`, fileSlug(ws.Name), time.Now().UTC().Format("20060102")))
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

var slugStrip = regexp.MustCompile(`[^A-Za-z0-9]+`)

// fileSlug is a name safe in a file name ("Year 11 — Maths" → "Year-11-Maths").
func fileSlug(name string) string {
	s := strings.Trim(slugStrip.ReplaceAllString(name, "-"), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	if s == "" {
		return "workspace"
	}
	return s
}

type importResponse struct {
	Workspace workspaceResponse `json:"workspace"`
	Report    transfer.Report   `json:"report"`
}

// importWorkspace makes a new workspace from an export (multipart field
// "file"; people=false leaves out the members found on this instance). The
// caller owns it.
//
//	@Summary	Import a workspace
//	@Tags		workspaces
//	@Accept		mpfd
//	@Produce	json
//	@Security	BearerAuth
//	@Param		file	formData	file	true	"An Orbit workspace export (.orbit.zip)"
//	@Param		people	formData	bool	false	"Add the members who have an account here (default true)"
//	@Success	201		{object}	importResponse
//	@Router		/workspaces/import [post]
func (h *Handlers) importWorkspace(c *gin.Context) {
	if auth.Effective(c).ReadOnly {
		c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
		return
	}
	max := h.cfg.ImportMaxBytes
	if max <= 0 {
		max = 512 << 20
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max+1<<20)
	fh, err := c.FormFile("file")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the file is too large (at most " + strconv.FormatInt(max>>20, 10) + " MB)"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "send the export in the form field \"file\""})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
		return
	}
	defer func() { _ = f.Close() }()
	a, z, err := transfer.Read(f, fh.Size, 256<<20)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := auth.UserID(c)
	ws, rep, err := transfer.Import(h.db, a, z, transfer.Options{
		UserID: uid, AddPeople: c.PostForm("people") != "false",
		Quota: h.cfg.UploadQuotaBytes, MaxFile: h.cfg.UploadMaxBytes,
	})
	if err != nil {
		h.internalError(c, err, "could not import the workspace")
		return
	}
	h.recordCtx(c, "workspace.import", "workspace", ws.ID, gin.H{"name": ws.Name, "tasks": rep.Tasks, "pages": rep.Pages, "files": rep.Files, "added": rep.Added})
	var members []models.WorkspaceMember
	h.db.Where("workspace_id = ?", ws.ID).Find(&members)
	var mine models.WorkspaceMember
	for _, m := range members {
		h.events.Publish(m.UserID, liveEvent("workspaces"))
		if m.UserID == uid {
			mine = m
		} else {
			h.notifyMembership(c, ws, m.UserID, "added", m.Role)
		}
	}
	c.JSON(http.StatusCreated, importResponse{Workspace: h.toWorkspaceResponse(ws, mine), Report: rep})
}

package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/assistant"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"
)

// Attachments: files a person adds to a conversation with the assistant
// (📎). Each is an upload of theirs alone (target chat, on the conversation
// once the message goes), deleted with the conversation. The model reads
// what they say (read_attachment); in Agent mode it can line up putting one
// in a workspace's Files — the file itself goes, never through the model —
// or making a page of one as it is (page_from_attachment), whatever its
// size. A PDF's text is read by the app (pdf.js), as for Files. Up to 10
// go with one message (assistantChatRequest.Files).

// attachmentChunk is how much of a file's text read_attachment gives at once.
const attachmentChunk = 40000

// attachmentShown is how much of the files sent with a message goes along
// with it, so a short one needs no reading.
const attachmentShown = 12000

type attachmentResponse struct {
	assistant.File
	URL      string `json:"url"`
	Readable bool   `json:"readable"` // the model can read what it says
}

func (h *Handlers) toAttachment(u models.Upload) attachmentResponse {
	return attachmentResponse{File: assistant.File{ID: u.ID, Name: u.Name, Type: u.Type, Size: u.Size}, URL: uploads.URL(u), Readable: h.attachmentText(u) != ""}
}

// myAttachment loads :id, a file the caller attached to a conversation.
func (h *Handlers) myAttachment(c *gin.Context) (models.Upload, bool) {
	var u models.Upload
	if h.db.First(&u, "id = ? AND target = ? AND owner_id = ?", c.Param("id"), uploads.Chat, auth.UserID(c)).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such file"})
		return u, false
	}
	return u, true
}

// attachmentText is what a file says, its lines kept: a text file as it is,
// a document's paragraphs, a PDF's text as the app read it ("" when there's
// nothing Orbit can read).
func (h *Handlers) attachmentText(u models.Upload) string {
	if u.Type != "application/pdf" {
		if data, err := uploads.Read(h.db, u); err == nil {
			if s := uploads.Lines(u.Name, u.Type, data); s != "" {
				return s
			}
		}
	}
	var t models.UploadText
	h.db.Limit(1).Find(&t, "id = ?", u.ID)
	return strings.TrimSpace(t.Text)
}

// attachFile keeps a file for a message to the assistant (multipart "file").
//
//	@Summary	Attach a file for the assistant
//	@Tags		assistant
//	@Accept		mpfd
//	@Produce	json
//	@Security	BearerAuth
//	@Param		file	formData	file	true	"The file"
//	@Success	201		{object}	attachmentResponse
//	@Router		/assistant/files [post]
func (h *Handlers) attachFile(c *gin.Context) {
	if _, _, ok := h.assistantFor(auth.UserID(c)); !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "the assistant isn't on"})
		return
	}
	max := h.cfg.UploadMaxBytes
	if max <= 0 {
		max = 25 << 20
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max+1<<20)
	fh, err := c.FormFile("file")
	if err != nil || fh.Size > max {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) || (err == nil && fh.Size > max) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the file is too large (at most " + strconv.FormatInt(max>>20, 10) + " MB)"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "send the file in the form field \"file\""})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(data)) > max {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the file is too large"})
		return
	}
	if len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the file is empty"})
		return
	}
	u := models.Upload{OwnerID: auth.UserID(c), Target: uploads.Chat, Name: fh.Filename}
	if err := uploads.Create(h.db, &u, data, h.cfg.UploadQuotaBytes); err != nil {
		if errors.Is(err, uploads.ErrQuota) {
			c.JSON(http.StatusInsufficientStorage, gin.H{"error": err.Error()})
			return
		}
		h.internalError(c, err, "could not keep the file")
		return
	}
	c.JSON(http.StatusCreated, h.toAttachment(u))
}

type attachmentTextRequest struct {
	Text string `json:"text" binding:"max=2000000"`
}

// setAttachmentText keeps what an attached PDF says, read by the app
// (pdf.js), its lines kept.
//
//	@Summary	An attached PDF's text
//	@Tags		assistant
//	@Accept		json
//	@Security	BearerAuth
//	@Param		id		path	string					true	"File id"
//	@Param		body	body	attachmentTextRequest	true	"Its text"
//	@Success	204
//	@Router		/assistant/files/{id}/text [put]
func (h *Handlers) setAttachmentText(c *gin.Context) {
	u, ok := h.myAttachment(c)
	if !ok {
		return
	}
	var req attachmentTextRequest
	if !bindJSON(c, &req) {
		return
	}
	if u.Type != "application/pdf" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only a PDF's text is sent; the rest is read here"})
		return
	}
	text := req.Text
	if len(text) > uploads.MaxText {
		text = text[:uploads.MaxText]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	if err := h.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&models.UploadText{ID: u.ID, Text: text}).Error; err != nil {
		h.internalError(c, err, "could not keep the text")
		return
	}
	c.Status(http.StatusNoContent)
}

// dropAttachment deletes a file attached for the assistant.
//
//	@Summary	Remove an attached file
//	@Tags		assistant
//	@Security	BearerAuth
//	@Param		id	path	string	true	"File id"
//	@Success	204
//	@Router		/assistant/files/{id} [delete]
func (h *Handlers) dropAttachment(c *gin.Context) {
	u, ok := h.myAttachment(c)
	if !ok {
		return
	}
	if err := uploads.Delete(h.db, "id = ?", u.ID); err != nil {
		h.internalError(c, err, "could not remove it")
		return
	}
	c.Status(http.StatusNoContent)
}

type attachmentTextResponse struct {
	Name  string `json:"name"`
	Text  string `json:"text"`
	From  int    `json:"from"`           // where this part starts, in characters
	Next  int    `json:"next,omitempty"` // where the next part starts (0: none)
	Total int    `json:"total"`
}

// getAttachmentText gives what an attached file says, a part at a time
// (from; whole=1 all of it).
//
//	@Summary	What an attached file says
//	@Tags		assistant
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string	true	"File id"
//	@Param		from	query		int		false	"Where to start, in characters"
//	@Param		whole	query		bool	false	"All of it"
//	@Success	200		{object}	attachmentTextResponse
//	@Router		/assistant/files/{id}/text [get]
func (h *Handlers) getAttachmentText(c *gin.Context) {
	u, ok := h.myAttachment(c)
	if !ok {
		return
	}
	text := []rune(h.attachmentText(u))
	if len(text) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Orbit can't read what " + u.Name + " says (it can still be put in Files)"})
		return
	}
	from, _ := strconv.Atoi(c.Query("from"))
	from = min(max(from, 0), len(text))
	to := len(text)
	if c.Query("whole") != "1" {
		to = min(from+attachmentChunk, len(text))
	}
	out := attachmentTextResponse{Name: u.Name, Text: string(text[from:to]), From: from, Total: len(text)}
	if to < len(text) {
		out.Next = to
	}
	c.JSON(http.StatusOK, out)
}

type saveAttachmentRequest struct {
	WorkspaceID string `json:"workspace_id" binding:"required,max=36"`
	FolderID    string `json:"folder_id" binding:"max=36"`
}

// saveAttachment puts an attached file in a workspace's Files, as a file of
// its own (editors).
//
//	@Summary	Put an attached file in Files
//	@Tags		assistant
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string					true	"File id"
//	@Param		body	body		saveAttachmentRequest	true	"Where"
//	@Success	201		{object}	uploadResponse
//	@Router		/assistant/files/{id}/save [post]
func (h *Handlers) saveAttachment(c *gin.Context) {
	a, ok := h.myAttachment(c)
	if !ok {
		return
	}
	var req saveAttachmentRequest
	if !bindJSON(c, &req) {
		return
	}
	if _, _, ok := h.wsAccess(c, req.WorkspaceID, models.WSEditor); !ok {
		return
	}
	if !h.fileFolderOK(c, req.WorkspaceID, req.FolderID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no such folder there"})
		return
	}
	data, err := uploads.Read(h.db, a)
	if err != nil {
		h.internalError(c, err, "could not read the file")
		return
	}
	u := models.Upload{WorkspaceID: req.WorkspaceID, OwnerID: a.OwnerID, Target: uploads.File, TargetID: req.WorkspaceID, Name: a.Name, FolderID: req.FolderID}
	if err := uploads.Add(h.db, &u, data, h.cfg.UploadQuotaBytes); err != nil {
		if errors.Is(err, uploads.ErrQuota) {
			c.JSON(http.StatusInsufficientStorage, gin.H{"error": err.Error()})
			return
		}
		h.internalError(c, err, "could not store the file")
		return
	}
	if a.Type == "application/pdf" { // what it says, for search
		if t := h.attachmentText(a); t != "" {
			_ = uploads.SetText(h.db, u.ID, t)
		}
	}
	h.publishWS(c, u.WorkspaceID, "uploads")
	c.JSON(http.StatusCreated, toUploadResponse(u))
}

// asMarkdown is a file's text for a page: Markdown as it is; a PDF's lines
// joined into paragraphs; a document's paragraphs; data in a code block.
func asMarkdown(name, typ, text string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown", ".txt":
		return text
	case ".csv", ".tsv", ".json", ".xml", ".yaml", ".yml", ".ini", ".log", ".html", ".htm":
		return "```\n" + strings.ReplaceAll(text, "```", "ˋˋˋ") + "\n```"
	}
	if typ == "application/pdf" {
		return pdfParagraphs(text)
	}
	var paras []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			paras = append(paras, l)
		}
	}
	return strings.Join(paras, "\n\n")
}

// pdfParagraphs joins a PDF's lines into paragraphs: one ends at a blank
// line, or at a short line that ends a sentence; a word broken with a hyphen
// is put back together.
func pdfParagraphs(text string) string {
	lines := strings.Split(text, "\n")
	longest := 0
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
		longest = max(longest, utf8.RuneCountInString(lines[i]))
	}
	var out []string
	var b strings.Builder
	end := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for i, l := range lines {
		if l == "" {
			end()
			continue
		}
		if b.Len() > 0 {
			s := b.String()
			if strings.HasSuffix(s, "-") && unicode.IsLower([]rune(l)[0]) {
				b.Reset()
				b.WriteString(strings.TrimSuffix(s, "-"))
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(l)
		last, _ := utf8.DecodeLastRuneInString(l)
		n, next := utf8.RuneCountInString(l), ""
		if i+1 < len(lines) {
			next = lines[i+1]
		}
		// A short line ends a paragraph when it ends a sentence, or (a
		// heading) when a capital starts the next.
		heading := n*100 < longest*60 && next != "" && unicode.IsUpper([]rune(next)[0]) && !strings.ContainsRune(",;", last)
		if n*100 < longest*85 && (strings.ContainsRune(".!?:", last) || heading) {
			end()
		}
	}
	end()
	return strings.Join(out, "\n\n")
}

// attachmentTools are the assistant's own tools for the files attached to
// a conversation (not the MCP server's: those files are the conversation's).
var attachmentTools = []mcpTool{
	{
		Name: "read_attachment", Title: "Read an attached file", ReadOnly: true,
		Description: "What a file they attached to this conversation says (its text), a part at a time: if there's more, \"next\" says where the next part starts (give it as from).",
		Props: map[string]mcpProp{
			"file_id": {Type: "string", Description: "The attached file's id"},
			"from":    {Type: "integer", Description: "Where to start, in characters (0 the beginning)"},
		},
		Required: []string{"file_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			q := url.Values{}
			if v, ok := a["from"].(float64); ok {
				q.Set("from", strconv.Itoa(int(v)))
			}
			var out map[string]any
			if err := m.do("GET", "/assistant/files/"+url.PathEscape(a.str("file_id"))+"/text", q, nil, &out); err != nil {
				return nil, err
			}
			return out, nil
		},
	},
	{
		Name: "save_attachment", Title: "Put a file in Files",
		Description: "Put a file they attached in a workspace's Files (in a folder, if given), as it is — the file itself, whatever it is (a draw.io diagram, a PDF, an image…).",
		Props: map[string]mcpProp{
			"file_id": {Type: "string", Description: "The attached file's id"}, "workspace_id": pWS,
			"folder_id": {Type: "string", Description: "A folder of that workspace's Files (none: the top)"},
		},
		Required: []string{"file_id", "workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var u map[string]any
			if err := m.do("POST", "/assistant/files/"+url.PathEscape(a.str("file_id"))+"/save", nil, a.pick("workspace_id", "folder_id"), &u); err != nil {
				return nil, err
			}
			return keepOne(u, "id", "name", "workspace_id", "folder_id"), nil
		},
	},
	{
		Name: "page_from_attachment", Title: "Make a page from a file",
		Description: "A new page with an attached file's text as it is — Markdown kept, a PDF's or a document's text as paragraphs — however long. To rewrite it instead (sum it up, split it into several pages), read it with read_attachment and use create_page.",
		Props: map[string]mcpProp{
			"file_id": {Type: "string", Description: "The attached file's id"}, "workspace_id": pWS,
			"title":     {Type: "string", Description: "Its title (none: the file's name)"},
			"parent_id": {Type: "string", Description: "The page it goes inside"}, "folder_id": {Type: "string", Description: "A folder to file it in"},
		},
		Required: []string{"file_id", "workspace_id"},
		Run: func(m *mcpCall, a mcpArgs) (any, error) {
			var t attachmentTextResponse
			if err := m.do("GET", "/assistant/files/"+url.PathEscape(a.str("file_id"))+"/text", url.Values{"whole": {"1"}}, nil, &t); err != nil {
				return nil, err
			}
			var f attachmentResponse
			_ = m.do("GET", "/assistant/files/"+url.PathEscape(a.str("file_id")), nil, nil, &f)
			body := a.pick("parent_id", "folder_id")
			body["title"] = a.str("title")
			if body["title"] == "" {
				body["title"] = strings.TrimSuffix(t.Name, path.Ext(t.Name))
			}
			body["content"] = asMarkdown(t.Name, f.Type, t.Text)
			var p map[string]any
			if err := m.do("POST", "/workspaces/"+a.str("workspace_id")+"/pages", nil, body, &p); err != nil {
				return nil, err
			}
			return keepOne(p, "id", "title", "workspace_id", "parent_id", "folder_id"), nil
		},
	},
}

// getAttachment describes a file attached for the assistant.
//
//	@Summary	An attached file
//	@Tags		assistant
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"File id"
//	@Success	200	{object}	attachmentResponse
//	@Router		/assistant/files/{id} [get]
func (h *Handlers) getAttachment(c *gin.Context) {
	u, ok := h.myAttachment(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, h.toAttachment(u))
}

// toolByName finds a tool of the MCP server's or the assistant's own.
func toolByName(name string) *mcpTool {
	if t := mcpToolByName(name); t != nil {
		return t
	}
	for i := range attachmentTools {
		if attachmentTools[i].Name == name {
			return &attachmentTools[i]
		}
	}
	return nil
}

// theirFiles tells the model about the files attached in the conversation,
// and what those sent with this message say (as much as fits).
func (h *Handlers) theirFiles(turns []assistant.Turn) string {
	var b strings.Builder
	room := attachmentShown
	for i, t := range turns {
		for _, f := range t.Files {
			if b.Len() == 0 {
				b.WriteString("Files they attached to this conversation (file_id: name, type, size); read one with read_attachment. In Agent mode, save_attachment puts one in a workspace's Files as it is, and page_from_attachment makes a page of its text as it is:")
			}
			fmt.Fprintf(&b, "\n- %s: %s, %s, %d KB", f.ID, f.Name, f.Type, (f.Size+1023)/1024)
			if i != len(turns)-1 {
				continue
			}
			b.WriteString(" (sent with this message)")
			var u models.Upload
			if h.db.First(&u, "id = ?", f.ID).Error != nil {
				continue
			}
			text := []rune(h.attachmentText(u))
			switch {
			case len(text) == 0:
				b.WriteString(" — Orbit can't read what it says")
			case len(text) <= room:
				fmt.Fprintf(&b, "\n<file name=%q>\n%s\n</file>", f.Name, string(text))
				room -= len(text)
			default:
				fmt.Fprintf(&b, " — %d characters; read it with read_attachment", len(text))
			}
		}
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

// attachTo checks the files sent with a message are the caller's, not yet on
// another conversation, and describes them.
func (h *Handlers) attachTo(uid, chatID string, ids []string) ([]assistant.File, bool) {
	var out []assistant.File
	for _, id := range ids {
		var u models.Upload
		if h.db.First(&u, "id = ? AND target = ? AND owner_id = ?", id, uploads.Chat, uid).Error != nil || (u.TargetID != "" && u.TargetID != chatID) {
			return nil, false
		}
		out = append(out, assistant.File{ID: u.ID, Name: u.Name, Type: u.Type, Size: u.Size})
	}
	return out, true
}

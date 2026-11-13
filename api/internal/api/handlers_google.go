package api

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/safedial"

	"github.com/gin-gonic/gin"
)

// A Google Sheets spreadsheet, by its link: Orbit asks Google for it as an
// Excel file (the app reads that). It must be shared "Anyone with the link".
// Only the export address made from the link's id is asked for, and only
// Google's own hosts are followed (no fetching of any other address).

const maxGoogleSheet = 20 << 20

var (
	googleDocs   = "https://docs.google.com"
	googleClient = func() *http.Client { return safedial.PublicClient(30 * time.Second) }
	// googleHost: the hosts a download may go through (its redirects).
	googleHost = func(u *url.URL) bool {
		h := strings.ToLower(u.Hostname())
		return u.Scheme == "https" && (h == "docs.google.com" || strings.HasSuffix(h, ".googleusercontent.com"))
	}
	googleID  = regexp.MustCompile(`^/spreadsheets/d/([A-Za-z0-9_-]{20,200})(?:/|$)`)
	googlePub = regexp.MustCompile(`^/spreadsheets/d/e/([A-Za-z0-9_-]{20,200})(?:/|$)`)
)

// googleExport is the Excel download address of a Google Sheets link
// ("" when it isn't one).
func googleExport(link string) string {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || !strings.EqualFold(u.Hostname(), "docs.google.com") {
		return ""
	}
	// Published to the web: …/d/e/<id>/pubhtml.
	if m := googlePub.FindStringSubmatch(u.Path); m != nil {
		return googleDocs + "/spreadsheets/d/e/" + m[1] + "/pub?output=xlsx"
	}
	if m := googleID.FindStringSubmatch(u.Path); m != nil && m[1] != "e" {
		return googleDocs + "/spreadsheets/d/" + m[1] + "/export?format=xlsx"
	}
	return ""
}

type googleSheetRequest struct {
	URL string `json:"url" binding:"required,max=2000"`
}

// googleSheet downloads a shared Google Sheets spreadsheet as .xlsx
// (editors), for the app to bring into a sheet or a database.
//
//	@Summary	A Google Sheets spreadsheet, as Excel
//	@Tags		sheets
//	@Accept		json
//	@Produce	application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security	BearerAuth
//	@Param		ws		path	string				true	"Workspace id"
//	@Param		body	body	googleSheetRequest	true	"Its link"
//	@Success	200		{file}	binary
//	@Failure	422		{object}	map[string]string
//	@Router		/workspaces/{ws}/google-sheet [post]
func (h *Handlers) googleSheet(c *gin.Context) {
	if _, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor); !ok {
		return
	}
	var req googleSheetRequest
	if !bindJSON(c, &req) {
		return
	}
	addr := googleExport(req.URL)
	if addr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "that isn't a Google Sheets link"})
		return
	}
	client := googleClient()
	for hop := 0; ; hop++ {
		u, err := url.Parse(addr)
		if err != nil || !googleHost(u) || hop > 5 {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Google sent the download somewhere unexpected"})
			return
		}
		r, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, addr, nil)
		if err != nil {
			h.internalError(c, err, "could not ask Google")
			return
		}
		res, err := client.Do(r)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "could not reach Google"})
			return
		}
		if res.StatusCode >= 300 && res.StatusCode < 400 {
			next, err := u.Parse(res.Header.Get("Location"))
			_ = res.Body.Close()
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": "Google sent the download somewhere unexpected"})
				return
			}
			addr = next.String()
			continue
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, maxGoogleSheet+1))
		_ = res.Body.Close()
		switch {
		case err != nil:
			c.JSON(http.StatusBadGateway, gin.H{"error": "could not read Google's answer"})
		case len(body) > maxGoogleSheet:
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the spreadsheet is too large (20 MB at most)"})
		case res.StatusCode == http.StatusNotFound:
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Google has no spreadsheet at that link"})
		// Not shared: Google answers with its sign-in page (or a 401 / 403).
		case res.StatusCode != http.StatusOK || !bytes.HasPrefix(body, []byte("PK")):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "Google didn't hand it over: share it as \"Anyone with the link\""})
		default:
			// Its name, as Google gives it ("Budget.xlsx").
			if _, ps, err := mime.ParseMediaType(res.Header.Get("Content-Disposition")); err == nil {
				if name := strings.TrimSpace(ps["filename"]); name != "" && len(name) <= 300 && !strings.ContainsAny(name, "/\\\x00") {
					c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
				}
			}
			c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", body)
		}
		return
	}
}

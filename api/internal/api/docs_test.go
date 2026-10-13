package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestAPIDocs: the Scalar page, the OpenAPI document, and the old Swagger link.
func TestAPIDocs(t *testing.T) {
	r := newTestRouter(t)
	w := do(r, "GET", "/api/docs", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `src="/scalar/standalone.js"`) || !strings.Contains(w.Body.String(), `data-url="/api/v1/openapi.json"`) {
		t.Fatalf("docs page: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "https://") {
		t.Error("the docs page must not load anything from another origin")
	}
	w = do(r, "GET", "/api/v1/openapi.json", "")
	var spec struct {
		Swagger string         `json:"swagger"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil || spec.Paths["/workspaces"] == nil || spec.Paths["/pages/{id}"] == nil {
		t.Fatalf("openapi: %v %d paths", err, len(spec.Paths))
	}
	w = do(r, "GET", "/api/v1/swagger/index.html", "")
	if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/api/docs" {
		t.Errorf("old swagger link: %d %s", w.Code, w.Header().Get("Location"))
	}
}

package api

import (
	"mime"
	"testing"

	"github.com/gin-gonic/gin"
)

// The install manifest must go out as application/manifest+json on any host,
// not only where /etc/mime.types knows .webmanifest.
func TestSPARegistersWebManifestType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registerSPA(gin.New(), nil)
	if got := mime.TypeByExtension(".webmanifest"); got != "application/manifest+json" {
		t.Errorf(".webmanifest = %q, want application/manifest+json", got)
	}
}

// Package webui embeds the built Vue SPA so the API ships as a single binary.
//
// At image-build time the Dockerfile copies web/dist/* into ./dist here, then
// `go build` embeds it. A placeholder index.html is checked in so the package
// compiles before any frontend build has run.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the SPA filesystem rooted at dist/.
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist is embedded at compile time; this cannot fail in practice.
		panic(err)
	}
	return sub
}

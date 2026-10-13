package api

import (
	"regexp"
)

// hexColorRe matches a #rrggbb colour (the only accent format the SPA emits).
var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// validHexColor reports whether s is empty or a #rrggbb hex colour.
func validHexColor(s string) bool { return s == "" || hexColorRe.MatchString(s) }

// imageDataURLRe matches an allowed image data: URL prefix.
var imageDataURLRe = regexp.MustCompile(`^data:image/(png|jpe?g|gif|webp|svg\+xml|x-icon|vnd\.microsoft\.icon)[;,]`)

// validImageDataURL reports whether s is empty or an allowed image data: URL.
// Branding/avatar blobs are only ever rendered via <img src>, but pinning the
// MIME keeps stored values sane and blocks data:text/html surprises.
func validImageDataURL(s string) bool { return s == "" || imageDataURLRe.MatchString(s) }

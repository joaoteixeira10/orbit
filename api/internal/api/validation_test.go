package api

import "testing"

func TestValidHexColor(t *testing.T) {
	ok := []string{"", "#3b82f6", "#FFFFFF", "#000000", "#abcdef"}
	for _, s := range ok {
		if !validHexColor(s) {
			t.Errorf("validHexColor(%q) = false, want true", s)
		}
	}
	bad := []string{"3b82f6", "#fff", "#12345g", "#1234567", "red", "javascript:alert(1)", "#3b82f6 "}
	for _, s := range bad {
		if validHexColor(s) {
			t.Errorf("validHexColor(%q) = true, want false", s)
		}
	}
}

func TestValidImageDataURL(t *testing.T) {
	ok := []string{"", "data:image/png;base64,AAAA", "data:image/svg+xml,<svg/>", "data:image/jpeg;base64,/9j/"}
	for _, s := range ok {
		if !validImageDataURL(s) {
			t.Errorf("validImageDataURL(%q) = false, want true", s)
		}
	}
	bad := []string{"data:text/html,<h1>", "https://x/y.png", "data:application/json,{}"}
	for _, s := range bad {
		if validImageDataURL(s) {
			t.Errorf("validImageDataURL(%q) = true, want false", s)
		}
	}
}

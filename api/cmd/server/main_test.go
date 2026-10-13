package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestProbeHealth covers the healthcheck subcommand's probe: 0 on a 200, 1 on a
// non-200, and 1 when the endpoint is unreachable.
func TestProbeHealth(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	if code := probeHealth(ok.URL); code != 0 {
		t.Errorf("healthy endpoint: got exit %d, want 0", code)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	if code := probeHealth(bad.URL); code != 1 {
		t.Errorf("503 endpoint: got exit %d, want 1", code)
	}

	// Unreachable address (nothing listening).
	if code := probeHealth("http://127.0.0.1:0/api/v1/healthz"); code != 1 {
		t.Errorf("unreachable endpoint: got exit %d, want 1", code)
	}
}

// TestProbeHealthTLS: the probe works against an HTTPS listener (in-app TLS).
func TestProbeHealthTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	if code := probeHealth(srv.URL); code != 0 {
		t.Errorf("TLS probe: %d", code)
	}
}

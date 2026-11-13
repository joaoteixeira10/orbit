package assistant

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOpenAIErrors: a refused key reads as one, Gemini's 400 included.
func TestOpenAIErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		badKey bool
	}{
		{http.StatusUnauthorized, `{"error":{"message":"Incorrect API key provided"}}`, true},
		{http.StatusBadRequest, `[{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}]`, true},
		{http.StatusBadRequest, `{"error":{"message":"API key not valid. Please pass a valid API key."}}`, true},
		{http.StatusBadRequest, `{"error":"bad request"}`, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		p, err := Config{Provider: OpenAI, URL: srv.URL, Model: "m", Key: "k"}.New(srv.Client())
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.Answer(context.Background(), Request{History: []Turn{{Role: "user", Text: "hi"}}})
		srv.Close()
		var he *HTTPError
		if !errors.As(err, &he) || he.Status != tc.status || he.BadKey() != tc.badKey {
			t.Errorf("%d %s: %v", tc.status, tc.body, err)
		}
	}
}

// TestGeminiConfig: Gemini needs a key and goes to Google's address.
func TestGeminiConfig(t *testing.T) {
	if _, err := (Config{Provider: Gemini, Model: "gemini-flash-latest"}).New(http.DefaultClient); !errors.Is(err, ErrNoConfig) {
		t.Fatalf("no key: %v", err)
	}
	p, err := Config{Provider: Gemini, Model: "gemini-flash-latest", Key: "k", URL: "http://elsewhere"}.New(http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if o := p.(*openAI); o.base != GeminiURL {
		t.Fatalf("base: %s", o.base)
	}
}

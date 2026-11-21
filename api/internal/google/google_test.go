package google

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestErrorsSayWhy: a refusal says Google's own reason (an API's message,
// or the sign-in's description), and an unreachable Google says the
// network's reason, not only that it failed.
func TestErrorsSayWhy(t *testing.T) {
	for _, c := range []struct {
		body, want string
	}{
		{`{"error":{"code":403,"message":"Google Calendar API has not been used in project 123 before or it is disabled.","status":"PERMISSION_DENIED"}}`, "refused by Google (403 Forbidden): Google Calendar API has not been used in project 123"},
		{`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`, "refused by Google (403 Forbidden): Token has been expired or revoked."},
		{`{"error":"invalid_client"}`, "refused by Google (403 Forbidden): invalid_client"},
		{`<html>nope</html>`, "refused by Google: 403 Forbidden"},
	} {
		if got := refused("403 Forbidden", []byte(c.body)).Error(); !strings.HasPrefix(got, c.want) {
			t.Errorf("refused(%s) = %q, want %q", c.body, got, c.want)
		}
	}

	// Through the calls: the calendar list, refused for want of a scope.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":403,"message":"Request had insufficient authentication scopes."}}`))
	}))
	old, oldHTTP := CalendarListURL, HTTP
	t.Cleanup(func() { CalendarListURL, HTTP = old, oldHTTP })
	CalendarListURL = srv.URL
	HTTP = func() *http.Client { return srv.Client() }
	_, err := Calendars(context.Background(), "acc")
	if err == nil || !strings.Contains(err.Error(), "insufficient authentication scopes") {
		t.Errorf("calendars: %v", err)
	}
	srv.Close()
	// And Google gone: the network's reason.
	_, err = Calendars(context.Background(), "acc")
	if err == nil || !strings.HasPrefix(err.Error(), "couldn't reach Google: ") || len(err.Error()) <= len("couldn't reach Google: ") {
		t.Errorf("unreachable: %v", err)
	}
	if got := unreachable(errors.New("dial tcp: i/o timeout")).Error(); got != "couldn't reach Google: dial tcp: i/o timeout" {
		t.Errorf("unreachable: %q", got)
	}
}

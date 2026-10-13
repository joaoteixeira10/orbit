package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestLoginCounter: each call adds one to its (method, result) series.
func TestLoginCounter(t *testing.T) {
	before := testutil.ToFloat64(logins.WithLabelValues("password", "failure"))
	Login("password", "failure")
	Login("password", "failure")
	Login("oidc", "success")
	if got := testutil.ToFloat64(logins.WithLabelValues("password", "failure")) - before; got != 2 {
		t.Errorf("password/failure grew by %v, want 2", got)
	}
	if testutil.ToFloat64(logins.WithLabelValues("oidc", "success")) < 1 {
		t.Error("oidc/success should be counted")
	}
}

// Package metrics defines Orbit's own Prometheus metrics and thin helpers
// the services call to record them. Domain gauges (workspaces, pages, open
// tasks) are collected from the database in package api.
//
// It imports nothing from the rest of the app, so any package can record metrics
// without an import cycle. All series live on the default Prometheus registry —
// the same one promhttp serves at /metrics — so they appear automatically when
// METRICS_ENABLED is on. Recording is cheap and always happens; only the
// exposition endpoint is gated.
//
// Label cardinality is deliberately bounded: `method` and `result` take small
// fixed sets of values.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var logins = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "orbit_logins_total",
	Help: "Authentication outcomes, by method (password|oidc|passkey) and result (success|failure).",
}, []string{"method", "result"})

// Login records an authentication outcome.
func Login(method, result string) { logins.WithLabelValues(method, result).Inc() }

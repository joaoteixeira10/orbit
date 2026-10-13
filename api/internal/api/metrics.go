package api

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"
)

// HTTP metrics are keyed by the matched route template (not the raw path) to
// keep label cardinality bounded. The default Prometheus registry also exposes
// Go runtime + process collectors.
var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "orbit_http_requests_total",
		Help: "Total HTTP requests by method, route, and status code.",
	}, []string{"method", "route", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "orbit_http_request_duration_seconds",
		Help:    "HTTP request latency in seconds by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})
)

// metricsMiddleware records request count + latency for every request.
func metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		httpDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
		httpRequests.WithLabelValues(c.Request.Method, route, strconv.Itoa(c.Writer.Status())).Inc()
	}
}

// metricsHandler serves the Prometheus exposition endpoint.
func metricsHandler() gin.HandlerFunc { return gin.WrapH(promhttp.Handler()) }

// Domain metrics: counted from the database when Prometheus scrapes (cheap
// COUNTs, and scrapes are rare). One collector for the process (tests build
// several routers: the last one's database is exported).
var (
	domainMetricsOnce sync.Once
	domainMetricsDB   atomic.Pointer[gorm.DB]

	descUsers      = prometheus.NewDesc("orbit_users", "Accounts (disabled ones included).", nil, nil)
	descWorkspaces = prometheus.NewDesc("orbit_workspaces", "Workspaces, archived ones included.", nil, nil)
	descPages      = prometheus.NewDesc("orbit_pages", "Pages not in the trash.", nil, nil)
	descOpenTasks  = prometheus.NewDesc("orbit_tasks_open", "Tasks not completed.", nil, nil)
	descSubsFailed = prometheus.NewDesc("orbit_calendar_subscriptions_failing", "Calendar subscriptions whose last refresh failed.", nil, nil)
)

type domainCollector struct{}

func (domainCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{descUsers, descWorkspaces, descPages, descOpenTasks, descSubsFailed} {
		ch <- d
	}
}

func (domainCollector) Collect(ch chan<- prometheus.Metric) {
	db := domainMetricsDB.Load()
	if db == nil {
		return
	}
	count := func(d *prometheus.Desc, q *gorm.DB) {
		var n int64
		if q.Count(&n).Error == nil {
			ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, float64(n))
		}
	}
	count(descUsers, db.Model(&models.User{}))
	count(descWorkspaces, db.Model(&models.Workspace{}))
	count(descPages, db.Model(&models.Page{}).Where("trashed_at IS NULL"))
	count(descOpenTasks, db.Model(&models.Task{}).Where("completed_at IS NULL"))
	count(descSubsFailed, db.Model(&models.CalendarSubscription{}).Where("last_error <> ''"))
}

// exportDomainMetrics registers the domain collector once and points it at db.
func exportDomainMetrics(db *gorm.DB) {
	domainMetricsDB.Store(db)
	domainMetricsOnce.Do(func() { prometheus.MustRegister(domainCollector{}) })
}

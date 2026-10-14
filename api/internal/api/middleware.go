package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const ctxRequestID = "request_id"

// requestID assigns each request a correlation id (honouring an inbound
// X-Request-Id), exposes it on the response, and stashes it in the context for
// the logger and handlers.
func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		c.Set(ctxRequestID, id)
		c.Writer.Header().Set("X-Request-Id", id)
		c.Next()
	}
}

// requestLogger logs each request with method, path, status, latency, and the
// correlation id. Runs after requestID().
func requestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
			zap.String("request_id", c.GetString(ctxRequestID)),
		)
	}
}

// securityHeaders sets baseline hardening headers on every response. TLS is
// terminated upstream, so HSTS is left to the proxy. The CSP is tuned for the
// embedded Vite SPA (external module scripts, data: images for QR codes and
// branding logos, same-origin /api XHR).
// extraConnectSrc, when non-nil, returns an extra origin to allow in the CSP
// connect-src. Orbit passes nil (same-origin only).
func securityHeaders(extraConnectSrc func() string) gin.HandlerFunc {
	build := func() string {
		connect := "'self'"
		if extraConnectSrc != nil {
			if o := extraConnectSrc(); o != "" {
				connect += " " + o
			}
		}
		// img-src allows https images: pages are Markdown and may show them.
		return "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data: https:; font-src 'self' data:; connect-src " + connect + "; " +
			// The SPA builds its branded web manifest as a blob: URL.
			"manifest-src 'self' blob:; " +
			"object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
	}
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy", build())
		c.Next()
	}
}

// limitBody caps each request body at max bytes (via http.MaxBytesReader) so an
// oversized JSON / data-URL payload can't exhaust memory. exempt returns true
// for paths that legitimately need a larger body (backup restore keeps its own
// io.LimitReader). A handler reading past the cap gets a clear error and the
// connection is closed by the reader.
func limitBody(max int64, exempt func(path string) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if max > 0 && c.Request.Body != nil && (exempt == nil || !exempt(c.Request.URL.Path)) {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		}
		c.Next()
	}
}

// ipRateLimiter is a small in-memory per-IP token bucket used as defence-in-depth
// brute-force protection on the authentication endpoints (the upstream proxy can
// add coarser limits). burst is the bucket size; refill is tokens-per-second.
type ipRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rlBucket
	burst   float64
	refill  float64
	lastGC  time.Time
}

type rlBucket struct {
	tokens float64
	last   time.Time
}

func newIPRateLimiter(burst, refillPerSec float64) *ipRateLimiter {
	return &ipRateLimiter{buckets: map[string]*rlBucket{}, burst: burst, refill: refillPerSec, lastGC: time.Now()}
}

// allow reports whether the IP may proceed, consuming a token if so.
func (l *ipRateLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastGC) > 10*time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.last) > 10*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.lastGC = now
	}
	b := l.buckets[ip]
	if b == nil {
		b = &rlBucket{tokens: l.burst, last: now}
		l.buckets[ip] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * l.refill
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// middleware throttles by client IP, returning 429 when the bucket is empty.
func (l *ipRateLimiter) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.allow(c.ClientIP(), time.Now()) {
			c.Header("Retry-After", "30")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts; please slow down"})
			return
		}
		c.Next()
	}
}

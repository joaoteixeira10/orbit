// Package safedial provides an SSRF-guarded net dialer and HTTP client used for
// every admin-configurable outbound request (notification webhooks, OIDC
// discovery, the update check).
//
// It resolves the target and refuses to connect to loopback, link-local
// (including the cloud-metadata address 169.254.169.254) or unspecified
// addresses, checking the concrete IP it will dial so DNS-rebinding can't slip
// past. The client also never follows redirects, so a public URL can't 302 to a
// blocked target. Private LAN ranges remain reachable on purpose: homelab
// targets (self-hosted ntfy/Gotify, internal OIDC) are legitimate.
package safedial

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Blocked reports whether an IP is one we refuse to dial (loopback, link-local,
// unspecified). Exported so URL pre-checks share one source of truth.
func Blocked(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsInterfaceLocalMulticast()
}

// Private reports addresses of a private network: RFC 1918, carrier-grade
// NAT (RFC 6598) and IPv6 unique-local — refused by the public-only dialer.
func Private(ip net.IP) bool {
	return ip.IsPrivate() || cgnat.Contains(ip)
}

var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// DialContext resolves host and dials only an allowed IP, defeating DNS-rebind
// (it checks the concrete address it connects to, not the hostname).
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return dial(ctx, network, addr, Blocked)
}

// PublicDialContext is DialContext that also refuses private networks: for
// targets any user may name that are never on the LAN (Web Push services,
// git servers not allowed by the operator), so the request can't probe it.
func PublicDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return dial(ctx, network, addr, func(ip net.IP) bool { return Blocked(ip) || Private(ip) })
}

func dial(ctx context.Context, network, addr string, blocked func(net.IP) bool) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("safedial: could not resolve %q: %w", host, err)
	}
	var dialer net.Dialer
	dialer.Timeout = 8 * time.Second
	var lastErr error
	for _, ip := range ips {
		if blocked(ip) {
			continue // skip blocked targets
		}
		conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	if lastErr == nil {
		lastErr = errors.New("safedial: no allowed address for host")
	}
	return nil, lastErr
}

// GuardedClient returns an *http.Client that dials through DialContext, never
// follows redirects, and disables keep-alives.
func GuardedClient(timeout time.Duration) *http.Client {
	return client(timeout, DialContext)
}

// PublicClient is GuardedClient over PublicDialContext (no private networks).
func PublicClient(timeout time.Duration) *http.Client {
	return client(timeout, PublicDialContext)
}

func client(timeout time.Duration, dial func(context.Context, string, string) (net.Conn, error)) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DialContext:           dial,
			DisableKeepAlives:     true,
			ResponseHeaderTimeout: 8 * time.Second,
		},
	}
}

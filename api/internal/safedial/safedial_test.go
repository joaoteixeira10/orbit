package safedial

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestBlocked(t *testing.T) {
	blocked := []string{"127.0.0.1", "::1", "169.254.169.254", "0.0.0.0", "fe80::1"}
	for _, s := range blocked {
		if !Blocked(net.ParseIP(s)) {
			t.Errorf("Blocked(%s) = false, want true", s)
		}
	}
	// Public + private-LAN addresses stay reachable on purpose (homelab targets).
	allowed := []string{"8.8.8.8", "1.1.1.1", "10.0.0.5", "192.168.1.10", "172.16.0.1"}
	for _, s := range allowed {
		if Blocked(net.ParseIP(s)) {
			t.Errorf("Blocked(%s) = true, want false", s)
		}
	}
}

func TestDialContextBlocksLoopbackAndMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := DialContext(ctx, "tcp", "127.0.0.1:9"); err == nil {
		t.Error("DialContext to loopback succeeded, want blocked")
	}
	if _, err := DialContext(ctx, "tcp", "169.254.169.254:80"); err == nil {
		t.Error("DialContext to cloud-metadata link-local succeeded, want blocked")
	}
}

func TestGuardedClientDoesNotFollowRedirects(t *testing.T) {
	c := GuardedClient(5 * time.Second)
	// CheckRedirect must signal "don't follow" so a public URL can't 302 to a
	// blocked target; the caller sees the redirect response itself.
	if c.CheckRedirect == nil {
		t.Fatal("GuardedClient has no CheckRedirect policy")
	}
	if err := c.CheckRedirect(nil, nil); err == nil {
		t.Error("CheckRedirect returned nil (would follow redirects), want ErrUseLastResponse")
	}
}

// TestPublicDialRefusesPrivate: the public-only dialer refuses private
// networks (and still everything the guarded one does); the guarded one
// allows the LAN.
func TestPublicDialRefusesPrivate(t *testing.T) {
	for _, ip := range []string{"10.1.2.3", "172.16.0.9", "192.168.1.10", "100.64.0.1", "fd00::1"} {
		if !Private(net.ParseIP(ip)) {
			t.Errorf("%s not private", ip)
		}
		if _, err := PublicDialContext(context.Background(), "tcp", net.JoinHostPort(ip, "443")); err == nil {
			t.Errorf("public dial reached %s", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if Private(net.ParseIP(ip)) {
			t.Errorf("%s counted as private", ip)
		}
	}
	if _, err := PublicDialContext(context.Background(), "tcp", "127.0.0.1:80"); err == nil {
		t.Error("public dial reached loopback")
	}
}

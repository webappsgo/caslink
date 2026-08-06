package config

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestPeerIP(t *testing.T) {
	cases := map[string]string{
		"1.2.3.4:5678":    "1.2.3.4",
		"[::1]:8080":      "::1",
		"1.2.3.4":         "1.2.3.4",
		" 1.2.3.4 ":       "1.2.3.4",
		"[2001:db8::1]:0": "2001:db8::1",
	}
	for in, want := range cases {
		if got := PeerIP(in); got != want {
			t.Errorf("PeerIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsTrustedProxy(t *testing.T) {
	// Default-trusted ranges: loopback, RFC1918, link-local, ULA.
	trustedDefault := []string{
		"127.0.0.1", "10.1.2.3", "172.16.5.5", "192.168.0.1",
		"169.254.1.1", "::1", "fe80::1", "fd00::1",
	}
	for _, ip := range trustedDefault {
		if !IsTrustedProxy(ip, nil) {
			t.Errorf("IsTrustedProxy(%q, nil) = false, want true (default-trusted)", ip)
		}
	}

	// Public addresses must NOT be trusted without explicit configuration —
	// this is the security property: a direct public peer cannot spoof XFF.
	untrusted := []string{"8.8.8.8", "1.1.1.1", "203.0.113.7", "2001:db8::1"}
	for _, ip := range untrusted {
		if IsTrustedProxy(ip, nil) {
			t.Errorf("IsTrustedProxy(%q, nil) = true, want false (public peer)", ip)
		}
	}

	// Additional entries: exact IP and CIDR.
	additional := []string{"203.0.113.7", "198.51.100.0/24", "  ", "garbage"}
	if !IsTrustedProxy("203.0.113.7", additional) {
		t.Error("configured exact IP should be trusted")
	}
	if !IsTrustedProxy("198.51.100.42", additional) {
		t.Error("IP inside configured CIDR should be trusted")
	}
	if IsTrustedProxy("198.51.101.1", additional) {
		t.Error("IP outside configured CIDR must not be trusted")
	}
	if IsTrustedProxy("not-an-ip", additional) {
		t.Error("unparseable peer must not be trusted")
	}
}

func TestTrustedProxyResolver_StaticAndDefault(t *testing.T) {
	// A resolver with no hostnames behaves like IsTrustedProxy: default
	// ranges plus static IP/CIDR entries match, everything else does not.
	r := NewTrustedProxyResolver([]string{"203.0.113.7", "198.51.100.0/24", "   "})
	if len(r.hostnames) != 0 {
		t.Fatalf("expected 0 hostname entries, got %v", r.hostnames)
	}
	trusted := []string{"127.0.0.1", "10.0.0.5", "203.0.113.7", "198.51.100.42"}
	for _, ip := range trusted {
		if !r.IsTrusted(ip) {
			t.Errorf("IsTrusted(%q) = false, want true", ip)
		}
	}
	untrusted := []string{"8.8.8.8", "203.0.113.8", "not-an-ip"}
	for _, ip := range untrusted {
		if r.IsTrusted(ip) {
			t.Errorf("IsTrusted(%q) = true, want false", ip)
		}
	}
}

func TestTrustedProxyResolver_HostnameResolvedOnRefresh(t *testing.T) {
	// Before Refresh a hostname entry contributes nothing; after a
	// successful Refresh its resolved IPs are trusted.
	r := NewTrustedProxyResolver([]string{"proxy.example.com"})
	if len(r.hostnames) != 1 {
		t.Fatalf("expected 1 hostname entry, got %v", r.hostnames)
	}
	r.warnf = func(string, ...any) {}
	r.lookup = func(host string) ([]net.IP, error) {
		if host != "proxy.example.com" {
			t.Fatalf("unexpected lookup host %q", host)
		}
		return []net.IP{net.ParseIP("203.0.113.9"), net.ParseIP("2001:db8::9")}, nil
	}

	if r.IsTrusted("203.0.113.9") {
		t.Fatal("hostname IP trusted before Refresh")
	}
	r.Refresh(context.Background())
	if !r.IsTrusted("203.0.113.9") {
		t.Error("resolved IPv4 not trusted after Refresh")
	}
	if !r.IsTrusted("2001:db8::9") {
		t.Error("resolved IPv6 not trusted after Refresh")
	}
	if r.IsTrusted("203.0.113.10") {
		t.Error("unrelated IP trusted after Refresh")
	}
}

func TestTrustedProxyResolver_LookupFailureKeepsLastGood(t *testing.T) {
	// A lookup that fails after a prior success must retain the previous IPs
	// and log a warning, never strip a legitimate proxy mid-flight.
	r := NewTrustedProxyResolver([]string{"proxy.example.com"})
	var warned bool
	r.warnf = func(string, ...any) { warned = true }

	calls := 0
	r.lookup = func(string) ([]net.IP, error) {
		calls++
		if calls == 1 {
			return []net.IP{net.ParseIP("203.0.113.11")}, nil
		}
		return nil, errors.New("dns timeout")
	}

	r.Refresh(context.Background())
	if !r.IsTrusted("203.0.113.11") {
		t.Fatal("IP not trusted after first successful refresh")
	}
	r.Refresh(context.Background()) // fails
	if !warned {
		t.Error("expected a warning to be logged on lookup failure")
	}
	if !r.IsTrusted("203.0.113.11") {
		t.Error("last-good IP dropped after a failed refresh")
	}
}

func TestTrustedProxyResolver_StartInitialRefreshAndCancel(t *testing.T) {
	r := NewTrustedProxyResolver([]string{"proxy.example.com"})
	r.warnf = func(string, ...any) {}
	resolved := make(chan struct{}, 1)
	r.lookup = func(string) ([]net.IP, error) {
		select {
		case resolved <- struct{}{}:
		default:
		}
		return []net.IP{net.ParseIP("203.0.113.12")}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Start(ctx, 10*time.Millisecond); close(done) }()

	select {
	case <-resolved:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("Start did not perform initial resolution")
	}
	if !r.IsTrusted("203.0.113.12") {
		t.Error("IP not trusted after Start's initial refresh")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}

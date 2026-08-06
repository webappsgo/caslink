package config

import (
	"context"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// defaultTrustedProxyNets are always trusted as reverse proxies without any
// explicit configuration: loopback, RFC1918 private ranges, IPv4 and IPv6
// link-local, and IPv6 unique-local (ULA). Per AI.md PART 12, X-Forwarded-*
// headers are only honored from a peer in one of these ranges or in the
// operator-configured `additional` list.
var defaultTrustedProxyNets = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",
		"::1/128",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"fe80::/10",
		"fc00::/7",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}()

// PeerIP returns the connecting peer's bare IP from an "ip:port" RemoteAddr,
// falling back to the trimmed input when it carries no port.
func PeerIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return strings.TrimSpace(remoteAddr)
}

// TrustedProxyRefreshInterval is how often the resolver re-resolves hostname
// entries in server.trusted_proxies.additional (config-rules.md: "refreshed
// every 5 min").
const TrustedProxyRefreshInterval = 5 * time.Minute

// TrustedProxyResolver evaluates the X-Forwarded-* trust gate for a peer IP,
// including operator-configured `additional` entries that are DNS hostnames.
//
// Hostnames are never resolved on the request path — a DNS lookup per request
// would block every request and hand an attacker a trivial latency oracle.
// Instead a background loop (Start) re-resolves them on a fixed cycle into a
// cache that IsTrusted reads under a read lock. A lookup that fails keeps the
// previous good result and logs a warning rather than silently dropping a
// proxy mid-flight, so a transient DNS outage never strips forwarded headers
// from a legitimate proxy.
type TrustedProxyResolver struct {
	static    []string // bare-IP and CIDR entries, matched directly
	hostnames []string // DNS entries resolved on the refresh cycle
	lookup    func(host string) ([]net.IP, error)
	warnf     func(format string, args ...any)

	mu       sync.RWMutex
	resolved map[string][]net.IP // hostname -> last successful resolution
}

// NewTrustedProxyResolver splits additional into directly-matchable IP/CIDR
// entries and DNS hostnames. Hostname entries carry no cached IPs until the
// first Refresh, so until then only default ranges and static entries match.
func NewTrustedProxyResolver(additional []string) *TrustedProxyResolver {
	r := &TrustedProxyResolver{
		lookup:   net.LookupIP,
		warnf:    log.Printf,
		resolved: make(map[string][]net.IP),
	}
	for _, entry := range additional {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") || net.ParseIP(entry) != nil {
			r.static = append(r.static, entry)
			continue
		}
		r.hostnames = append(r.hostnames, entry)
	}
	return r
}

// Refresh re-resolves every hostname entry once. On success the hostname's
// cache entry is replaced; on failure the previous good entry is retained and
// a warning is logged. Safe to call concurrently with IsTrusted.
func (r *TrustedProxyResolver) Refresh(ctx context.Context) {
	for _, host := range r.hostnames {
		if ctx != nil && ctx.Err() != nil {
			return
		}
		ips, err := r.lookup(host)
		if err != nil {
			r.warnf("trusted_proxies: could not resolve %q, keeping last known IPs: %v", host, err)
			continue
		}
		cp := make([]net.IP, len(ips))
		copy(cp, ips)
		r.mu.Lock()
		r.resolved[host] = cp
		r.mu.Unlock()
	}
}

// Start runs an initial Refresh, then re-resolves on interval until ctx is
// cancelled. Intended to be launched in its own goroutine.
func (r *TrustedProxyResolver) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = TrustedProxyRefreshInterval
	}
	r.Refresh(ctx)
	if len(r.hostnames) == 0 {
		return // nothing to keep refreshing
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Refresh(ctx)
		}
	}
}

// IsTrusted reports whether peerIP (a bare IP string) is a trusted reverse
// proxy: in a default-trusted range, matching a static IP/CIDR entry, or
// matching one of the currently-cached IPs for a hostname entry. Callers MUST
// ignore X-Forwarded-* headers when this returns false.
func (r *TrustedProxyResolver) IsTrusted(peerIP string) bool {
	if IsTrustedProxy(peerIP, r.static) {
		return true
	}
	ip := net.ParseIP(strings.TrimSpace(peerIP))
	if ip == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ips := range r.resolved {
		for _, cached := range ips {
			if cached.Equal(ip) {
				return true
			}
		}
	}
	return false
}

// IsTrustedProxy reports whether peerIP (a bare IP string) is a trusted
// reverse proxy: either in a default-trusted range or matching one of the
// operator-configured `additional` entries (a bare IP or a CIDR). Callers
// MUST ignore X-Forwarded-* headers when this returns false.
//
// Hostname entries in additional are not resolved here — resolving DNS on the
// request path would block every request. Use TrustedProxyResolver (which
// wraps this for the static case and adds a background-refreshed hostname
// cache) when hostname entries must be honored.
func IsTrustedProxy(peerIP string, additional []string) bool {
	ip := net.ParseIP(strings.TrimSpace(peerIP))
	if ip == nil {
		return false
	}
	for _, n := range defaultTrustedProxyNets {
		if n.Contains(ip) {
			return true
		}
	}
	for _, entry := range additional {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			if _, n, err := net.ParseCIDR(entry); err == nil && n.Contains(ip) {
				return true
			}
			continue
		}
		if p := net.ParseIP(entry); p != nil && p.Equal(ip) {
			return true
		}
	}
	return false
}

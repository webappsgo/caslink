package config

import (
	"net"
	"strings"
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

// IsTrustedProxy reports whether peerIP (a bare IP string) is a trusted
// reverse proxy: either in a default-trusted range or matching one of the
// operator-configured `additional` entries (a bare IP or a CIDR). Callers
// MUST ignore X-Forwarded-* headers when this returns false.
//
// Hostname entries in additional are not resolved here — resolving DNS on
// the request path would block every request; see TODO.AI.md for the
// periodic-refresh cache that will add hostname support.
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

package config

import "testing"

func TestPeerIP(t *testing.T) {
	cases := map[string]string{
		"1.2.3.4:5678":   "1.2.3.4",
		"[::1]:8080":     "::1",
		"1.2.3.4":        "1.2.3.4",
		" 1.2.3.4 ":      "1.2.3.4",
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

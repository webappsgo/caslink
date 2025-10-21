package proxy

import (
	"net"
	"net/http"
	"strings"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/sirupsen/logrus"
)

// Detector handles proxy detection and header processing
type Detector struct {
	config         *config.ServerConfig
	logger         *logrus.Logger
	trustedNetworks []*net.IPNet
}

// NewDetector creates a new proxy detector
func NewDetector(cfg *config.ServerConfig, logger *logrus.Logger) *Detector {
	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	// Parse trusted proxy networks
	detector.parseTrustedNetworks()

	return detector
}

// GetClientIP extracts the real client IP from the request
func (d *Detector) GetClientIP(r *http.Request) string {
	// Get the remote address
	remoteAddr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}

	// If not behind proxy or remote IP is not trusted, return remote IP
	if !d.isBehindProxy() || !d.isTrustedProxy(remoteAddr) {
		return remoteAddr
	}

	// Try different proxy headers in order of preference
	clientIP := d.getClientIPFromHeaders(r)
	if clientIP != "" && d.isValidIP(clientIP) {
		return clientIP
	}

	// Fallback to remote address
	return remoteAddr
}

// ProcessHeaders processes and validates proxy headers
func (d *Detector) ProcessHeaders(r *http.Request) {
	if !d.isBehindProxy() {
		return
	}

	// Process X-Forwarded-For chain
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		d.processXFFHeader(r, xff)
	}

	// Process X-Real-IP
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		d.processRealIPHeader(r, realIP)
	}

	// Process Forwarded header (RFC 7239)
	if forwarded := r.Header.Get("Forwarded"); forwarded != "" {
		d.processForwardedHeader(r, forwarded)
	}
}

// GetOriginalHost returns the original host from proxy headers
func (d *Detector) GetOriginalHost(r *http.Request) string {
	if !d.isBehindProxy() {
		return r.Host
	}

	// Try X-Forwarded-Host first
	if host := r.Header.Get("X-Forwarded-Host"); host != "" {
		return strings.Split(host, ",")[0]
	}

	// Try X-Original-Host
	if host := r.Header.Get("X-Original-Host"); host != "" {
		return host
	}

	// Try X-Host
	if host := r.Header.Get("X-Host"); host != "" {
		return host
	}

	// Fallback to request host
	return r.Host
}

// GetOriginalScheme returns the original scheme from proxy headers
func (d *Detector) GetOriginalScheme(r *http.Request) string {
	if !d.isBehindProxy() {
		if r.TLS != nil {
			return "https"
		}
		return "http"
	}

	// Try X-Forwarded-Proto
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return strings.Split(proto, ",")[0]
	}

	// Try X-Forwarded-Protocol
	if proto := r.Header.Get("X-Forwarded-Protocol"); proto != "" {
		return proto
	}

	// Try X-Scheme
	if scheme := r.Header.Get("X-Scheme"); scheme != "" {
		return scheme
	}

	// Try Cloudflare CF-Visitor
	if visitor := r.Header.Get("CF-Visitor"); visitor != "" {
		if strings.Contains(visitor, `"scheme":"https"`) {
			return "https"
		}
		if strings.Contains(visitor, `"scheme":"http"`) {
			return "http"
		}
	}

	// Fallback based on TLS
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// parseTrustedNetworks parses the trusted proxy networks configuration
func (d *Detector) parseTrustedNetworks() {
	for _, cidr := range d.config.TrustedProxies {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// Try parsing as single IP
			if ip := net.ParseIP(cidr); ip != nil {
				if ip.To4() != nil {
					network = &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
				} else {
					network = &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
				}
			} else {
				d.logger.WithError(err).WithField("cidr", cidr).Warn("Invalid trusted proxy network")
				continue
			}
		}
		d.trustedNetworks = append(d.trustedNetworks, network)
	}
}

// isBehindProxy checks if the server is configured to be behind a proxy
func (d *Detector) isBehindProxy() bool {
	switch d.config.BehindProxy {
	case "true":
		return true
	case "false":
		return false
	case "auto":
		return d.autoDetectProxy()
	default:
		return d.autoDetectProxy()
	}
}

// autoDetectProxy automatically detects if running behind a proxy
func (d *Detector) autoDetectProxy() bool {
	// Check for common proxy environment variables
	proxyEnvVars := []string{
		"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy",
		"KUBERNETES_SERVICE_HOST", "DOCKER_HOST",
	}

	for _, envVar := range proxyEnvVars {
		if value := strings.TrimSpace(strings.ToLower(envVar)); value != "" {
			return true
		}
	}

	// Check if binding to all interfaces (common in containerized environments)
	if d.config.Host == "0.0.0.0" {
		return true
	}

	return false
}

// isTrustedProxy checks if an IP is from a trusted proxy
func (d *Detector) isTrustedProxy(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, network := range d.trustedNetworks {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// getClientIPFromHeaders extracts client IP from various proxy headers
func (d *Detector) getClientIPFromHeaders(r *http.Request) string {
	// Priority order for IP detection
	ipHeaders := []struct {
		header string
		parser func(string) string
	}{
		{"CF-Connecting-IP", d.parseSimpleIP},      // Cloudflare (highest priority)
		{"True-Client-IP", d.parseSimpleIP},        // Cloudflare Enterprise
		{"X-Real-IP", d.parseSimpleIP},             // nginx
		{"X-Client-IP", d.parseSimpleIP},           // Generic
		{"X-Forwarded-For", d.parseXFFIP},          // Standard (comma-separated)
		{"X-Cluster-Client-IP", d.parseSimpleIP},   // Kubernetes
		{"X-Original-Forwarded-For", d.parseXFFIP}, // Proxy chains
		{"Forwarded", d.parseForwardedIP},          // RFC 7239
	}

	for _, header := range ipHeaders {
		if value := r.Header.Get(header.header); value != "" {
			if ip := header.parser(value); ip != "" && d.isValidIP(ip) {
				return ip
			}
		}
	}

	return ""
}

// parseSimpleIP parses a simple IP address
func (d *Detector) parseSimpleIP(value string) string {
	return strings.TrimSpace(value)
}

// parseXFFIP parses X-Forwarded-For header (comma-separated list)
func (d *Detector) parseXFFIP(value string) string {
	// X-Forwarded-For can contain multiple IPs: "client, proxy1, proxy2"
	// We want the first (leftmost) IP which should be the original client
	ips := strings.Split(value, ",")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if d.isValidIP(ip) && !d.isTrustedProxy(ip) {
			return ip
		}
	}
	return ""
}

// parseForwardedIP parses RFC 7239 Forwarded header
func (d *Detector) parseForwardedIP(value string) string {
	// Forwarded header format: for=192.0.2.60;proto=http;by=203.0.113.43
	parts := strings.Split(value, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "for=") {
			ip := strings.TrimPrefix(part, "for=")
			// Remove quotes if present
			ip = strings.Trim(ip, `"`)
			// Handle IPv6 brackets
			if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
				ip = ip[1 : len(ip)-1]
			}
			// Remove port if present
			if colonIndex := strings.LastIndex(ip, ":"); colonIndex != -1 {
				if net.ParseIP(ip[:colonIndex]) != nil {
					ip = ip[:colonIndex]
				}
			}
			if d.isValidIP(ip) {
				return ip
			}
		}
	}
	return ""
}

// processXFFHeader processes and validates X-Forwarded-For header
func (d *Detector) processXFFHeader(r *http.Request, xff string) {
	ips := strings.Split(xff, ",")
	var validIPs []string

	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if d.isValidIP(ip) {
			validIPs = append(validIPs, ip)
		}
	}

	if len(validIPs) > 0 {
		// Set cleaned header
		r.Header.Set("X-Forwarded-For", strings.Join(validIPs, ", "))
	}
}

// processRealIPHeader processes and validates X-Real-IP header
func (d *Detector) processRealIPHeader(r *http.Request, realIP string) {
	ip := strings.TrimSpace(realIP)
	if !d.isValidIP(ip) {
		// Remove invalid header
		r.Header.Del("X-Real-IP")
	}
}

// processForwardedHeader processes and validates Forwarded header
func (d *Detector) processForwardedHeader(r *http.Request, forwarded string) {
	// Basic validation - ensure it follows RFC 7239 format
	if !strings.Contains(forwarded, "=") {
		r.Header.Del("Forwarded")
	}
}

// isValidIP checks if a string represents a valid IP address
func (d *Detector) isValidIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// Reject private/reserved addresses in proxy headers
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
		return false
	}

	return true
}

// GetProxyChain returns the complete proxy chain from headers
func (d *Detector) GetProxyChain(r *http.Request) []string {
	var chain []string

	// Get from X-Forwarded-For
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if d.isValidIP(ip) {
				chain = append(chain, ip)
			}
		}
	}

	return chain
}

// GetProxyInfo returns comprehensive proxy information
func (d *Detector) GetProxyInfo(r *http.Request) *ProxyInfo {
	return &ProxyInfo{
		IsBehindProxy:  d.isBehindProxy(),
		ClientIP:       d.GetClientIP(r),
		OriginalHost:   d.GetOriginalHost(r),
		OriginalScheme: d.GetOriginalScheme(r),
		ProxyChain:     d.GetProxyChain(r),
		Headers:        d.extractProxyHeaders(r),
	}
}

// extractProxyHeaders extracts all proxy-related headers
func (d *Detector) extractProxyHeaders(r *http.Request) map[string]string {
	proxyHeaders := map[string]string{}

	headerNames := []string{
		"X-Forwarded-For", "X-Real-IP", "X-Client-IP", "X-Forwarded-Proto",
		"X-Forwarded-Host", "X-Forwarded-Port", "X-Original-Host",
		"CF-Connecting-IP", "CF-Visitor", "True-Client-IP",
		"Forwarded", "X-Cluster-Client-IP", "X-Original-Forwarded-For",
	}

	for _, header := range headerNames {
		if value := r.Header.Get(header); value != "" {
			proxyHeaders[header] = value
		}
	}

	return proxyHeaders
}

// ProxyInfo contains comprehensive proxy information
type ProxyInfo struct {
	IsBehindProxy  bool              `json:"is_behind_proxy"`
	ClientIP       string            `json:"client_ip"`
	OriginalHost   string            `json:"original_host"`
	OriginalScheme string            `json:"original_scheme"`
	ProxyChain     []string          `json:"proxy_chain"`
	Headers        map[string]string `json:"headers"`
}
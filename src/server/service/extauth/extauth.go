// Package extauth provides the external identity-provider "test connection"
// boundaries (OIDC discovery, LDAP reachability, SAML metadata) used by the
// admin panel per AI.md PART 34. Each boundary is an interface so the admin
// "Test" action works against mocks in tests and real endpoints in production.
//
// This is the CONFIG + ADMIN MANAGEMENT layer only. Full OIDC code exchange,
// LDAP bind/search, and SAML assertion validation belong to the separate
// login-flow feature; the default implementations here deliberately do the
// minimum needed to confirm an operator's provider config is reachable and
// well-formed, using only the standard library so no heavy auth dependencies
// are pulled in before the login feature needs them.
package extauth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/webappsgo/caslink/src/config"
)

// OIDCEndpoints holds the subset of the discovery document the admin panel
// surfaces after a successful "Test" against an OIDC issuer.
type OIDCEndpoints struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

// SAMLMetadata holds the identity extracted from an IdP metadata document.
type SAMLMetadata struct {
	EntityID string   `json:"entity_id"`
	SSOURLs  []string `json:"sso_urls"`
}

// OIDCDiscoverer fetches an issuer's discovery document.
type OIDCDiscoverer interface {
	Discover(ctx context.Context, issuer string) (OIDCEndpoints, error)
}

// LDAPBinder verifies that an LDAP provider is reachable.
type LDAPBinder interface {
	TestBind(ctx context.Context, provider config.LDAPProvider) error
}

// SAMLMetadataFetcher fetches and parses IdP metadata from a URL.
type SAMLMetadataFetcher interface {
	Fetch(ctx context.Context, metadataURL string) (SAMLMetadata, error)
}

// Testers bundles the three boundaries so a handler can hold one field.
type Testers struct {
	OIDC OIDCDiscoverer
	LDAP LDAPBinder
	SAML SAMLMetadataFetcher
}

// Default returns Testers backed by the standard-library implementations.
func Default() Testers {
	return Testers{
		OIDC: HTTPOIDCDiscoverer{},
		LDAP: DialLDAPBinder{},
		SAML: HTTPSAMLMetadataFetcher{},
	}
}

// httpGet performs a bounded GET honoring the request context.
func httpGet(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return body, nil
}

// HTTPOIDCDiscoverer fetches .well-known/openid-configuration over HTTP.
type HTTPOIDCDiscoverer struct{}

// Discover implements OIDCDiscoverer.
func (HTTPOIDCDiscoverer) Discover(ctx context.Context, issuer string) (OIDCEndpoints, error) {
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		return OIDCEndpoints{}, fmt.Errorf("issuer is empty")
	}
	well := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	body, err := httpGet(ctx, well)
	if err != nil {
		return OIDCEndpoints{}, fmt.Errorf("discover %s: %w", issuer, err)
	}
	var ep OIDCEndpoints
	if err := json.Unmarshal(body, &ep); err != nil {
		return OIDCEndpoints{}, fmt.Errorf("parse discovery document: %w", err)
	}
	if ep.AuthorizationEndpoint == "" || ep.TokenEndpoint == "" {
		return OIDCEndpoints{}, fmt.Errorf("discovery document missing required endpoints")
	}
	return ep, nil
}

// DialLDAPBinder confirms the directory server accepts a TCP/TLS connection.
// A full authenticated bind is deferred to the login-flow feature.
type DialLDAPBinder struct{}

// TestBind implements LDAPBinder.
func (DialLDAPBinder) TestBind(ctx context.Context, provider config.LDAPProvider) error {
	host, useTLS, err := ldapDialTarget(provider)
	if err != nil {
		return err
	}
	timeout := 5 * time.Second
	if d, perr := time.ParseDuration(provider.ConnectTimeout); perr == nil && d > 0 {
		timeout = d
	}
	dialer := &net.Dialer{Timeout: timeout}
	if useTLS {
		serverName := host
		if h, _, serr := net.SplitHostPort(host); serr == nil {
			serverName = h
		}
		conn, terr := tls.DialWithDialer(dialer, "tcp", host, &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: !provider.EffectiveTLSVerify(),
		})
		if terr != nil {
			return fmt.Errorf("ldaps dial %s: %w", host, terr)
		}
		return conn.Close()
	}
	conn, derr := dialer.DialContext(ctx, "tcp", host)
	if derr != nil {
		return fmt.Errorf("ldap dial %s: %w", host, derr)
	}
	return conn.Close()
}

// ldapDialTarget resolves a host:port and whether TLS is used from the provider
// server URL and tls_mode.
func ldapDialTarget(provider config.LDAPProvider) (string, bool, error) {
	server := strings.TrimSpace(provider.Server)
	if server == "" {
		return "", false, fmt.Errorf("ldap server is empty")
	}
	scheme := "ldap"
	rest := server
	if i := strings.Index(server, "://"); i >= 0 {
		scheme = strings.ToLower(server[:i])
		rest = server[i+3:]
	}
	rest = strings.TrimSuffix(rest, "/")
	host := rest
	port := "389"
	useTLS := scheme == "ldaps" || provider.TLSMode == "ldaps"
	if useTLS {
		port = "636"
	}
	if h, p, err := net.SplitHostPort(rest); err == nil {
		host, port = h, p
	} else if u, perr := url.Parse(server); perr == nil && u.Host != "" {
		host = u.Hostname()
		if u.Port() != "" {
			port = u.Port()
		}
	}
	if host == "" {
		return "", false, fmt.Errorf("ldap server %q has no host", server)
	}
	return net.JoinHostPort(host, port), useTLS, nil
}

// HTTPSAMLMetadataFetcher fetches and parses SAML IdP metadata over HTTP.
type HTTPSAMLMetadataFetcher struct{}

// samlEntityDescriptor is the minimal metadata shape needed to confirm the
// document parses and to extract the IdP entityID and SSO endpoints.
type samlEntityDescriptor struct {
	XMLName    xml.Name `xml:"EntityDescriptor"`
	EntityID   string   `xml:"entityID,attr"`
	IDPSSODesc struct {
		SSOServices []struct {
			Location string `xml:"Location,attr"`
		} `xml:"SingleSignOnService"`
	} `xml:"IDPSSODescriptor"`
}

// Fetch implements SAMLMetadataFetcher.
func (HTTPSAMLMetadataFetcher) Fetch(ctx context.Context, metadataURL string) (SAMLMetadata, error) {
	metadataURL = strings.TrimSpace(metadataURL)
	if metadataURL == "" {
		return SAMLMetadata{}, fmt.Errorf("metadata url is empty")
	}
	body, err := httpGet(ctx, metadataURL)
	if err != nil {
		return SAMLMetadata{}, fmt.Errorf("fetch metadata: %w", err)
	}
	return ParseSAMLMetadata(body)
}

// ParseSAMLMetadata decodes an EntityDescriptor document.
func ParseSAMLMetadata(body []byte) (SAMLMetadata, error) {
	var ed samlEntityDescriptor
	if err := xml.Unmarshal(body, &ed); err != nil {
		return SAMLMetadata{}, fmt.Errorf("parse metadata xml: %w", err)
	}
	if strings.TrimSpace(ed.EntityID) == "" {
		return SAMLMetadata{}, fmt.Errorf("metadata missing entityID")
	}
	out := SAMLMetadata{EntityID: ed.EntityID}
	for _, s := range ed.IDPSSODesc.SSOServices {
		if s.Location != "" {
			out.SSOURLs = append(out.SSOURLs, s.Location)
		}
	}
	return out, nil
}

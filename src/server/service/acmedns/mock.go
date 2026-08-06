package acmedns

import (
	"context"
	"sync"
)

// MockProvider is an in-memory DNSChallengeProvider for tests. It records every
// TXT record it is asked to present and clean up and can be configured to fail.
type MockProvider struct {
	ProviderName string
	PresentErr   error
	CleanUpErr   error

	mu        sync.Mutex
	presented map[string]string
	cleaned   []string
}

// NewMockProvider returns a MockProvider with the given reported Name().
func NewMockProvider(name string) *MockProvider {
	return &MockProvider{ProviderName: name, presented: make(map[string]string)}
}

// Present records the TXT record, or returns PresentErr when set.
func (m *MockProvider) Present(_ context.Context, fqdn, value string) error {
	if m.PresentErr != nil {
		return m.PresentErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.presented[fqdn] = value
	return nil
}

// CleanUp records the removed record name, or returns CleanUpErr when set.
func (m *MockProvider) CleanUp(_ context.Context, fqdn, _ string) error {
	if m.CleanUpErr != nil {
		return m.CleanUpErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleaned = append(m.cleaned, fqdn)
	return nil
}

// Name reports the provider name.
func (m *MockProvider) Name() string { return m.ProviderName }

// Presented returns a copy of the currently-published records (name -> value).
func (m *MockProvider) Presented() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]string, len(m.presented))
	for k, v := range m.presented {
		out[k] = v
	}
	return out
}

// CleanedUp returns the record names CleanUp was called for, in order.
func (m *MockProvider) CleanedUp() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.cleaned))
	copy(out, m.cleaned)
	return out
}

// MockIssuer is an in-memory Issuer for testing callers of IssueDNS01. It
// records the domains and provider it was called with and returns a canned
// result or error.
type MockIssuer struct {
	Result *CertResult
	Err    error

	mu           sync.Mutex
	CalledDomain []string
	CalledWith   DNSChallengeProvider
}

// IssueDNS01 records the call and returns the configured Result/Err.
func (m *MockIssuer) IssueDNS01(_ context.Context, provider DNSChallengeProvider, domains ...string) (*CertResult, error) {
	m.mu.Lock()
	m.CalledDomain = append([]string(nil), domains...)
	m.CalledWith = provider
	m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Result, nil
}

// Domains returns a copy of the domains the mock was last called with.
func (m *MockIssuer) Domains() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.CalledDomain...)
}

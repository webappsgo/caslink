package geoip

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/caslink/src/config"
)

// TestNew_DefaultDirUsesDataDir verifies that an empty cfg.Dir falls back to
// {dataDir}/security/geoip and that the directory is created on disk.
func TestNew_DefaultDirUsesDataDir(t *testing.T) {
	dataDir := t.TempDir()
	svc, err := New(config.GeoIPConfig{Enabled: true}, dataDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()

	want := filepath.Join(dataDir, "security", "geoip")
	if svc.Dir() != want {
		t.Errorf("Dir() = %q, want %q", svc.Dir(), want)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Errorf("expected directory %q to exist", want)
	}
}

// TestNew_ExplicitDirWins verifies cfg.Dir overrides the dataDir default.
func TestNew_ExplicitDirWins(t *testing.T) {
	dataDir := t.TempDir()
	explicit := filepath.Join(t.TempDir(), "custom", "geoip")
	svc, err := New(config.GeoIPConfig{Enabled: true, Dir: explicit}, dataDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()

	if svc.Dir() != explicit {
		t.Errorf("Dir() = %q, want %q", svc.Dir(), explicit)
	}
	if _, err := os.Stat(explicit); err != nil {
		t.Errorf("expected explicit dir to be created: %v", err)
	}
}

// TestEnabled covers the nil-safe and enabled/disabled branches — a nil
// *Service must never panic when treated as "not enabled".
func TestEnabled(t *testing.T) {
	var nilSvc *Service
	if nilSvc.Enabled() {
		t.Error("nil Service should report Enabled() == false")
	}

	dataDir := t.TempDir()
	disabled, err := New(config.GeoIPConfig{Enabled: false}, dataDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer disabled.Close()
	if disabled.Enabled() {
		t.Error("Enabled() should be false when cfg.Enabled is false")
	}

	enabled, err := New(config.GeoIPConfig{Enabled: true}, dataDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer enabled.Close()
	if !enabled.Enabled() {
		t.Error("Enabled() should be true when cfg.Enabled is true")
	}
}

// TestSelected verifies the enabled-databases subset matches config exactly,
// including the "nothing selected" and "everything selected" edges.
func TestSelected(t *testing.T) {
	tests := []struct {
		name string
		dbs  config.GeoIPDatabasesConfig
		want []string
	}{
		{"none", config.GeoIPDatabasesConfig{}, nil},
		{"asn only", config.GeoIPDatabasesConfig{ASN: true}, []string{"asn"}},
		{"country only", config.GeoIPDatabasesConfig{Country: true}, []string{"country"}},
		{"city only", config.GeoIPDatabasesConfig{City: true}, []string{"city"}},
		{"all", config.GeoIPDatabasesConfig{ASN: true, Country: true, City: true}, []string{"asn", "country", "city"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{cfg: config.GeoIPConfig{Databases: tt.dbs}}
			got := s.selected()
			if len(got) != len(tt.want) {
				t.Fatalf("selected() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("selected()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}

	// A nil *Service must return a nil slice, never panic.
	var nilSvc *Service
	if got := nilSvc.selected(); got != nil {
		t.Errorf("nil Service.selected() = %v, want nil", got)
	}
}

// TestCountryAllowed_NilAndDisabled covers the fail-open guard clauses: a nil
// Service or a disabled Service must never block traffic.
func TestCountryAllowed_NilAndDisabled(t *testing.T) {
	var nilSvc *Service
	if !nilSvc.CountryAllowed(net.ParseIP("8.8.8.8")) {
		t.Error("nil Service must allow all traffic")
	}

	disabled := &Service{cfg: config.GeoIPConfig{Enabled: false, DenyCountries: []string{"US"}}}
	if !disabled.CountryAllowed(net.ParseIP("8.8.8.8")) {
		t.Error("disabled Service must allow all traffic even with a deny list configured")
	}
}

// TestCountryAllowed_PrivateIPsBypass verifies loopback/private/link-local
// addresses are never country-blocked, per PART 20 ("never block private/
// internal IPs"), even when the address is nil or the deny list would
// otherwise reject everything.
func TestCountryAllowed_PrivateIPsBypass(t *testing.T) {
	s := &Service{cfg: config.GeoIPConfig{Enabled: true, DenyCountries: []string{"US"}}}

	tests := []struct {
		name string
		ip   net.IP
	}{
		{"nil IP", nil},
		{"loopback v4", net.ParseIP("127.0.0.1")},
		{"loopback v6", net.ParseIP("::1")},
		{"private v4 (RFC1918)", net.ParseIP("10.0.0.5")},
		{"private v4 (172.16/12)", net.ParseIP("172.20.1.1")},
		{"private v4 (192.168/16)", net.ParseIP("192.168.1.1")},
		{"link-local", net.ParseIP("169.254.1.1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !s.CountryAllowed(tt.ip) {
				t.Errorf("expected %v to bypass country blocking", tt.ip)
			}
		})
	}
}

// TestCountryAllowed_UnknownCountryAllows verifies that when no country
// database is loaded (LookupCountry returns ""), the request is allowed —
// graceful degradation per spec rather than fail-closed.
func TestCountryAllowed_UnknownCountryAllows(t *testing.T) {
	s := &Service{cfg: config.GeoIPConfig{Enabled: true, DenyCountries: []string{"US"}}}
	// A public, non-private IP with no countryDB loaded.
	if !s.CountryAllowed(net.ParseIP("8.8.8.8")) {
		t.Error("expected unknown country (no DB loaded) to be allowed")
	}
}

// TestLookupCountry_NilSafe verifies LookupCountry never panics on the
// documented nil-guard cases and returns "" when no database is loaded.
func TestLookupCountry_NilSafe(t *testing.T) {
	var nilSvc *Service
	if got := nilSvc.LookupCountry(net.ParseIP("8.8.8.8")); got != "" {
		t.Errorf("nil Service.LookupCountry() = %q, want \"\"", got)
	}

	s := &Service{}
	if got := s.LookupCountry(nil); got != "" {
		t.Errorf("LookupCountry(nil) = %q, want \"\"", got)
	}
	if got := s.LookupCountry(net.ParseIP("8.8.8.8")); got != "" {
		t.Errorf("LookupCountry with no DB loaded = %q, want \"\"", got)
	}
}

// TestLookupCity_NilSafe mirrors TestLookupCountry_NilSafe for LookupCity,
// and checks the country-only fallback path when cityDB is absent but no
// countryDB is loaded either (both nil -> empty result).
func TestLookupCity_NilSafe(t *testing.T) {
	var nilSvc *Service
	if got := nilSvc.LookupCity(net.ParseIP("8.8.8.8")); got != (CityResult{}) {
		t.Errorf("nil Service.LookupCity() = %+v, want zero value", got)
	}

	s := &Service{}
	if got := s.LookupCity(nil); got != (CityResult{}) {
		t.Errorf("LookupCity(nil) = %+v, want zero value", got)
	}
	if got := s.LookupCity(net.ParseIP("8.8.8.8")); got != (CityResult{}) {
		t.Errorf("LookupCity with no DBs loaded = %+v, want zero value", got)
	}
}

// TestClose_NilSafeAndIdempotent verifies Close is safe on a nil Service and
// safe to call twice on the same Service without panicking.
func TestClose_NilSafeAndIdempotent(t *testing.T) {
	var nilSvc *Service
	if err := nilSvc.Close(); err != nil {
		t.Errorf("nil Service.Close() = %v, want nil", err)
	}

	s := &Service{}
	if err := s.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}

// TestUpdate_DisabledIsNoop verifies Update short-circuits (no HTTP calls,
// no error) when the service is disabled.
func TestUpdate_DisabledIsNoop(t *testing.T) {
	s := &Service{cfg: config.GeoIPConfig{Enabled: false}, dir: t.TempDir()}
	if err := s.Update(context.Background()); err != nil {
		t.Errorf("Update() on disabled service = %v, want nil", err)
	}
}

// TestDownloadOne_SuccessWritesAtomically verifies a 200 response is written
// to {dir}/{name}.mmdb with the expected content and 0640 permissions, and
// that no leftover temp file remains.
func TestDownloadOne_SuccessWritesAtomically(t *testing.T) {
	const body = "fake-mmdb-content"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	s := &Service{dir: dir}
	if err := s.downloadOne(context.Background(), "country", srv.URL); err != nil {
		t.Fatalf("downloadOne: %v", err)
	}

	dst := filepath.Join(dir, "country.mmdb")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != body {
		t.Errorf("downloaded content = %q, want %q", got, body)
	}

	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o640 {
		t.Errorf("file mode = %o, want 0640", perm)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "-") && strings.HasPrefix(e.Name(), ".") {
			t.Errorf("leftover temp file not cleaned up: %s", e.Name())
		}
	}
}

// TestDownloadOne_NonOKStatusErrors verifies a non-200 response is treated
// as an error and no file is written.
func TestDownloadOne_NonOKStatusErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	s := &Service{dir: dir}
	err := s.downloadOne(context.Background(), "country", srv.URL)
	if err == nil {
		t.Fatal("expected error for HTTP 404 response, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "country.mmdb")); statErr == nil {
		t.Error("expected no file to be written on non-200 response")
	}
}

// TestDownloadOne_ContextTimeout verifies a slow server past the deadline
// surfaces as an error rather than hanging or silently succeeding.
func TestDownloadOne_ContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("too-slow"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	s := &Service{dir: dir}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if err := s.downloadOne(ctx, "country", srv.URL); err == nil {
		t.Fatal("expected error when context deadline is exceeded, got nil")
	}
}

// TestDownloadOne_InvalidURLErrors verifies a malformed URL fails at request
// construction rather than panicking.
func TestDownloadOne_InvalidURLErrors(t *testing.T) {
	s := &Service{dir: t.TempDir()}
	err := s.downloadOne(context.Background(), "country", "://not-a-valid-url")
	if err == nil {
		t.Fatal("expected error for malformed URL, got nil")
	}
}

// TestLastUpdate_NoFilesReturnsZero verifies the graceful "never updated"
// case returns the zero time rather than an error.
func TestLastUpdate_NoFilesReturnsZero(t *testing.T) {
	s := &Service{dir: t.TempDir()}
	if got := s.LastUpdate(); !got.IsZero() {
		t.Errorf("LastUpdate() = %v, want zero time", got)
	}

	var nilSvc *Service
	if got := nilSvc.LastUpdate(); !got.IsZero() {
		t.Errorf("nil Service.LastUpdate() = %v, want zero time", got)
	}
}

// TestLastUpdate_ReturnsNewestModTime verifies LastUpdate picks the most
// recently modified of the three database files, not just the first found.
func TestLastUpdate_ReturnsNewestModTime(t *testing.T) {
	dir := t.TempDir()
	s := &Service{dir: dir}

	older := time.Now().Add(-time.Hour)
	newer := time.Now()

	countryPath := filepath.Join(dir, "country.mmdb")
	cityPath := filepath.Join(dir, "city.mmdb")
	if err := os.WriteFile(countryPath, []byte("x"), 0o640); err != nil {
		t.Fatalf("write country.mmdb: %v", err)
	}
	if err := os.WriteFile(cityPath, []byte("x"), 0o640); err != nil {
		t.Fatalf("write city.mmdb: %v", err)
	}
	if err := os.Chtimes(countryPath, older, older); err != nil {
		t.Fatalf("chtimes country: %v", err)
	}
	if err := os.Chtimes(cityPath, newer, newer); err != nil {
		t.Fatalf("chtimes city: %v", err)
	}

	got := s.LastUpdate()
	// Allow a small tolerance since filesystem mtime resolution varies.
	if got.Before(newer.Add(-time.Second)) {
		t.Errorf("LastUpdate() = %v, want approximately %v (newest file)", got, newer)
	}
}

// TestOpenReader_MissingFileReturnsNilNil verifies the "best effort" contract
// documented on openReader: an absent file is not an error.
func TestOpenReader_MissingFileReturnsNilNil(t *testing.T) {
	dir := t.TempDir()
	r, err := openReader(filepath.Join(dir, "does-not-exist.mmdb"))
	if err != nil {
		t.Errorf("openReader on missing file: err = %v, want nil", err)
	}
	if r != nil {
		t.Errorf("openReader on missing file: reader = %v, want nil", r)
	}
}

// TestOpenReader_InvalidFileErrors verifies a present-but-corrupt file
// surfaces an error rather than a nil reader (which would be silently
// mistaken for "absent").
func TestOpenReader_InvalidFileErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.mmdb")
	if err := os.WriteFile(path, []byte("not a real mmdb file"), 0o640); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	r, err := openReader(path)
	if err == nil {
		t.Error("expected error opening a corrupt MMDB file, got nil")
	}
	if r != nil {
		t.Error("expected nil reader on error")
	}
}

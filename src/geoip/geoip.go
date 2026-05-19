// Package geoip downloads and serves the ip-location-db MMDB databases used
// by caslink for country/city/ASN/WHOIS lookups (AI.md PART 20).
//
// Databases are NEVER embedded — they are downloaded on first run and refreshed
// weekly by the scheduler. The directory layout under {data_dir}/security/geoip
// is:
//
//	asn.mmdb
//	country.mmdb
//	city.mmdb
//	whois.mmdb
//
// Each file is downloaded atomically (.tmp → rename) so a half-written database
// can never be read by the lookup path.
//
// Lookups themselves require a MaxMind-format MMDB reader. To preserve the
// CGO_ENABLED=0 + zero-extra-dependency contract, this package ships the
// download + dispatch surface. When the project adds a pure-Go MMDB reader
// (planned: github.com/oschwald/maxminddb-golang), wire it into Lookup —
// the calling code in click analytics and country blocking already calls
// the methods on this Service.
package geoip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/oschwald/maxminddb-golang"
)

// downloadTimeout caps each individual database download.
const downloadTimeout = 5 * time.Minute

// Sources maps database name → CDN URL (sapics/ip-location-db).
var sources = map[string]string{
	"asn":     "https://cdn.jsdelivr.net/npm/@ip-location-db/asn-mmdb/asn.mmdb",
	"country": "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country-mmdb/geo-whois-asn-country.mmdb",
	"city":    "https://cdn.jsdelivr.net/npm/@ip-location-db/dbip-city-mmdb/dbip-city-ipv4.mmdb",
	"whois":   "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country-mmdb/geo-whois-asn-country.mmdb",
}

// Service holds the configured GeoIP directory and selection. Lookups are
// safe for concurrent use.
type Service struct {
	dir string
	cfg config.GeoIPConfig

	mu        sync.RWMutex
	countryDB *maxminddb.Reader
	cityDB    *maxminddb.Reader
	asnDB     *maxminddb.Reader
}

// CityResult holds the subset of fields the application records for a click.
type CityResult struct {
	CountryCode string
	City        string
}

// openReader opens an MMDB file if it exists. Returns (nil, nil) when the file
// is absent so callers can treat geo lookup as best-effort.
func openReader(path string) (*maxminddb.Reader, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	r, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// loadReaders opens (or re-opens) the MMDB readers for country/city/ASN.
// Safe to call repeatedly; closes any previously held readers first.
func (s *Service) loadReaders() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range []**maxminddb.Reader{&s.countryDB, &s.cityDB, &s.asnDB} {
		if *r != nil {
			_ = (*r).Close()
			*r = nil
		}
	}
	if rdr, err := openReader(filepath.Join(s.dir, "country.mmdb")); err == nil {
		s.countryDB = rdr
	} else {
		log.Printf("[geoip] open country.mmdb: %v", err)
	}
	if rdr, err := openReader(filepath.Join(s.dir, "city.mmdb")); err == nil {
		s.cityDB = rdr
	} else {
		log.Printf("[geoip] open city.mmdb: %v", err)
	}
	if rdr, err := openReader(filepath.Join(s.dir, "asn.mmdb")); err == nil {
		s.asnDB = rdr
	} else {
		log.Printf("[geoip] open asn.mmdb: %v", err)
	}
}

// Close releases all open MMDB readers.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range []**maxminddb.Reader{&s.countryDB, &s.cityDB, &s.asnDB} {
		if *r != nil {
			_ = (*r).Close()
			*r = nil
		}
	}
	return nil
}

// New returns a Service backed by cfg. If cfg.Dir is empty, dataDir is used
// as the base and the directory is created (mode 0o750) if missing.
func New(cfg config.GeoIPConfig, dataDir string) (*Service, error) {
	dir := cfg.Dir
	if dir == "" {
		dir = filepath.Join(dataDir, "security", "geoip")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("geoip: create dir %q: %w", dir, err)
	}
	s := &Service{dir: dir, cfg: cfg}
	// Best-effort: if any databases already exist on disk, open them now.
	s.loadReaders()
	return s, nil
}

// Dir returns the resolved database directory.
func (s *Service) Dir() string { return s.dir }

// Enabled reports whether the service is configured to run.
func (s *Service) Enabled() bool { return s != nil && s.cfg.Enabled }

// selected returns the subset of database names enabled in config.
func (s *Service) selected() []string {
	if s == nil {
		return nil
	}
	var out []string
	if s.cfg.Databases.ASN {
		out = append(out, "asn")
	}
	if s.cfg.Databases.Country {
		out = append(out, "country")
	}
	if s.cfg.Databases.City {
		out = append(out, "city")
	}
	if s.cfg.Databases.WHOIS {
		out = append(out, "whois")
	}
	return out
}

// Update downloads (or refreshes) every enabled database. Errors per database
// are logged but do not abort the run; a single CDN hiccup must not prevent
// the other three databases from refreshing.
func (s *Service) Update(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	var lastErr error
	for _, name := range s.selected() {
		url, ok := sources[name]
		if !ok {
			continue
		}
		if err := s.downloadOne(ctx, name, url); err != nil {
			log.Printf("[geoip] update %s: %v", name, err)
			lastErr = err
		} else {
			log.Printf("[geoip] update %s: OK", name)
		}
	}
	// Re-open MMDB readers so subsequent lookups use the freshly downloaded files.
	s.loadReaders()
	return lastErr
}

// downloadOne fetches url and writes it atomically to {dir}/{name}.mmdb.
func (s *Service) downloadOne(ctx context.Context, name, url string) error {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch: HTTP %d", resp.StatusCode)
	}

	dst := filepath.Join(s.dir, name+".mmdb")
	tmp, err := os.CreateTemp(s.dir, "."+name+"-*.mmdb")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	// Cap copy at 200 MB so a malicious CDN response cannot exhaust disk.
	const maxBytes = 200 * 1024 * 1024
	if _, err := io.Copy(tmp, io.LimitReader(resp.Body, maxBytes)); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("temp close: %w", err)
	}
	if err := os.Chmod(tmpName, 0o640); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// LastUpdate returns the modification time of the most recently refreshed
// database, or zero time if no database has been downloaded yet.
func (s *Service) LastUpdate() time.Time {
	if s == nil {
		return time.Time{}
	}
	var newest time.Time
	for _, name := range []string{"asn", "country", "city", "whois"} {
		fi, err := os.Stat(filepath.Join(s.dir, name+".mmdb"))
		if err != nil {
			continue
		}
		if fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
	}
	return newest
}

// CountryAllowed evaluates the deny/allow lists from PART 20. Returns true
// when no country information is available (graceful degradation per spec —
// "if country.mmdb missing, country blocking skipped with a warning").
func (s *Service) CountryAllowed(ip net.IP) bool {
	if s == nil || !s.cfg.Enabled {
		return true
	}
	// Private/internal IPs are never country-blocked (PART 20).
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	code := s.LookupCountry(ip)
	if code == "" {
		return true // unknown → allow (warn elsewhere)
	}
	code = strings.ToUpper(code)
	// Allowlist mode wins when both set.
	if len(s.cfg.AllowCountries) > 0 {
		for _, c := range s.cfg.AllowCountries {
			if strings.EqualFold(c, code) {
				return true
			}
		}
		return false
	}
	for _, c := range s.cfg.DenyCountries {
		if strings.EqualFold(c, code) {
			return false
		}
	}
	return true
}

// LookupCountry returns the ISO 3166-1 alpha-2 country code for ip, or "" when
// the database is absent or the IP is not present. Uses the
// github.com/oschwald/maxminddb-golang pure-Go reader (CGO_ENABLED=0 safe).
func (s *Service) LookupCountry(ip net.IP) string {
	if s == nil || ip == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.countryDB == nil {
		return ""
	}
	var rec struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	if err := s.countryDB.Lookup(ip, &rec); err != nil {
		return ""
	}
	return strings.ToUpper(rec.Country.ISOCode)
}

// LookupCity returns the country code and city name for ip. Both fields may be
// empty when the database is absent, the IP is not present, or the record
// lacks the field.
func (s *Service) LookupCity(ip net.IP) CityResult {
	if s == nil || ip == nil {
		return CityResult{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cityDB == nil {
		// Fall back to country-only DB so callers still get a country code.
		if s.countryDB == nil {
			return CityResult{}
		}
		return CityResult{CountryCode: s.LookupCountryLocked(ip)}
	}
	var rec struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
	}
	if err := s.cityDB.Lookup(ip, &rec); err != nil {
		return CityResult{}
	}
	city := rec.City.Names["en"]
	return CityResult{CountryCode: strings.ToUpper(rec.Country.ISOCode), City: city}
}

// LookupCountryLocked is LookupCountry without taking the lock — callers must
// already hold s.mu (read or write).
func (s *Service) LookupCountryLocked(ip net.IP) string {
	if s == nil || ip == nil || s.countryDB == nil {
		return ""
	}
	var rec struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	if err := s.countryDB.Lookup(ip, &rec); err != nil {
		return ""
	}
	return strings.ToUpper(rec.Country.ISOCode)
}

// ErrDatabaseMissing is returned when a required MMDB file is absent.
var ErrDatabaseMissing = errors.New("geoip: database missing")

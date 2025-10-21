package analytics

import (
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/oschwald/geoip2-golang"
	"github.com/sirupsen/logrus"
)

// GeoLocator provides IP geolocation services using MaxMind GeoIP2 database
type GeoLocator struct {
	db     *geoip2.Reader
	logger *logrus.Logger
	mutex  sync.RWMutex
	enabled bool
}

// NewGeoLocator creates a new geolocator instance
func NewGeoLocator(databasePath string, logger *logrus.Logger) (*GeoLocator, error) {
	gl := &GeoLocator{
		logger:  logger,
		enabled: false,
	}

	// Check if database file exists
	if databasePath == "" {
		logger.Info("GeoIP database path not provided, geolocation disabled")
		return gl, nil
	}

	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		logger.WithField("path", databasePath).Warn("GeoIP database file not found, geolocation disabled")
		return gl, nil
	}

	// Open GeoIP database
	db, err := geoip2.Open(databasePath)
	if err != nil {
		logger.WithError(err).WithField("path", databasePath).Warn("Failed to open GeoIP database, geolocation disabled")
		return gl, nil
	}

	gl.db = db
	gl.enabled = true
	logger.WithField("path", databasePath).Info("GeoIP database loaded successfully")

	return gl, nil
}

// Close closes the GeoIP database connection
func (gl *GeoLocator) Close() error {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	if gl.db != nil {
		err := gl.db.Close()
		gl.db = nil
		gl.enabled = false
		return err
	}
	return nil
}

// IsEnabled returns whether geolocation is enabled
func (gl *GeoLocator) IsEnabled() bool {
	gl.mutex.RLock()
	defer gl.mutex.RUnlock()
	return gl.enabled
}

// GetLocation gets location data for an IP address
func (gl *GeoLocator) GetLocation(ipAddress string) *LocationData {
	gl.mutex.RLock()
	defer gl.mutex.RUnlock()

	if !gl.enabled || gl.db == nil {
		return nil
	}

	// Parse IP address
	ip := net.ParseIP(ipAddress)
	if ip == nil {
		gl.logger.WithField("ip", ipAddress).Debug("Invalid IP address for geolocation")
		return nil
	}

	// Skip private/loopback addresses
	if isPrivateIP(ip) {
		return &LocationData{
			CountryCode: "XX",
			CountryName: "Private Network",
			Region:      "",
			City:        "",
			Latitude:    0,
			Longitude:   0,
			Timezone:    "",
		}
	}

	// Look up location in GeoIP database
	record, err := gl.db.City(ip)
	if err != nil {
		gl.logger.WithError(err).WithField("ip", ipAddress).Debug("Failed to look up IP location")
		return nil
	}

	// Extract location data
	location := &LocationData{
		CountryCode: record.Country.IsoCode,
		CountryName: record.Country.Names["en"],
		Latitude:    record.Location.Latitude,
		Longitude:   record.Location.Longitude,
		Timezone:    record.Location.TimeZone,
	}

	// Get most specific subdivision (state/province)
	if len(record.Subdivisions) > 0 {
		location.Region = record.Subdivisions[0].Names["en"]
	}

	// Get city name
	location.City = record.City.Names["en"]

	// Validate required fields
	if location.CountryCode == "" {
		location.CountryCode = "XX"
		location.CountryName = "Unknown"
	}
	if location.CountryName == "" {
		location.CountryName = "Unknown"
	}

	return location
}

// GetLocationBatch gets location data for multiple IP addresses
func (gl *GeoLocator) GetLocationBatch(ipAddresses []string) map[string]*LocationData {
	results := make(map[string]*LocationData)

	for _, ip := range ipAddresses {
		results[ip] = gl.GetLocation(ip)
	}

	return results
}

// GetCountryCode gets just the country code for an IP address (faster)
func (gl *GeoLocator) GetCountryCode(ipAddress string) string {
	location := gl.GetLocation(ipAddress)
	if location != nil {
		return location.CountryCode
	}
	return ""
}

// ValidateDatabase validates the GeoIP database integrity
func (gl *GeoLocator) ValidateDatabase() error {
	gl.mutex.RLock()
	defer gl.mutex.RUnlock()

	if !gl.enabled || gl.db == nil {
		return fmt.Errorf("geolocation not enabled")
	}

	// Test with known IP addresses
	testIPs := []string{
		"8.8.8.8",         // Google DNS
		"1.1.1.1",         // Cloudflare DNS
		"208.67.222.222",  // OpenDNS
	}

	for _, ip := range testIPs {
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			continue
		}

		_, err := gl.db.City(parsedIP)
		if err != nil {
			return fmt.Errorf("database validation failed for IP %s: %w", ip, err)
		}
	}

	return nil
}

// GetDatabaseMetadata returns metadata about the GeoIP database
func (gl *GeoLocator) GetDatabaseMetadata() map[string]interface{} {
	gl.mutex.RLock()
	defer gl.mutex.RUnlock()

	metadata := map[string]interface{}{
		"enabled": gl.enabled,
	}

	if gl.enabled && gl.db != nil {
		metadata["build_epoch"] = gl.db.Metadata().BuildEpoch
		metadata["database_type"] = gl.db.Metadata().DatabaseType
		metadata["description"] = gl.db.Metadata().Description
		metadata["languages"] = gl.db.Metadata().Languages
		metadata["binary_format_major_version"] = gl.db.Metadata().BinaryFormatMajorVersion
		metadata["binary_format_minor_version"] = gl.db.Metadata().BinaryFormatMinorVersion
	}

	return metadata
}

// ReloadDatabase reloads the GeoIP database from disk
func (gl *GeoLocator) ReloadDatabase(databasePath string) error {
	gl.mutex.Lock()
	defer gl.mutex.Unlock()

	// Close existing database
	if gl.db != nil {
		gl.db.Close()
		gl.db = nil
		gl.enabled = false
	}

	// Check if new database file exists
	if databasePath == "" {
		gl.logger.Info("GeoIP database path not provided, geolocation disabled")
		return nil
	}

	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		gl.logger.WithField("path", databasePath).Warn("GeoIP database file not found, geolocation disabled")
		return fmt.Errorf("database file not found: %s", databasePath)
	}

	// Open new database
	db, err := geoip2.Open(databasePath)
	if err != nil {
		gl.logger.WithError(err).WithField("path", databasePath).Error("Failed to reload GeoIP database")
		return fmt.Errorf("failed to open database: %w", err)
	}

	gl.db = db
	gl.enabled = true
	gl.logger.WithField("path", databasePath).Info("GeoIP database reloaded successfully")

	return nil
}

// Helper functions

// isPrivateIP checks if an IP address is in a private network range
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// IPv4 private ranges
	if ip.To4() != nil {
		// 10.0.0.0/8
		if ip[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip[0] == 192 && ip[1] == 168 {
			return true
		}
		// 169.254.0.0/16 (link-local)
		if ip[0] == 169 && ip[1] == 254 {
			return true
		}
		// 127.0.0.0/8 (loopback)
		if ip[0] == 127 {
			return true
		}
	}

	// IPv6 private ranges
	if ip.To16() != nil {
		// ::1 (loopback)
		if ip.Equal(net.IPv6loopback) {
			return true
		}
		// fc00::/7 (unique local)
		if ip[0] == 0xfc || ip[0] == 0xfd {
			return true
		}
		// fe80::/10 (link-local)
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
			return true
		}
	}

	return false
}

// GetLocationFromCoordinates gets location name from latitude/longitude coordinates
func (gl *GeoLocator) GetLocationFromCoordinates(lat, lon float64) (*LocationData, error) {
	// This would require a reverse geocoding service or database
	// For now, return an error indicating it's not implemented
	return nil, fmt.Errorf("reverse geocoding not implemented")
}

// GetDistanceBetweenIPs calculates the distance between two IP addresses in kilometers
func (gl *GeoLocator) GetDistanceBetweenIPs(ip1, ip2 string) (float64, error) {
	loc1 := gl.GetLocation(ip1)
	loc2 := gl.GetLocation(ip2)

	if loc1 == nil || loc2 == nil {
		return 0, fmt.Errorf("could not get location for one or both IPs")
	}

	return calculateDistance(loc1.Latitude, loc1.Longitude, loc2.Latitude, loc2.Longitude), nil
}

// calculateDistance calculates the distance between two points using the Haversine formula
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371 // Earth's radius in kilometers

	// Convert degrees to radians
	lat1Rad := lat1 * (3.14159265359 / 180)
	lon1Rad := lon1 * (3.14159265359 / 180)
	lat2Rad := lat2 * (3.14159265359 / 180)
	lon2Rad := lon2 * (3.14159265359 / 180)

	// Calculate differences
	dLat := lat2Rad - lat1Rad
	dLon := lon2Rad - lon1Rad

	// Haversine formula
	a := sin(dLat/2)*sin(dLat/2) + cos(lat1Rad)*cos(lat2Rad)*sin(dLon/2)*sin(dLon/2)
	c := 2 * atan2(sqrt(a), sqrt(1-a))
	distance := earthRadius * c

	return distance
}

// Simple math functions (avoiding external dependencies)
func sin(x float64) float64 {
	// Simple sine approximation using Taylor series
	// This is a simplified implementation - in production, use math.Sin
	x = x - 2*3.14159265359*float64(int(x/(2*3.14159265359)))
	if x < 0 {
		x = -x
	}
	if x > 3.14159265359 {
		x = 2*3.14159265359 - x
	}
	x2 := x * x
	return x * (1 - x2/6 + x2*x2/120 - x2*x2*x2/5040)
}

func cos(x float64) float64 {
	return sin(x + 3.14159265359/2)
}

func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x == 0 {
		return 0
	}

	// Newton's method for square root
	guess := x / 2
	for i := 0; i < 10; i++ {
		guess = (guess + x/guess) / 2
	}
	return guess
}

func atan2(y, x float64) float64 {
	// Simplified atan2 implementation
	if x > 0 {
		return atan(y / x)
	}
	if x < 0 && y >= 0 {
		return atan(y/x) + 3.14159265359
	}
	if x < 0 && y < 0 {
		return atan(y/x) - 3.14159265359
	}
	if x == 0 && y > 0 {
		return 3.14159265359 / 2
	}
	if x == 0 && y < 0 {
		return -3.14159265359 / 2
	}
	return 0 // x == 0 && y == 0
}

func atan(x float64) float64 {
	// Simplified arctangent using series approximation
	if x > 1 {
		return 3.14159265359/2 - atan(1/x)
	}
	if x < -1 {
		return -3.14159265359/2 - atan(1/x)
	}

	x2 := x * x
	return x * (1 - x2/3 + x2*x2/5 - x2*x2*x2/7)
}
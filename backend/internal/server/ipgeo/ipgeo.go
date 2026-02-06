// Package ipgeo provides IP-to-country geolocation using MaxMind MMDB files.

package ipgeo

import (
	"net/netip"

	"github.com/oschwald/maxminddb-golang/v2"
)

// Checker resolves IP addresses to ISO 3166-1 alpha-2 country codes.
type Checker struct {
	reader *maxminddb.Reader
}

// Open opens an MMDB file for country lookups.
func Open(dbPath string) (*Checker, error) {
	r, err := maxminddb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return &Checker{reader: r}, nil
}

// Close releases the MMDB reader resources.
func (c *Checker) Close() error {
	return c.reader.Close()
}

// countryRecord is the minimal struct for MMDB country lookups.
type countryRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

// tailscalePrefix is the Tailscale CGNAT range 100.64.0.0/10.
var tailscalePrefix = netip.MustParsePrefix("100.64.0.0/10")

// CountryCode returns the ISO 3166-1 alpha-2 country code for the given IP string.
// Returns "local" for loopback, private, and unspecified IPs.
// Returns "tailscale" for Tailscale CGNAT IPs (100.64.0.0/10).
// Returns "" on parse or lookup error.
func (c *Checker) CountryCode(ipStr string) string {
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return ""
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() || addr.IsLinkLocalUnicast() {
		return "local"
	}
	if tailscalePrefix.Contains(addr) {
		return "tailscale"
	}
	var rec countryRecord
	if err := c.reader.Lookup(addr).Decode(&rec); err != nil {
		return ""
	}
	return rec.Country.ISOCode
}

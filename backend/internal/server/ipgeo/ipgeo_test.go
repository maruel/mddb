package ipgeo

import (
	"net/netip"
	"testing"
)

func TestTailscalePrefix(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", true},
		{"100.127.255.254", true},
		{"100.63.255.255", false},
		{"100.128.0.0", false},
	}
	for _, tt := range tests {
		addr := netip.MustParseAddr(tt.ip)
		if got := tailscalePrefix.Contains(addr); got != tt.want {
			t.Errorf("tailscalePrefix.Contains(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestCountryCodeLocal(t *testing.T) {
	// We test the classification logic without an MMDB file by constructing a
	// minimal Checker. Since local IPs never reach the MMDB reader, a nil
	// reader is fine.
	c := &Checker{}
	tests := []struct {
		ip   string
		want string
	}{
		// Loopback
		{"127.0.0.1", "local"},
		{"::1", "local"},
		// Private
		{"10.0.0.1", "local"},
		{"192.168.1.1", "local"},
		{"172.16.0.1", "local"},
		// Unspecified
		{"0.0.0.0", "local"},
		{"::", "local"},
		// Link-local
		{"169.254.1.1", "local"},
		{"fe80::1", "local"},
		// Tailscale
		{"100.64.0.1", "tailscale"},
		{"100.100.100.100", "tailscale"},
		// Invalid
		{"not-an-ip", ""},
	}
	for _, tt := range tests {
		got := c.CountryCode(tt.ip)
		if got != tt.want {
			t.Errorf("CountryCode(%q) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

// Tests for request context utilities.

package reqctx

import (
	"net/http"
	"net/netip"
	"testing"
)

func TestResolveClientIP(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128"), netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name       string
		headers    http.Header
		remoteAddr string
		proxies    []netip.Prefix
		want       string
	}{
		{
			name:       "direct peer",
			remoteAddr: "192.168.1.1:12345",
			proxies:    proxies,
			want:       "192.168.1.1",
		},
		{
			name:       "IPv6 direct peer",
			remoteAddr: "[2001:db8::2]:8080",
			proxies:    proxies,
			want:       "2001:db8::2",
		},
		{
			name:       "peer without port",
			remoteAddr: "192.168.1.1",
			proxies:    proxies,
			want:       "192.168.1.1",
		},
		{
			name:       "unparsable peer",
			remoteAddr: "pipe",
			proxies:    proxies,
			want:       "",
		},
		{
			name:       "untrusted peer cannot set X-Forwarded-For",
			headers:    http.Header{"X-Forwarded-For": {"203.0.113.9"}},
			remoteAddr: "192.168.1.1:12345",
			proxies:    proxies,
			want:       "192.168.1.1",
		},
		{
			name:       "untrusted peer cannot set X-Real-IP",
			headers:    http.Header{"X-Real-Ip": {"203.0.113.9"}},
			remoteAddr: "192.168.1.1:12345",
			proxies:    proxies,
			want:       "192.168.1.1",
		},
		{
			name:       "no trusted proxies ignores headers",
			headers:    http.Header{"X-Forwarded-For": {"203.0.113.9"}},
			remoteAddr: "127.0.0.1:8080",
			want:       "127.0.0.1",
		},
		{
			name:       "trusted peer X-Forwarded-For",
			headers:    http.Header{"X-Forwarded-For": {"203.0.113.195"}},
			remoteAddr: "127.0.0.1:8080",
			proxies:    proxies,
			want:       "203.0.113.195",
		},
		{
			name:       "trusted peer IPv6 X-Forwarded-For",
			headers:    http.Header{"X-Forwarded-For": {"2001:db8::1"}},
			remoteAddr: "[::1]:8080",
			proxies:    proxies,
			want:       "2001:db8::1",
		},
		{
			name:       "IPv4-mapped trusted peer",
			headers:    http.Header{"X-Forwarded-For": {"203.0.113.195"}},
			remoteAddr: "[::ffff:127.0.0.1]:8080",
			proxies:    proxies,
			want:       "203.0.113.195",
		},
		{
			name:       "rightmost untrusted address wins over a spoofed leftmost",
			headers:    http.Header{"X-Forwarded-For": {"198.51.100.7, 203.0.113.195, 10.0.0.2"}},
			remoteAddr: "127.0.0.1:8080",
			proxies:    proxies,
			want:       "203.0.113.195",
		},
		{
			name:       "X-Forwarded-For across several header lines",
			headers:    http.Header{"X-Forwarded-For": {"198.51.100.7", "203.0.113.195, 10.0.0.2"}},
			remoteAddr: "127.0.0.1:8080",
			proxies:    proxies,
			want:       "203.0.113.195",
		},
		{
			name:       "malformed X-Forwarded-For uses the direct peer",
			headers:    http.Header{"X-Forwarded-For": {"203.0.113.195, garbage"}},
			remoteAddr: "127.0.0.1:8080",
			proxies:    proxies,
			want:       "127.0.0.1",
		},
		{
			name:       "all-trusted chain uses the direct peer",
			headers:    http.Header{"X-Forwarded-For": {"10.0.0.2, 10.0.0.3"}},
			remoteAddr: "127.0.0.1:8080",
			proxies:    proxies,
			want:       "127.0.0.1",
		},
		{
			name:       "trusted peer X-Real-IP",
			headers:    http.Header{"X-Real-Ip": {"203.0.113.195"}},
			remoteAddr: "127.0.0.1:8080",
			proxies:    proxies,
			want:       "203.0.113.195",
		},
		{
			name:       "X-Forwarded-For takes precedence over X-Real-IP",
			headers:    http.Header{"X-Forwarded-For": {"203.0.113.195"}, "X-Real-Ip": {"10.9.9.9"}},
			remoteAddr: "127.0.0.1:8080",
			proxies:    proxies,
			want:       "203.0.113.195",
		},
		{
			name:       "repeated X-Real-IP uses the direct peer",
			headers:    http.Header{"X-Real-Ip": {"203.0.113.195", "203.0.113.196"}},
			remoteAddr: "127.0.0.1:8080",
			proxies:    proxies,
			want:       "127.0.0.1",
		},
		{
			name:       "malformed X-Real-IP uses the direct peer",
			headers:    http.Header{"X-Real-Ip": {"garbage"}},
			remoteAddr: "127.0.0.1:8080",
			proxies:    proxies,
			want:       "127.0.0.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.RemoteAddr = tt.remoteAddr
			req.Header = tt.headers
			if got := ResolveClientIP(req, tt.proxies); got != tt.want {
				t.Errorf("ResolveClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

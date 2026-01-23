package reqctx

import (
	"net/http"
	"testing"
)

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		want       string
	}{
		{
			name:       "X-Forwarded-For single IP",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.195"},
			remoteAddr: "127.0.0.1:8080",
			want:       "203.0.113.195",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.195, 70.41.3.18, 150.172.238.178"},
			remoteAddr: "127.0.0.1:8080",
			want:       "203.0.113.195",
		},
		{
			name:       "X-Forwarded-For with spaces",
			headers:    map[string]string{"X-Forwarded-For": "  203.0.113.195  "},
			remoteAddr: "127.0.0.1:8080",
			want:       "203.0.113.195",
		},
		{
			name:       "X-Real-IP",
			headers:    map[string]string{"X-Real-IP": "203.0.113.195"},
			remoteAddr: "127.0.0.1:8080",
			want:       "203.0.113.195",
		},
		{
			name:       "X-Forwarded-For takes precedence over X-Real-IP",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.195", "X-Real-IP": "10.0.0.1"},
			remoteAddr: "127.0.0.1:8080",
			want:       "203.0.113.195",
		},
		{
			name:       "RemoteAddr with port",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.1:12345",
			want:       "192.168.1.1",
		},
		{
			name:       "RemoteAddr without port",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.1",
			want:       "192.168.1.1",
		},
		{
			name:       "IPv6 RemoteAddr with port",
			headers:    map[string]string{},
			remoteAddr: "[::1]:8080",
			want:       "::1",
		},
		{
			name:       "IPv6 X-Forwarded-For",
			headers:    map[string]string{"X-Forwarded-For": "2001:db8::1"},
			remoteAddr: "127.0.0.1:8080",
			want:       "2001:db8::1",
		},
		{
			name:       "Empty headers fallback to RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "10.0.0.50:9999",
			want:       "10.0.0.50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/", http.NoBody)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := GetClientIP(req)
			if got != tt.want {
				t.Errorf("GetClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

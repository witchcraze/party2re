package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xri        string
		remoteAddr string
		expectedIP string
	}{
		{
			name:       "X-Forwarded-For multiple IPs returns first trimmed IP",
			xff:        " 203.0.113.195 , 70.41.3.18, 150.172.238.178",
			xri:        "198.51.100.1",
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "203.0.113.195",
		},
		{
			name:       "X-Forwarded-For single IP returns IP",
			xff:        "198.51.100.1",
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "198.51.100.1",
		},
		{
			name:       "X-Forwarded-For empty first element falls back to X-Real-IP",
			xff:        " , 70.41.3.18",
			xri:        " 192.0.2.1 ",
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "192.0.2.1",
		},
		{
			name:       "X-Forwarded-For empty falls back to X-Real-IP",
			xff:        "",
			xri:        " 192.0.2.1 ",
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "192.0.2.1",
		},
		{
			name:       "X-Real-IP whitespace falls back to RemoteAddr host",
			xff:        "",
			xri:        "   ",
			remoteAddr: "192.168.1.100:8080",
			expectedIP: "192.168.1.100",
		},
		{
			name:       "No headers returns host from host:port RemoteAddr",
			xff:        "",
			xri:        "",
			remoteAddr: "10.0.0.1:9999",
			expectedIP: "10.0.0.1",
		},
		{
			name:       "SplitHostPort error returns RemoteAddr directly",
			xff:        "",
			xri:        "",
			remoteAddr: "invalid-host-port-string",
			expectedIP: "invalid-host-port-string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.xri != "" {
				req.Header.Set("X-Real-IP", tc.xri)
			}
			req.RemoteAddr = tc.remoteAddr

			got := extractClientIP(req)
			if got != tc.expectedIP {
				t.Fatalf("expected client IP %q, got %q", tc.expectedIP, got)
			}
		})
	}
}

package http

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestExtractClientIP_DirectConnection_SpoofingBlocked(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xri        string
		remoteAddr string
		trusted    []netip.Prefix
		expectedIP string
	}{
		{
			name:       "Default config (no trusted proxies): XFF ignored",
			xff:        "203.0.113.195",
			remoteAddr: "192.168.1.100:12345",
			expectedIP: "192.168.1.100",
		},
		{
			name:       "Default config (no trusted proxies): X-Real-IP ignored",
			xri:        "203.0.113.195",
			remoteAddr: "192.168.1.100:12345",
			expectedIP: "192.168.1.100",
		},
		{
			name:       "Default config (no trusted proxies): both XFF and X-Real-IP ignored",
			xff:        "203.0.113.195, 70.41.3.18",
			xri:        "198.51.100.1",
			remoteAddr: "192.168.1.100:12345",
			expectedIP: "192.168.1.100",
		},
		{
			name:       "Configured trusted proxy exists, but peer is untrusted: headers ignored",
			xff:        "1.1.1.1",
			xri:        "2.2.2.2",
			remoteAddr: "192.168.1.100:12345",
			trusted:    []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
			expectedIP: "192.168.1.100",
		},
		{
			name:       "No headers returns host from host:port RemoteAddr",
			remoteAddr: "10.0.0.1:9999",
			expectedIP: "10.0.0.1",
		},
		{
			name:       "SplitHostPort error returns RemoteAddr directly",
			remoteAddr: "invalid-host-port-string",
			expectedIP: "invalid-host-port-string",
		},
		{
			name:       "IPv6 without brackets or port returns host",
			remoteAddr: "::1",
			expectedIP: "::1",
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

			got := ExtractClientIP(req, tc.trusted...)
			if got != tc.expectedIP {
				t.Fatalf("expected client IP %q, got %q", tc.expectedIP, got)
			}
		})
	}
}

func TestExtractClientIP_TrustedProxy(t *testing.T) {
	loopbackPrefix := netip.MustParsePrefix("127.0.0.1/32")
	privatePrefix := netip.MustParsePrefix("10.0.0.0/8")
	ipv6LoopbackPrefix := netip.MustParsePrefix("::1/128")

	tests := []struct {
		name       string
		xff        string
		xri        string
		remoteAddr string
		trusted    []netip.Prefix
		expectedIP string
	}{
		{
			name:       "Single XFF IP through trusted loopback proxy",
			xff:        "198.51.100.1",
			remoteAddr: "127.0.0.1:12345",
			trusted:    []netip.Prefix{loopbackPrefix},
			expectedIP: "198.51.100.1",
		},
		{
			name:       "Multi-hop XFF: client -> trusted intermediate -> trusted peer",
			xff:        "203.0.113.195, 10.0.0.2",
			remoteAddr: "127.0.0.1:12345",
			trusted:    []netip.Prefix{loopbackPrefix, privatePrefix},
			expectedIP: "203.0.113.195",
		},
		{
			name:       "Attacker prepended fake IP before connecting to reverse proxy",
			xff:        "8.8.8.8, 203.0.113.195",
			remoteAddr: "127.0.0.1:12345",
			trusted:    []netip.Prefix{loopbackPrefix},
			expectedIP: "203.0.113.195",
		},
		{
			name:       "All hops in XFF are trusted: leftmost valid IP returned",
			xff:        "10.0.0.2, 10.0.0.3",
			remoteAddr: "127.0.0.1:12345",
			trusted:    []netip.Prefix{loopbackPrefix, privatePrefix},
			expectedIP: "10.0.0.2",
		},
		{
			name:       "XFF absent, falls back to X-Real-IP",
			xri:        "192.0.2.1",
			remoteAddr: "127.0.0.1:12345",
			trusted:    []netip.Prefix{loopbackPrefix},
			expectedIP: "192.0.2.1",
		},
		{
			name:       "XFF empty/whitespace, falls back to X-Real-IP",
			xff:        "   ,   ",
			xri:        "192.0.2.1",
			remoteAddr: "127.0.0.1:12345",
			trusted:    []netip.Prefix{loopbackPrefix},
			expectedIP: "192.0.2.1",
		},
		{
			name:       "XFF and X-Real-IP invalid, falls back to RemoteAddr host",
			xff:        "invalid-ip",
			xri:        "also-invalid",
			remoteAddr: "127.0.0.1:12345",
			trusted:    []netip.Prefix{loopbackPrefix},
			expectedIP: "127.0.0.1",
		},
		{
			name:       "IPv6 trusted proxy forwards client IP",
			xff:        "2001:db8::99",
			remoteAddr: "[::1]:54321",
			trusted:    []netip.Prefix{ipv6LoopbackPrefix},
			expectedIP: "2001:db8::99",
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

			got := ExtractClientIP(req, tc.trusted...)
			if got != tc.expectedIP {
				t.Fatalf("expected client IP %q, got %q", tc.expectedIP, got)
			}
		})
	}
}

func TestExtractClientIP_NilRequest(t *testing.T) {
	if got := ExtractClientIP(nil); got != "" {
		t.Fatalf("expected empty string for nil request, got %q", got)
	}
}

func TestParseCIDRorIP(t *testing.T) {
	tests := []struct {
		input       string
		wantPrefix  string
		expectError bool
	}{
		{input: "10.0.0.0/8", wantPrefix: "10.0.0.0/8"},
		{input: "192.168.1.1/32", wantPrefix: "192.168.1.1/32"},
		{input: "127.0.0.1", wantPrefix: "127.0.0.1/32"},
		{input: "::1", wantPrefix: "::1/128"},
		{input: "2001:db8::/32", wantPrefix: "2001:db8::/32"},
		{input: "", expectError: true},
		{input: "   ", expectError: true},
		{input: "invalid", expectError: true},
		{input: "10.0.0.1/99", expectError: true},
	}

	for _, tc := range tests {
		prefix, err := ParseCIDRorIP(tc.input)
		if tc.expectError {
			if err == nil {
				t.Errorf("ParseCIDRorIP(%q) expected error, got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseCIDRorIP(%q) unexpected error: %v", tc.input, err)
			} else if prefix.String() != tc.wantPrefix {
				t.Errorf("ParseCIDRorIP(%q) = %q, want %q", tc.input, prefix.String(), tc.wantPrefix)
			}
		}
	}
}

func TestParseTrustedProxies(t *testing.T) {
	t.Run("valid comma-separated list", func(t *testing.T) {
		prefixes, err := ParseTrustedProxies("127.0.0.1/32, 10.0.0.0/8, ::1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(prefixes) != 3 {
			t.Fatalf("expected 3 prefixes, got %d", len(prefixes))
		}
		if prefixes[0].String() != "127.0.0.1/32" || prefixes[1].String() != "10.0.0.0/8" || prefixes[2].String() != "::1/128" {
			t.Errorf("unexpected prefixes: %v", prefixes)
		}
	})

	t.Run("empty segments ignored", func(t *testing.T) {
		prefixes, err := ParseTrustedProxies("127.0.0.1, , 10.0.0.1/32")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(prefixes) != 2 {
			t.Fatalf("expected 2 prefixes, got %d", len(prefixes))
		}
	})

	t.Run("invalid entry returns error", func(t *testing.T) {
		_, err := ParseTrustedProxies("127.0.0.1/32, bad-ip")
		if err == nil {
			t.Fatal("expected error for invalid proxy entry, got nil")
		}
	})
}

func TestWithTrustedProxiesFromEnv(t *testing.T) {
	t.Setenv("TEST_TRUSTED_PROXIES", "127.0.0.1/32, 10.0.0.0/8")

	h := &Handler{}
	opt := WithTrustedProxiesFromEnv("TEST_TRUSTED_PROXIES")
	opt(h)

	if len(h.trustedProxies) != 2 {
		t.Fatalf("expected 2 trusted proxy prefixes, got %d", len(h.trustedProxies))
	}

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.5")

	got := h.extractClientIP(req)
	if got != "203.0.113.5" {
		t.Fatalf("expected 203.0.113.5, got %s", got)
	}
}

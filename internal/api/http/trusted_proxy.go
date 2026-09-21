package http

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
)

// ParseCIDRorIP parses an IP address or CIDR network into a netip.Prefix.
// If an individual IP is provided without a CIDR mask, a single-host prefix is returned (/32 for IPv4, /128 for IPv6).
func ParseCIDRorIP(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Prefix{}, errors.New("empty IP or CIDR string")
	}
	if strings.Contains(s, "/") {
		return netip.ParsePrefix(s)
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// ParseTrustedProxies parses a comma-separated list of CIDR prefixes or IP addresses.
// Empty segments are ignored. If any segment is invalid, an error is returned.
func ParseTrustedProxies(s string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		prefix, err := ParseCIDRorIP(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: %w", trimmed, err)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

// WithTrustedProxies configures explicit trusted proxy CIDRs/prefixes.
func WithTrustedProxies(proxies ...netip.Prefix) Option {
	return func(h *Handler) {
		h.trustedProxies = append(h.trustedProxies, proxies...)
	}
}

// WithTrustedProxyCIDRs parses and configures trusted proxy CIDRs/IPs for the Handler.
func WithTrustedProxyCIDRs(cidrs ...string) Option {
	return func(h *Handler) {
		for _, c := range cidrs {
			prefixes, err := ParseTrustedProxies(c)
			if err == nil {
				h.trustedProxies = append(h.trustedProxies, prefixes...)
			}
		}
	}
}

// WithTrustedProxiesFromEnv loads trusted proxy CIDRs from an environment variable (default: "PARTY2_TRUSTED_PROXIES").
func WithTrustedProxiesFromEnv(envKey string) Option {
	return func(h *Handler) {
		if envKey == "" {
			envKey = "PARTY2_TRUSTED_PROXIES"
		}
		raw := os.Getenv(envKey)
		if raw != "" {
			prefixes, err := ParseTrustedProxies(raw)
			if err == nil {
				h.trustedProxies = append(h.trustedProxies, prefixes...)
			}
		}
	}
}

// ExtractClientIP extracts the effective client IP address from an incoming HTTP request.
// When direct clients connect (no trusted proxies configured or immediate peer is not in trusted CIDRs),
// RemoteAddr is strictly returned, preventing forwarding header spoofing.
// When the immediate peer is a verified trusted proxy, forwarding headers (X-Forwarded-For, X-Real-IP)
// are consulted using right-to-left traversal of untrusted hops.
func ExtractClientIP(r *http.Request, trustedProxies ...netip.Prefix) string {
	if r == nil {
		return ""
	}

	remoteIPStr := extractHost(r.RemoteAddr)
	peerAddr, err := netip.ParseAddr(remoteIPStr)
	if err != nil {
		return remoteIPStr
	}

	if len(trustedProxies) == 0 || !isTrusted(peerAddr, trustedProxies) {
		return peerAddr.String()
	}

	// Immediate peer is trusted. Consult forwarding headers.
	var xffParts []string
	for _, val := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(val, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				xffParts = append(xffParts, trimmed)
			}
		}
	}

	if len(xffParts) > 0 {
		if clientIP := parseXFF(xffParts, trustedProxies); clientIP != "" {
			return clientIP
		}
	}

	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if xriAddr, err := netip.ParseAddr(xri); err == nil {
			return xriAddr.String()
		}
	}

	return peerAddr.String()
}

func extractHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return strings.Trim(host, "[]")
}

func isTrusted(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func parseXFF(xffParts []string, trustedProxies []netip.Prefix) string {
	for i := len(xffParts) - 1; i >= 0; i-- {
		part := xffParts[i]
		addr, err := netip.ParseAddr(part)
		if err != nil {
			continue
		}
		if !isTrusted(addr, trustedProxies) {
			return addr.String()
		}
	}
	for _, part := range xffParts {
		if addr, err := netip.ParseAddr(part); err == nil {
			return addr.String()
		}
	}
	return ""
}

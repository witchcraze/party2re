package http

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := strings.TrimSpace(xri); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimitMiddleware applies distributed / in-memory rate limiting to incoming HTTP requests.
func (h *Handler) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.limiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		ip := extractClientIP(r)
		var key string
		var limit int64
		var window time.Duration

		// Public registration/login endpoints
		if r.Method == http.MethodPost && (r.URL.Path == "/players" || r.URL.Path == "/sessions") {
			key = "http:public:" + ip + ":" + r.URL.Path
			limit = h.rateLimitCfg.PublicLimit
			window = h.rateLimitCfg.PublicWindow
		} else {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				sessionID := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
				key = "http:auth:" + sessionID
			} else {
				key = "http:general:" + ip
			}
			limit = h.rateLimitCfg.GeneralLimit
			window = h.rateLimitCfg.GeneralWindow
		}

		if limit <= 0 || window <= 0 {
			next.ServeHTTP(w, r)
			return
		}

		res, err := h.limiter.Allow(r.Context(), key, limit, window)
		if err != nil {
			// Fail-open gracefully on limiter error
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(res.ResetAfter.Seconds()), 10))

		if !res.Allowed {
			retrySec := int64(res.ResetAfter.Seconds())
			if retrySec < 1 {
				retrySec = 1
			}
			w.Header().Set("Retry-After", strconv.FormatInt(retrySec, 10))
			writeError(w, http.StatusTooManyRequests, errors.New("rate limit exceeded"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// corsMiddleware handles CORS headers and preflight OPTIONS requests based on allowed origins.
func (h *Handler) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" || origin == "*" {
			next.ServeHTTP(w, r)
			return
		}

		if _, allowed := h.allowedOrigins[origin]; allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware injects standard security response headers on every response.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		next.ServeHTTP(w, r)
	})
}

package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/witchcraze/party2re/internal/ratelimit"
)

func (h *Handler) extractClientIP(r *http.Request) string {
	return ExtractClientIP(r, h.trustedProxies...)
}

func (h *Handler) setRateLimitHeaders(w http.ResponseWriter, res ratelimit.Result) {
	w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(res.Limit, 10))
	w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(res.Remaining, 10))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(res.ResetAfter.Seconds()), 10))
}

func (h *Handler) writeRateLimitExceeded(w http.ResponseWriter, res ratelimit.Result) {
	retrySec := int64(res.ResetAfter.Seconds())
	if retrySec < 1 {
		retrySec = 1
	}
	w.Header().Set("Retry-After", strconv.FormatInt(retrySec, 10))
	writeError(w, http.StatusTooManyRequests, errors.New("rate limit exceeded"))
}

// rateLimitMiddleware applies distributed / in-memory rate limiting to incoming HTTP requests.
func (h *Handler) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.limiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		ip := h.extractClientIP(r)

		// Public registration/login endpoints: strictly keyed by client IP
		if r.Method == http.MethodPost && (r.URL.Path == "/players" || r.URL.Path == "/sessions") {
			limit := h.rateLimitCfg.PublicLimit
			window := h.rateLimitCfg.PublicWindow
			if limit <= 0 || window <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			key := "http:public:" + ip + ":" + r.URL.Path
			res, err := h.limiter.Allow(r.Context(), key, limit, window)
			if err != nil {
				// Fail-open gracefully on limiter error
				next.ServeHTTP(w, r)
				return
			}

			h.setRateLimitHeaders(w, res)
			if !res.Allowed {
				h.writeRateLimitExceeded(w, res)
				return
			}

			next.ServeHTTP(w, r)
			return
		}

		// General endpoints: enforce dual-layer rate limiting (IP-level protection first, then token-level if present)
		limit := h.rateLimitCfg.GeneralLimit
		window := h.rateLimitCfg.GeneralWindow
		if limit <= 0 || window <= 0 {
			next.ServeHTTP(w, r)
			return
		}

		// 1. Client IP rate limit check (protects against rotating unauthenticated token flooding)
		ipRes, ipErr := h.limiter.Allow(r.Context(), "http:general:"+ip, limit, window)
		if ipErr == nil && !ipRes.Allowed {
			h.setRateLimitHeaders(w, ipRes)
			h.writeRateLimitExceeded(w, ipRes)
			return
		}

		// 2. Token-based rate limit check if Authorization: Bearer is present
		var tokenRes ratelimit.Result
		var hasTokenCheck bool
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			if token != "" {
				var tokenErr error
				tokenRes, tokenErr = h.limiter.Allow(r.Context(), "http:auth:"+token, limit, window)
				if tokenErr == nil {
					hasTokenCheck = true
					if !tokenRes.Allowed {
						h.setRateLimitHeaders(w, tokenRes)
						h.writeRateLimitExceeded(w, tokenRes)
						return
					}
				}
			}
		}

		// Set rate limit headers reflecting the tighter bound
		if hasTokenCheck && ipErr == nil {
			if tokenRes.Remaining < ipRes.Remaining {
				h.setRateLimitHeaders(w, tokenRes)
			} else {
				h.setRateLimitHeaders(w, ipRes)
			}
		} else if hasTokenCheck {
			h.setRateLimitHeaders(w, tokenRes)
		} else if ipErr == nil {
			h.setRateLimitHeaders(w, ipRes)
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
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Key")
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

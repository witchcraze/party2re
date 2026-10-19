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

		// General endpoints:
		// 1. Check aggregate client IP rate limit (flood-protection against unauthenticated or rotating token flooding).
		ipLimit := h.rateLimitCfg.IPLimit
		ipWindow := h.rateLimitCfg.IPWindow
		if ipLimit <= 0 && h.rateLimitCfg.GeneralLimit > 0 {
			ipLimit = h.rateLimitCfg.GeneralLimit * 5
		}
		if ipWindow <= 0 {
			ipWindow = h.rateLimitCfg.GeneralWindow
		}

		var ipRes ratelimit.Result
		var hasIPRes bool
		if ipLimit > 0 && ipWindow > 0 {
			res, ipErr := h.limiter.Allow(r.Context(), "http:ip:"+ip, ipLimit, ipWindow)
			if ipErr == nil {
				if !res.Allowed {
					h.setRateLimitHeaders(w, res)
					h.writeRateLimitExceeded(w, res)
					return
				}
				ipRes = res
				hasIPRes = true
			}
		}

		// 2. Check token-based limit if Bearer token present, or general IP limit if unauthenticated
		tokenLimit := h.rateLimitCfg.GeneralLimit
		tokenWindow := h.rateLimitCfg.GeneralWindow
		if tokenLimit <= 0 || tokenWindow <= 0 {
			if hasIPRes {
				h.setRateLimitHeaders(w, ipRes)
			}
			next.ServeHTTP(w, r)
			return
		}

		var targetKey string
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			if token != "" {
				targetKey = "http:auth:" + token
			}
		}
		if targetKey == "" {
			targetKey = "http:general:" + ip
		}

		targetRes, targetErr := h.limiter.Allow(r.Context(), targetKey, tokenLimit, tokenWindow)
		if targetErr != nil {
			// Fail-open gracefully on limiter error
			next.ServeHTTP(w, r)
			return
		}

		if !targetRes.Allowed {
			h.setRateLimitHeaders(w, targetRes)
			h.writeRateLimitExceeded(w, targetRes)
			return
		}

		effectiveRes := targetRes
		if hasIPRes {
			if ipRes.Remaining < targetRes.Remaining {
				effectiveRes = ipRes
			} else if ipRes.Remaining == targetRes.Remaining && ipRes.ResetAfter > targetRes.ResetAfter {
				effectiveRes = ipRes
			}
		}

		h.setRateLimitHeaders(w, effectiveRes)
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

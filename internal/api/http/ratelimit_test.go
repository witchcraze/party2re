package http_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/ratelimit"
)

type stubRateLimiter struct {
	allowFn func(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Result, error)
}

func (s *stubRateLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Result, error) {
	if s.allowFn != nil {
		return s.allowFn(ctx, key, limit, window)
	}
	return ratelimit.Result{Allowed: true, Limit: limit, Remaining: limit - 1, ResetAfter: window}, nil
}

func TestRateLimitMiddleware_PublicEndpointAllowedAndBlocked(t *testing.T) {
	memLimiter := ratelimit.NewMemoryLimiter()
	cfg := apihttp.RateLimitConfig{
		PublicLimit:   2,
		PublicWindow:  time.Minute,
		GeneralLimit:  10,
		GeneralWindow: time.Minute,
	}

	playerSvc := &stubPlayerService{
		registerFn: func(ctx context.Context, username, password string) (coreplayer.Player, error) {
			return coreplayer.Player{ID: "p1", Username: username}, nil
		},
	}
	charSvc := &stubCharacterService{}
	advSvc := &stubAdventureService{}
	shopSvc := &stubShopService{}

	handler := newTestHandler(t, playerSvc, charSvc, advSvc, shopSvc, apihttp.WithRateLimiter(memLimiter, cfg))
	router := handler.Router()

	body := `{"username":"testuser","password":"password123"}`

	// 1. First request -> 201 Created with rate limit headers
	req1 := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.RemoteAddr = "192.168.1.100:12345"
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d (body: %s)", rec1.Code, rec1.Body.String())
	}
	if rec1.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("expected limit 2, got %s", rec1.Header().Get("X-RateLimit-Limit"))
	}
	if rec1.Header().Get("X-RateLimit-Remaining") != "1" {
		t.Errorf("expected remaining 1, got %s", rec1.Header().Get("X-RateLimit-Remaining"))
	}

	// 2. Second request -> 201 Created
	req2 := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "192.168.1.100:12345"
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected remaining 0, got %s", rec2.Header().Get("X-RateLimit-Remaining"))
	}

	// 3. Third request -> 429 Too Many Requests with Retry-After header
	req3 := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req3.Header.Set("Content-Type", "application/json")
	req3.RemoteAddr = "192.168.1.100:12345"
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d (body: %s)", rec3.Code, rec3.Body.String())
	}
	if rec3.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header on 429 response")
	}

	// 4. Request from different IP allowed
	req4 := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req4.Header.Set("Content-Type", "application/json")
	req4.RemoteAddr = "192.168.1.101:12345"
	rec4 := httptest.NewRecorder()
	router.ServeHTTP(rec4, req4)

	if rec4.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for other IP, got %d", rec4.Code)
	}
}

func TestRateLimitMiddleware_FailOpenOnError(t *testing.T) {
	errLimiter := &stubRateLimiter{
		allowFn: func(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Result, error) {
			return ratelimit.Result{}, errors.New("valkey connection failure")
		},
	}

	playerSvc := &stubPlayerService{
		registerFn: func(ctx context.Context, username, password string) (coreplayer.Player, error) {
			return coreplayer.Player{ID: "p1", Username: username}, nil
		},
	}
	charSvc := &stubCharacterService{}
	advSvc := &stubAdventureService{}
	shopSvc := &stubShopService{}

	handler := newTestHandler(t, playerSvc, charSvc, advSvc, shopSvc, apihttp.WithRateLimiter(errLimiter))
	router := handler.Router()

	body := `{"username":"testuser","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected fail-open to allow request (201 Created), got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestRateLimitMiddleware_DirectConnectionCannotSpoofRateLimitIdentity(t *testing.T) {
	memLimiter := ratelimit.NewMemoryLimiter()
	cfg := apihttp.RateLimitConfig{
		PublicLimit:   1,
		PublicWindow:  time.Minute,
		GeneralLimit:  10,
		GeneralWindow: time.Minute,
	}

	playerSvc := &stubPlayerService{
		registerFn: func(ctx context.Context, username, password string) (coreplayer.Player, error) {
			return coreplayer.Player{ID: "p1", Username: username}, nil
		},
	}

	handler := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithRateLimiter(memLimiter, cfg),
	)
	router := handler.Router()

	body := `{"username":"testuser","password":"password123"}`

	// Request 1: uses XFF "1.1.1.1" on direct connection -> 201 Created
	req1 := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Forwarded-For", "1.1.1.1")
	req1.RemoteAddr = "192.168.1.50:12345"
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("req1: expected 201 Created, got %d", rec1.Code)
	}

	// Request 2: attempts to bypass limit by rotating XFF to "2.2.2.2" from same RemoteAddr -> must be blocked (429)
	req2 := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Forwarded-For", "2.2.2.2")
	req2.RemoteAddr = "192.168.1.50:12345"
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("req2: expected 429 Too Many Requests (spoofing prevented), got %d", rec2.Code)
	}

	// Request 3: attempts to bypass limit using X-Real-IP "3.3.3.3" -> must be blocked (429)
	req3 := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-Real-IP", "3.3.3.3")
	req3.RemoteAddr = "192.168.1.50:12345"
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("req3: expected 429 Too Many Requests (X-Real-IP spoofing prevented), got %d", rec3.Code)
	}
}

func TestRateLimitMiddleware_TrustedProxyForwardingHonorsClientIP(t *testing.T) {
	memLimiter := ratelimit.NewMemoryLimiter()
	cfg := apihttp.RateLimitConfig{
		PublicLimit:   1,
		PublicWindow:  time.Minute,
		GeneralLimit:  10,
		GeneralWindow: time.Minute,
	}

	playerSvc := &stubPlayerService{
		registerFn: func(ctx context.Context, username, password string) (coreplayer.Player, error) {
			return coreplayer.Player{ID: "p1", Username: username}, nil
		},
	}

	handler := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithRateLimiter(memLimiter, cfg),
		apihttp.WithTrustedProxyCIDRs("10.0.0.0/8"),
	)
	router := handler.Router()

	body := `{"username":"testuser","password":"password123"}`

	// Request 1 from client A via trusted proxy (10.0.0.1) -> 201 Created
	req1 := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Forwarded-For", "203.0.113.1")
	req1.RemoteAddr = "10.0.0.1:12345"
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("req1: expected 201 Created, got %d", rec1.Code)
	}

	// Request 2 from client A via same trusted proxy -> 429 Too Many Requests
	req2 := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Forwarded-For", "203.0.113.1")
	req2.RemoteAddr = "10.0.0.1:12345"
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("req2: expected 429 Too Many Requests for same client A, got %d", rec2.Code)
	}

	// Request 3 from client B via same trusted proxy -> 201 Created (distinct client IP)
	req3 := httptest.NewRequest(http.MethodPost, "/players", bytes.NewBufferString(body))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-Forwarded-For", "203.0.113.2")
	req3.RemoteAddr = "10.0.0.1:12345"
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusCreated {
		t.Fatalf("req3: expected 201 Created for different client B, got %d", rec3.Code)
	}
}

func TestRateLimitMiddleware_RotatingInvalidBearerTokensThrottledByIP(t *testing.T) {
	memLimiter := ratelimit.NewMemoryLimiter()
	cfg := apihttp.RateLimitConfig{
		PublicLimit:   10,
		PublicWindow:  time.Minute,
		GeneralLimit:  10,
		GeneralWindow: time.Minute,
		IPLimit:       3,
		IPWindow:      time.Minute,
	}

	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			return coreplayer.Player{}, coreplayer.ErrInvalidSession
		},
	}

	handler := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithRateLimiter(memLimiter, cfg),
	)
	router := handler.Router()

	// 1. Send 3 requests with rotating dummy Bearer tokens from the same IP (192.168.1.50)
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
		req.Header.Set("Authorization", "Bearer dummy-token-"+strconv.Itoa(i))
		req.RemoteAddr = "192.168.1.50:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d: unexpectedly throttled with 429", i)
		}
	}

	// 2. 4th request from same IP with a new dummy Bearer token must be throttled with 429 by IP limit
	req4 := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req4.Header.Set("Authorization", "Bearer dummy-token-4")
	req4.RemoteAddr = "192.168.1.50:12345"
	rec4 := httptest.NewRecorder()
	router.ServeHTTP(rec4, req4)

	if rec4.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 4th request from same IP to be throttled with 429, got %d (body: %s)", rec4.Code, rec4.Body.String())
	}
	if rec4.Header().Get("X-RateLimit-Limit") != "3" {
		t.Errorf("expected IP limit 3 in header, got %s", rec4.Header().Get("X-RateLimit-Limit"))
	}
	if rec4.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header on 429 response")
	}

	// 3. Request from different IP (192.168.1.51) with another dummy token should NOT be throttled
	req5 := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req5.Header.Set("Authorization", "Bearer dummy-token-5")
	req5.RemoteAddr = "192.168.1.51:12345"
	rec5 := httptest.NewRecorder()
	router.ServeHTTP(rec5, req5)

	if rec5.Code == http.StatusTooManyRequests {
		t.Fatalf("request from different IP should not be throttled with 429, got %d", rec5.Code)
	}
}

func TestRateLimitMiddleware_MultipleSessionsSharingNATNotThrottledBySingleUserLimit(t *testing.T) {
	memLimiter := ratelimit.NewMemoryLimiter()
	cfg := apihttp.RateLimitConfig{
		PublicLimit:   10,
		PublicWindow:  time.Minute,
		GeneralLimit:  2, // single token limit = 2
		GeneralWindow: time.Minute,
		IPLimit:       7, // shared IP limit = 7 (accommodates 3 users @ 2 reqs + 1 req headroom)
		IPWindow:      time.Minute,
	}

	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			return coreplayer.Player{ID: "p-" + sessionID}, nil
		},
	}

	handler := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithRateLimiter(memLimiter, cfg),
	)
	router := handler.Router()

	natIP := "198.51.100.1:12345"

	// 3 distinct users behind the same NAT IP make 2 requests each (total 6 requests from this IP)
	tokens := []string{"session-user-1", "session-user-2", "session-user-3"}
	for _, tok := range tokens {
		for reqNum := 1; reqNum <= 2; reqNum++ {
			req := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
			req.Header.Set("Authorization", "Bearer "+tok)
			req.RemoteAddr = natIP
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusTooManyRequests {
				t.Fatalf("token %s req %d: unexpectedly throttled with 429 (NAT sharing broke legitimate user)", tok, reqNum)
			}
			if rec.Header().Get("X-RateLimit-Limit") != "2" {
				t.Errorf("expected token limit 2, got %s", rec.Header().Get("X-RateLimit-Limit"))
			}
		}
	}

	// User 1 attempts 3rd request (7th from IP) -> throttled by token limit (Limit: 2) since IP count 7 <= 7
	reqUser1Over := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	reqUser1Over.Header.Set("Authorization", "Bearer session-user-1")
	reqUser1Over.RemoteAddr = natIP
	recUser1Over := httptest.NewRecorder()
	router.ServeHTTP(recUser1Over, reqUser1Over)

	if recUser1Over.Code != http.StatusTooManyRequests {
		t.Fatalf("expected user 1 to be throttled with 429, got %d", recUser1Over.Code)
	}
	if recUser1Over.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("expected token limit 2 on user-1 block, got %s", recUser1Over.Header().Get("X-RateLimit-Limit"))
	}

	// User 4 on same NAT attempts request (8th from IP) -> throttled by IP limit (Limit: 7)
	reqUser4Over := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	reqUser4Over.Header.Set("Authorization", "Bearer session-user-4")
	reqUser4Over.RemoteAddr = natIP
	recUser4Over := httptest.NewRecorder()
	router.ServeHTTP(recUser4Over, reqUser4Over)

	if recUser4Over.Code != http.StatusTooManyRequests {
		t.Fatalf("expected user 4 to be throttled by IP limit with 429, got %d", recUser4Over.Code)
	}
	if recUser4Over.Header().Get("X-RateLimit-Limit") != "7" {
		t.Errorf("expected IP limit 7 on IP block, got %s", recUser4Over.Header().Get("X-RateLimit-Limit"))
	}
}

func TestRateLimitMiddleware_PATUsageHonorsTokenLimit(t *testing.T) {
	memLimiter := ratelimit.NewMemoryLimiter()
	cfg := apihttp.RateLimitConfig{
		PublicLimit:   10,
		PublicWindow:  time.Minute,
		GeneralLimit:  2,
		GeneralWindow: time.Minute,
		IPLimit:       10,
		IPWindow:      time.Minute,
	}

	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			return coreplayer.Player{ID: "p1"}, nil
		},
	}

	handler := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithRateLimiter(memLimiter, cfg),
	)
	router := handler.Router()

	pat := "pat_prod_secret_token_123"

	// Req 1 & 2 allowed
	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
		req.Header.Set("Authorization", "Bearer "+pat)
		req.RemoteAddr = "192.168.1.10:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("PAT request %d unexpectedly throttled", i)
		}
	}

	// Req 3 throttled
	req3 := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req3.Header.Set("Authorization", "Bearer "+pat)
	req3.RemoteAddr = "192.168.1.10:12345"
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected PAT request 3 to be throttled with 429, got %d", rec3.Code)
	}
	if rec3.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("expected limit 2 in header, got %s", rec3.Header().Get("X-RateLimit-Limit"))
	}
}

func TestRateLimitMiddleware_SingleBearerTokenThrottledAcrossIPs(t *testing.T) {
	memLimiter := ratelimit.NewMemoryLimiter()
	cfg := apihttp.RateLimitConfig{
		PublicLimit:   10,
		PublicWindow:  time.Minute,
		GeneralLimit:  2,
		GeneralWindow: time.Minute,
		IPLimit:       10,
		IPWindow:      time.Minute,
	}

	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			return coreplayer.Player{ID: "p1"}, nil
		},
	}

	handler := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithRateLimiter(memLimiter, cfg),
	)
	router := handler.Router()

	token := "shared-valid-token"

	// Req 1 from IP 1
	req1 := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req1.Header.Set("Authorization", "Bearer "+token)
	req1.RemoteAddr = "192.168.1.1:12345"
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code == http.StatusTooManyRequests {
		t.Fatalf("req1 unexpectedly throttled")
	}

	// Req 2 from IP 2
	req2 := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.RemoteAddr = "192.168.1.2:12345"
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code == http.StatusTooManyRequests {
		t.Fatalf("req2 unexpectedly throttled")
	}

	// Req 3 from IP 3 with same token -> throttled by token limit!
	req3 := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	req3.RemoteAddr = "192.168.1.3:12345"
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected req3 to be throttled with 429, got %d", rec3.Code)
	}
	if rec3.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("expected token limit 2 in header, got %s", rec3.Header().Get("X-RateLimit-Limit"))
	}
}

func TestRateLimitMiddleware_RemainingHeaderReflectsTighterBound(t *testing.T) {
	memLimiter := ratelimit.NewMemoryLimiter()
	cfg := apihttp.RateLimitConfig{
		PublicLimit:   10,
		PublicWindow:  time.Minute,
		GeneralLimit:  5,
		GeneralWindow: time.Minute,
		IPLimit:       8,
		IPWindow:      time.Minute,
	}

	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			return coreplayer.Player{ID: "player-1"}, nil
		},
	}

	handler := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithRateLimiter(memLimiter, cfg),
	)
	router := handler.Router()

	ip := "192.168.1.80:12345"

	// 1. Send 7 requests with different tokens from IP -> IP count reaches 7 (Remaining: 1, out of 8)
	for i := 1; i <= 7; i++ {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
		req.Header.Set("Authorization", "Bearer warmup-tok-"+strconv.Itoa(i))
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("warmup req %d unexpectedly throttled", i)
		}
	}

	// 2. 8th request from IP uses a brand new token "token-alpha":
	// IP count becomes 8 -> IP remaining = 0 (Limit: 8)
	// Token count becomes 1 -> Token remaining = 4 (Limit: 5)
	// Tighter remaining bound is IP (0 < 4), so headers must reflect IP limit (8) and remaining (0)!
	reqAlpha := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	reqAlpha.Header.Set("Authorization", "Bearer token-alpha")
	reqAlpha.RemoteAddr = ip
	recAlpha := httptest.NewRecorder()
	router.ServeHTTP(recAlpha, reqAlpha)

	if recAlpha.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for 8th request, got %d", recAlpha.Code)
	}
	if recAlpha.Header().Get("X-RateLimit-Limit") != "8" {
		t.Errorf("expected IP limit 8 in header (tighter bound), got %s", recAlpha.Header().Get("X-RateLimit-Limit"))
	}
	if recAlpha.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected IP remaining 0 in header, got %s", recAlpha.Header().Get("X-RateLimit-Remaining"))
	}
	if recAlpha.Header().Get("X-RateLimit-Reset") == "" {
		t.Errorf("expected X-RateLimit-Reset header")
	}

	// 3. 9th request from same IP uses another brand new token "token-beta":
	// Token "token-beta" has not been used, but IP has reached limit (8/8).
	// Must be blocked with 429 Too Many Requests by IP limit!
	reqBeta := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	reqBeta.Header.Set("Authorization", "Bearer token-beta")
	reqBeta.RemoteAddr = ip
	recBeta := httptest.NewRecorder()
	router.ServeHTTP(recBeta, reqBeta)

	if recBeta.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests due to IP limit breach, got %d", recBeta.Code)
	}
	if recBeta.Header().Get("X-RateLimit-Limit") != "8" {
		t.Errorf("expected IP limit 8 on IP breach, got %s", recBeta.Header().Get("X-RateLimit-Limit"))
	}
	if recBeta.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected IP remaining 0 on IP breach, got %s", recBeta.Header().Get("X-RateLimit-Remaining"))
	}
	if recBeta.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header on 429 response")
	}

	// 4. Token side tighter bound:
	// Use fresh IP "192.168.1.81:12345" with token "token-gamma"
	freshIP := "192.168.1.81:12345"
	// Send 4 requests with "token-gamma":
	// On req 4: IP count = 4 (IP remaining = 4), Token count = 4 (Token remaining = 1).
	// Tighter bound is Token (1 < 4).
	for i := 1; i <= 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
		req.Header.Set("Authorization", "Bearer token-gamma")
		req.RemoteAddr = freshIP
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("token-gamma req %d unexpectedly throttled", i)
		}
		if i == 4 {
			if rec.Header().Get("X-RateLimit-Limit") != "5" {
				t.Errorf("expected token limit 5 (tighter bound), got %s", rec.Header().Get("X-RateLimit-Limit"))
			}
			if rec.Header().Get("X-RateLimit-Remaining") != "1" {
				t.Errorf("expected token remaining 1, got %s", rec.Header().Get("X-RateLimit-Remaining"))
			}
		}
	}

	// 5th request with "token-gamma": Token count becomes 5 (Token remaining = 0). IP count = 5 (IP remaining = 3).
	// Tighter bound is Token (0 < 3).
	reqGamma5 := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	reqGamma5.Header.Set("Authorization", "Bearer token-gamma")
	reqGamma5.RemoteAddr = freshIP
	recGamma5 := httptest.NewRecorder()
	router.ServeHTTP(recGamma5, reqGamma5)

	if recGamma5.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for 5th request, got %d", recGamma5.Code)
	}
	if recGamma5.Header().Get("X-RateLimit-Limit") != "5" {
		t.Errorf("expected token limit 5 (tighter bound), got %s", recGamma5.Header().Get("X-RateLimit-Limit"))
	}
	if recGamma5.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected token remaining 0, got %s", recGamma5.Header().Get("X-RateLimit-Remaining"))
	}

	// 6th request with "token-gamma": Token limit breached!
	reqGamma6 := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	reqGamma6.Header.Set("Authorization", "Bearer token-gamma")
	reqGamma6.RemoteAddr = freshIP
	recGamma6 := httptest.NewRecorder()
	router.ServeHTTP(recGamma6, reqGamma6)

	if recGamma6.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests due to token limit breach, got %d", recGamma6.Code)
	}
	if recGamma6.Header().Get("X-RateLimit-Limit") != "5" {
		t.Errorf("expected token limit 5 on token breach, got %s", recGamma6.Header().Get("X-RateLimit-Limit"))
	}
	if recGamma6.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected token remaining 0 on token breach, got %s", recGamma6.Header().Get("X-RateLimit-Remaining"))
	}
	if recGamma6.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header on 429 response")
	}
}

func TestRateLimitMiddleware_UnauthenticatedGeneralEndpointHonorsHeadersAndLimits(t *testing.T) {
	memLimiter := ratelimit.NewMemoryLimiter()
	cfg := apihttp.RateLimitConfig{
		PublicLimit:   10,
		PublicWindow:  time.Minute,
		GeneralLimit:  2,
		GeneralWindow: time.Minute,
		IPLimit:       5,
		IPWindow:      time.Minute,
	}

	handler := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithRateLimiter(memLimiter, cfg),
	)
	router := handler.Router()

	clientIP := "192.168.1.90:12345"

	// Req 1 (no Authorization header): IP count = 1 (rem 4), General count = 1 (rem 1)
	req1 := httptest.NewRequest(http.MethodGet, "/health", nil)
	req1.RemoteAddr = clientIP
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec1.Code)
	}
	if rec1.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("expected General limit 2 in header, got %s", rec1.Header().Get("X-RateLimit-Limit"))
	}
	if rec1.Header().Get("X-RateLimit-Remaining") != "1" {
		t.Errorf("expected General remaining 1 in header, got %s", rec1.Header().Get("X-RateLimit-Remaining"))
	}

	// Req 2: General count = 2 (rem 0)
	req2 := httptest.NewRequest(http.MethodGet, "/health", nil)
	req2.RemoteAddr = clientIP
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected General remaining 0 in header, got %s", rec2.Header().Get("X-RateLimit-Remaining"))
	}

	// Req 3: General limit breached (2/2) -> 429 Too Many Requests
	req3 := httptest.NewRequest(http.MethodGet, "/health", nil)
	req3.RemoteAddr = clientIP
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", rec3.Code)
	}
	if rec3.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("expected General limit 2 on breach, got %s", rec3.Header().Get("X-RateLimit-Limit"))
	}
	if rec3.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header on 429 response")
	}
}

func TestRateLimitMiddleware_GeneralEndpointFailOpenOnError(t *testing.T) {
	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			return coreplayer.Player{ID: "player-1"}, nil
		},
	}

	cfg := apihttp.RateLimitConfig{
		PublicLimit:   10,
		PublicWindow:  time.Minute,
		GeneralLimit:  5,
		GeneralWindow: time.Minute,
		IPLimit:       10,
		IPWindow:      time.Minute,
	}

	// 1. IP limiter returns error, target limiter succeeds -> request allowed, headers reflect token limit
	ipErrLimiter := &stubRateLimiter{
		allowFn: func(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Result, error) {
			if strings.HasPrefix(key, "http:ip:") {
				return ratelimit.Result{}, errors.New("valkey IP limiter error")
			}
			return ratelimit.Result{Allowed: true, Limit: limit, Remaining: limit - 1, ResetAfter: window}, nil
		},
	}

	handler1 := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithRateLimiter(ipErrLimiter, cfg),
	)
	router1 := handler1.Router()

	req1 := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req1.Header.Set("Authorization", "Bearer valid-token")
	req1.RemoteAddr = "192.168.1.100:12345"
	rec1 := httptest.NewRecorder()
	router1.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on IP limiter error (fail-open), got %d", rec1.Code)
	}
	if rec1.Header().Get("X-RateLimit-Limit") != "5" {
		t.Errorf("expected token limit 5 in header, got %s", rec1.Header().Get("X-RateLimit-Limit"))
	}

	// 2. Target limiter returns error -> request allowed (fail-open), no inconsistent rate limit headers
	targetErrLimiter := &stubRateLimiter{
		allowFn: func(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Result, error) {
			if strings.HasPrefix(key, "http:auth:") || strings.HasPrefix(key, "http:general:") {
				return ratelimit.Result{}, errors.New("valkey target limiter error")
			}
			return ratelimit.Result{Allowed: true, Limit: limit, Remaining: limit - 1, ResetAfter: window}, nil
		},
	}

	handler2 := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithRateLimiter(targetErrLimiter, cfg),
	)
	router2 := handler2.Router()

	req2 := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req2.Header.Set("Authorization", "Bearer valid-token")
	req2.RemoteAddr = "192.168.1.100:12345"
	rec2 := httptest.NewRecorder()
	router2.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on target limiter error (fail-open), got %d", rec2.Code)
	}
	if rec2.Header().Get("X-RateLimit-Limit") != "" {
		t.Errorf("expected no rate limit headers on target limiter failure, got %s", rec2.Header().Get("X-RateLimit-Limit"))
	}
}

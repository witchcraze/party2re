package http_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

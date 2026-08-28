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

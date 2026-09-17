package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/witchcraze/party2re/internal/alchemy"
	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/pvp"
)

func TestPlayerDeletion_RejectsInvalidJSON_ZeroDomainMutations(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		contentType    string
		hasBody        bool
		wantStatus     int
		wantDomainCall bool
	}{
		{
			name:           "absent body succeeds",
			hasBody:        false,
			wantStatus:     http.StatusOK,
			wantDomainCall: true,
		},
		{
			name:           "empty body with json content-type succeeds",
			body:           "",
			contentType:    "application/json",
			hasBody:        true,
			wantStatus:     http.StatusOK,
			wantDomainCall: true,
		},
		{
			name:           "valid json body succeeds",
			body:           `{"password":"secret"}`,
			contentType:    "application/json",
			hasBody:        true,
			wantStatus:     http.StatusOK,
			wantDomainCall: true,
		},
		{
			name:           "malformed json returns 400 and halts",
			body:           `{"password":`,
			contentType:    "application/json",
			hasBody:        true,
			wantStatus:     http.StatusBadRequest,
			wantDomainCall: false,
		},
		{
			name:           "unknown field returns 400 and halts",
			body:           `{"password":"secret","unknown_prop":123}`,
			contentType:    "application/json",
			hasBody:        true,
			wantStatus:     http.StatusBadRequest,
			wantDomainCall: false,
		},
		{
			name:           "unsupported media type returns 415 and halts",
			body:           `{"password":"secret"}`,
			contentType:    "text/plain",
			hasBody:        true,
			wantStatus:     http.StatusUnsupportedMediaType,
			wantDomainCall: false,
		},
		{
			name:           "missing content-type with non-empty body returns 415 and halts",
			body:           `{"password":"secret"}`,
			contentType:    "",
			hasBody:        true,
			wantStatus:     http.StatusUnsupportedMediaType,
			wantDomainCall: false,
		},
		{
			name:           "multiple json documents returns 400 and halts",
			body:           `{"password":"secret"}{"extra":"doc"}`,
			contentType:    "application/json",
			hasBody:        true,
			wantStatus:     http.StatusBadRequest,
			wantDomainCall: false,
		},
		{
			name:           "trailing non-whitespace garbage returns 400 and halts",
			body:           `{"password":"secret"} trailing garbage`,
			contentType:    "application/json",
			hasBody:        true,
			wantStatus:     http.StatusBadRequest,
			wantDomainCall: false,
		},
		{
			name:           "oversized body returns 400 and halts",
			body:           `{"password":"` + strings.Repeat("a", 65*1024) + `"}`,
			contentType:    "application/json",
			hasBody:        true,
			wantStatus:     http.StatusBadRequest,
			wantDomainCall: false,
		},
	}

	for _, tc := range tests {
		t.Run("DELETE /players/me - "+tc.name, func(t *testing.T) {
			var deleteCalls int32
			playerSvc := &stubPlayerService{
				authenticateFn: func(_ context.Context, sessionID string) (coreplayer.Player, error) {
					if sessionID == "valid-session" {
						return coreplayer.Player{ID: "player-123"}, nil
					}
					return coreplayer.Player{}, errors.New("invalid session")
				},
				deleteAccountFn: func(_ context.Context, _, _ string) error {
					atomic.AddInt32(&deleteCalls, 1)
					return nil
				},
			}

			h := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})

			var req *http.Request
			if tc.hasBody {
				req = httptest.NewRequest(http.MethodDelete, "/players/me", strings.NewReader(tc.body))
				if tc.contentType != "" {
					req.Header.Set("Content-Type", tc.contentType)
				}
			} else {
				req = httptest.NewRequest(http.MethodDelete, "/players/me", nil)
			}
			req.Header.Set("Authorization", "Bearer valid-session")

			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}

			calls := atomic.LoadInt32(&deleteCalls)
			if tc.wantDomainCall && calls != 1 {
				t.Errorf("expected 1 deleteAccount call, got %d", calls)
			}
			if !tc.wantDomainCall && calls != 0 {
				t.Errorf("expected 0 deleteAccount calls on invalid input, got %d", calls)
			}
		})

		t.Run("DELETE /players/{id} - "+tc.name, func(t *testing.T) {
			var deleteCalls int32
			playerSvc := &stubPlayerService{
				authenticateFn: func(_ context.Context, sessionID string) (coreplayer.Player, error) {
					if sessionID == "valid-session" {
						return coreplayer.Player{ID: "player-123"}, nil
					}
					return coreplayer.Player{}, errors.New("invalid session")
				},
				deleteAccountFn: func(_ context.Context, _, _ string) error {
					atomic.AddInt32(&deleteCalls, 1)
					return nil
				},
			}

			h := newTestHandler(t, playerSvc, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})

			var req *http.Request
			if tc.hasBody {
				req = httptest.NewRequest(http.MethodDelete, "/players/player-123", strings.NewReader(tc.body))
				if tc.contentType != "" {
					req.Header.Set("Content-Type", tc.contentType)
				}
			} else {
				req = httptest.NewRequest(http.MethodDelete, "/players/player-123", nil)
			}
			req.Header.Set("Authorization", "Bearer valid-session")

			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}

			calls := atomic.LoadInt32(&deleteCalls)
			if tc.wantDomainCall && calls != 1 {
				t.Errorf("expected 1 deleteAccount call, got %d", calls)
			}
			if !tc.wantDomainCall && calls != 0 {
				t.Errorf("expected 0 deleteAccount calls on invalid input, got %d", calls)
			}
		})
	}
}

func setupAuthenticatedHandler(t *testing.T, opts ...apihttp.Option) http.Handler {
	t.Helper()
	playerSvc := &stubPlayerService{
		authenticateFn: func(_ context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return coreplayer.Player{ID: "p1"}, nil
			}
			return coreplayer.Player{}, errors.New("invalid session")
		},
	}
	charSvc := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero"}, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	h := newTestHandler(t, playerSvc, charSvc, &stubAdventureService{}, &stubShopService{}, opts...)
	return h.Router()
}

func TestOptionalJSONEndpoints_RejectsInvalidJSON_ZeroDomainMutations(t *testing.T) {
	invalidCases := []struct {
		name        string
		body        string
		contentType string
		wantStatus  int
	}{
		{
			name:        "malformed json returns 400",
			body:        `{"bad":`,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "unknown field returns 400",
			body:        `{"unrecognized_property": 123}`,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "unsupported media type returns 415",
			body:        `{"password":"xyz"}`,
			contentType: "text/plain",
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "multiple json documents returns 400",
			body:        `{"password":"a"}{"password":"b"}`,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
		},
	}

	t.Run("POST /characters/{id}/alchemy/learn", func(t *testing.T) {
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				var calls int32
				alchSvc := &stubAlchemyService{
					learnRecipeFn: func(_ context.Context, _ string, _ []string) (alchemy.Recipe, error) {
						atomic.AddInt32(&calls, 1)
						return alchemy.Recipe{ID: "r1"}, nil
					},
				}
				router := setupAuthenticatedHandler(t, apihttp.WithAlchemy(alchSvc))

				req := httptest.NewRequest(http.MethodPost, "/characters/c1/alchemy/learn", strings.NewReader(tc.body))
				req.Header.Set("Authorization", "Bearer valid-session")
				req.Header.Set("Content-Type", tc.contentType)

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != tc.wantStatus {
					t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
				}
				if got := atomic.LoadInt32(&calls); got != 0 {
					t.Errorf("expected 0 domain calls on rejected input, got %d", got)
				}
			})
		}

		t.Run("absent body succeeds and invokes domain", func(t *testing.T) {
			var calls int32
			alchSvc := &stubAlchemyService{
				learnRecipeFn: func(_ context.Context, _ string, _ []string) (alchemy.Recipe, error) {
					atomic.AddInt32(&calls, 1)
					return alchemy.Recipe{ID: "r1"}, nil
				},
			}
			router := setupAuthenticatedHandler(t, apihttp.WithAlchemy(alchSvc))

			req := httptest.NewRequest(http.MethodPost, "/characters/c1/alchemy/learn", nil)
			req.Header.Set("Authorization", "Bearer valid-session")

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := atomic.LoadInt32(&calls); got != 1 {
				t.Errorf("expected 1 domain call on absent body, got %d", got)
			}
		})
	})

	t.Run("POST /characters/{id}/pvp/rooms/{room_id}/join", func(t *testing.T) {
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				var calls int32
				pvpSvc := &stubColosseumPvPService{
					joinRoomFn: func(_ context.Context, _, _, _ string) (pvp.RoomDetail, error) {
						atomic.AddInt32(&calls, 1)
						return pvp.RoomDetail{}, nil
					},
				}
				router := setupAuthenticatedHandler(t, apihttp.WithPvP(pvpSvc))

				req := httptest.NewRequest(http.MethodPost, "/characters/c1/pvp/rooms/r1/join", strings.NewReader(tc.body))
				req.Header.Set("Authorization", "Bearer valid-session")
				req.Header.Set("Content-Type", tc.contentType)

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != tc.wantStatus {
					t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
				}
				if got := atomic.LoadInt32(&calls); got != 0 {
					t.Errorf("expected 0 domain calls on rejected input, got %d", got)
				}
			})
		}
	})

	t.Run("POST /characters/{id}/gvg/rooms/{room_id}/join", func(t *testing.T) {
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				var calls int32
				gvgSvc := &stubGvGService{
					joinRoomFn: func(_ context.Context, _, _, _ string) (gvg.RoomDetail, error) {
						atomic.AddInt32(&calls, 1)
						return gvg.RoomDetail{}, nil
					},
				}
				router := setupAuthenticatedHandler(t, apihttp.WithGvG(gvgSvc))

				req := httptest.NewRequest(http.MethodPost, "/characters/c1/gvg/rooms/r1/join", strings.NewReader(tc.body))
				req.Header.Set("Authorization", "Bearer valid-session")
				req.Header.Set("Content-Type", tc.contentType)

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != tc.wantStatus {
					t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
				}
				if got := atomic.LoadInt32(&calls); got != 0 {
					t.Errorf("expected 0 domain calls on rejected input, got %d", got)
				}
			})
		}
	})

	t.Run("POST /contest/settle", func(t *testing.T) {
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				var calls int32
				contestSvc := &stubContestService{
					settleContestFn: func(_ context.Context, _ bool) (contest.SettlementResult, error) {
						atomic.AddInt32(&calls, 1)
						return contest.SettlementResult{Round: 1}, nil
					},
				}
				router := setupAuthenticatedHandler(t,
					apihttp.WithContest(contestSvc),
					apihttp.WithAdminAPIKey("test-admin-key"),
				)

				req := httptest.NewRequest(http.MethodPost, "/contest/settle", strings.NewReader(tc.body))
				req.Header.Set("X-Admin-Key", "test-admin-key")
				req.Header.Set("Content-Type", tc.contentType)

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != tc.wantStatus {
					t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
				}
				if got := atomic.LoadInt32(&calls); got != 0 {
					t.Errorf("expected 0 domain calls on rejected input, got %d", got)
				}
			})
		}
	})

	t.Run("POST /characters/{id}/casino/rooms/{roomId}/join", func(t *testing.T) {
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				var calls int32
				casSvc := &stubCasinoService{
					joinRoomFn: func(_ context.Context, _, _, _ string, _ int) (*casino.RoomDetail, error) {
						atomic.AddInt32(&calls, 1)
						return &casino.RoomDetail{}, nil
					},
				}
				router := setupAuthenticatedHandler(t, apihttp.WithCasino(casSvc))

				req := httptest.NewRequest(http.MethodPost, "/characters/c1/casino/rooms/r1/join", strings.NewReader(tc.body))
				req.Header.Set("Authorization", "Bearer valid-session")
				req.Header.Set("Content-Type", tc.contentType)

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != tc.wantStatus {
					t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
				}
				if got := atomic.LoadInt32(&calls); got != 0 {
					t.Errorf("expected 0 domain calls on rejected input, got %d", got)
				}
			})
		}
	})

	t.Run("POST /characters/{id}/casino/rooms/{roomId}/spectate", func(t *testing.T) {
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				var calls int32
				casSvc := &stubCasinoService{
					spectateRoomFn: func(_ context.Context, _, _, _ string) (*casino.RoomDetail, error) {
						atomic.AddInt32(&calls, 1)
						return &casino.RoomDetail{}, nil
					},
				}
				router := setupAuthenticatedHandler(t, apihttp.WithCasino(casSvc))

				req := httptest.NewRequest(http.MethodPost, "/characters/c1/casino/rooms/r1/spectate", strings.NewReader(tc.body))
				req.Header.Set("Authorization", "Bearer valid-session")
				req.Header.Set("Content-Type", tc.contentType)

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != tc.wantStatus {
					t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
				}
				if got := atomic.LoadInt32(&calls); got != 0 {
					t.Errorf("expected 0 domain calls on rejected input, got %d", got)
				}
			})
		}
	})

	t.Run("POST /characters/{id}/home/sleep", func(t *testing.T) {
		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				var calls int32
				homeSvc := &mockHomeService{
					sleepFn: func(_ context.Context, _, _ string) (home.SleepResult, error) {
						atomic.AddInt32(&calls, 1)
						return home.SleepResult{}, nil
					},
				}
				router := setupAuthenticatedHandler(t, apihttp.WithHome(homeSvc))

				req := httptest.NewRequest(http.MethodPost, "/characters/c1/home/sleep", strings.NewReader(tc.body))
				req.Header.Set("Authorization", "Bearer valid-session")
				req.Header.Set("Content-Type", tc.contentType)

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != tc.wantStatus {
					t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
				}
				if got := atomic.LoadInt32(&calls); got != 0 {
					t.Errorf("expected 0 domain calls on rejected input, got %d", got)
				}
			})
		}
	})
}

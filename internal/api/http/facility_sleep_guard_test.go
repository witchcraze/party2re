package http_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/store"
)

func setupFacilitySleepGuardTest(t *testing.T, sleepStatus home.SleepStatus) http.Handler {
	return setupFacilitySleepGuardTestWithError(t, sleepStatus, nil)
}

func setupFacilitySleepGuardTestWithError(t *testing.T, sleepStatus home.SleepStatus, sleepErr error) http.Handler {
	t.Helper()

	player := coreplayer.Player{ID: "player-1", Username: "hero"}
	char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "Hero", Money: 5000, Deposit: 10000}

	players := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, errors.New("invalid session")
		},
	}

	chars := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == "char-1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	mockHome := &mockHomeService{
		getSleepStatusFn: func(ctx context.Context, characterID string) (home.SleepStatus, error) {
			if sleepErr != nil {
				return home.SleepStatus{}, sleepErr
			}
			return sleepStatus, nil
		},
	}

	readers := &contextReaders{char: char}
	h, err := apihttp.NewHandler(
		players,
		chars,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithHome(mockHome),
		apihttp.WithBank(&stubBankService{}),
		apihttp.WithPlayerContext(playercontext.NewService(readers, readers, timer.NewService(nil))),
		apihttp.WithJob(&stubJobService{}),
		apihttp.WithDepot(&stubDepotService{}),
		apihttp.WithContest(&stubContestService{}),
		apihttp.WithEventPlaza(&mockEventPlazaService{}),
		apihttp.WithGuild(&stubGuildService{}),
		apihttp.WithBlackMarket(&stubBlackMarketService{}),
		apihttp.WithSecretShop(&stubSecretShopService{}),
		apihttp.WithBlacksmith(&stubBlacksmithService{}),
		apihttp.WithStore(noopStoreService{}),
		apihttp.WithMonster(&stubMonsterService{}),
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	return h.Router()
}

func TestTownFacilitiesGuardSleepingCharacter(t *testing.T) {
	testCases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		// Bank
		{"bank_deposit", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"bank_deposit","params":{"amount":100}}`},
		{"bank_withdraw", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"bank_withdraw","params":{"amount":100}}`},
		// Job
		{"job_change", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"job_change","params":{"job_id":"job-01"}}`},
		// Depot
		{"depot_deposit", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"depot_deposit","params":{"item_id":"item-1"}}`},
		{"depot_withdraw", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"depot_withdraw","params":{"item_id":"item-1"}}`},
		{"depot_sell", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"depot_sell","params":{"item_id":"item-1"}}`},
		{"depot_sell_batch", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"depot_sell_batch","params":{"item_ids":["item-1"]}}`},
		{"depot_sort", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"depot_sort"}`},
		{"depot_expand", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"depot_expand"}`},
		{"depot_send_money", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"depot_send_money","params":{"recipient_character_id":"char-2","amount":100}}`},
		{"depot_send_item", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"depot_send_item","params":{"recipient_character_id":"char-2","item_id":"item-1"}}`},
		// Contest
		{"contest_save_photo", http.MethodPost, "/characters/char-1/photos", `{"title":"Photo","location":"Field","image_url":"https://example.com/p.jpg"}`},
		// Event Plaza
		{"eventplaza_presence", http.MethodPost, "/eventplaza/presence", `{"character_id":"char-1"}`},
		{"eventplaza_merchant_purchase", http.MethodPost, "/eventplaza/merchant/purchase", `{"character_id":"char-1","item_id":"item-1","quantity":1}`},
		// Guild
		{"guild_create", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"guild_create","params":{"name":"Guild"}}`},
		// Home
		{"house_build", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"house_build","params":{"town_id":"town1","house_style":"001"}}`},
		// Store
		{"store_build", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_build","params":{"town_id":"town1","house_style":"001"}}`},
		{"store_list_gold", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_list_gold","params":{"item_id":"inst-1","price":1000}}`},
		{"store_list_barter", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_list_barter","params":{"item_id":"inst-1","wish_item_name":"やくそう"}}`},
		{"store_withdraw", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_withdraw","params":{"sale_id":"sale-1"}}`},
		{"store_buy", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_buy","params":{"sale_id":"sale-1"}}`},
		{"store_trade", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_trade","params":{"sale_id":"sale-1","item_id":"inst-barter"}}`},
		{"store_rename", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_rename","params":{"store_name":"新看板"}}`},
		{"store_wallpaper", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_wallpaper","params":{"wallpaper":"farm"}}`},
		{"store_interior_add", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_interior_add","params":{"furniture_id":"001"}}`},
		{"store_interior_rename", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_interior_rename","params":{"interior_id":"int-1","name":"お気に入りの机"}}`},
		{"store_interior_clean", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"store_interior_clean"}`},
		// Monster
		{"monster_bring_home", http.MethodPost, "/characters/char-1/monsters/m-1/bring-home", ``},
		{"monster_deposit", http.MethodPost, "/characters/char-1/monsters/m-1/deposit", ``},
		{"monster_rename", http.MethodPost, "/characters/char-1/monsters/m-1/rename", `{"custom_name":"newname"}`},
		{"monster_send", http.MethodPost, "/characters/char-1/monsters/m-1/send", `{"recipient_character_id":"char-2"}`},
		{"monster_release", http.MethodPost, "/characters/char-1/monsters/m-1/release", ``},
	}

	t.Run("sleeping character is rejected with 409 Conflict across all facility endpoints", func(t *testing.T) {
		router := setupFacilitySleepGuardTest(t, home.SleepStatus{
			Sleeping:         true,
			RemainingSeconds: 300,
		})

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var req *http.Request
				if tc.body != "" {
					req = httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
					req.Header.Set("Content-Type", "application/json")
				} else {
					req = httptest.NewRequest(tc.method, tc.path, nil)
				}
				req.Header.Set("Authorization", "Bearer valid-session")

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != http.StatusConflict {
					t.Errorf("expected 409 Conflict for %s (%s %s), got %d: %s",
						tc.name, tc.method, tc.path, rec.Code, rec.Body.String())
				}
			})
		}
	})

	t.Run("can_wake character is rejected with 409 Conflict across all facility endpoints", func(t *testing.T) {
		router := setupFacilitySleepGuardTest(t, home.SleepStatus{
			Sleeping:         false,
			CanWake:          true,
			RemainingSeconds: 0,
		})

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var req *http.Request
				if tc.body != "" {
					req = httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
					req.Header.Set("Content-Type", "application/json")
				} else {
					req = httptest.NewRequest(tc.method, tc.path, nil)
				}
				req.Header.Set("Authorization", "Bearer valid-session")

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != http.StatusConflict {
					t.Errorf("expected 409 Conflict when can_wake for %s (%s %s), got %d: %s",
						tc.name, tc.method, tc.path, rec.Code, rec.Body.String())
				}
			})
		}
	})

	t.Run("awake character is not blocked with 409 Conflict", func(t *testing.T) {
		router := setupFacilitySleepGuardTest(t, home.SleepStatus{
			Sleeping: false,
			CanWake:  false,
		})

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var req *http.Request
				if tc.body != "" {
					req = httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
					req.Header.Set("Content-Type", "application/json")
				} else {
					req = httptest.NewRequest(tc.method, tc.path, nil)
				}
				req.Header.Set("Authorization", "Bearer valid-session")

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code == http.StatusConflict {
					t.Errorf("expected awake character not to be blocked by 409 Conflict for %s (%s %s), got %d: %s",
						tc.name, tc.method, tc.path, rec.Code, rec.Body.String())
				}
			})
		}
	})

	t.Run("sleep status lookup failure returns 500 across all facility endpoints", func(t *testing.T) {
		router := setupFacilitySleepGuardTestWithError(t, home.SleepStatus{}, errors.New("timer service error"))

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var req *http.Request
				if tc.body != "" {
					req = httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
					req.Header.Set("Content-Type", "application/json")
				} else {
					req = httptest.NewRequest(tc.method, tc.path, nil)
				}
				req.Header.Set("Authorization", "Bearer valid-session")

				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != http.StatusInternalServerError {
					t.Errorf("expected 500 Internal Server Error when sleep lookup fails for %s (%s %s), got %d: %s",
						tc.name, tc.method, tc.path, rec.Code, rec.Body.String())
				}
			})
		}
	})

	t.Run("explicit wake recovery command is exempt from action guard when sleeping or can_wake", func(t *testing.T) {
		for _, status := range []home.SleepStatus{
			{Sleeping: true, RemainingSeconds: 300},
			{Sleeping: false, CanWake: true, RemainingSeconds: 0},
		} {
			router := setupFacilitySleepGuardTest(t, status)
			req := httptest.NewRequest(http.MethodPost, "/characters/char-1/home/wake", nil)
			req.Header.Set("Authorization", "Bearer valid-session")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusConflict {
				t.Errorf("expected explicit wake command not to be blocked by 409 action guard, got %d: %s",
					rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("monster mutations never invoke service when sleeping, can_wake, or sleep lookup fails", func(t *testing.T) {
		conditions := []struct {
			name         string
			sleepStatus  home.SleepStatus
			sleepErr     error
			expectedCode int
		}{
			{"sleeping", home.SleepStatus{Sleeping: true, RemainingSeconds: 300}, nil, http.StatusConflict},
			{"can_wake", home.SleepStatus{CanWake: true}, nil, http.StatusConflict},
			{"lookup_error", home.SleepStatus{}, errors.New("sleep lookup failed"), http.StatusInternalServerError},
		}

		monsterRoutes := []struct {
			name   string
			method string
			path   string
			body   string
		}{
			{"bring_home", http.MethodPost, "/characters/char-1/monsters/m-1/bring-home", ""},
			{"deposit", http.MethodPost, "/characters/char-1/monsters/m-1/deposit", ""},
			{"rename", http.MethodPost, "/characters/char-1/monsters/m-1/rename", `{"custom_name":"newname"}`},
			{"send", http.MethodPost, "/characters/char-1/monsters/m-1/send", `{"recipient_character_id":"char-2"}`},
			{"release", http.MethodPost, "/characters/char-1/monsters/m-1/release", ""},
		}

		for _, cond := range conditions {
			t.Run(cond.name, func(t *testing.T) {
				called := false
				mStub := &stubMonsterService{
					bringToHomeFn: func(_ context.Context, _, _ string) (monster.MonsterInstance, error) {
						called = true
						return monster.MonsterInstance{}, nil
					},
					depositToBoxFn: func(_ context.Context, _, _ string) (monster.MonsterInstance, error) {
						called = true
						return monster.MonsterInstance{}, nil
					},
					renameFn: func(_ context.Context, _, _, _ string) (monster.MonsterInstance, error) {
						called = true
						return monster.MonsterInstance{}, nil
					},
					sendMonsterFn: func(_ context.Context, _, _, _ string) error {
						called = true
						return nil
					},
					releaseMonsterFn: func(_ context.Context, _, _ string) error {
						called = true
						return nil
					},
				}

				player := coreplayer.Player{ID: "player-1", Username: "hero"}
				char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "Hero"}
				players := &stubPlayerService{
					authenticateFn: func(_ context.Context, _ string) (coreplayer.Player, error) { return player, nil },
				}
				chars := &stubCharacterService{
					getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
				}
				mockHome := &mockHomeService{
					getSleepStatusFn: func(_ context.Context, _ string) (home.SleepStatus, error) {
						return cond.sleepStatus, cond.sleepErr
					},
				}

				h, err := apihttp.NewHandler(
					players,
					chars,
					&stubAdventureService{},
					&stubShopService{},
					apihttp.WithHome(mockHome),
					apihttp.WithMonster(mStub),
				)
				if err != nil {
					t.Fatal(err)
				}
				router := h.Router()

				for _, route := range monsterRoutes {
					called = false
					var req *http.Request
					if route.body != "" {
						req = httptest.NewRequest(route.method, route.path, bytes.NewReader([]byte(route.body)))
						req.Header.Set("Content-Type", "application/json")
					} else {
						req = httptest.NewRequest(route.method, route.path, nil)
					}
					req.Header.Set("Authorization", "Bearer valid-session")
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)

					if rec.Code != cond.expectedCode {
						t.Errorf("[%s - %s] expected status %d, got %d: %s", cond.name, route.name, cond.expectedCode, rec.Code, rec.Body.String())
					}
					if called {
						t.Errorf("[%s - %s] expected service method not to be called, but was called", cond.name, route.name)
					}
				}
			})
		}
	})

	t.Run("monster read endpoints are exempt from sleep guard", func(t *testing.T) {
		router := setupFacilitySleepGuardTest(t, home.SleepStatus{
			Sleeping:         true,
			RemainingSeconds: 300,
		})

		readRoutes := []string{
			"/characters/char-1/monsters",
			"/characters/char-1/monsters?location=box",
			"/monster/dialogue",
		}

		for _, path := range readRoutes {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer valid-session")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusConflict {
				t.Errorf("expected GET %s not to be blocked by sleep guard, got %d: %s", path, rec.Code, rec.Body.String())
			}
		}
	})
}

// noopStoreService accepts every store command so awake cases reach execution.
type noopStoreService struct{ apihttp.StoreService }

func (noopStoreService) BuildStore(context.Context, string, string, string) (*store.StoreCheckResult, error) {
	return &store.StoreCheckResult{}, nil
}
func (noopStoreService) ListGoldItem(context.Context, string, string, int) (*store.Sale, error) {
	return &store.Sale{}, nil
}
func (noopStoreService) ListBarterItem(context.Context, string, string, string) (*store.Sale, error) {
	return &store.Sale{}, nil
}
func (noopStoreService) WithdrawListing(context.Context, string, string) error        { return nil }
func (noopStoreService) BuyItem(context.Context, string, string) error                { return nil }
func (noopStoreService) TradeItem(context.Context, string, string, string) error      { return nil }
func (noopStoreService) ChangeStoreName(context.Context, string, string) error        { return nil }
func (noopStoreService) ChangeWallpaper(context.Context, string, string) error        { return nil }
func (noopStoreService) RenameInterior(context.Context, string, string, string) error { return nil }
func (noopStoreService) CleanInteriors(context.Context, string) error                 { return nil }
func (noopStoreService) AddInterior(context.Context, string, string) (*store.Interior, error) {
	return &store.Interior{}, nil
}

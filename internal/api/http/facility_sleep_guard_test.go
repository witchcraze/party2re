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
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/helperquest"
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
		apihttp.WithCustomSkill(&stubCustomSkillService{}),
		apihttp.WithGuild(&stubGuildService{}),
		apihttp.WithMedal(&mockMedalService{}),
		apihttp.WithBlackMarket(&stubBlackMarketService{}),
		apihttp.WithSecretShop(&stubSecretShopService{}),
		apihttp.WithBlacksmith(&stubBlacksmithService{}),
		apihttp.WithStore(noopStoreService{}),
		apihttp.WithMonster(&stubMonsterService{}),
		apihttp.WithHelper(&mockHelperService{}),
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
		// Adventure
		{"adventure_start", http.MethodPost, "/adventures", `{"character_id":"char-1","stage_id":"stage-1"}`},
		// Bank
		{"bank_deposit", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"bank_deposit","params":{"amount":100}}`},
		{"bank_withdraw", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"bank_withdraw","params":{"amount":100}}`},
		// Job
		{"job_change", http.MethodPost, "/characters/char-1/change-job", `{"job_id":"warrior"}`},
		{"job_exchange", http.MethodPost, "/characters/char-1/exchange-job", `{"job_id":"mage","old_job_id":"warrior"}`},
		{"job_memory_save", http.MethodPost, "/characters/char-1/future-memories", `{}`},
		{"job_memory_recall", http.MethodPost, "/characters/char-1/recall-future", `{"memory_id":"mem-1"}`},
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
		// Tavern
		// Custom Skill
		{"custom_skill_set", http.MethodPost, "/characters/char-1/custom-skills", `{"name":"Skill","comment":"Desc","gems":["g1","g2","g3"]}`},
		// Guild
		{"guild_create", http.MethodPost, "/guilds", `{"character_id":"char-1","name":"Guild"}`},
		{"guild_apply", http.MethodPost, "/guilds/guild-1/apply", `{"character_id":"char-1"}`},
		{"guild_approve", http.MethodPost, "/guilds/guild-1/applications/char-2/approve", `{"character_id":"char-1"}`},
		{"guild_reject", http.MethodPost, "/guilds/guild-1/applications/char-2/reject", `{"character_id":"char-1"}`},
		{"guild_callout", http.MethodPost, "/guilds/guild-1/callout", `{"character_id":"char-1","message":"Hello"}`},
		{"guild_assign_title", http.MethodPut, "/guilds/guild-1/members/char-2/title", `{"character_id":"char-1","title":"Officer"}`},
		{"guild_customization", http.MethodPut, "/guilds/guild-1/customization", `{"character_id":"char-1","color":"#123456"}`},
		{"guild_leave", http.MethodPost, "/guilds/guild-1/leave", `{"character_id":"char-1"}`},
		{"guild_kick", http.MethodDelete, "/guilds/guild-1/members/char-2", `{"character_id":"char-1"}`},
		{"guild_disband", http.MethodDelete, "/guilds/guild-1", `{"character_id":"char-1"}`},
		// Medal
		{"medal_claim_reward", http.MethodPost, "/medals/claim", `{"character_id":"char-1","item_id":"reward-1"}`},
		{"medal_claim_achievement", http.MethodPost, "/characters/char-1/achievements/ach-1/claim", `{}`},
		// Home
		{"home_build_house", http.MethodPost, "/towns/town1/houses", `{"character_id":"char-1","house_style":"001"}`},
		{"home_set_character_color", http.MethodPost, "/characters/char-1/color", `{"color":"#ffffff"}`},
		{"home_use_item", http.MethodPost, "/characters/char-1/home/items/use", `{"instance_id":"item-1","source":"inventory"}`},
		{"home_settings", http.MethodPost, "/homes/char-1/settings", `{"companion_name":"ペット"}`},
		{"home_send_letter", http.MethodPost, "/letters", `{"sender_character_id":"char-1","recipient_character_id":"char-2","content":"hello"}`},
		{"home_read_letter", http.MethodPost, "/letters/letter-1/read", `{"character_id":"char-1"}`},
		{"home_delete_letter", http.MethodDelete, "/letters/letter-1?character_id=char-1", ``},
		{"home_teach_phrase", http.MethodPost, "/homes/char-1/companion/phrases", `{"phrase":"hello"}`},
		{"home_forget_phrase", http.MethodDelete, "/homes/char-1/companion/phrases/phrase-1", ``},
		{"home_clear_notices", http.MethodPost, "/homes/char-1/notices/clear", `{}`},
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
		// Helper
		{"helper_complete", http.MethodPost, "/helpers/complete", `{"character_id":"char-1","quest_id":"quest-1"}`},
		{"helper_complete_renew_only", http.MethodPost, "/helpers/complete", `{"character_id":"char-1"}`},
	}

	t.Run("sleeping character is rejected with 409 Conflict across all facility endpoints", func(t *testing.T) {
		router := setupFacilitySleepGuardTest(t, home.SleepStatus{
			Sleeping:         true,
			RemainingSeconds: 300,
			Message:          "お休み中「Zzz...」 目覚めるまで 5分00秒",
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
			Message:          "お休み中「Zzz...」 目を覚ましてください",
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
			Message:  "起きています",
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

	t.Run("sleep lookup failure never invokes facility action callback", func(t *testing.T) {
		called := false
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
				return home.SleepStatus{}, errors.New("timer lookup failed")
			},
			sendLetterFn: func(_ context.Context, _, _, _, _ string) (home.Letter, error) {
				called = true
				return home.Letter{}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/letters", bytes.NewReader([]byte(`{"sender_character_id":"char-1","recipient_character_id":"char-2","content":"hello"}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
		if called {
			t.Error("expected letter callback not to be invoked on sleep lookup failure")
		}
	})

	t.Run("canceled context during sleep lookup returns 500 and never invokes callback", func(t *testing.T) {
		called := false
		player := coreplayer.Player{ID: "player-1", Username: "hero"}
		char := corecharacter.Character{ID: "char-1", PlayerID: "player-1", Name: "Hero"}
		players := &stubPlayerService{
			authenticateFn: func(_ context.Context, _ string) (coreplayer.Player, error) { return player, nil },
		}
		chars := &stubCharacterService{
			getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
		}
		mockHome := &mockHomeService{
			getSleepStatusFn: func(ctx context.Context, _ string) (home.SleepStatus, error) {
				return home.SleepStatus{}, ctx.Err()
			},
			sendLetterFn: func(_ context.Context, _, _, _, _ string) (home.Letter, error) {
				called = true
				return home.Letter{}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, &stubShopService{}, apihttp.WithHome(mockHome))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, "/letters", bytes.NewReader([]byte(`{"sender_character_id":"char-1","recipient_character_id":"char-2","content":"hello"}`))).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
		if called {
			t.Error("expected letter callback not to be invoked on canceled context")
		}
	})

	t.Run("explicit wake recovery command is exempt from action guard when sleeping or can_wake", func(t *testing.T) {
		for _, status := range []home.SleepStatus{
			{Sleeping: true, RemainingSeconds: 300, Message: "お休み中「Zzz...」"},
			{Sleeping: false, CanWake: true, RemainingSeconds: 0, Message: "お休み中「Zzz...」 目を覚ましてください"},
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
			{"sleeping", home.SleepStatus{Sleeping: true, RemainingSeconds: 300, Message: "お休み中「Zzz...」"}, nil, http.StatusConflict},
			{"can_wake", home.SleepStatus{CanWake: true, Message: "お休み中「Zzz...」 目を覚ましてください"}, nil, http.StatusConflict},
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
			Message:          "お休み中「Zzz...」",
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

	t.Run("helper mutations never invoke service when sleeping, can_wake, or sleep lookup fails", func(t *testing.T) {
		conditions := []struct {
			name         string
			sleepStatus  home.SleepStatus
			sleepErr     error
			expectedCode int
		}{
			{"sleeping", home.SleepStatus{Sleeping: true, RemainingSeconds: 300, Message: "お休み中「Zzz...」"}, nil, http.StatusConflict},
			{"can_wake", home.SleepStatus{CanWake: true, Message: "お休み中「Zzz...」 目を覚ましてください"}, nil, http.StatusConflict},
			{"lookup_error", home.SleepStatus{}, errors.New("sleep lookup failed"), http.StatusInternalServerError},
		}

		helperRoutes := []struct {
			name string
			body string
		}{
			{"complete", `{"character_id":"char-1","quest_id":"quest-1"}`},
			{"renew_only", `{"character_id":"char-1"}`},
			{"renew_only_empty_quest_id", `{"character_id":"char-1","quest_id":""}`},
		}

		for _, cond := range conditions {
			t.Run(cond.name, func(t *testing.T) {
				called := false
				helperStub := &mockHelperService{
					completeQuestFn: func(_ context.Context, _, _ string, _ time.Time) (helperquest.CompletionResult, error) {
						called = true
						return helperquest.CompletionResult{}, nil
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
					apihttp.WithHelper(helperStub),
				)
				if err != nil {
					t.Fatal(err)
				}
				router := h.Router()

				for _, route := range helperRoutes {
					called = false
					req := httptest.NewRequest(http.MethodPost, "/helpers/complete", bytes.NewReader([]byte(route.body)))
					req.Header.Set("Content-Type", "application/json")
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

	t.Run("helper read endpoints are exempt from sleep guard", func(t *testing.T) {
		router := setupFacilitySleepGuardTest(t, home.SleepStatus{
			Sleeping:         true,
			RemainingSeconds: 300,
			Message:          "お休み中「Zzz...」",
		})

		req := httptest.NewRequest(http.MethodGet, "/helpers/quests", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusConflict {
			t.Errorf("expected GET /helpers/quests not to be blocked by sleep guard, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("home estate mutations never invoke service when sleeping, can_wake, or sleep lookup fails", func(t *testing.T) {
		conditions := []struct {
			name         string
			sleepStatus  home.SleepStatus
			sleepErr     error
			expectedCode int
		}{
			{"sleeping", home.SleepStatus{Sleeping: true, RemainingSeconds: 300, Message: "お休み中「Zzz...」"}, nil, http.StatusConflict},
			{"can_wake", home.SleepStatus{CanWake: true, Message: "お休み中「Zzz...」 目を覚ましてください"}, nil, http.StatusConflict},
			{"lookup_error", home.SleepStatus{}, errors.New("sleep lookup failed"), http.StatusInternalServerError},
		}

		homeRoutes := []struct {
			name   string
			method string
			path   string
			body   string
		}{
			{"build_house", http.MethodPost, "/towns/town1/houses", `{"character_id":"char-1","house_style":"001"}`},
			{"set_character_color", http.MethodPost, "/characters/char-1/color", `{"color":"#ffffff"}`},
			{"use_home_item", http.MethodPost, "/characters/char-1/home/items/use", `{"instance_id":"item-1","source":"inventory"}`},
		}

		for _, cond := range conditions {
			t.Run(cond.name, func(t *testing.T) {
				called := false
				mockHome := &mockHomeService{
					getSleepStatusFn: func(_ context.Context, _ string) (home.SleepStatus, error) {
						return cond.sleepStatus, cond.sleepErr
					},
					buildHouseFn: func(_ context.Context, _, _, _ string) (*home.HomeCheckResult, error) {
						called = true
						return &home.HomeCheckResult{}, nil
					},
					setCharacterColorFn: func(_ context.Context, _, _ string) error {
						called = true
						return nil
					},
					useHomeItemFn: func(_ context.Context, _, _, _ string) (*home.UseHomeItemResult, error) {
						called = true
						return &home.UseHomeItemResult{}, nil
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

				h, err := apihttp.NewHandler(
					players,
					chars,
					&stubAdventureService{},
					&stubShopService{},
					apihttp.WithHome(mockHome),
				)
				if err != nil {
					t.Fatal(err)
				}
				router := h.Router()

				for _, route := range homeRoutes {
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

	t.Run("home estate read endpoints are exempt from sleep guard", func(t *testing.T) {
		router := setupFacilitySleepGuardTest(t, home.SleepStatus{
			Sleeping:         true,
			RemainingSeconds: 300,
			Message:          "お休み中「Zzz...」",
		})

		readRoutes := []struct {
			method string
			path   string
		}{
			{http.MethodGet, "/towns/town1/houses"},
			{http.MethodGet, "/houses/check?target=Hero"},
			{http.MethodGet, "/characters/char-1/home/items"},
		}

		for _, route := range readRoutes {
			req := httptest.NewRequest(route.method, route.path, nil)
			req.Header.Set("Authorization", "Bearer valid-session")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusConflict {
				t.Errorf("expected %s %s not to be blocked by sleep guard, got %d: %s", route.method, route.path, rec.Code, rec.Body.String())
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

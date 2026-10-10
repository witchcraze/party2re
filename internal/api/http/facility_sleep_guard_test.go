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
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/shop"
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
		apihttp.WithPlantation(&stubPlantationService{}),
		apihttp.WithAltar(&stubAltarService{}),
		apihttp.WithChapel(&stubChapelService{}),
		apihttp.WithWishingWell(&stubWishingWellService{}),
		apihttp.WithContest(&stubContestService{}),
		apihttp.WithGemStore(&stubGemStoreService{}),
		apihttp.WithLottery(&stubLotteryService{}),
		apihttp.WithEventPlaza(&mockEventPlazaService{}),
		apihttp.WithTavern(&stubTavernService{}),
		apihttp.WithCustomSkill(&stubCustomSkillService{}),
		apihttp.WithGuild(&stubGuildService{}),
		apihttp.WithMedal(&mockMedalService{}),
		apihttp.WithAlchemy(&stubAlchemyService{}),
		apihttp.WithBlackMarket(&stubBlackMarketService{}),
		apihttp.WithSecretShop(&stubSecretShopService{}),
		apihttp.WithBlacksmith(&stubBlacksmithService{}),
		apihttp.WithFleaMarket(&stubFleaMarketService{}),
		apihttp.WithStore(&stubStoreService{}),
		apihttp.WithPark(&mockParkService{}),
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
		{"depot_deposit", http.MethodPost, "/characters/char-1/depot/deposit", `{"item_id":"item-1"}`},
		{"depot_withdraw", http.MethodPost, "/characters/char-1/depot/withdraw", `{"item_id":"item-1"}`},
		{"depot_sell", http.MethodPost, "/characters/char-1/depot/sell", `{"item_id":"item-1"}`},
		{"depot_sell_batch", http.MethodPost, "/characters/char-1/depot/sell-batch", `{"item_ids":["item-1"]}`},
		{"depot_sort", http.MethodPost, "/characters/char-1/depot/sort", `{}`},
		{"depot_expand", http.MethodPost, "/api/v1/characters/char-1/actions", `{"action":"depot_expand"}`},
		{"depot_send_money", http.MethodPost, "/characters/char-1/depot/send-money", `{"recipient_character_id":"char-2","amount":100}`},
		{"depot_send_item", http.MethodPost, "/characters/char-1/depot/send-item", `{"recipient_character_id":"char-2","item_id":"item-1"}`},
		// Plantation
		{"plantation_sow", http.MethodPost, "/characters/char-1/plantation/sow", `{"seed_id":"seed-1"}`},
		{"plantation_fertilize", http.MethodPost, "/characters/char-1/plantation/fertilize", `{"fertilizer_id":"fert-1"}`},
		{"plantation_harvest", http.MethodPost, "/characters/char-1/plantation/harvest", `{}`},
		// Altar
		{"altar_pray", http.MethodPost, "/characters/char-1/altar/pray", `{}`},
		{"altar_wish", http.MethodPost, "/characters/char-1/altar/wish", `{"item_id":"sword-1"}`},
		{"altar_offer", http.MethodPost, "/characters/char-1/altar/offer", `{"orb":"silver"}`},
		// Chapel
		{"chapel_pray", http.MethodPost, "/characters/char-1/chapel/pray", `{"blessing":"EXP"}`},
		// Wishing Well
		{"wishingwell_exchange", http.MethodPost, "/characters/char-1/wishing-well/exchange", `{"stat":"STR","sp":10}`},
		// Contest
		{"contest_enter", http.MethodPost, "/characters/char-1/contest/enter", `{"photo_id":"p-1","title":"Entry"}`},
		{"contest_vote", http.MethodPost, "/characters/char-1/contest/vote", `{"entry_id":"e-1","comment":"Nice"}`},
		{"contest_save_photo", http.MethodPost, "/characters/char-1/photos", `{"title":"Photo","location":"Field","image_url":"https://example.com/p.jpg"}`},
		{"contest_delete_photo", http.MethodDelete, "/characters/char-1/photos/photo-1", ``},
		// Gem Store
		{"gemstore_buy", http.MethodPost, "/characters/char-1/gemstore/buy", `{"gem_id":"gem-1"}`},
		{"gemstore_sell", http.MethodPost, "/characters/char-1/gemstore/sell", `{"item_id":"item-1"}`},
		{"gemstore_send", http.MethodPost, "/characters/char-1/gemstore/send", `{"recipient_character_id":"char-2","item_id":"gem-1"}`},
		{"gemstore_synthesize", http.MethodPost, "/characters/char-1/gemstore/synthesize", `{"recipe_id":"rec-1"}`},
		{"gemstore_appraise", http.MethodPost, "/characters/char-1/gemstore/appraise", `{"item_id":"item-1"}`},
		{"gemstore_sort_gembox", http.MethodPost, "/characters/char-1/gembox/sort", `{}`},
		// Lottery
		{"lottery_play", http.MethodPost, "/characters/char-1/lottery/raffle", `{"raffle_type":"STANDARD"}`},
		{"lottery_takarakuji_buy", http.MethodPost, "/characters/char-1/lottery/takarakuji/buy", `{}`},
		// Event Plaza
		{"eventplaza_presence", http.MethodPost, "/eventplaza/presence", `{"character_id":"char-1"}`},
		{"eventplaza_merchant_purchase", http.MethodPost, "/eventplaza/merchant/purchase", `{"character_id":"char-1","item_id":"item-1","quantity":1}`},
		{"eventplaza_toast", http.MethodPost, "/eventplaza/banquets/banquet-1/toast", `{"character_id":"char-1"}`},
		// Tavern
		{"tavern_order", http.MethodPost, "/characters/char-1/tavern/order", `{"item_id":"ale"}`},
		{"tavern_reserve_delivery", http.MethodPost, "/characters/char-1/tavern/delivery", `{"item_id":"ale"}`},
		{"tavern_cancel_delivery", http.MethodDelete, "/characters/char-1/tavern/delivery", ``},
		{"tavern_claim_delivery", http.MethodPost, "/characters/char-1/tavern/delivery/claim", `{}`},
		{"tavern_talk", http.MethodPost, "/characters/char-1/tavern/talk", `{}`},
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
		// Shop & Commercial
		{"shop_purchase", http.MethodPost, "/shop/purchase", `{"character_id":"char-1","item_definition_id":"herb","quantity":1}`},
		{"shop_sell", http.MethodPost, "/shop/sell", `{"character_id":"char-1","item_instance_id":"item-1","quantity":1}`},
		{"shop_batch_purchase", http.MethodPost, "/characters/char-1/shop/batch-purchase", `{"shop_type":"item","items":[{"item_definition_id":"herb","quantity":1}]}`},
		{"shop_talk", http.MethodPost, "/characters/char-1/shop/item/talk", `{}`},
		{"shop_discover_secret", http.MethodPost, "/characters/char-1/shop/discover-secret", `{}`},
		{"shop_accessory_buy", http.MethodPost, "/characters/char-1/shop/accessory/buy", `{"item_definition_id":"acc-1","quantity":1}`},
		{"shop_accessory_sell", http.MethodPost, "/characters/char-1/shop/accessory/sell", `{"item_instance_id":"item-1","quantity":1}`},
		{"shop_accessory_synthesize", http.MethodPost, "/characters/char-1/shop/accessory/synthesize", `{"recipe_target":"acc-2"}`},
		// Alchemy
		{"alchemy_synthesize", http.MethodPost, "/characters/char-1/alchemy/synthesize", `{"recipe_id":"rec-1"}`},
		{"alchemy_claim", http.MethodPost, "/characters/char-1/alchemy/claim", `{}`},
		{"alchemy_learn", http.MethodPost, "/characters/char-1/alchemy/learn", `{}`},
		// Black Market
		{"blackmarket_sacrifice", http.MethodPost, "/characters/char-1/blackmarket/sacrifice", `{"item_instance_id":"item-1"}`},
		{"blackmarket_trade", http.MethodPost, "/characters/char-1/blackmarket/trade", `{"prize_id":"prize-1"}`},
		// Secret Shop
		{"secretshop_puffpuff", http.MethodPost, "/characters/char-1/secretshop/puffpuff", `{}`},
		// Blacksmith
		{"blacksmith_seal", http.MethodPost, "/characters/char-1/blacksmith/seal", `{"seal_id":1}`},
		{"blacksmith_name", http.MethodPost, "/characters/char-1/blacksmith/name", `{"target":"weapon","name":"Excalibur"}`},
		{"blacksmith_deposit", http.MethodPost, "/characters/char-1/blacksmith/storage/deposit", `{}`},
		{"blacksmith_withdraw", http.MethodPost, "/characters/char-1/blacksmith/storage/withdraw", `{"slot":1}`},
		// Flea Market
		{"fleamarket_create", http.MethodPost, "/characters/char-1/fleamarket/listings", `{"item_id":"item-1","price":100}`},
		{"fleamarket_purchase", http.MethodPost, "/characters/char-1/fleamarket/listings/list-1/purchase", `{}`},
		{"fleamarket_cancel", http.MethodDelete, "/characters/char-1/fleamarket/listings/list-1", ``},
		// Home
		{"home_settings", http.MethodPost, "/homes/char-1/settings", `{"companion_name":"ペット"}`},
		{"home_send_letter", http.MethodPost, "/letters", `{"sender_character_id":"char-1","recipient_character_id":"char-2","content":"hello"}`},
		{"home_read_letter", http.MethodPost, "/letters/letter-1/read", `{"character_id":"char-1"}`},
		{"home_delete_letter", http.MethodDelete, "/letters/letter-1?character_id=char-1", ``},
		{"home_teach_phrase", http.MethodPost, "/homes/char-1/companion/phrases", `{"phrase":"hello"}`},
		{"home_forget_phrase", http.MethodDelete, "/homes/char-1/companion/phrases/phrase-1", ``},
		{"home_clear_notices", http.MethodPost, "/homes/char-1/notices/clear", `{}`},
		// Store
		{"store_build", http.MethodPost, "/towns/town1/stores", `{"character_id":"char-1","house_style":"001","store_name":"マイショップ"}`},
		{"store_list_gold", http.MethodPost, "/characters/char-1/store/listings/gold", `{"depot_item_instance_id":"inst-1","price":1000}`},
		{"store_list_barter", http.MethodPost, "/characters/char-1/store/listings/barter", `{"depot_item_instance_id":"inst-1","wish_item_name":"やくそう"}`},
		{"store_withdraw_listing", http.MethodDelete, "/characters/char-1/store/listings/sale-1", ``},
		{"store_buy", http.MethodPost, "/characters/char-1/store/sales/sale-1/buy", ``},
		{"store_trade", http.MethodPost, "/characters/char-1/store/sales/sale-1/trade", `{"depot_item_instance_id":"inst-barter"}`},
		{"store_change_name", http.MethodPost, "/characters/char-1/store/name", `{"store_name":"新看板"}`},
		{"store_change_wallpaper", http.MethodPost, "/characters/char-1/store/wallpaper", `{"wallpaper":"farm"}`},
		{"store_add_interior", http.MethodPost, "/characters/char-1/store/interiors", `{"furniture_id":"001"}`},
		{"store_rename_interior", http.MethodPut, "/characters/char-1/store/interiors/int-1/name", `{"name":"お気に入りの机"}`},
		{"store_clean_interiors", http.MethodDelete, "/characters/char-1/store/interiors", ``},
		// Park
		{"park_post_message", http.MethodPost, "/park/posts", `{"character_id":"char-1","content":"hello"}`},
		{"park_npc_talk", http.MethodPost, "/park/npc/talk", `{"character_id":"char-1"}`},
		{"park_npc_divinate", http.MethodPost, "/park/npc/divinate", `{"character_id":"char-1"}`},
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
		}
		mockShop := &stubShopService{
			purchaseFn: func(_ context.Context, _, _ string, _ int) (shop.PurchaseResult, error) {
				called = true
				return shop.PurchaseResult{}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, mockShop, apihttp.WithHome(mockHome))
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/shop/purchase", bytes.NewReader([]byte(`{"character_id":"char-1","item_definition_id":"herb","quantity":1}`)))
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
		if called {
			t.Error("expected shop purchase callback not to be invoked on sleep lookup failure")
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
		}
		mockShop := &stubShopService{
			purchaseFn: func(_ context.Context, _, _ string, _ int) (shop.PurchaseResult, error) {
				called = true
				return shop.PurchaseResult{}, nil
			},
		}
		h, err := apihttp.NewHandler(players, chars, &stubAdventureService{}, mockShop, apihttp.WithHome(mockHome))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		req := httptest.NewRequest(http.MethodPost, "/shop/purchase", bytes.NewReader([]byte(`{"character_id":"char-1","item_definition_id":"herb","quantity":1}`))).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer valid-session")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.Router().ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
		if called {
			t.Error("expected shop purchase callback not to be invoked on canceled context")
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
}

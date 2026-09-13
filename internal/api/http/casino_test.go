package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubCasinoService struct {
	getAccountFn            func(ctx context.Context, characterID string) (casino.Account, error)
	exchangeGoldToCoinsFn   func(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error)
	exchangeCoinsToGoldFn   func(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error)
	spinSlotFn              func(ctx context.Context, characterID string, bet int64) (casino.SpinResult, casino.Account, error)
	playHighLowFn           func(ctx context.Context, characterID string, betCoins int64, guess casino.GuessType) (casino.HighLowResult, casino.Account, error)
	playDoppelFn            func(ctx context.Context, characterID string, bet int64, poolSize int, playerMark casino.DoppelMark) (casino.DoppelResult, casino.Account, error)
	exchangePrizeFn         func(ctx context.Context, characterID string, costCoins int64, count int) (casino.PrizeExchangeResult, error)
	listRoomsFn             func(ctx context.Context) ([]casino.RoomDetail, error)
	createRoomFn            func(ctx context.Context, characterID string, req casino.CreateRoomRequest) (*casino.RoomDetail, error)
	getRoomDetailFn         func(ctx context.Context, roomID string, viewingCharID string) (*casino.RoomDetail, error)
	joinRoomFn              func(ctx context.Context, roomID string, characterID string, password string, fatigue int) (*casino.RoomDetail, error)
	spectateRoomFn          func(ctx context.Context, roomID string, characterID string, password string) (*casino.RoomDetail, error)
	leaveRoomFn             func(ctx context.Context, roomID string, characterID string) error
	kickMemberFn            func(ctx context.Context, roomID string, leaderID string, targetID string) error
	startIndianPokerFn      func(ctx context.Context, roomID string, leaderID string) (*casino.RoomDetail, error)
	playIndianPokerActionFn func(ctx context.Context, roomID string, characterID string, action casino.Action) (*casino.RoomDetail, error)
}

func (s *stubCasinoService) GetAccount(ctx context.Context, characterID string) (casino.Account, error) {
	if s.getAccountFn != nil {
		return s.getAccountFn(ctx, characterID)
	}
	return casino.Account{CharacterID: characterID, Coins: 100, UpdatedAt: time.Now()}, nil
}

func (s *stubCasinoService) ExchangeGoldToCoins(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error) {
	if s.exchangeGoldToCoinsFn != nil {
		return s.exchangeGoldToCoinsFn(ctx, characterID, coins)
	}
	return casino.Account{CharacterID: characterID, Coins: coins, UpdatedAt: time.Now()}, corecharacter.Character{ID: characterID}, nil
}

func (s *stubCasinoService) ExchangeCoinsToGold(ctx context.Context, characterID string, coins int64) (casino.Account, corecharacter.Character, error) {
	if s.exchangeCoinsToGoldFn != nil {
		return s.exchangeCoinsToGoldFn(ctx, characterID, coins)
	}
	return casino.Account{CharacterID: characterID, Coins: 0, UpdatedAt: time.Now()}, corecharacter.Character{ID: characterID, Money: int(coins * 20)}, nil
}

func (s *stubCasinoService) SpinSlot(ctx context.Context, characterID string, bet int64) (casino.SpinResult, casino.Account, error) {
	if s.spinSlotFn != nil {
		return s.spinSlotFn(ctx, characterID, bet)
	}
	return casino.SpinResult{BetCoins: bet, Reels: [3]casino.SlotSymbol{casino.SymbolCherry, casino.SymbolCherry, casino.SymbolCherry}, IsWin: true, Multiplier: 2, PayoutCoins: bet * 2}, casino.Account{CharacterID: characterID, Coins: bet * 2}, nil
}

func (s *stubCasinoService) PlayHighLow(ctx context.Context, characterID string, betCoins int64, guess casino.GuessType) (casino.HighLowResult, casino.Account, error) {
	if s.playHighLowFn != nil {
		return s.playHighLowFn(ctx, characterID, betCoins, guess)
	}
	return casino.HighLowResult{BetCoins: betCoins, Guess: guess, Outcome: casino.OutcomeWin, Multiplier: 2, PayoutCoins: betCoins * 2}, casino.Account{CharacterID: characterID, Coins: betCoins * 2}, nil
}

func (s *stubCasinoService) PlayDoppel(ctx context.Context, characterID string, bet int64, poolSize int, playerMark casino.DoppelMark) (casino.DoppelResult, casino.Account, error) {
	if s.playDoppelFn != nil {
		return s.playDoppelFn(ctx, characterID, bet, poolSize, playerMark)
	}
	return casino.DoppelResult{BetCoins: bet, PlayerMark: playerMark, IsWin: true, Multiplier: poolSize, PayoutCoins: bet * int64(poolSize)}, casino.Account{CharacterID: characterID, Coins: bet * int64(poolSize)}, nil
}

func (s *stubCasinoService) ExchangePrize(ctx context.Context, characterID string, costCoins int64, count int) (casino.PrizeExchangeResult, error) {
	if s.exchangePrizeFn != nil {
		return s.exchangePrizeFn(ctx, characterID, costCoins, count)
	}
	prize, _ := casino.GetPrizeByCost(costCoins)
	return casino.PrizeExchangeResult{
		Prize:              prize,
		Count:              count,
		TotalCostCoins:     costCoins * int64(count),
		TransferredToDepot: true,
		RemainingCoins:     100,
	}, nil
}

func (s *stubCasinoService) ListRooms(ctx context.Context) ([]casino.RoomDetail, error) {
	if s.listRoomsFn != nil {
		return s.listRoomsFn(ctx)
	}
	return []casino.RoomDetail{}, nil
}

func (s *stubCasinoService) CreateRoom(ctx context.Context, characterID string, req casino.CreateRoomRequest) (*casino.RoomDetail, error) {
	if s.createRoomFn != nil {
		return s.createRoomFn(ctx, characterID, req)
	}
	return &casino.RoomDetail{
		Room:    casino.Room{ID: "r1", Name: req.Name, GameType: req.GameType, LeaderCharacterID: characterID, Rate: req.Rate, Speed: req.Speed, MaxPlayers: req.MaxPlayers},
		Members: []casino.RoomMember{{RoomID: "r1", CharacterID: characterID, Action: "待機中"}},
	}, nil
}

func (s *stubCasinoService) GetRoomDetail(ctx context.Context, roomID string, viewingCharID string) (*casino.RoomDetail, error) {
	if s.getRoomDetailFn != nil {
		return s.getRoomDetailFn(ctx, roomID, viewingCharID)
	}
	return &casino.RoomDetail{
		Room:    casino.Room{ID: roomID, Name: "TestRoom", GameType: casino.GameTypeIndian, LeaderCharacterID: "c1", Rate: 10},
		Members: []casino.RoomMember{{RoomID: roomID, CharacterID: "c1", Card: -1}},
	}, nil
}

func (s *stubCasinoService) JoinRoom(ctx context.Context, roomID string, characterID string, password string, fatigue int) (*casino.RoomDetail, error) {
	if s.joinRoomFn != nil {
		return s.joinRoomFn(ctx, roomID, characterID, password, fatigue)
	}
	return s.GetRoomDetail(ctx, roomID, characterID)
}

func (s *stubCasinoService) SpectateRoom(ctx context.Context, roomID string, characterID string, password string) (*casino.RoomDetail, error) {
	if s.spectateRoomFn != nil {
		return s.spectateRoomFn(ctx, roomID, characterID, password)
	}
	return s.GetRoomDetail(ctx, roomID, characterID)
}

func (s *stubCasinoService) LeaveRoom(ctx context.Context, roomID string, characterID string) error {
	if s.leaveRoomFn != nil {
		return s.leaveRoomFn(ctx, roomID, characterID)
	}
	return nil
}

func (s *stubCasinoService) KickMember(ctx context.Context, roomID string, leaderID string, targetID string) error {
	if s.kickMemberFn != nil {
		return s.kickMemberFn(ctx, roomID, leaderID, targetID)
	}
	return nil
}

func (s *stubCasinoService) StartIndianPoker(ctx context.Context, roomID string, leaderID string) (*casino.RoomDetail, error) {
	if s.startIndianPokerFn != nil {
		return s.startIndianPokerFn(ctx, roomID, leaderID)
	}
	detail, _ := s.GetRoomDetail(ctx, roomID, leaderID)
	detail.Room.Round = 1
	return detail, nil
}

func (s *stubCasinoService) PlayIndianPokerAction(ctx context.Context, roomID string, characterID string, action casino.Action) (*casino.RoomDetail, error) {
	if s.playIndianPokerActionFn != nil {
		return s.playIndianPokerActionFn(ctx, roomID, characterID, action)
	}
	detail, _ := s.GetRoomDetail(ctx, roomID, characterID)
	return detail, nil
}

func TestCasinoEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Money: 1000}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	casService := &stubCasinoService{}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithCasino(casService),
	)
	router := h.Router()

	t.Run("GET /characters/{id}/casino - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/casino", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/exchange - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/exchange", `{"direction":"gold_to_coins","coins":10}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/slot - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/slot", `{"bet":10}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/highlow - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/highlow", `{"bet":10,"guess":"HIGH"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/doppel - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/doppel", `{"bet":10,"pool_size":4,"player_mark":"★"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /casino/prizes - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/casino/prizes", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/prizes/exchange - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/prizes/exchange", `{"cost_coins":100,"count":1}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /casino/rooms - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/casino/rooms", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/rooms - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/rooms", `{"name":"Room1","game_type":"indian","speed":12,"max_players":4,"rate":10}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /casino/rooms/{roomId} - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/casino/rooms/r1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/rooms/{roomId}/join - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/rooms/r1/join", `{}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/rooms/{roomId}/spectate - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/rooms/r1/spectate", `{}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/rooms/{roomId}/start - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/rooms/r1/start", `{}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/rooms/{roomId}/action - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/rooms/r1/action", `{"action":"call"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/rooms/{roomId}/leave - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/rooms/r1/leave", `{}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/casino/rooms/{roomId}/kick - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/casino/rooms/r1/kick", `{"target_character_id":"c2"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/gvg"
)

type stubGvGService struct {
	createRoomFn     func(ctx context.Context, creatorCharID string, req gvg.CreateRoomRequest) (gvg.RoomDetail, error)
	getRoomFn        func(ctx context.Context, roomID string) (gvg.RoomDetail, error)
	listRoomsFn      func(ctx context.Context) ([]gvg.RoomSummary, error)
	joinRoomFn       func(ctx context.Context, charID string, roomID string, password string) (gvg.RoomDetail, error)
	leaveRoomFn      func(ctx context.Context, charID string, roomID string) error
	startMatchFn     func(ctx context.Context, leaderID string, roomID string) (gvg.RoomDetail, error)
	advanceRoundFn   func(ctx context.Context, leaderID string, roomID string) (gvg.RoundResolution, error)
	getStandingFn    func(ctx context.Context, guildID string) (gvg.GvGStanding, error)
	getLeaderboardFn func(ctx context.Context, limit int) ([]gvg.GvGStanding, error)
}

func (s *stubGvGService) CreateRoom(ctx context.Context, creatorCharID string, req gvg.CreateRoomRequest) (gvg.RoomDetail, error) {
	if s.createRoomFn != nil {
		return s.createRoomFn(ctx, creatorCharID, req)
	}
	return gvg.RoomDetail{
		Room: gvg.GvGRoom{
			ID:                "gvg-room-1",
			Name:              req.Name,
			LeaderCharacterID: creatorCharID,
			LeaderName:        "Hero",
			PrizePool:         gvg.InitialPrizeGP,
			MaxMembers:        req.MaxMembers,
			TargetWins:        req.TargetWins,
			Status:            gvg.StatusRecruiting,
			CreatedAt:         time.Now(),
		},
		Members: []gvg.GvGMember{
			{RoomID: "gvg-room-1", CharacterID: creatorCharID, CharacterName: "Hero", GuildID: "g1", GuildColor: "#FF3333", IsLeader: true},
		},
	}, nil
}

func (s *stubGvGService) GetRoom(ctx context.Context, roomID string) (gvg.RoomDetail, error) {
	if s.getRoomFn != nil {
		return s.getRoomFn(ctx, roomID)
	}
	if roomID != "gvg-room-1" {
		return gvg.RoomDetail{}, gvg.ErrRoomNotFound
	}
	return gvg.RoomDetail{
		Room: gvg.GvGRoom{
			ID:                roomID,
			Name:              "Test GvG Room",
			LeaderCharacterID: "c1",
			LeaderName:        "Hero",
			Status:            gvg.StatusRecruiting,
		},
	}, nil
}

func (s *stubGvGService) ListRooms(ctx context.Context) ([]gvg.RoomSummary, error) {
	if s.listRoomsFn != nil {
		return s.listRoomsFn(ctx)
	}
	return []gvg.RoomSummary{
		{
			ID:                "gvg-room-1",
			Name:              "Test GvG Room",
			LeaderCharacterID: "c1",
			LeaderName:        "Hero",
			CurrentMembers:    1,
			MaxMembers:        8,
			PrizePool:         2,
			Status:            gvg.StatusRecruiting,
		},
	}, nil
}

func (s *stubGvGService) JoinRoom(ctx context.Context, charID string, roomID string, password string) (gvg.RoomDetail, error) {
	if s.joinRoomFn != nil {
		return s.joinRoomFn(ctx, charID, roomID, password)
	}
	if roomID != "gvg-room-1" {
		return gvg.RoomDetail{}, gvg.ErrRoomNotFound
	}
	return gvg.RoomDetail{
		Room: gvg.GvGRoom{ID: roomID, Status: gvg.StatusRecruiting, PrizePool: 3},
		Members: []gvg.GvGMember{
			{RoomID: roomID, CharacterID: "c1", IsLeader: true},
			{RoomID: roomID, CharacterID: charID, IsLeader: false},
		},
	}, nil
}

func (s *stubGvGService) LeaveRoom(ctx context.Context, charID string, roomID string) error {
	if s.leaveRoomFn != nil {
		return s.leaveRoomFn(ctx, charID, roomID)
	}
	if roomID != "gvg-room-1" {
		return gvg.ErrRoomNotFound
	}
	return nil
}

func (s *stubGvGService) StartMatch(ctx context.Context, leaderID string, roomID string) (gvg.RoomDetail, error) {
	if s.startMatchFn != nil {
		return s.startMatchFn(ctx, leaderID, roomID)
	}
	if leaderID != "c1" {
		return gvg.RoomDetail{}, gvg.ErrNotRoomLeader
	}
	return gvg.RoomDetail{
		Room: gvg.GvGRoom{ID: roomID, Status: gvg.StatusInProgress, Round: 1},
	}, nil
}

func (s *stubGvGService) AdvanceRound(ctx context.Context, leaderID string, roomID string) (gvg.RoundResolution, error) {
	if s.advanceRoundFn != nil {
		return s.advanceRoundFn(ctx, leaderID, roomID)
	}
	if leaderID != "c1" {
		return gvg.RoundResolution{}, gvg.ErrNotRoomLeader
	}
	return gvg.RoundResolution{
		Round:            1,
		Outcome:          "round_win",
		WinnerGuildID:    "g1",
		WinnerGuildName:  "Crimson",
		WinnerGuildColor: "#FF3333",
		GuildScores:      map[string]int{"g1": 1},
	}, nil
}

func (s *stubGvGService) GetStanding(ctx context.Context, guildID string) (gvg.GvGStanding, error) {
	if s.getStandingFn != nil {
		return s.getStandingFn(ctx, guildID)
	}
	if guildID != "g1" {
		return gvg.GvGStanding{}, gvg.ErrGuildNotFound
	}
	return gvg.GvGStanding{
		GuildID:       guildID,
		GuildName:     "Crimson",
		Wins:          5,
		BronzeMedals:  2,
		VictoryPoints: 50,
	}, nil
}

func (s *stubGvGService) GetLeaderboard(ctx context.Context, limit int) ([]gvg.GvGStanding, error) {
	if s.getLeaderboardFn != nil {
		return s.getLeaderboardFn(ctx, limit)
	}
	return []gvg.GvGStanding{
		{GuildID: "g1", GuildName: "Crimson", Wins: 10, VictoryPoints: 100},
	}, nil
}

func TestGvGEndpoints(t *testing.T) {
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

	stubGvG := &stubGvGService{}
	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithGvG(stubGvG),
	)
	router := h.Router()

	t.Run("POST /characters/{id}/gvg/rooms - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms", `{"name":"Clan War","max_members":8,"target_wins":2}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/gvg/rooms - friendly guild rejected", func(t *testing.T) {
		stubGvG.createRoomFn = func(_ context.Context, _ string, _ gvg.CreateRoomRequest) (gvg.RoomDetail, error) {
			return gvg.RoomDetail{}, gvg.ErrFriendlyGuildCannotBattle
		}
		defer func() { stubGvG.createRoomFn = nil }()

		req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms", `{"name":"Clan War"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 Unprocessable Entity, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /gvg/rooms - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/gvg/rooms", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /gvg/rooms/{room_id} - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/gvg/rooms/gvg-room-1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/gvg/rooms/{room_id}/join - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms/gvg-room-1/join", `{}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/gvg/rooms/{room_id}/leave - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms/gvg-room-1/leave", `{}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/gvg/rooms/{room_id}/start - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms/gvg-room-1/start", `{}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/gvg/rooms/{room_id}/advance - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms/gvg-room-1/advance", `{}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /gvg/standings/{guild_id} - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/gvg/standings/g1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET /gvg/leaderboard - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/gvg/leaderboard", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

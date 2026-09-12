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
	"github.com/witchcraze/party2re/internal/pvp"
)

type stubColosseumPvPService struct {
	createRoomFn   func(ctx context.Context, leaderID string, req pvp.CreateRoomRequest) (pvp.RoomDetail, error)
	getRoomFn      func(ctx context.Context, roomID string) (pvp.RoomDetail, error)
	listRoomsFn    func(ctx context.Context) ([]pvp.RoomSummary, error)
	joinRoomFn     func(ctx context.Context, characterID string, roomID string, password string) (pvp.RoomDetail, error)
	leaveRoomFn    func(ctx context.Context, characterID string, roomID string) error
	selectTeamFn   func(ctx context.Context, characterID string, roomID string, teamColor string) (pvp.RoomDetail, error)
	startMatchFn   func(ctx context.Context, leaderID string, roomID string) (pvp.RoomDetail, error)
	advanceRoundFn func(ctx context.Context, leaderID string, roomID string) (pvp.RoundResolution, error)
}

func (s *stubColosseumPvPService) CreateRoom(ctx context.Context, leaderID string, req pvp.CreateRoomRequest) (pvp.RoomDetail, error) {
	if s.createRoomFn != nil {
		return s.createRoomFn(ctx, leaderID, req)
	}
	return pvp.RoomDetail{
		Room: pvp.ColosseumRoom{
			ID:                "room-1",
			Name:              req.Name,
			LeaderCharacterID: leaderID,
			LeaderName:        "Hero",
			Bet:               req.Bet,
			MaxMembers:        req.MaxMembers,
			TargetWins:        req.TargetWins,
			Status:            pvp.StatusRecruiting,
			CreatedAt:         time.Now(),
		},
		Members: []pvp.RoomMember{
			{RoomID: "room-1", CharacterID: leaderID, CharacterName: "Hero", IsLeader: true},
		},
	}, nil
}

func (s *stubColosseumPvPService) GetRoom(ctx context.Context, roomID string) (pvp.RoomDetail, error) {
	if s.getRoomFn != nil {
		return s.getRoomFn(ctx, roomID)
	}
	if roomID != "room-1" {
		return pvp.RoomDetail{}, pvp.ErrRoomNotFound
	}
	return pvp.RoomDetail{
		Room: pvp.ColosseumRoom{
			ID:                roomID,
			Name:              "Test Room",
			LeaderCharacterID: "c1",
			LeaderName:        "Hero",
			Status:            pvp.StatusRecruiting,
		},
	}, nil
}

func (s *stubColosseumPvPService) ListRooms(ctx context.Context) ([]pvp.RoomSummary, error) {
	if s.listRoomsFn != nil {
		return s.listRoomsFn(ctx)
	}
	return []pvp.RoomSummary{
		{
			ID:                "room-1",
			Name:              "Test Room",
			LeaderCharacterID: "c1",
			LeaderName:        "Hero",
			CurrentMembers:    1,
			MaxMembers:        4,
			Bet:               100,
			Status:            pvp.StatusRecruiting,
		},
	}, nil
}

func (s *stubColosseumPvPService) JoinRoom(ctx context.Context, characterID string, roomID string, password string) (pvp.RoomDetail, error) {
	if s.joinRoomFn != nil {
		return s.joinRoomFn(ctx, characterID, roomID, password)
	}
	if roomID != "room-1" {
		return pvp.RoomDetail{}, pvp.ErrRoomNotFound
	}
	return pvp.RoomDetail{
		Room: pvp.ColosseumRoom{ID: roomID, Status: pvp.StatusRecruiting},
		Members: []pvp.RoomMember{
			{RoomID: roomID, CharacterID: "c1", IsLeader: true},
			{RoomID: roomID, CharacterID: characterID, IsLeader: false},
		},
	}, nil
}

func (s *stubColosseumPvPService) LeaveRoom(ctx context.Context, characterID string, roomID string) error {
	if s.leaveRoomFn != nil {
		return s.leaveRoomFn(ctx, characterID, roomID)
	}
	if roomID != "room-1" {
		return pvp.ErrRoomNotFound
	}
	return nil
}

func (s *stubColosseumPvPService) SelectTeam(ctx context.Context, characterID string, roomID string, teamColor string) (pvp.RoomDetail, error) {
	if s.selectTeamFn != nil {
		return s.selectTeamFn(ctx, characterID, roomID, teamColor)
	}
	if !pvp.IsValidTeamColor(teamColor) {
		return pvp.RoomDetail{}, pvp.ErrInvalidTeamColor
	}
	return pvp.RoomDetail{
		Room: pvp.ColosseumRoom{ID: roomID, Status: pvp.StatusRecruiting},
		Members: []pvp.RoomMember{
			{RoomID: roomID, CharacterID: characterID, TeamColor: teamColor},
		},
	}, nil
}

func (s *stubColosseumPvPService) StartMatch(ctx context.Context, leaderID string, roomID string) (pvp.RoomDetail, error) {
	if s.startMatchFn != nil {
		return s.startMatchFn(ctx, leaderID, roomID)
	}
	if leaderID != "c1" {
		return pvp.RoomDetail{}, pvp.ErrNotRoomLeader
	}
	return pvp.RoomDetail{
		Room: pvp.ColosseumRoom{ID: roomID, Status: pvp.StatusInProgress, Round: 1},
	}, nil
}

func (s *stubColosseumPvPService) AdvanceRound(ctx context.Context, leaderID string, roomID string) (pvp.RoundResolution, error) {
	if s.advanceRoundFn != nil {
		return s.advanceRoundFn(ctx, leaderID, roomID)
	}
	if leaderID != "c1" {
		return pvp.RoundResolution{}, pvp.ErrNotRoomLeader
	}
	return pvp.RoundResolution{
		Round:          1,
		Outcome:        "round_win",
		WinnerTeam:     pvp.ColorRed,
		WinnerTeamName: "Red",
		TeamScores:     map[string]int{pvp.ColorRed: 1},
	}, nil
}

func TestColosseumPvPEndpoints(t *testing.T) {
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

	stubPvP := &stubColosseumPvPService{}
	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithPvP(stubPvP),
	)
	router := h.Router()

	t.Run("POST /characters/{id}/pvp/rooms - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/pvp/rooms", `{"name":"Grand Arena","bet":100,"max_members":4,"target_wins":2}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/pvp/rooms - invalid bet bad request", func(t *testing.T) {
		stubPvP.createRoomFn = func(_ context.Context, _ string, _ pvp.CreateRoomRequest) (pvp.RoomDetail, error) {
			return pvp.RoomDetail{}, pvp.ErrInvalidBet
		}
		defer func() { stubPvP.createRoomFn = nil }()

		req := jsonRequest(t, http.MethodPost, "/characters/c1/pvp/rooms", `{"name":"Grand Arena","bet":0}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 BadRequest, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/pvp/rooms - insufficient funds 422", func(t *testing.T) {
		stubPvP.createRoomFn = func(_ context.Context, _ string, _ pvp.CreateRoomRequest) (pvp.RoomDetail, error) {
			return pvp.RoomDetail{}, pvp.ErrInsufficientBetFunds
		}
		defer func() { stubPvP.createRoomFn = nil }()

		req := jsonRequest(t, http.MethodPost, "/characters/c1/pvp/rooms", `{"name":"Grand Arena","bet":999999}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 UnprocessableEntity, got %d", rec.Code)
		}
	})

	t.Run("GET /pvp/rooms", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/pvp/rooms", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("GET /pvp/rooms/{room_id} - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/pvp/rooms/room-1", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("GET /pvp/rooms/{room_id} - not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/pvp/rooms/unknown", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 NotFound, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/pvp/rooms/{room_id}/join - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/pvp/rooms/room-1/join", `{"password":""}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("POST /characters/{id}/pvp/rooms/{room_id}/join - wrong password 403", func(t *testing.T) {
		stubPvP.joinRoomFn = func(_ context.Context, _ string, _ string, _ string) (pvp.RoomDetail, error) {
			return pvp.RoomDetail{}, pvp.ErrInvalidPassword
		}
		defer func() { stubPvP.joinRoomFn = nil }()

		req := jsonRequest(t, http.MethodPost, "/characters/c1/pvp/rooms/room-1/join", `{"password":"wrong"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/pvp/rooms/{room_id}/team - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/pvp/rooms/room-1/team", `{"team_color":"#FF3333"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/pvp/rooms/{room_id}/team - invalid color 400", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/pvp/rooms/room-1/team", `{"team_color":"invalid"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 BadRequest, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/pvp/rooms/{room_id}/leave - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/pvp/rooms/room-1/leave", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/pvp/rooms/{room_id}/start - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/pvp/rooms/room-1/start", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/pvp/rooms/{room_id}/start - not leader 403", func(t *testing.T) {
		stubPvP.startMatchFn = func(_ context.Context, _ string, _ string) (pvp.RoomDetail, error) {
			return pvp.RoomDetail{}, pvp.ErrNotRoomLeader
		}
		defer func() { stubPvP.startMatchFn = nil }()

		req := httptest.NewRequest(http.MethodPost, "/characters/c1/pvp/rooms/room-1/start", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/pvp/rooms/{room_id}/advance - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/characters/c1/pvp/rooms/room-1/advance", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})
}

package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type stubGvGService struct {
	createRoomFn     func(ctx context.Context, creatorCharID string, req gvg.CreateRoomRequest) (gvg.RoomDetail, error)
	getRoomFn        func(ctx context.Context, roomID string) (gvg.RoomDetail, error)
	listRoomsFn      func(ctx context.Context) ([]gvg.RoomSummary, error)
	joinRoomFn       func(ctx context.Context, charID string, roomID string, password string) (gvg.RoomDetail, error)
	leaveRoomFn      func(ctx context.Context, charID string, roomID string) error
	startMatchFn     func(ctx context.Context, leaderID string, roomID string) (gvg.RoomDetail, error)
	advanceRoundFn   func(ctx context.Context, leaderID string, roomID string, expectedRound int) (gvg.RoundResolution, error)
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

func (s *stubGvGService) AdvanceRound(ctx context.Context, leaderID string, roomID string, expectedRound int) (gvg.RoundResolution, error) {
	if s.advanceRoundFn != nil {
		return s.advanceRoundFn(ctx, leaderID, roomID, expectedRound)
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

	t.Run("POST /characters/{id}/gvg/rooms/{room_id}/join - guild member limit exceeded", func(t *testing.T) {
		stubGvG.joinRoomFn = func(_ context.Context, _ string, _ string, _ string) (gvg.RoomDetail, error) {
			return gvg.RoomDetail{}, gvg.ErrGuildMemberLimitExceeded
		}
		defer func() { stubGvG.joinRoomFn = nil }()

		req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms/gvg-room-1/join", `{}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 Unprocessable Entity, got %d: %s", rec.Code, rec.Body.String())
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
		req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms/gvg-room-1/advance", `{"expected_round":1}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("advance requires an explicit bounded integer", func(t *testing.T) {
		calls := 0
		stubGvG.advanceRoundFn = func(_ context.Context, actor, room string, round int) (gvg.RoundResolution, error) {
			calls++
			if actor != "c1" || room != "gvg-room-1" || (round != 1 && round != gvg.MaxRounds) {
				t.Fatalf("lost explicit request: %s %s %d", actor, room, round)
			}
			return gvg.RoundResolution{Round: round}, nil
		}
		defer func() { stubGvG.advanceRoundFn = nil }()
		for _, body := range []string{"", `{}`, `null`, `{"expected_round":null}`, `{"expected_round":"1"}`, `{"expected_round":true}`, `{"expected_round":1.5}`, `{"expected_round":0}`, `{"expected_round":-1}`, `{"expected_round":11}`, `{"expected_round":999999999999999999999}`} {
			req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms/gvg-room-1/advance", body)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != 400 || calls != 0 {
				t.Fatalf("invalid body %q: status=%d calls=%d", body, rec.Code, calls)
			}
		}
		for _, body := range []string{`{"expected_round":1}`, `{"expected_round":10}`} {
			req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms/gvg-room-1/advance", body)
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("valid body %q: %d %s", body, rec.Code, rec.Body.String())
			}
		}
		if calls != 2 {
			t.Fatalf("service calls=%d", calls)
		}
	})

	t.Run("advance maps domain failures", func(t *testing.T) {
		defer func() { stubGvG.advanceRoundFn = nil }()
		for _, tc := range []struct {
			err    error
			status int
		}{
			{gvg.ErrInvalidExpectedRound, 400}, {gvg.ErrRoundMismatch, 409},
			{gvg.ErrMatchNotInProgress, 422}, {gvg.ErrNotRoomLeader, 403},
			{gvg.ErrRoomNotFound, 404}, {errors.New("storage unavailable"), 500},
		} {
			stubGvG.advanceRoundFn = func(context.Context, string, string, int) (gvg.RoundResolution, error) {
				return gvg.RoundResolution{}, tc.err
			}
			req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms/gvg-room-1/advance", `{"expected_round":1}`)
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("%v: status=%d want=%d", tc.err, rec.Code, tc.status)
			}
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

func TestOpenAPIGvGExpectedRoundInput(t *testing.T) {
	var spec map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(apihttp.OpenAPISpec())))
	decoder.UseNumber()
	if err := decoder.Decode(&spec); err != nil {
		t.Fatal(err)
	}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	op := spec["paths"].(map[string]any)["/characters/{id}/gvg/rooms/{room_id}/advance"].(map[string]any)["post"].(map[string]any)
	body := op["requestBody"].(map[string]any)
	if body["required"] != true {
		t.Fatal("advance body is optional")
	}
	rest := body["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	var continuation map[string]any
	for _, raw := range schemas["CharacterActionRequest"].(map[string]any)["allOf"].([]any) {
		condition := raw.(map[string]any)
		if condition["if"].(map[string]any)["properties"].(map[string]any)["action"].(map[string]any)["const"] == "gvg_advance" {
			continuation = condition["then"].(map[string]any)["properties"].(map[string]any)["params"].(map[string]any)
		}
	}
	def, ok := playercontext.GetAction("gvg_advance")
	if !ok || !reflect.DeepEqual(def.RequiredParams, []string{"room_id", "expected_round"}) || continuation == nil || continuation["additionalProperties"] != false || !reflect.DeepEqual(continuation["required"], []any{"room_id", "expected_round"}) {
		t.Fatalf("continuation contract: %+v %v", def, continuation)
	}
	for _, schema := range []map[string]any{rest, continuation} {
		for _, tc := range []struct {
			body  string
			valid bool
		}{
			{`{"expected_round":1}`, true}, {`{"expected_round":10}`, true},
			{`{}`, false}, {`{"expected_round":null}`, false}, {`{"expected_round":"1"}`, false},
			{`{"expected_round":0}`, false}, {`{"expected_round":11}`, false}, {`{"expected_round":1.5}`, false},
		} {
			value := map[string]any{}
			decoder := json.NewDecoder(strings.NewReader(tc.body))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
			if schema["additionalProperties"] == false {
				value["room_id"] = "room"
			}
			err := checkOpenAPIPayload(schemas, schema, value)
			if (err == nil) != tc.valid {
				t.Fatalf("schema accepted/rejected %s: %v", tc.body, err)
			}
		}
	}
}

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
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/guild"
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

	t.Run("POST /characters/{id}/gvg/rooms - invalid room name bad request", func(t *testing.T) {
		stubGvG.createRoomFn = func(_ context.Context, _ string, _ gvg.CreateRoomRequest) (gvg.RoomDetail, error) {
			return gvg.RoomDetail{}, gvg.ErrInvalidRoomName
		}
		defer func() { stubGvG.createRoomFn = nil }()

		req := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms", `{"name":"bad room"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 BadRequest, got %d: %s", rec.Code, rec.Body.String())
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

type gvgHTTPStandingRepo struct{}

func (gvgHTTPStandingRepo) GetOrCreateStanding(context.Context, string) (gvg.GvGStanding, error) {
	return gvg.GvGStanding{}, nil
}
func (gvgHTTPStandingRepo) GetLeaderboard(context.Context, int) ([]gvg.GvGStanding, error) {
	return nil, nil
}
func (gvgHTTPStandingRepo) FindRoundTransition(context.Context, string) (gvg.RoundTransition, bool, error) {
	return gvg.RoundTransition{}, false, nil
}
func (gvgHTTPStandingRepo) CommitRoundTransition(_ context.Context, _ string, _ int, tr gvg.RoundTransition) (gvg.RoundTransition, error) {
	return tr, nil
}

type gvgHTTPGuildRepo struct {
	guilds     map[string]guild.Guild
	charGuilds map[string]string
}

func (r gvgHTTPGuildRepo) GetGuild(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	g, ok := r.guilds[guildID]
	if !ok {
		return guild.Guild{}, nil, guild.ErrGuildNotFound
	}
	return g, nil, nil
}

func (r gvgHTTPGuildRepo) GetGuildByCharacter(_ context.Context, charID string) (guild.Guild, guild.Member, error) {
	gid, ok := r.charGuilds[charID]
	if !ok {
		return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
	}
	g, ok := r.guilds[gid]
	if !ok {
		return guild.Guild{}, guild.Member{}, guild.ErrGuildNotFound
	}
	return g, guild.Member{GuildID: gid, CharacterID: charID}, nil
}

type gvgHTTPCharRepo struct {
	chars map[string]corecharacter.Character
}

func (r gvgHTTPCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r gvgHTTPCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

func (r gvgHTTPCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	r.chars[c.ID] = c
	return nil
}

type gvgHTTPBattleEngine struct{}

func (gvgHTTPBattleEngine) ResolvePartyBattle(corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return corebattle.PartyBattleResult{}, nil
}

func TestGvGHTTPPasswordPolicyAndNoVerifierLeak(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	chars := map[string]corecharacter.Character{
		"c1": {ID: "c1", PlayerID: "p1", Name: "Leader", JobID: "warrior", Level: 10, Stats: corecharacter.Stats{HP: 100, MaxHP: 100}},
		"c2": {ID: "c2", PlayerID: "p1", Name: "Joiner", JobID: "mage", Level: 10, Stats: corecharacter.Stats{HP: 100, MaxHP: 100}},
	}
	pService := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			c, ok := chars[id]
			if !ok {
				return corecharacter.Character{}, corecharacter.ErrNotFound
			}
			return c, nil
		},
	}
	guildRepo := gvgHTTPGuildRepo{
		guilds: map[string]guild.Guild{
			"g1": {ID: "g1", Name: "Crimson", Color: "#FF3333"},
			"g2": {ID: "g2", Name: "Azure", Color: "#33CCFF"},
		},
		charGuilds: map[string]string{"c1": "g1", "c2": "g2"},
	}
	svc, err := gvg.NewService(
		gvg.NewMemoryRoomRepository(),
		gvgHTTPStandingRepo{},
		guildRepo,
		gvgHTTPCharRepo{chars: chars},
		gvgHTTPBattleEngine{},
	)
	if err != nil {
		t.Fatal(err)
	}
	router := newTestHandler(t, pService, cService, &stubAdventureService{}, &stubShopService{}, apihttp.WithGvG(svc)).Router()

	assertNoSecret := func(label, body, hash string) {
		t.Helper()
		if strings.Contains(body, "password_hash") || strings.Contains(body, `"secret"`) || (hash != "" && strings.Contains(body, hash)) {
			t.Fatalf("%s exposed secret verifier in HTTP JSON: %s", label, body)
		}
	}

	createReq := jsonRequest(t, http.MethodPost, "/characters/c1/gvg/rooms", `{"name":"SecretWar","password":"secret"}`)
	createReq.Header.Set("Authorization", "Bearer valid-token")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var created gvg.RoomDetail
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	gotRoom, err := svc.GetRoom(context.Background(), created.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSecret("create", createRec.Body.String(), gotRoom.Room.PasswordHash)

	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/gvg/rooms", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Rooms []gvg.RoomSummary `json:"rooms"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Rooms) != 1 || !listResp.Rooms[0].HasPassword {
		t.Fatalf("expected HasPassword=true in listing, got %+v", listResp.Rooms)
	}
	assertNoSecret("list", listRec.Body.String(), gotRoom.Room.PasswordHash)

	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/gvg/rooms/"+created.Room.ID, nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRec.Code, getRec.Body.String())
	}
	assertNoSecret("get", getRec.Body.String(), gotRoom.Room.PasswordHash)

	wrongReq := jsonRequest(t, http.MethodPost, "/characters/c2/gvg/rooms/"+created.Room.ID+"/join", `{"password":"wrong"}`)
	wrongReq.Header.Set("Authorization", "Bearer valid-token")
	wrongRec := httptest.NewRecorder()
	router.ServeHTTP(wrongRec, wrongReq)
	if wrongRec.Code != http.StatusForbidden {
		t.Fatalf("wrong password status=%d body=%s", wrongRec.Code, wrongRec.Body.String())
	}

	okReq := jsonRequest(t, http.MethodPost, "/characters/c2/gvg/rooms/"+created.Room.ID+"/join", `{"password":"secret"}`)
	okReq.Header.Set("Authorization", "Bearer valid-token")
	okRec := httptest.NewRecorder()
	router.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("valid password status=%d body=%s", okRec.Code, okRec.Body.String())
	}
	assertNoSecret("join", okRec.Body.String(), gotRoom.Room.PasswordHash)
}

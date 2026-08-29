package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"

	"github.com/witchcraze/party2re/internal/adventure"
	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/shop"
)

// -------------------------------------------------------------------
// Stub implementations
// -------------------------------------------------------------------

type stubPlayerService struct {
	registerFn     func(ctx context.Context, username, password string) (coreplayer.Player, error)
	loginFn        func(ctx context.Context, username, password string) (coreplayer.Session, error)
	logoutFn       func(ctx context.Context, sessionID string) error
	authenticateFn func(ctx context.Context, sessionID string) (coreplayer.Player, error)
}

func (s *stubPlayerService) Register(ctx context.Context, username, password string) (coreplayer.Player, error) {
	return s.registerFn(ctx, username, password)
}
func (s *stubPlayerService) Login(ctx context.Context, username, password string) (coreplayer.Session, error) {
	return s.loginFn(ctx, username, password)
}
func (s *stubPlayerService) Logout(ctx context.Context, sessionID string) error {
	return s.logoutFn(ctx, sessionID)
}
func (s *stubPlayerService) Authenticate(ctx context.Context, sessionID string) (coreplayer.Player, error) {
	return s.authenticateFn(ctx, sessionID)
}

type stubCharacterService struct {
	createFn  func(ctx context.Context, playerID, name string) (corecharacter.Character, error)
	getFn     func(ctx context.Context, id string) (corecharacter.Character, error)
	rebirthFn func(ctx context.Context, id string) (corecharacter.Character, error)
}

func (s *stubCharacterService) Create(ctx context.Context, playerID, name string) (corecharacter.Character, error) {
	if s.createFn != nil {
		return s.createFn(ctx, playerID, name)
	}
	return corecharacter.Character{}, nil
}
func (s *stubCharacterService) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return corecharacter.Character{}, nil
}
func (s *stubCharacterService) Rebirth(ctx context.Context, id string) (corecharacter.Character, error) {
	if s.rebirthFn != nil {
		return s.rebirthFn(ctx, id)
	}
	return corecharacter.Character{}, nil
}

type stubAdventureService struct {
	startStageFn   func(ctx context.Context, characterID, stageID string) (adventure.Adventure, error)
	claimFn        func(ctx context.Context, id string) (adventure.Adventure, error)
	listHistoryFn  func(ctx context.Context, characterID string, limit, offset int) (adventure.PaginatedAdventures, error)
	getChronicleFn func(ctx context.Context, characterID string) (adventure.AdventureChronicle, error)
}

func (s *stubAdventureService) StartStage(ctx context.Context, characterID, stageID string) (adventure.Adventure, error) {
	if s.startStageFn != nil {
		return s.startStageFn(ctx, characterID, stageID)
	}
	return adventure.Adventure{}, nil
}

func (s *stubAdventureService) Claim(ctx context.Context, id string) (adventure.Adventure, error) {
	if s.claimFn != nil {
		return s.claimFn(ctx, id)
	}
	return adventure.Adventure{}, nil
}

func (s *stubAdventureService) ListHistory(ctx context.Context, characterID string, limit, offset int) (adventure.PaginatedAdventures, error) {
	if s.listHistoryFn != nil {
		return s.listHistoryFn(ctx, characterID, limit, offset)
	}
	return adventure.PaginatedAdventures{CharacterID: characterID}, nil
}

func (s *stubAdventureService) GetChronicle(ctx context.Context, characterID string) (adventure.AdventureChronicle, error) {
	if s.getChronicleFn != nil {
		return s.getChronicleFn(ctx, characterID)
	}
	return adventure.AdventureChronicle{CharacterID: characterID}, nil
}

type stubShopService struct {
	purchaseFn func(ctx context.Context, characterID, itemDefinitionID string, quantity int) (shop.PurchaseResult, error)
	sellFn     func(ctx context.Context, characterID, itemInstanceID string, quantity int) (shop.SaleResult, error)
}

func (s *stubShopService) Purchase(ctx context.Context, characterID, itemDefinitionID string, quantity int) (shop.PurchaseResult, error) {
	return s.purchaseFn(ctx, characterID, itemDefinitionID, quantity)
}
func (s *stubShopService) Sell(ctx context.Context, characterID, itemInstanceID string, quantity int) (shop.SaleResult, error) {
	return s.sellFn(ctx, characterID, itemInstanceID, quantity)
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func newTestHandler(t *testing.T, players apihttp.PlayerService, chars apihttp.CharacterService, advs apihttp.AdventureService, shops apihttp.ShopService, opts ...apihttp.Option) *apihttp.Handler {
	t.Helper()
	h, err := apihttp.NewHandler(players, chars, advs, shops, opts...)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func alwaysAuthPlayer(p coreplayer.Player) func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
	return func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
		return p, nil
	}
}

func bearerToken(id string) string { return "Bearer " + id }

// jsonRequest creates an HTTP request with Content-Type: application/json set.
// Use this for all endpoints that consume a JSON body.
func jsonRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	if body == "" {
		return httptest.NewRequest(method, target, nil)
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func decodeResponseBody(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, body)
	}
}

// -------------------------------------------------------------------
// NewHandler
// -------------------------------------------------------------------

func TestNewHandler_NilServices(t *testing.T) {
	p := &stubPlayerService{}
	c := &stubCharacterService{}
	a := &stubAdventureService{}
	s := &stubShopService{}

	if _, err := apihttp.NewHandler(nil, c, a, s); err == nil {
		t.Error("expected error for nil player service")
	}
	if _, err := apihttp.NewHandler(p, nil, a, s); err == nil {
		t.Error("expected error for nil character service")
	}
	if _, err := apihttp.NewHandler(p, c, nil, s); err == nil {
		t.Error("expected error for nil adventure service")
	}
	if _, err := apihttp.NewHandler(p, c, a, nil); err == nil {
		t.Error("expected error for nil shop service")
	}
}

// -------------------------------------------------------------------
// Health
// -------------------------------------------------------------------

func TestHandleHealth(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var body map[string]string
	decodeResponseBody(t, rec.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

// -------------------------------------------------------------------
// Player registration
// -------------------------------------------------------------------

func TestHandleRegisterPlayer_Success(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	player := coreplayer.Player{ID: "p1", Username: "alice", CreatedAt: now}
	ps := &stubPlayerService{
		registerFn: func(_ context.Context, username, password string) (coreplayer.Player, error) {
			return player, nil
		},
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/players", `{"username":"alice","password":"secret"}`)
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var resp map[string]any
	decodeResponseBody(t, rec.Body.Bytes(), &resp)
	if resp["id"] != "p1" {
		t.Errorf("id = %v, want p1", resp["id"])
	}
	if resp["username"] != "alice" {
		t.Errorf("username = %v, want alice", resp["username"])
	}
}

func TestHandleRegisterPlayer_ServiceError(t *testing.T) {
	ps := &stubPlayerService{
		registerFn: func(_ context.Context, username, password string) (coreplayer.Player, error) {
			return coreplayer.Player{}, errors.New("username taken")
		},
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/players", `{"username":"alice","password":"secret"}`)
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestHandleRegisterPlayer_BadJSON(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/players", "not-json")
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleRegisterPlayer_WrongContentType(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/players", strings.NewReader(`{"username":"alice","password":"secret"}`))
	// Intentionally no Content-Type header set.
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want %d (missing Content-Type should be rejected)", rec.Code, http.StatusUnsupportedMediaType)
	}
}

func TestHandleRegisterPlayer_OversizedBody(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	// Generate a body larger than 64 KiB.
	huge := `{"username":"` + strings.Repeat("a", 64*1024+1) + `","password":"x"}`
	req := jsonRequest(t, http.MethodPost, "/players", huge)
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (oversized body should be rejected)", rec.Code, http.StatusBadRequest)
	}
}

// -------------------------------------------------------------------
// Login
// -------------------------------------------------------------------

func TestHandleLogin_Success(t *testing.T) {
	expires := time.Now().Add(24 * time.Hour).UTC()
	ps := &stubPlayerService{
		loginFn: func(_ context.Context, username, password string) (coreplayer.Session, error) {
			return coreplayer.Session{ID: "sess1", PlayerID: "p1", ExpiresAt: expires}, nil
		},
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/sessions", `{"username":"alice","password":"secret"}`)
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	var resp map[string]any
	decodeResponseBody(t, rec.Body.Bytes(), &resp)
	if resp["id"] != "sess1" {
		t.Errorf("id = %v, want sess1", resp["id"])
	}
}

func TestHandleLogin_AuthFailed(t *testing.T) {
	ps := &stubPlayerService{
		loginFn: func(_ context.Context, username, password string) (coreplayer.Session, error) {
			return coreplayer.Session{}, coreplayer.ErrAuthentication
		},
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/sessions", `{"username":"alice","password":"wrong"}`)
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// -------------------------------------------------------------------
// Logout
// -------------------------------------------------------------------

func TestHandleLogout_Success(t *testing.T) {
	ps := &stubPlayerService{
		logoutFn: func(_ context.Context, sessionID string) error { return nil },
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/sessions", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestHandleLogout_MissingSession(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/sessions", nil)
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// -------------------------------------------------------------------
// Character creation
// -------------------------------------------------------------------

func TestHandleCreateCharacter_Success(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "alice"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", JobID: "starter", Gender: "unspecified", Level: 1}
	var createdPlayerID string
	ps := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cs := &stubCharacterService{
		createFn: func(_ context.Context, playerID, name string) (corecharacter.Character, error) {
			createdPlayerID = playerID
			return char, nil
		},
	}
	h := newTestHandler(t, ps, cs, &stubAdventureService{}, &stubShopService{})

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/characters", `{"name":"Hero"}`)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d\nbody: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if createdPlayerID != "p1" {
		t.Errorf("createdPlayerID = %q, want p1", createdPlayerID)
	}
	var resp map[string]any
	decodeResponseBody(t, rec.Body.Bytes(), &resp)
	if resp["id"] != "c1" {
		t.Errorf("id = %v, want c1", resp["id"])
	}
	if resp["player_id"] != "p1" {
		t.Errorf("player_id = %v, want p1", resp["player_id"])
	}
}

func TestHandleCreateCharacter_Unauthenticated(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/characters", `{"name":"Hero"}`)
	// No Authorization header.
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateCharacter_InvalidSession(t *testing.T) {
	ps := &stubPlayerService{
		authenticateFn: func(_ context.Context, sessionID string) (coreplayer.Player, error) {
			return coreplayer.Player{}, coreplayer.ErrAuthentication
		},
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/characters", `{"name":"Hero"}`)
	req.Header.Set("Authorization", bearerToken("bad-sess"))
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// -------------------------------------------------------------------
// Character get
// -------------------------------------------------------------------

func TestHandleGetCharacter_Success(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 5}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	h := newTestHandler(t, ps, cs, &stubAdventureService{}, &stubShopService{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	decodeResponseBody(t, rec.Body.Bytes(), &resp)
	if resp["id"] != "c1" {
		t.Errorf("id = %v, want c1", resp["id"])
	}
	if resp["player_id"] != "p1" {
		t.Errorf("player_id = %v, want p1", resp["player_id"])
	}
	if resp["level"] != float64(5) {
		t.Errorf("level = %v, want 5", resp["level"])
	}
}

func TestHandleGetCharacter_Forbidden_DifferentPlayer(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "other_player", Name: "Hero", Level: 5}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	h := newTestHandler(t, ps, cs, &stubAdventureService{}, &stubShopService{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleGetCharacter_NotFound(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	h := newTestHandler(t, ps, cs, &stubAdventureService{}, &stubShopService{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/characters/missing", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// -------------------------------------------------------------------
// Adventure
// -------------------------------------------------------------------

func TestHandleStartAdventure_Success(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1"}
	now := time.Now().UTC()
	adv := adventure.Adventure{
		ID:          "adv1",
		CharacterID: "c1",
		StageID:     "stage-01",
		StartedAt:   now,
		AvailableAt: now.Add(time.Hour),
	}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	as := &stubAdventureService{
		startStageFn: func(_ context.Context, charID, stageID string) (adventure.Adventure, error) {
			return adv, nil
		},
	}
	h := newTestHandler(t, ps, cs, as, &stubShopService{})

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/adventures", `{"character_id":"c1","stage_id":"stage-01"}`)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d\nbody: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp map[string]any
	decodeResponseBody(t, rec.Body.Bytes(), &resp)
	if resp["id"] != "adv1" {
		t.Errorf("id = %v, want adv1", resp["id"])
	}
}

func TestHandleStartAdventure_Forbidden_DifferentPlayer(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "other_player"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	as := &stubAdventureService{
		startStageFn: func(_ context.Context, charID, stageID string) (adventure.Adventure, error) {
			return adventure.Adventure{}, nil
		},
	}
	h := newTestHandler(t, ps, cs, as, &stubShopService{})

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/adventures", `{"character_id":"c1","stage_id":"stage-01"}`)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleStartAdventure_LevelRequirementNotMet(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	as := &stubAdventureService{
		startStageFn: func(_ context.Context, charID, stageID string) (adventure.Adventure, error) {
			return adventure.Adventure{}, adventure.ErrLevelRequirementNotMet
		},
	}
	h := newTestHandler(t, ps, cs, as, &stubShopService{})
	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/adventures", `{"character_id":"c1","stage_id":"stage-99"}`)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandleClaimAdventure_Success(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	adv := adventure.Adventure{ID: "adv1", Claimed: true, ExperienceReward: 20}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	as := &stubAdventureService{
		claimFn: func(_ context.Context, id string) (adventure.Adventure, error) { return adv, nil },
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, as, &stubShopService{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/adventures/adv1/claim", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]any
	decodeResponseBody(t, rec.Body.Bytes(), &resp)
	if resp["experience_reward"] != float64(20) {
		t.Errorf("experience_reward = %v, want 20", resp["experience_reward"])
	}
}

func TestHandleClaimAdventure_NotReady(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	as := &stubAdventureService{
		claimFn: func(_ context.Context, id string) (adventure.Adventure, error) {
			return adventure.Adventure{}, adventure.ErrNotReady
		},
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, as, &stubShopService{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/adventures/adv1/claim", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleClaimAdventure_NotFound(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	as := &stubAdventureService{
		claimFn: func(_ context.Context, id string) (adventure.Adventure, error) {
			return adventure.Adventure{}, adventure.ErrNotFound
		},
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, as, &stubShopService{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/adventures/missing/claim", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// -------------------------------------------------------------------
// Shop purchase
// -------------------------------------------------------------------

func TestHandlePurchase_Success(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1"}
	instance, _ := coreitem.NewInstance("sword-01", 1)
	result := shop.PurchaseResult{
		Character:    corecharacter.Character{ID: "c1", PlayerID: "p1"},
		ItemInstance: instance,
		TotalPrice:   100,
	}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	ss := &stubShopService{
		purchaseFn: func(_ context.Context, charID, itemID string, qty int) (shop.PurchaseResult, error) {
			return result, nil
		},
	}
	h := newTestHandler(t, ps, cs, &stubAdventureService{}, ss)

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/shop/purchase", `{"character_id":"c1","item_definition_id":"sword-01","quantity":1}`)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]any
	decodeResponseBody(t, rec.Body.Bytes(), &resp)
	if resp["total_cost"] != float64(100) {
		t.Errorf("total_cost = %v, want 100", resp["total_cost"])
	}
}

func TestHandlePurchase_Forbidden_DifferentPlayer(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "other_player"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	ss := &stubShopService{
		purchaseFn: func(_ context.Context, charID, itemID string, qty int) (shop.PurchaseResult, error) {
			return shop.PurchaseResult{}, nil
		},
	}
	h := newTestHandler(t, ps, cs, &stubAdventureService{}, ss)

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/shop/purchase", `{"character_id":"c1","item_definition_id":"sword-01","quantity":1}`)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestHandlePurchase_ServiceError(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	ss := &stubShopService{
		purchaseFn: func(_ context.Context, charID, itemID string, qty int) (shop.PurchaseResult, error) {
			return shop.PurchaseResult{}, errors.New("insufficient funds")
		},
	}
	h := newTestHandler(t, ps, cs, &stubAdventureService{}, ss)

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/shop/purchase", `{"character_id":"c1","item_definition_id":"sword-01","quantity":1}`)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

// -------------------------------------------------------------------
// Shop sell
// -------------------------------------------------------------------

func TestHandleSell_Success(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1"}
	instance, _ := coreitem.NewInstance("sword-01", 1)
	result := shop.SaleResult{
		Character:    corecharacter.Character{ID: "c1", PlayerID: "p1"},
		SoldInstance: instance,
		TotalPayout:  50,
	}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	ss := &stubShopService{
		sellFn: func(_ context.Context, charID, instID string, qty int) (shop.SaleResult, error) {
			return result, nil
		},
	}
	h := newTestHandler(t, ps, cs, &stubAdventureService{}, ss)

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/shop/sell", `{"character_id":"c1","item_instance_id":"inst-1","quantity":1}`)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d\nbody: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp map[string]any
	decodeResponseBody(t, rec.Body.Bytes(), &resp)
	if resp["total_payout"] != float64(50) {
		t.Errorf("total_payout = %v, want 50", resp["total_payout"])
	}
}

func TestHandleSell_Forbidden_DifferentPlayer(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "other_player"}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	ss := &stubShopService{
		sellFn: func(_ context.Context, charID, instID string, qty int) (shop.SaleResult, error) {
			return shop.SaleResult{}, nil
		},
	}
	h := newTestHandler(t, ps, cs, &stubAdventureService{}, ss)

	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/shop/sell", `{"character_id":"c1","item_instance_id":"inst-1","quantity":1}`)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// -------------------------------------------------------------------
// Error response format
// -------------------------------------------------------------------

func TestErrorResponseJSON(t *testing.T) {
	// Verify error responses always contain an "error" field.
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	// Missing Content-Type triggers 415, which must still have the error key.
	req := httptest.NewRequest(http.MethodPost, "/players", bytes.NewReader(nil))
	h.Router().ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Errorf("expected an error status, got %d", rec.Code)
	}
	var resp map[string]any
	decodeResponseBody(t, rec.Body.Bytes(), &resp)
	if _, ok := resp["error"]; !ok {
		t.Error("error response missing 'error' key")
	}
}

// -------------------------------------------------------------------
// Security headers middleware
// -------------------------------------------------------------------

func assertSecurityHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	expectedHeaders := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Content-Security-Policy": "default-src 'none'",
	}
	for header, expectedVal := range expectedHeaders {
		got := rec.Header().Get(header)
		if got != expectedVal {
			t.Errorf("Header %q = %q, want %q", header, got, expectedVal)
		}
	}
}

func TestSecurityHeaders_HealthEndpoint(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	assertSecurityHeaders(t, rec)
}

func TestSecurityHeaders_PostPlayers(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "alice"}
	ps := &stubPlayerService{
		registerFn: func(_ context.Context, username, password string) (coreplayer.Player, error) {
			return player, nil
		},
	}
	h := newTestHandler(t, ps, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := jsonRequest(t, http.MethodPost, "/players", `{"username":"alice","password":"password123"}`)
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	assertSecurityHeaders(t, rec)
}

func TestSecurityHeaders_AuthenticatedEndpoint(t *testing.T) {
	player := coreplayer.Player{ID: "p1"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 1}
	ps := &stubPlayerService{authenticateFn: alwaysAuthPlayer(player)}
	cs := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) { return char, nil },
	}
	h := newTestHandler(t, ps, cs, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/characters/c1", nil)
	req.Header.Set("Authorization", bearerToken("sess1"))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	assertSecurityHeaders(t, rec)
}

func TestSecurityHeaders_ErrorResponse(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/players", bytes.NewReader(nil))
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnsupportedMediaType)
	}
	assertSecurityHeaders(t, rec)
}

// -------------------------------------------------------------------
// CORS middleware
// -------------------------------------------------------------------

func TestCORS_AllowedOrigin(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithAllowedOrigins("https://app.party2.game", "http://localhost:3000"),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://app.party2.game")
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.party2.game" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://app.party2.game")
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want %q", got, "Origin")
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithAllowedOrigins("https://app.party2.game"),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://unauthorized.evil.com")
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORS_EmptyAllowedOrigins(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://app.party2.game")
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORS_PreflightOptions_Allowed(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithAllowedOrigins("https://app.party2.game"),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/characters", nil)
	req.Header.Set("Origin", "https://app.party2.game")
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.party2.game" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://app.party2.game")
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, DELETE" {
		t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, "GET, POST, DELETE")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, "Content-Type, Authorization")
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Errorf("Access-Control-Max-Age = %q, want %q", got, "86400")
	}
	assertSecurityHeaders(t, rec)
}

func TestCORS_PreflightOptions_Disallowed(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithAllowedOrigins("https://app.party2.game"),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/characters", nil)
	req.Header.Set("Origin", "https://unauthorized.evil.com")
	h.Router().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORS_WildcardNeverEmitted(t *testing.T) {
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithAllowedOrigins("*", "https://app.party2.game"),
	)

	// Request with Origin: *
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/health", nil)
	req1.Header.Set("Origin", "*")
	h.Router().ServeHTTP(rec1, req1)
	if got := rec1.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for Origin: *", got)
	}

	// Request with unknown origin
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/health", nil)
	req2.Header.Set("Origin", "https://evil.com")
	h.Router().ServeHTTP(rec2, req2)
	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for unknown origin", got)
	}
}

func TestCORS_ParseCORSOrigins(t *testing.T) {
	input := " https://app.party2.game , https://localhost:3000 , * , , http://127.0.0.1:8080 "
	got := apihttp.ParseCORSOrigins(input)
	want := []string{"https://app.party2.game", "https://localhost:3000", "http://127.0.0.1:8080"}

	if len(got) != len(want) {
		t.Fatalf("ParseCORSOrigins() len = %d, want %d; got = %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ParseCORSOrigins()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCORS_WithAllowedOriginsFromEnv(t *testing.T) {
	t.Setenv("TEST_CORS_ORIGINS", "https://env.party2.game, https://localhost:4000")
	h := newTestHandler(t, &stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithAllowedOriginsFromEnv("TEST_CORS_ORIGINS"),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://env.party2.game")
	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://env.party2.game" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://env.party2.game")
	}
}

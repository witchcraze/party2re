package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/player"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

type memoryPlayerRepo struct {
	players map[string]coreplayer.Player
}

func (m *memoryPlayerRepo) Save(_ context.Context, p coreplayer.Player) error {
	m.players[p.ID] = p
	return nil
}

func (m *memoryPlayerRepo) FindByUsername(_ context.Context, username string) (coreplayer.Player, error) {
	for _, p := range m.players {
		if p.Username == username {
			return p, nil
		}
	}
	return coreplayer.Player{}, coreplayer.ErrAuthentication
}

func (m *memoryPlayerRepo) FindByID(_ context.Context, id string) (coreplayer.Player, error) {
	p, ok := m.players[id]
	if !ok {
		return coreplayer.Player{}, coreplayer.ErrAuthentication
	}
	return p, nil
}

func (m *memoryPlayerRepo) Delete(_ context.Context, id string) error {
	delete(m.players, id)
	return nil
}

func TestValkeySessionAuth_EndToEndFlow(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not set, skipping real Valkey test")
	}

	client, err := vk.NewClient()
	if err != nil {
		t.Fatalf("connect valkey: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	testSessionPrefix := "party2:test:auth_e2e:session:"
	testPlayerPrefix := "party2:test:auth_e2e:player:sessions:"

	valkeySessionRepo := player.NewValkeySessionRepository(client,
		player.WithSessionKeyPrefix(testSessionPrefix),
		player.WithPlayerSessionsKeyPrefix(testPlayerPrefix),
	)

	now := time.Now().UTC()
	p, err := coreplayer.New("e2e_hero", "securepass123", now)
	if err != nil {
		t.Fatalf("create coreplayer: %v", err)
	}

	playerRepo := &memoryPlayerRepo{
		players: map[string]coreplayer.Player{
			p.ID: p,
		},
	}

	charRepo := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == "hero-char-1" {
				return corecharacter.Character{
					ID:       "hero-char-1",
					PlayerID: p.ID,
					Name:     "HeroChar",
				}, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	playerService, err := player.NewService(playerRepo, valkeySessionRepo)
	if err != nil {
		t.Fatalf("create player service: %v", err)
	}

	handler, err := apihttp.NewHandler(playerService, charRepo, &stubAdventureService{}, &stubShopService{})
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	router := handler.Router()

	defer func() {
		_ = valkeySessionRepo.DeleteByPlayerID(ctx, p.ID)
	}()

	// 1. Login via POST /sessions
	loginPayload, _ := json.Marshal(map[string]string{
		"username": "e2e_hero",
		"password": "securepass123",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusCreated {
		t.Fatalf("login failed: code %d, body %s", loginRec.Code, loginRec.Body.String())
	}

	var sessionResp struct {
		ID       string `json:"id"`
		PlayerID string `json:"player_id"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &sessionResp); err != nil {
		t.Fatalf("unmarshal session response: %v", err)
	}
	if sessionResp.ID == "" || sessionResp.PlayerID != p.ID {
		t.Fatalf("unexpected session response: %+v", sessionResp)
	}

	// 2. Direct Valkey verification: session key exists with 7-day TTL
	ttl, err := client.Do(ctx, client.B().Ttl().Key(testSessionPrefix+sessionResp.ID).Build()).AsInt64()
	if err != nil || ttl <= 0 {
		t.Fatalf("expected positive TTL in Valkey, got %d, err: %v", ttl, err)
	}

	// 3. Authenticated request: GET /characters/hero-char-1
	charReq := httptest.NewRequest(http.MethodGet, "/characters/hero-char-1", nil)
	charReq.Header.Set("Authorization", "Bearer "+sessionResp.ID)
	charRec := httptest.NewRecorder()
	router.ServeHTTP(charRec, charReq)

	if charRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for authenticated character fetch, got %d, body %s", charRec.Code, charRec.Body.String())
	}

	// 4. Logout via DELETE /sessions
	logoutReq := httptest.NewRequest(http.MethodDelete, "/sessions", nil)
	logoutReq.Header.Set("Authorization", "Bearer "+sessionResp.ID)
	logoutRec := httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for logout, got %d", logoutRec.Code)
	}

	// 5. Subsequent request with logged-out session returns 401 Unauthorized
	charReq2 := httptest.NewRequest(http.MethodGet, "/characters/hero-char-1", nil)
	charReq2.Header.Set("Authorization", "Bearer "+sessionResp.ID)
	charRec2 := httptest.NewRecorder()
	router.ServeHTTP(charRec2, charReq2)

	if charRec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for revoked session, got %d", charRec2.Code)
	}

	// 6. Login again, then delete player account
	loginReq2 := httptest.NewRequest(http.MethodPost, "/sessions", bytes.NewReader(loginPayload))
	loginReq2.Header.Set("Content-Type", "application/json")
	loginRec2 := httptest.NewRecorder()
	router.ServeHTTP(loginRec2, loginReq2)
	if loginRec2.Code != http.StatusCreated {
		t.Fatalf("second login failed: %d", loginRec2.Code)
	}
	var sessionResp2 struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(loginRec2.Body.Bytes(), &sessionResp2)

	deleteAccountPayload, _ := json.Marshal(map[string]string{
		"password": "securepass123",
	})
	deleteReq := httptest.NewRequest(http.MethodDelete, "/players/me", bytes.NewReader(deleteAccountPayload))
	deleteReq.Header.Set("Authorization", "Bearer "+sessionResp2.ID)
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for account deletion, got %d, body %s", deleteRec.Code, deleteRec.Body.String())
	}

	// 7. Verify session in Valkey was cleaned up by DeleteAccount
	exists, _ := client.Do(ctx, client.B().Exists().Key(testSessionPrefix+sessionResp2.ID).Build()).AsInt64()
	if exists != 0 {
		t.Errorf("expected session token key deleted after account deletion, exists=%d", exists)
	}
}

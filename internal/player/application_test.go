package player

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/logging"
)

type playerRepositoryStub struct {
	value   coreplayer.Player
	saveErr error
}

func (r *playerRepositoryStub) Save(context.Context, coreplayer.Player) error {
	return r.saveErr
}

func (r *playerRepositoryStub) FindByUsername(context.Context, string) (coreplayer.Player, error) {
	if r.value.ID == "" {
		return coreplayer.Player{}, errors.New("player not found")
	}
	return r.value, nil
}

func (r *playerRepositoryStub) FindByID(context.Context, string) (coreplayer.Player, error) {
	return r.value, nil
}

func (r *playerRepositoryStub) Delete(context.Context, string) error {
	r.value = coreplayer.Player{}
	return nil
}

type sessionRepositoryStub struct {
	value     coreplayer.Session
	saveErr   error
	revokeErr error
}

func (r *sessionRepositoryStub) Save(_ context.Context, s coreplayer.Session) error {
	if r.saveErr == nil {
		r.value = s
	}
	return r.saveErr
}

func (r *sessionRepositoryStub) FindByID(context.Context, string) (coreplayer.Session, error) {
	if r.value.ID == "" {
		return coreplayer.Session{}, errors.New("session not found")
	}
	return r.value, nil
}

func (r *sessionRepositoryStub) Revoke(context.Context, string, time.Time) error {
	return r.revokeErr
}

func (r *sessionRepositoryStub) DeleteByPlayerID(context.Context, string) error {
	r.value = coreplayer.Session{}
	return nil
}

func TestRegisterLogsStructuredSafeOperation(t *testing.T) {
	var output bytes.Buffer
	logger := logging.NewJSON(&output)
	service, err := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{}, WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	ctx := logging.WithCorrelationID(context.Background(), "request-123")
	if _, err := service.Register(ctx, "alice", "password-value"); err != nil {
		t.Fatal(err)
	}

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("decode log output: %v", err)
	}
	if record["operation"] != "player.register" {
		t.Fatalf("operation = %#v", record["operation"])
	}
	if record["correlation_id"] != "request-123" {
		t.Fatalf("correlation_id = %#v", record["correlation_id"])
	}
	if record["username"] != "alice" {
		t.Fatalf("username = %#v", record["username"])
	}
	if strings.Contains(output.String(), "password-value") {
		t.Fatalf("log contains password: %s", output.String())
	}
}

func TestLoginFailureDoesNotLogAuthenticationValues(t *testing.T) {
	value, err := coreplayer.New("alice", "password-value", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	logger := logging.NewJSON(&output)
	service, err := NewService(&playerRepositoryStub{value: value}, &sessionRepositoryStub{}, WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Login(context.Background(), "alice", "wrong-password"); !errors.Is(err, coreplayer.ErrAuthentication) {
		t.Fatalf("login error = %v", err)
	}
	logOutput := output.String()
	if !strings.Contains(logOutput, `"operation":"player.login"`) {
		t.Fatalf("log does not contain operation: %s", logOutput)
	}
	for _, secret := range []string{"wrong-password", "password-value"} {
		if strings.Contains(logOutput, secret) {
			t.Fatalf("log contains authentication value %q: %s", secret, logOutput)
		}
	}
	if strings.Contains(logOutput, "session") && strings.Contains(logOutput, "session_id") {
		t.Fatalf("log contains session field: %s", logOutput)
	}
}

func TestLogoutDoesNotLogSessionValue(t *testing.T) {
	var output bytes.Buffer
	logger := logging.NewJSON(&output)
	service, err := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{}, WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	if err := service.Logout(context.Background(), "session-value"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "session-value") {
		t.Fatalf("log contains session value: %s", output.String())
	}
}

func TestLoginSuccessCreatesActiveSession(t *testing.T) {
	player, err := coreplayer.New("bob", "correct-password", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	sessions := &sessionRepositoryStub{}
	service, err := NewService(&playerRepositoryStub{value: player}, sessions)
	if err != nil {
		t.Fatal(err)
	}

	session, err := service.Login(context.Background(), "bob", "correct-password")
	if err != nil {
		t.Fatalf("Login() unexpected error: %v", err)
	}
	if session.ID == "" || session.PlayerID != player.ID {
		t.Fatalf("Login() session = %#v", session)
	}
}

func TestAuthenticateReturnsPlayerForActiveSession(t *testing.T) {
	player, err := coreplayer.New("carol", "pass", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	sessions := &sessionRepositoryStub{}
	service, err := NewService(&playerRepositoryStub{value: player}, sessions)
	if err != nil {
		t.Fatal(err)
	}

	// Login to create a real active session.
	session, err := service.Login(context.Background(), "carol", "pass")
	if err != nil {
		t.Fatal(err)
	}

	got, err := service.Authenticate(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if got.ID != player.ID {
		t.Fatalf("Authenticate() player = %#v, want %#v", got, player)
	}
}

func TestAuthenticateRejectsExpiredSession(t *testing.T) {
	player, _ := coreplayer.New("dave", "pass", time.Now())
	sessions := &sessionRepositoryStub{}
	service, err := NewService(&playerRepositoryStub{value: player}, sessions)
	if err != nil {
		t.Fatal(err)
	}

	// Manually insert an already-expired session by manipulating stub.
	expiredSession, _ := coreplayer.NewSession(player.ID, time.Now().Add(-8*24*time.Hour), SessionDuration)
	sessions.value = expiredSession

	if _, err := service.Authenticate(context.Background(), expiredSession.ID); !errors.Is(err, coreplayer.ErrAuthentication) {
		t.Fatalf("Authenticate(expired) error = %v, want %v", err, coreplayer.ErrAuthentication)
	}
}

type charServiceStub struct {
	deletedChars []string
}

func (c *charServiceStub) FindByPlayerID(ctx context.Context, playerID string) ([]coreplayer.Player, error) {
	return nil, nil
}

type charStubService struct {
	deletedChars []string
}

func (c *charStubService) FindByPlayerID(ctx context.Context, playerID string) ([]coreplayer.Player, error) {
	return nil, nil
}

func TestDeleteAccount(t *testing.T) {
	player, _ := coreplayer.New("eve", "correctpassword", time.Now())
	players := &playerRepositoryStub{value: player}
	sessions := &sessionRepositoryStub{
		value: coreplayer.Session{ID: "sess-eve", PlayerID: player.ID},
	}

	service, err := NewService(players, sessions)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("rejects deletion with invalid password", func(t *testing.T) {
		err := service.DeleteAccount(context.Background(), player.ID, "wrongpassword")
		if !errors.Is(err, coreplayer.ErrAuthentication) {
			t.Fatalf("expected ErrAuthentication, got %v", err)
		}
	})

	t.Run("successfully deletes account with correct password", func(t *testing.T) {
		err := service.DeleteAccount(context.Background(), player.ID, "correctpassword")
		if err != nil {
			t.Fatalf("DeleteAccount failed: %v", err)
		}
		if players.value.ID != "" {
			t.Errorf("expected player to be cleared from repository")
		}
		if sessions.value.ID != "" {
			t.Errorf("expected player sessions to be cleared from repository")
		}
	})
}

type apiTokenRepositoryStub struct {
	tokens         map[string]coreplayer.APIToken
	tokensByHash   map[string]coreplayer.APIToken
	lastUsedCalls  map[string]time.Time
	deletedPlayers []string
}

func newAPITokenRepositoryStub() *apiTokenRepositoryStub {
	return &apiTokenRepositoryStub{
		tokens:        make(map[string]coreplayer.APIToken),
		tokensByHash:  make(map[string]coreplayer.APIToken),
		lastUsedCalls: make(map[string]time.Time),
	}
}

func (r *apiTokenRepositoryStub) Save(_ context.Context, t coreplayer.APIToken) error {
	r.tokens[t.ID] = t
	r.tokensByHash[t.TokenHash] = t
	return nil
}

func (r *apiTokenRepositoryStub) FindByTokenHash(_ context.Context, hash string) (coreplayer.APIToken, error) {
	t, ok := r.tokensByHash[hash]
	if !ok {
		return coreplayer.APIToken{}, errors.New("token not found")
	}
	return t, nil
}

func (r *apiTokenRepositoryStub) FindByPlayerID(_ context.Context, playerID string) ([]coreplayer.APIToken, error) {
	var list []coreplayer.APIToken
	for _, t := range r.tokens {
		if t.PlayerID == playerID {
			list = append(list, t)
		}
	}
	return list, nil
}

func (r *apiTokenRepositoryStub) TouchLastUsed(_ context.Context, id string, lastUsed time.Time) error {
	r.lastUsedCalls[id] = lastUsed
	return nil
}

func (r *apiTokenRepositoryStub) Revoke(_ context.Context, playerID, tokenID string) error {
	t, ok := r.tokens[tokenID]
	if !ok {
		return errors.New("token not found")
	}
	if t.PlayerID != playerID {
		return errors.New("forbidden")
	}
	delete(r.tokens, tokenID)
	delete(r.tokensByHash, t.TokenHash)
	return nil
}

func (r *apiTokenRepositoryStub) DeleteByPlayerID(_ context.Context, playerID string) error {
	r.deletedPlayers = append(r.deletedPlayers, playerID)
	for id, t := range r.tokens {
		if t.PlayerID == playerID {
			delete(r.tokens, id)
			delete(r.tokensByHash, t.TokenHash)
		}
	}
	return nil
}

func TestAPITokenServiceLifecycleAndDualAuthentication(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	player, _ := coreplayer.New("bob", "password123", now)
	players := &playerRepositoryStub{value: player}
	sessions := &sessionRepositoryStub{
		value: coreplayer.Session{ID: "sess-bob", PlayerID: player.ID, ExpiresAt: now.Add(24 * time.Hour)},
	}
	tokenRepo := newAPITokenRepositoryStub()

	service, err := NewService(players, sessions, WithAPITokenRepository(tokenRepo))
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }

	ctx := context.Background()

	// 1. Create Token
	token, plaintext, err := service.CreateAPIToken(ctx, player.ID, "Agent Token", nil)
	if err != nil {
		t.Fatalf("CreateAPIToken error: %v", err)
	}
	if !strings.HasPrefix(plaintext, "p2_sk_") {
		t.Errorf("expected plaintext to start with p2_sk_, got %q", plaintext)
	}
	if token.Name != "Agent Token" {
		t.Errorf("expected name 'Agent Token', got %q", token.Name)
	}

	// 2. Dual Authenticate with API Token
	authPlayer, err := service.Authenticate(ctx, plaintext)
	if err != nil {
		t.Fatalf("Authenticate with API token failed: %v", err)
	}
	if authPlayer.ID != player.ID {
		t.Errorf("Authenticate player ID mismatch: got %q, want %q", authPlayer.ID, player.ID)
	}
	if _, ok := tokenRepo.lastUsedCalls[token.ID]; !ok {
		t.Error("expected TouchLastUsed to be called for active API token")
	}

	// 3. Dual Authenticate with Interactive Session Token
	authSessionPlayer, err := service.Authenticate(ctx, "sess-bob")
	if err != nil {
		t.Fatalf("Authenticate with session token failed: %v", err)
	}
	if authSessionPlayer.ID != player.ID {
		t.Errorf("Authenticate session player ID mismatch: got %q, want %q", authSessionPlayer.ID, player.ID)
	}

	// 4. Authenticate with invalid API token
	if _, err := service.Authenticate(ctx, "p2_sk_invalidtokenstring"); !errors.Is(err, coreplayer.ErrAuthentication) {
		t.Errorf("expected ErrAuthentication for invalid token, got %v", err)
	}

	// 5. Authenticate with expired API token
	past := now.Add(-10 * time.Minute)
	expiredTok, expiredPlaintext, err := coreplayer.NewAPIToken(player.ID, "Expired", &past, now.Add(-1*time.Hour))
	if err == nil {
		_ = tokenRepo.Save(ctx, expiredTok)
		if _, err := service.Authenticate(ctx, expiredPlaintext); !errors.Is(err, coreplayer.ErrAuthentication) {
			t.Errorf("expected ErrAuthentication for expired token, got %v", err)
		}
	}

	// 6. List Tokens
	list, err := service.ListAPITokens(ctx, player.ID)
	if err != nil {
		t.Fatalf("ListAPITokens failed: %v", err)
	}
	if len(list) < 1 {
		t.Errorf("expected at least 1 token in list, got %d", len(list))
	}

	// 7. Revoke Token
	if err := service.RevokeAPIToken(ctx, player.ID, token.ID); err != nil {
		t.Fatalf("RevokeAPIToken failed: %v", err)
	}
	if _, err := service.Authenticate(ctx, plaintext); !errors.Is(err, coreplayer.ErrAuthentication) {
		t.Errorf("expected ErrAuthentication after revocation, got %v", err)
	}

	// 8. DeleteAccount cascades to API tokens
	_, _, _ = service.CreateAPIToken(ctx, player.ID, "Cascade Test", nil)
	if err := service.DeleteAccount(ctx, player.ID, "password123"); err != nil {
		t.Fatalf("DeleteAccount failed: %v", err)
	}
	if len(tokenRepo.deletedPlayers) != 1 || tokenRepo.deletedPlayers[0] != player.ID {
		t.Errorf("expected API tokens deleted for %q, got %v", player.ID, tokenRepo.deletedPlayers)
	}
}

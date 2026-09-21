package player

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/logging"
)

type playerRepositoryStub struct {
	value       coreplayer.Player
	saveErr     error
	findByIDErr error
	deleteErr   error
}

func (r *playerRepositoryStub) Save(context.Context, coreplayer.Player) error {
	return r.saveErr
}

func (r *playerRepositoryStub) FindByUsername(context.Context, string) (coreplayer.Player, error) {
	if r.value.ID == "" && r.value.Username == "" {
		return coreplayer.Player{}, errors.New("player not found")
	}
	return r.value, nil
}

func (r *playerRepositoryStub) FindByID(context.Context, string) (coreplayer.Player, error) {
	if r.findByIDErr != nil {
		return coreplayer.Player{}, r.findByIDErr
	}
	return r.value, nil
}

func (r *playerRepositoryStub) Delete(context.Context, string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.value = coreplayer.Player{}
	return nil
}

type sessionRepositoryStub struct {
	value         coreplayer.Session
	saveErr       error
	revokeErr     error
	deleteByIDErr error
}

func (r *sessionRepositoryStub) Save(_ context.Context, s coreplayer.Session) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.value = s
	return nil
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
	if r.deleteByIDErr != nil {
		return r.deleteByIDErr
	}
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
	chars     []corecharacter.Character
	findErr   error
	deleteErr error
	deleted   []string
}

func (c *charServiceStub) FindByPlayerID(ctx context.Context, playerID string) ([]corecharacter.Character, error) {
	if c.findErr != nil {
		return nil, c.findErr
	}
	return c.chars, nil
}

func (c *charServiceStub) Delete(ctx context.Context, playerID, characterID string) error {
	if c.deleteErr != nil {
		return c.deleteErr
	}
	c.deleted = append(c.deleted, characterID)
	return nil
}

type txProviderStub struct {
	called bool
	err    error
}

func (t *txProviderStub) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	t.called = true
	if t.err != nil {
		return t.err
	}
	return fn(ctx)
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
	saveErr        error
	revokeErr      error
	deleteErr      error
}

func newAPITokenRepositoryStub() *apiTokenRepositoryStub {
	return &apiTokenRepositoryStub{
		tokens:        make(map[string]coreplayer.APIToken),
		tokensByHash:  make(map[string]coreplayer.APIToken),
		lastUsedCalls: make(map[string]time.Time),
	}
}

func (r *apiTokenRepositoryStub) Save(_ context.Context, t coreplayer.APIToken) error {
	if r.saveErr != nil {
		return r.saveErr
	}
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
	if r.revokeErr != nil {
		return r.revokeErr
	}
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
	if r.deleteErr != nil {
		return r.deleteErr
	}
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

func TestNewServiceValidation(t *testing.T) {
	players := &playerRepositoryStub{}
	sessions := &sessionRepositoryStub{}

	t.Run("nil players repository returns error", func(t *testing.T) {
		svc, err := NewService(nil, sessions)
		if err == nil || svc != nil {
			t.Fatalf("expected error for nil players, got svc=%v, err=%v", svc, err)
		}
	})

	t.Run("nil sessions repository returns error", func(t *testing.T) {
		svc, err := NewService(players, nil)
		if err == nil || svc != nil {
			t.Fatalf("expected error for nil sessions, got svc=%v, err=%v", svc, err)
		}
	})

	t.Run("handles nil and non-nil options cleanly", func(t *testing.T) {
		tx := &txProviderStub{}
		chars := &charServiceStub{}
		tokens := newAPITokenRepositoryStub()

		svc, err := NewService(players, sessions,
			nil,
			WithLogger(nil),
			WithTransactionProvider(tx),
			WithCharacterService(chars),
			WithAPITokenRepository(tokens),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if svc.txProvider != tx {
			t.Errorf("txProvider not set")
		}
		if svc.characters != chars {
			t.Errorf("characters not set")
		}
		if svc.apiTokens != tokens {
			t.Errorf("apiTokens not set")
		}
	})
}

func TestRegisterErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid username returns error", func(t *testing.T) {
		svc, _ := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{})
		_, err := svc.Register(ctx, "", "validpassword")
		if err == nil {
			t.Fatal("expected error for empty username, got nil")
		}
	})

	t.Run("players.Save returns error", func(t *testing.T) {
		expectedErr := errors.New("db save error")
		players := &playerRepositoryStub{saveErr: expectedErr}
		svc, _ := NewService(players, &sessionRepositoryStub{})
		_, err := svc.Register(ctx, "validuser", "validpassword")
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected error %v, got %v", expectedErr, err)
		}
	})
}

func TestLoginErrorBranches(t *testing.T) {
	ctx := context.Background()
	player, _ := coreplayer.New("alice", "password123", time.Now())

	t.Run("sessions.Save returns error", func(t *testing.T) {
		expectedErr := errors.New("valkey save error")
		players := &playerRepositoryStub{value: player}
		sessions := &sessionRepositoryStub{saveErr: expectedErr}
		svc, _ := NewService(players, sessions)

		_, err := svc.Login(ctx, "alice", "password123")
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("session creation error when player id is empty", func(t *testing.T) {
		emptyIDPlayer := player
		emptyIDPlayer.ID = ""
		players := &playerRepositoryStub{value: emptyIDPlayer}
		svc, _ := NewService(players, &sessionRepositoryStub{})

		_, err := svc.Login(ctx, "alice", "password123")
		if !errors.Is(err, coreplayer.ErrInvalidSession) {
			t.Fatalf("expected ErrInvalidSession, got %v", err)
		}
	})
}

func TestLogoutErrorBranches(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("valkey revoke error")
	sessions := &sessionRepositoryStub{revokeErr: expectedErr}
	svc, _ := NewService(&playerRepositoryStub{}, sessions)

	err := svc.Logout(ctx, "session-id")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestAuthenticateErrorBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	t.Run("empty or whitespace token returns ErrAuthentication", func(t *testing.T) {
		svc, _ := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{})
		for _, tok := range []string{"", "   ", "\t\n"} {
			_, err := svc.Authenticate(ctx, tok)
			if !errors.Is(err, coreplayer.ErrAuthentication) {
				t.Fatalf("expected ErrAuthentication for token %q, got %v", tok, err)
			}
		}
	})

	t.Run("api token prefix with nil apiTokens repo returns ErrAuthentication", func(t *testing.T) {
		svc, _ := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{})
		_, err := svc.Authenticate(ctx, coreplayer.APITokenPrefix+"somekey")
		if !errors.Is(err, coreplayer.ErrAuthentication) {
			t.Fatalf("expected ErrAuthentication, got %v", err)
		}
	})

	t.Run("api token valid but players.FindByID returns error", func(t *testing.T) {
		expectedErr := errors.New("db find error")
		players := &playerRepositoryStub{findByIDErr: expectedErr}
		tokens := newAPITokenRepositoryStub()
		tok, plaintext, _ := coreplayer.NewAPIToken("player-1", "Test", nil, now)
		_ = tokens.Save(ctx, tok)

		svc, _ := NewService(players, &sessionRepositoryStub{}, WithAPITokenRepository(tokens))
		svc.now = func() time.Time { return now }

		_, err := svc.Authenticate(ctx, plaintext)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("session valid but players.FindByID returns error", func(t *testing.T) {
		expectedErr := errors.New("db find error")
		players := &playerRepositoryStub{findByIDErr: expectedErr}
		sessions := &sessionRepositoryStub{
			value: coreplayer.Session{ID: "sess-1", PlayerID: "player-1", ExpiresAt: now.Add(time.Hour)},
		}

		svc, _ := NewService(players, sessions)
		svc.now = func() time.Time { return now }

		_, err := svc.Authenticate(ctx, "sess-1")
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})
}

func TestAPITokenServiceErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("methods return error when repository is nil", func(t *testing.T) {
		svc, _ := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{})

		_, _, err := svc.CreateAPIToken(ctx, "p1", "token", nil)
		if err == nil || !strings.Contains(err.Error(), "not configured") {
			t.Fatalf("expected not configured error, got %v", err)
		}

		_, err = svc.ListAPITokens(ctx, "p1")
		if err == nil || !strings.Contains(err.Error(), "not configured") {
			t.Fatalf("expected not configured error, got %v", err)
		}

		err = svc.RevokeAPIToken(ctx, "p1", "t1")
		if err == nil || !strings.Contains(err.Error(), "not configured") {
			t.Fatalf("expected not configured error, got %v", err)
		}
	})

	t.Run("CreateAPIToken invalid input returns error", func(t *testing.T) {
		tokens := newAPITokenRepositoryStub()
		svc, _ := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{}, WithAPITokenRepository(tokens))

		_, _, err := svc.CreateAPIToken(ctx, "p1", "", nil)
		if err == nil {
			t.Fatal("expected error for empty token name, got nil")
		}
	})

	t.Run("CreateAPIToken repository save error", func(t *testing.T) {
		expectedErr := errors.New("save failure")
		tokens := newAPITokenRepositoryStub()
		tokens.saveErr = expectedErr
		svc, _ := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{}, WithAPITokenRepository(tokens))

		_, _, err := svc.CreateAPIToken(ctx, "p1", "valid-name", nil)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("RevokeAPIToken repository revoke error", func(t *testing.T) {
		expectedErr := errors.New("revoke failure")
		tokens := newAPITokenRepositoryStub()
		tokens.revokeErr = expectedErr
		svc, _ := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{}, WithAPITokenRepository(tokens))

		err := svc.RevokeAPIToken(ctx, "p1", "t1")
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})
}

func TestDeleteAccountErrorBranches(t *testing.T) {
	ctx := context.Background()
	player, _ := coreplayer.New("eve", "password", time.Now())

	t.Run("empty playerID returns error", func(t *testing.T) {
		svc, _ := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{})
		err := svc.DeleteAccount(ctx, "   ", "password")
		if err == nil || !strings.Contains(err.Error(), "player ID is required") {
			t.Fatalf("expected player ID required error, got %v", err)
		}
	})

	t.Run("players.FindByID returns error", func(t *testing.T) {
		expectedErr := errors.New("player not found in db")
		players := &playerRepositoryStub{findByIDErr: expectedErr}
		svc, _ := NewService(players, &sessionRepositoryStub{})

		err := svc.DeleteAccount(ctx, "player-1", "password")
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("character deletion failure returns error", func(t *testing.T) {
		players := &playerRepositoryStub{value: player}
		expectedErr := errors.New("char delete failed")
		chars := &charServiceStub{
			chars:     []corecharacter.Character{{ID: "c1", Name: "Hero"}},
			deleteErr: expectedErr,
		}
		svc, _ := NewService(players, &sessionRepositoryStub{}, WithCharacterService(chars))

		err := svc.DeleteAccount(ctx, player.ID, "password")
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("sessions and api tokens deletion errors are logged but not blocking", func(t *testing.T) {
		players := &playerRepositoryStub{value: player}
		sessions := &sessionRepositoryStub{deleteByIDErr: errors.New("valkey del error")}
		tokens := newAPITokenRepositoryStub()
		tokens.deleteErr = errors.New("token del error")
		chars := &charServiceStub{
			chars: []corecharacter.Character{{ID: "c1", Name: "Hero"}},
		}

		svc, _ := NewService(players, sessions,
			WithCharacterService(chars),
			WithAPITokenRepository(tokens),
		)

		err := svc.DeleteAccount(ctx, player.ID, "password")
		if err != nil {
			t.Fatalf("expected success despite non-fatal warnings, got %v", err)
		}
		if len(chars.deleted) != 1 || chars.deleted[0] != "c1" {
			t.Errorf("expected char c1 deleted, got %v", chars.deleted)
		}
	})

	t.Run("players.Delete in transaction returns error", func(t *testing.T) {
		expectedErr := errors.New("tx player delete failed")
		players := &playerRepositoryStub{value: player, deleteErr: expectedErr}
		tx := &txProviderStub{}
		svc, _ := NewService(players, &sessionRepositoryStub{}, WithTransactionProvider(tx))

		err := svc.DeleteAccount(ctx, player.ID, "password")
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
		if !tx.called {
			t.Error("expected txProvider to be called")
		}
	})
}

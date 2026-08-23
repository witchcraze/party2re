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

func TestRegisterLogsStructuredSafeOperation(t *testing.T) {
	var output bytes.Buffer
	logger := logging.NewJSON(&output)
	service, err := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{}, logger)
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
	service, err := NewService(&playerRepositoryStub{value: value}, &sessionRepositoryStub{}, logger)
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
	service, err := NewService(&playerRepositoryStub{}, &sessionRepositoryStub{}, logger)
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
	service, err := NewService(&playerRepositoryStub{value: player}, sessions, logging.Nop())
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
	service, err := NewService(&playerRepositoryStub{value: player}, sessions, logging.Nop())
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
	service, err := NewService(&playerRepositoryStub{value: player}, sessions, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}

	// Manually insert an already-expired session by manipulating stub.
	expiredSession, _ := coreplayer.NewSession(player.ID, time.Now().Add(-48*time.Hour), SessionDuration)
	sessions.value = expiredSession

	if _, err := service.Authenticate(context.Background(), expiredSession.ID); !errors.Is(err, coreplayer.ErrAuthentication) {
		t.Fatalf("Authenticate(expired) error = %v, want %v", err, coreplayer.ErrAuthentication)
	}
}

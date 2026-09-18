package challenge_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func setupSessionTestService(t *testing.T) (*challenge.Service, *mockChallengeRepo, *mockCharRepo, *mockActiveStore) {
	t.Helper()
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"hero": {
				ID:         "hero",
				Level:      30,
				Experience: 10000,
				Stats: corecharacter.Stats{
					HP:      500,
					MaxHP:   500,
					Attack:  150,
					Defense: 100,
				},
			},
			"weakling": {
				ID:         "weakling",
				Level:      5,
				Experience: 100,
				Stats: corecharacter.Stats{
					HP:      10,
					MaxHP:   10,
					Attack:  5,
					Defense: 0,
				},
			},
		},
	}
	activeStore := newMockActiveStore(nil)
	svc, err := challenge.NewService(
		repo,
		charRepo,
		&corebattle.Engine{},
		challenge.WithActiveSessionStore(activeStore),
	)
	if err != nil {
		t.Fatalf("failed to create challenge service: %v", err)
	}
	return svc, repo, charRepo, activeStore
}

func TestAdvanceRound_PersistenceErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("empty character ID returns ErrCharacterNotFound", func(t *testing.T) {
		svc, _, _, _ := setupSessionTestService(t)
		_, _, err := svc.AdvanceRound(ctx, "", "sess-1")
		if !errors.Is(err, challenge.ErrCharacterNotFound) {
			t.Errorf("expected ErrCharacterNotFound, got %v", err)
		}
	})

	t.Run("activeStore GetActiveSession error propagates", func(t *testing.T) {
		svc, _, _, activeStore := setupSessionTestService(t)
		activeStore.getActiveSessionErr = errors.New("valkey connection reset")

		_, _, err := svc.AdvanceRound(ctx, "hero", "sess-1")
		if err == nil || !strings.Contains(err.Error(), "valkey connection reset") {
			t.Errorf("expected valkey error, got %v", err)
		}
	})

	t.Run("charRepo FindByID error propagates", func(t *testing.T) {
		svc, _, charRepo, _ := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "novice")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		charRepo.findByIDErr = errors.New("character db read failed")

		_, _, err = svc.AdvanceRound(ctx, "hero", sess.ID)
		if err == nil || !strings.Contains(err.Error(), "character db read failed") {
			t.Errorf("expected charRepo error, got %v", err)
		}
	})

	t.Run("repo GetHallOfFame error propagates with context", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "novice")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		repo.getHofErr = errors.New("db connection refused")

		_, _, err = svc.AdvanceRound(ctx, "hero", sess.ID)
		if err == nil || !strings.Contains(err.Error(), "getting hall of fame: db connection refused") {
			t.Errorf("expected wrapped hall of fame error, got %v", err)
		}
	})

	t.Run("repo SaveHallOfFame error propagates with context", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "novice")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		repo.saveHofErr = errors.New("hall of fame write lock failed")

		_, _, err = svc.AdvanceRound(ctx, "hero", sess.ID)
		if err == nil || !strings.Contains(err.Error(), "saving hall of fame: hall of fame write lock failed") {
			t.Errorf("expected wrapped save hall of fame error, got %v", err)
		}
	})

	t.Run("activeStore AdvanceRound error propagates", func(t *testing.T) {
		svc, _, _, activeStore := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "novice")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		activeStore.advanceRoundErr = errors.New("lua script timeout")

		_, _, err = svc.AdvanceRound(ctx, "hero", sess.ID)
		if err == nil || !strings.Contains(err.Error(), "lua script timeout") {
			t.Errorf("expected advance round error, got %v", err)
		}
	})

	t.Run("activeStore SaveActiveSession error propagates with context", func(t *testing.T) {
		svc, _, _, activeStore := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "novice")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		activeStore.saveActiveSessionErr = errors.New("valkey oom")

		_, _, err = svc.AdvanceRound(ctx, "hero", sess.ID)
		if err == nil || !strings.Contains(err.Error(), "saving active session: valkey oom") {
			t.Errorf("expected wrapped save active session error, got %v", err)
		}
	})

	t.Run("repo FinalizeSession on defeat error propagates", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "weakling", "novice")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		repo.finalizeSessionErr = errors.New("mariadb deadlocked")

		_, _, err = svc.AdvanceRound(ctx, "weakling", sess.ID)
		if err == nil || !strings.Contains(err.Error(), "mariadb deadlocked") {
			t.Errorf("expected finalize session error, got %v", err)
		}
	})
}

func TestRetireSession_PersistenceErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("empty character ID returns ErrCharacterNotFound", func(t *testing.T) {
		svc, _, _, _ := setupSessionTestService(t)
		_, err := svc.RetireSession(ctx, "", "sess-1")
		if !errors.Is(err, challenge.ErrCharacterNotFound) {
			t.Errorf("expected ErrCharacterNotFound, got %v", err)
		}
	})

	t.Run("activeStore GetActiveSession error propagates", func(t *testing.T) {
		svc, _, _, activeStore := setupSessionTestService(t)
		activeStore.getActiveSessionErr = errors.New("valkey unreachable")

		_, err := svc.RetireSession(ctx, "hero", "sess-1")
		if err == nil || !strings.Contains(err.Error(), "valkey unreachable") {
			t.Errorf("expected valkey error, got %v", err)
		}
	})

	t.Run("repo FinalizeSession error propagates", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "novice")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		repo.finalizeSessionErr = errors.New("mariadb foreign key violation")

		_, err = svc.RetireSession(ctx, "hero", sess.ID)
		if err == nil || !strings.Contains(err.Error(), "mariadb foreign key violation") {
			t.Errorf("expected finalize session error, got %v", err)
		}
	})
}

func TestExecuteRound_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("repo FindSessionByID error propagates", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		repo.findSessionByIDErr = errors.New("db session query failed")

		_, err := svc.ExecuteRound(ctx, "sess-1")
		if err == nil || !strings.Contains(err.Error(), "db session query failed") {
			t.Errorf("expected repo find error, got %v", err)
		}
	})
}

func TestCashout_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("repo FindSessionByID error propagates", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		repo.findSessionByIDErr = errors.New("db cashout query failed")

		_, err := svc.Cashout(ctx, "sess-1")
		if err == nil || !strings.Contains(err.Error(), "db cashout query failed") {
			t.Errorf("expected repo find error, got %v", err)
		}
	})

	t.Run("RetireSession failure propagates during cashout", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		sess, err := svc.StartSession(ctx, "hero", "novice")
		if err != nil {
			t.Fatalf("StartSession failed: %v", err)
		}

		repo.finalizeSessionErr = errors.New("finalize in cashout failed")

		_, err = svc.Cashout(ctx, sess.ID)
		if err == nil || !strings.Contains(err.Error(), "finalize in cashout failed") {
			t.Errorf("expected finalize error during cashout, got %v", err)
		}
	})
}

func TestGetSession_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("repo FindSessionByID error propagates when not in active store", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		repo.findSessionByIDErr = errors.New("durable query timeout")

		_, err := svc.GetSession(ctx, "hero", "non-existent-sess")
		if err == nil || !strings.Contains(err.Error(), "durable query timeout") {
			t.Errorf("expected repo error, got %v", err)
		}
	})
}

func TestGetCharacterRecords_ValidationAndErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("empty character ID returns error", func(t *testing.T) {
		svc, _, _, _ := setupSessionTestService(t)
		_, err := svc.GetCharacterRecords(ctx, "")
		if err == nil || !strings.Contains(err.Error(), "character id is required") {
			t.Errorf("expected character id required error, got %v", err)
		}
	})

	t.Run("repo FindRecordsByCharacter error propagates", func(t *testing.T) {
		svc, repo, _, _ := setupSessionTestService(t)
		repo.findRecordsByCharErr = errors.New("records db timeout")

		_, err := svc.GetCharacterRecords(ctx, "hero")
		if err == nil || !strings.Contains(err.Error(), "records db timeout") {
			t.Errorf("expected repo records error, got %v", err)
		}
	})
}

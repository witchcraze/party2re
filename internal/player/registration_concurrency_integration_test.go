package player_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/player"
	"github.com/witchcraze/party2re/internal/testutil"
)

type nopSessionRepo struct{}

func (n *nopSessionRepo) Save(context.Context, coreplayer.Session) error { return nil }
func (n *nopSessionRepo) FindByID(context.Context, string) (coreplayer.Session, error) {
	return coreplayer.Session{}, nil
}
func (n *nopSessionRepo) Revoke(context.Context, string, time.Time) error { return nil }
func (n *nopSessionRepo) DeleteByPlayerID(context.Context, string) error  { return nil }

func registrationIsolatedDB(t *testing.T) (*sql.DB, *database.PlayerRepository) {
	t.Helper()
	db := database.CreateTestDatabase(t, []string{"players", "system_maintenance", "player_notifications", "player_api_tokens"}...)
	repo, err := database.NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	return db, repo
}

func TestPlayerRegistration_ConcurrentStress_MariaDB(t *testing.T) {
	db, playerRepo := registrationIsolatedDB(t)

	ctx := context.Background()
	initialCount, err := playerRepo.CountActive(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// We allow exactly M = 3 new players above current initialCount
	const allowedSlots = 3
	maxPlayers := initialCount + allowedSlots

	txProvider := database.NewTransactionProvider(db)
	svc, err := player.NewService(
		playerRepo,
		&nopSessionRepo{},
		player.WithTransactionProvider(txProvider),
		player.WithMaxPlayers(maxPlayers),
	)
	if err != nil {
		t.Fatal(err)
	}

	var createdIDs []string
	var mu sync.Mutex

	cfg := testutil.ConcurrencyStressConfig{
		Workers:      6,
		OpsPerWorker: 2,
	}

	prefix := fmt.Sprintf("stresstest_%d_", time.Now().UnixNano())

	res := testutil.RunConcurrentStressTest(t, cfg, func(workerID, op int) error {
		username := fmt.Sprintf("%sw%d_op%d", prefix, workerID, op)
		p, err := svc.Register(ctx, username, "stresspass123")
		if err == nil {
			mu.Lock()
			createdIDs = append(createdIDs, p.ID)
			mu.Unlock()
		}
		return err
	})

	if res.Deadlocks != 0 {
		t.Fatalf("expected 0 deadlocks, got %d", res.Deadlocks)
	}

	// Exactly allowedSlots registrations must succeed
	if res.Successes != int64(allowedSlots) {
		t.Fatalf("expected exactly %d successful registrations, got %d (failures: %d)", allowedSlots, res.Successes, res.Failures)
	}

	// All other attempts must fail
	expectedFailures := res.TotalOps - int64(allowedSlots)
	if res.Failures != expectedFailures {
		t.Fatalf("expected %d failures, got %d", expectedFailures, res.Failures)
	}

	// Final active count in MariaDB must equal maxPlayers exactly
	finalCount, err := playerRepo.CountActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if finalCount != maxPlayers {
		t.Fatalf("expected final count %d, got %d", maxPlayers, finalCount)
	}
}

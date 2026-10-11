package ranking_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Use a future clock so concurrently running production-wiring tests cannot settle these deadlines.
func weeklyIntegrationClock() time.Time {
	return ranking.NextSundayMidnightJST(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
}

func weeklyRotationDB(t *testing.T) (*sql.DB, *database.RankingRepository) {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	repo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), "DELETE FROM weekly_job_changes"); err != nil {
		t.Fatal(err)
	}
	return db, repo
}

type failingWeeklyResetRepo struct {
	*interceptingRankingRepo
}

func (r *failingWeeklyResetRepo) ResetWeeklyJobChanges(context.Context) error {
	return errors.New("injected reset failure")
}

func TestWeeklyRotation_DurableProgress_RealDB(t *testing.T) {
	db, repo := weeklyRotationDB(t)
	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "Weekly_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	now := weeklyIntegrationClock()
	setWeeklyRotationDue(t, repo, now)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordJobChange(ctx, char.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatal(err)
	}
	due, err := repo.GetWeeklyRotationDue(ctx)
	if err != nil || !due.Equal(now.AddDate(0, 0, 7)) {
		t.Fatalf("durable deadline %v: %v", due, err)
	}
	// Administrative snapshot refresh must not overwrite dedicated rotation progress.
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeWeeklyJobChange); err != nil {
		t.Fatal(err)
	}
	afterRefresh, err := repo.GetWeeklyRotationDue(ctx)
	if err != nil || !afterRefresh.Equal(due) {
		t.Fatalf("refresh changed durable deadline: %v / %v", afterRefresh, err)
	}
	if err := svc.RecordJobChange(ctx, char.ID); err != nil {
		t.Fatal(err)
	}
	// New service/process, several missed weeks, then a reset failure after snapshot save.
	now = due.AddDate(0, 0, 22)
	before, err := repo.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatal(err)
	}
	failing, err := ranking.NewService(&failingWeeklyResetRepo{&interceptingRankingRepo{Repository: repo}}, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failing.RotateWeeklyJobChangeRanking(ctx); err == nil {
		t.Fatal("expected reset failure")
	}
	after, err := repo.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil || before != after {
		t.Fatalf("rollback changed snapshot: %v", err)
	}
	afterFailure, err := repo.GetWeeklyRotationDue(ctx)
	if err != nil || !afterFailure.Equal(due) {
		t.Fatalf("rollback changed progress: %v / %v", afterFailure, err)
	}
	fresh, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	next, err := fresh.RotateWeeklyJobChangeRanking(ctx)
	if err != nil || !next.Equal(ranking.NextSundayMidnightJST(now)) {
		t.Fatalf("recovery: %v / %v", next, err)
	}
	if err := fresh.RecordJobChange(ctx, char.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.RotateWeeklyJobChangeRanking(ctx); err != nil {
		t.Fatal(err)
	}
	active, _, err := repo.GetActiveWeeklyJobChangeRanking(ctx, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].CharacterID != char.ID || active[0].Score != 1 {
		t.Fatalf("replay consumed fresh writes: %+v", active)
	}
}

func TestWeeklyRotation_ConcurrentReconciliationConservesCounters_RealDB(t *testing.T) {
	db, repo := weeklyRotationDB(t)
	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, db, "WeeklyRace_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	now := weeklyIntegrationClock()
	setWeeklyRotationDue(t, repo, now)
	if err := repo.IncrementJobChangeCount(ctx, char.ID); err != nil {
		t.Fatal(err)
	}
	cfg := testutil.GetStressConfig()
	services := make([]*ranking.Service, cfg.Workers)
	for i := range services {
		services[i], err = ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
		if err != nil {
			t.Fatal(err)
		}
	}
	res := testutil.RunConcurrentStressTest(t, cfg, func(worker, op int) error {
		if err := services[worker].RecordJobChange(ctx, char.ID); err != nil {
			return err
		}
		_, err := services[worker].RotateWeeklyJobChangeRanking(ctx)
		return err
	})
	if res.Failures != 0 {
		t.Fatalf("stress failures: %+v", res)
	}
	snap, err := repo.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatal(err)
	}
	var frozen []ranking.CharacterRankingEntry
	if err := json.Unmarshal([]byte(snap.SnapshotData), &frozen); err != nil {
		t.Fatal(err)
	}
	active, _, err := repo.GetActiveWeeklyJobChangeRanking(ctx, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var counted int64
	for _, entries := range [][]ranking.CharacterRankingEntry{frozen, active} {
		for _, entry := range entries {
			if entry.CharacterID == char.ID {
				counted += entry.Score
			}
		}
	}
	if want := int64(1 + cfg.Workers*cfg.OpsPerWorker); counted != want {
		t.Fatalf("counted %d job changes, want %d", counted, want)
	}
	due, err := repo.GetWeeklyRotationDue(ctx)
	if err != nil || !due.Equal(now.AddDate(0, 0, 7)) {
		t.Fatalf("progress advanced more than once: %v / %v", due, err)
	}
	t.Logf("%d operations conserved counters in %v", res.TotalOps, res.Duration)
}

package ranking_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Keep the clock deterministic; database isolation is provided by weeklyRotationDB.
func weeklyIntegrationClock() time.Time {
	return ranking.NextSundayMidnightJST(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
}

func weeklyRotationDB(t *testing.T) (*sql.DB, *database.RankingRepository) {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	})
	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated weekly rotation tests")
	}
	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})
	// Each test owns a fresh schema. All workers use real, independent transactions;
	// cleanup only drops this schema and never restores over another test's writes.
	name := "party2_weekly_test_" + id.New()
	if _, err := admin.ExecContext(context.Background(), "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`"); err != nil {
			t.Error(err)
		}
	})
	cfg.DBName = name
	db, err := database.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	// Copy definitions, including foreign keys, in dependency order; copy no data.
	for _, table := range []string{"players", "characters", "character_depots", "ranking_snapshots", "weekly_job_changes"} {
		var tableName, ddl string
		if err := source.QueryRowContext(context.Background(), "SHOW CREATE TABLE `"+table+"`").Scan(&tableName, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(context.Background(), ddl); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := database.NewRankingRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	return db, repo
}

type failingWeeklyResetRepo struct {
	*interceptingRankingRepo
}

func TestWeeklyRotation_DatabaseIsolation_RealDB(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dbA, repoA := weeklyRotationDB(t)
	dbB, repoB := weeklyRotationDB(t)
	var nameA, nameB string
	if err := dbA.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&nameA); err != nil {
		t.Fatal(err)
	}
	if err := dbB.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&nameB); err != nil {
		t.Fatal(err)
	}
	if nameA == nameB {
		t.Fatal("weekly fixtures share a database")
	}
	charA, err := database.CreateTestCharacter(ctx, dbA, "IsolationA")
	if err != nil {
		t.Fatal(err)
	}
	charB, err := database.CreateTestCharacter(ctx, dbB, "IsolationB")
	if err != nil {
		t.Fatal(err)
	}
	now := weeklyIntegrationClock()
	setWeeklyRotationDue(t, repoA, now)
	setWeeklyRotationDue(t, repoB, now.AddDate(0, 0, 7))
	if err := repoB.IncrementJobChangeCount(ctx, charB.ID); err != nil {
		t.Fatal(err)
	}
	before, err := repoB.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil {
		t.Fatal(err)
	}
	svcA, err := ranking.NewService(repoA, ranking.WithNowFunc(weeklyIntegrationClock))
	if err != nil {
		t.Fatal(err)
	}
	if err := svcA.RecordJobChange(ctx, charA.ID); err != nil {
		t.Fatal(err)
	}
	rotateErr, writeErr := database.RunRace2(func() error {
		_, err := svcA.RotateWeeklyJobChangeRanking(ctx)
		return err
	}, func() error {
		return repoB.IncrementJobChangeCount(ctx, charB.ID)
	})
	if rotateErr != nil || writeErr != nil {
		t.Fatalf("isolated rotation/write: %v / %v", rotateErr, writeErr)
	}
	after, err := repoB.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil || after != before {
		t.Fatalf("rotation changed another database's snapshot: %v", err)
	}
	due, err := repoB.GetWeeklyRotationDue(ctx)
	if err != nil || !due.Equal(now.AddDate(0, 0, 7)) {
		t.Fatalf("rotation changed another database's deadline: %v / %v", due, err)
	}
	active, total, err := repoB.GetActiveWeeklyJobChangeRanking(ctx, 10, 0)
	if err != nil || total != 1 || len(active) != 1 || active[0].CharacterID != charB.ID || active[0].Score != 2 {
		t.Fatalf("rotation lost another database's counters or concurrent write: %+v / %v", active, err)
	}
}

func (r *failingWeeklyResetRepo) ResetWeeklyJobChanges(context.Context) error {
	return errors.New("injected reset failure")
}

func TestWeeklyRotation_DurableProgress_RealDB(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

package ranking_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	corescheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Keep the clock deterministic; database isolation is provided by weeklyRotationDB.
func weeklyIntegrationClock() time.Time {
	return ranking.NextSundayMidnightJST(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC))
}

func weeklyRotationDB(t *testing.T) (*sql.DB, *database.RankingRepository) {
	t.Helper()
	db := database.CreateTestDatabase(t, []string{"players", "characters", "character_depots", "ranking_snapshots", "weekly_job_changes"}...)
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
	// Administrative snapshot refresh must not overwrite dedicated rotation progress or its finalized week.
	finalized, err := repo.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange)
	if err != nil || finalized.TotalCount != 1 {
		t.Fatalf("finalized snapshot %+v: %v", finalized, err)
	}
	if err := svc.RefreshSnapshot(ctx, ranking.RankingTypeWeeklyJobChange); err != nil {
		t.Fatal(err)
	}
	if refreshed, err := repo.GetSnapshot(ctx, ranking.RankingTypeWeeklyJobChange); err != nil || refreshed.SnapshotData != finalized.SnapshotData || refreshed.TotalCount != finalized.TotalCount {
		t.Fatalf("refresh overwrote finalized week: %+v: %v", refreshed, err)
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

type isolatedRankingSchedRepo struct {
	*scheduling.ValkeyRepository
	actor        string
	logicalActor string
}

func (r *isolatedRankingSchedRepo) Schedule(ctx context.Context, action corescheduling.ScheduledAction) error {
	action.ActorID = r.actor
	return r.ValkeyRepository.Schedule(ctx, action)
}

func (r *isolatedRankingSchedRepo) Save(ctx context.Context, action corescheduling.ScheduledAction) error {
	action.ActorID = r.actor
	return r.ValkeyRepository.Save(ctx, action)
}

func (r *isolatedRankingSchedRepo) FindPendingByActorID(ctx context.Context, _ string) ([]corescheduling.ScheduledAction, error) {
	return r.ValkeyRepository.FindPendingByActorID(ctx, r.actor)
}

func TestRankingReconcile_RealValkey_MissingQueueEntryRepair_Integration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, repo := weeklyRotationDB(t)
	vkClient := testutil.OpenValkey(t)
	baseRepo := scheduling.NewValkeyRepository(vkClient)

	hasDueID := func(due []corescheduling.ScheduledAction, actionID string) (corescheduling.ScheduledAction, bool) {
		for _, a := range due {
			if a.ID == actionID {
				return a, true
			}
		}
		return corescheduling.ScheduledAction{}, false
	}

	verifyRepairAndDelivery := func(
		t *testing.T,
		actionType string,
		firstTarget time.Time,
		nextTarget time.Time,
		advanceToDue func(),
		reconcile func(context.Context) error,
		handler scheduling.ActionHandler,
		schedSvc *scheduling.Service,
		isolated *isolatedRankingSchedRepo,
	) {
		t.Helper()
		if err := reconcile(ctx); err != nil {
			t.Fatalf("initial reconcile: %v", err)
		}
		pending, err := isolated.FindPendingByActorID(ctx, "")
		if err != nil || len(pending) != 1 {
			t.Fatalf("expected 1 pending reservation, got %+v, err=%v", pending, err)
		}
		act := pending[0]
		dueBefore, err := baseRepo.FetchDue(ctx, firstTarget, 200)
		if err != nil {
			t.Fatalf("FetchDue before drop: %v", err)
		}
		if _, ok := hasDueID(dueBefore, act.ID); !ok {
			t.Fatalf("expected %q in FetchDue before queue drop", act.ID)
		}

		// Drop ONLY the sorted-set queue entry while keeping payload and actor index.
		if err := vkClient.Do(ctx, vkClient.B().Zrem().Key("party2:scheduled:pending").Member(act.ID).Build()).Error(); err != nil {
			t.Fatalf("ZREM pending queue: %v", err)
		}
		dueAfterDrop, err := baseRepo.FetchDue(ctx, firstTarget, 200)
		if err != nil {
			t.Fatalf("FetchDue after drop: %v", err)
		}
		if _, ok := hasDueID(dueAfterDrop, act.ID); ok {
			t.Fatalf("unexpected %q in FetchDue after queue drop", act.ID)
		}

		for range 2 {
			if err := reconcile(ctx); err != nil {
				t.Fatalf("repair reconcile: %v", err)
			}
		}
		afterRepair, err := isolated.FindPendingByActorID(ctx, "")
		if err != nil || len(afterRepair) != 1 || afterRepair[0].ID != act.ID {
			t.Fatalf("expected existing reservation %q preserved, got %+v, err=%v", act.ID, afterRepair, err)
		}
		dueAfterRepair, err := baseRepo.FetchDue(ctx, firstTarget, 200)
		if err != nil {
			t.Fatalf("FetchDue after repair: %v", err)
		}
		dueAct, ok := hasDueID(dueAfterRepair, act.ID)
		if !ok {
			t.Fatalf("expected %q restored in FetchDue after repair", act.ID)
		}

		// Stale Pending snapshot must never overwrite concurrent Processing or make it executable.
		processing := act
		processing.State = corescheduling.StateProcessing
		if err := baseRepo.Save(ctx, processing); err != nil {
			t.Fatalf("save processing: %v", err)
		}
		if err := vkClient.Do(ctx, vkClient.B().Zrem().Key("party2:scheduled:pending").Member(act.ID).Build()).Error(); err != nil {
			t.Fatalf("ZREM processing: %v", err)
		}
		if err := schedSvc.RequeuePending(ctx, act); err != nil {
			t.Fatalf("stale RequeuePending on Processing: %v", err)
		}
		dueProc, err := baseRepo.FetchDue(ctx, firstTarget, 200)
		if err != nil {
			t.Fatalf("FetchDue during processing: %v", err)
		}
		if _, ok := hasDueID(dueProc, act.ID); ok {
			t.Fatalf("stale RequeuePending made Processing action %q executable", act.ID)
		}
		if err := vkClient.Do(ctx, vkClient.B().Zscore().Key("party2:scheduled:pending").Member(act.ID).Build()).Error(); !valkey.IsValkeyNil(err) {
			t.Fatalf("expected Processing queue entry cleaned by FetchDue, got %v", err)
		}
		procFound, err := isolated.FindPendingByActorID(ctx, "")
		if err != nil || len(procFound) != 1 || procFound[0].State != corescheduling.StateProcessing {
			t.Fatalf("expected StateProcessing preserved, got %+v, err=%v", procFound, err)
		}

		// Restore Pending and advance clock to verify due delivery through Worker.
		if err := baseRepo.Save(ctx, act); err != nil {
			t.Fatalf("restore pending payload: %v", err)
		}
		if err := schedSvc.RequeuePending(ctx, act); err != nil {
			t.Fatalf("restore pending queue: %v", err)
		}
		advanceToDue()
		dueAct.ActorID = isolated.logicalActor
		worker := scheduling.NewWorker(isolated, time.Second, logging.Nop())
		worker.RegisterHandler(actionType, handler)
		worker.ProcessAction(ctx, dueAct)

		afterDelivery, err := isolated.FindPendingByActorID(ctx, "")
		if err != nil {
			t.Fatalf("FindPendingByActorID after delivery: %v", err)
		}
		if len(afterDelivery) != 1 || !afterDelivery[0].ExecuteAt.Equal(nextTarget) || afterDelivery[0].State != corescheduling.StatePending {
			t.Fatalf("expected next reservation at %v, got %+v", nextTarget, afterDelivery)
		}
	}

	t.Run("daily_refresh", func(t *testing.T) {
		dailyDB := database.CreateTestDatabase(t, []string{"players", "characters", "character_depots", "character_alchemy", "guilds", "guild_members", "job_popularity_stats", "ranking_snapshots", "weekly_job_changes"}...)
		dailyRepo, err := database.NewRankingRepository(dailyDB)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Date(2025, 1, 2, 6, 0, 0, 0, time.UTC) // 2025-01-02 15:00 JST
		firstTarget := ranking.NextMidnightJST(now)
		nextTarget := ranking.NextMidnightJST(firstTarget)
		actor := "ranking-test-daily:" + id.New()
		isolated := &isolatedRankingSchedRepo{ValkeyRepository: baseRepo, actor: actor, logicalActor: "ranking:daily"}
		t.Cleanup(func() {
			if _, err := isolated.CancelByActorID(context.Background(), actor); err != nil {
				t.Error(err)
			}
		})
		schedSvc := scheduling.NewService(isolated)
		svc, err := ranking.NewService(dailyRepo, ranking.WithNowFunc(func() time.Time { return now }))
		if err != nil {
			t.Fatal(err)
		}
		handler := ranking.NewRefreshHandler(svc, ranking.WithRefreshScheduler(schedSvc))
		verifyRepairAndDelivery(t, ranking.RankingActionTypeRefresh, firstTarget, nextTarget, func() {
			now = firstTarget
		}, handler.Reconcile, handler, schedSvc, isolated)
	})

	t.Run("weekly_rotation", func(t *testing.T) {
		now := time.Date(2025, 1, 4, 15, 0, 0, 0, time.UTC) // Sunday 2025-01-05 00:00 JST
		setWeeklyRotationDue(t, repo, now.AddDate(0, 0, 7))
		firstTarget := now.AddDate(0, 0, 7)
		nextTarget := firstTarget.AddDate(0, 0, 7)
		actor := "ranking-test-weekly:" + id.New()
		isolated := &isolatedRankingSchedRepo{ValkeyRepository: baseRepo, actor: actor, logicalActor: "ranking:weekly"}
		t.Cleanup(func() {
			if _, err := isolated.CancelByActorID(context.Background(), actor); err != nil {
				t.Error(err)
			}
		})
		schedSvc := scheduling.NewService(isolated)
		svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return now }))
		if err != nil {
			t.Fatal(err)
		}
		handler := ranking.NewRotateWeeklyHandler(svc, schedSvc)
		verifyRepairAndDelivery(t, ranking.RankingActionTypeRotateWeekly, firstTarget, nextTarget, func() {
			now = firstTarget
		}, handler.Reconcile, handler, schedSvc, isolated)
	})
}

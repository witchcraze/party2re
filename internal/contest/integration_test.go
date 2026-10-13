package contest_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/contest"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
)

func contestIntegrationDB(t *testing.T) (*sql.DB, *database.ContestRepository, *database.CharacterRepository, database.TransactionProvider) {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	ctx := context.Background()
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
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated contest settlement tests")
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

	// Create an isolated database schema for this test run.
	name := "party2_contest_test_" + id.New()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
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

	// Copy table DDL in dependency order without copying shared data.
	for _, table := range []string{"players", "characters", "character_photos", "contest_rounds", "contest_entries", "contest_votes", "contest_legends"} {
		var tableName, ddl string
		if err := source.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&tableName, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}

	contestRepo, err := database.NewContestRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	return db, contestRepo, charRepo, txProvider
}

func TestContestSettlement_OverduePostpone_DatabaseIntegration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, contestRepo, charRepo, txProvider := contestIntegrationDB(t)

	// Step 1: Delayed recovery case (25 days overdue)
	// Active round absent, round 1 preparing with EndTime 2026-09-18T00:00:00Z, 0 entries
	oldEndTime := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	delayedCheckTime := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)                     // 25 days late
	expectedDelayedEnd := delayedCheckTime.Add(contest.ContestCycleDays * 24 * time.Hour) // 2026-10-23T00:00:00Z

	r1Init := contest.ContestRound{
		Round:     1,
		Status:    contest.StatusPreparing,
		StartTime: oldEndTime.Add(-10 * 24 * time.Hour),
		EndTime:   oldEndTime,
		CreatedAt: oldEndTime.Add(-10 * 24 * time.Hour),
		UpdatedAt: oldEndTime.Add(-10 * 24 * time.Hour),
	}
	if err := contestRepo.SaveRound(ctx, r1Init); err != nil {
		t.Fatalf("SaveRound failed: %v", err)
	}

	svcDelayed, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithNowFunc(func() time.Time { return delayedCheckTime }),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	schedDelayed := &mockScheduler{}
	handlerDelayed := contest.NewContestSettlementHandler(svcDelayed, contest.WithScheduler(schedDelayed))

	actionDelayed := core_scheduling.ScheduledAction{
		ID:          contest.SettlementActionID(1, oldEndTime),
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     "system",
		ScheduledAt: r1Init.StartTime,
		ExecuteAt:   oldEndTime,
		State:       core_scheduling.StatePending,
	}

	if err := handlerDelayed.Handle(ctx, actionDelayed); err != nil {
		t.Fatalf("handler.Handle failed on delayed settlement: %v", err)
	}

	// Reload round 1 directly from database
	r1Reloaded, err := contestRepo.GetRoundByNumber(ctx, 1)
	if err != nil {
		t.Fatalf("GetRoundByNumber failed: %v", err)
	}

	if r1Reloaded.Status != contest.StatusPreparing {
		t.Errorf("expected round 1 still preparing, got %s", r1Reloaded.Status)
	}
	if !r1Reloaded.EndTime.Equal(expectedDelayedEnd) {
		t.Errorf("DB EndTime = %v, want %v (2026-10-23 00:00:00 UTC)", r1Reloaded.EndTime, expectedDelayedEnd)
	}

	// Verify scheduled action with mock scheduler: must be scheduled in the future
	if len(schedDelayed.scheduledActions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(schedDelayed.scheduledActions))
	}
	scheduledDelayedAction := schedDelayed.scheduledActions[0]
	if !scheduledDelayedAction.ExecuteAt.Equal(expectedDelayedEnd) {
		t.Errorf("scheduled ExecuteAt = %v, want %v", scheduledDelayedAction.ExecuteAt, expectedDelayedEnd)
	}
	expectedDelayedActionID := contest.SettlementActionID(1, expectedDelayedEnd)
	if scheduledDelayedAction.ID != expectedDelayedActionID {
		t.Errorf("scheduled action ID = %q, want %q", scheduledDelayedAction.ID, expectedDelayedActionID)
	}
	if !scheduledDelayedAction.ExecuteAt.After(delayedCheckTime) {
		t.Errorf("scheduled ExecuteAt %v is not strictly after check time %v (past catch-up hazard!)", scheduledDelayedAction.ExecuteAt, delayedCheckTime)
	}

	// Step 2: Normal on-time comparison
	// Settle round 1 at expectedDelayedEnd (2026-10-23T00:00:00Z)
	onTimeCheck := expectedDelayedEnd
	expectedOnTimeEnd := onTimeCheck.Add(contest.ContestCycleDays * 24 * time.Hour) // 2026-11-02T00:00:00Z

	svcOnTime, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithNowFunc(func() time.Time { return onTimeCheck }),
	)
	if err != nil {
		t.Fatalf("NewService on-time failed: %v", err)
	}

	schedOnTime := &mockScheduler{}
	handlerOnTime := contest.NewContestSettlementHandler(svcOnTime, contest.WithScheduler(schedOnTime))

	actionOnTime := core_scheduling.ScheduledAction{
		ID:          expectedDelayedActionID,
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     "system",
		ScheduledAt: delayedCheckTime,
		ExecuteAt:   expectedDelayedEnd,
		State:       core_scheduling.StatePending,
	}

	if err := handlerOnTime.Handle(ctx, actionOnTime); err != nil {
		t.Fatalf("handler.Handle failed on on-time settlement: %v", err)
	}

	r1OnTimeReloaded, err := contestRepo.GetRoundByNumber(ctx, 1)
	if err != nil {
		t.Fatalf("GetRoundByNumber failed: %v", err)
	}
	if !r1OnTimeReloaded.EndTime.Equal(expectedOnTimeEnd) {
		t.Errorf("DB EndTime on-time = %v, want %v", r1OnTimeReloaded.EndTime, expectedOnTimeEnd)
	}

	if len(schedOnTime.scheduledActions) != 1 {
		t.Fatalf("expected 1 scheduled action on-time, got %d", len(schedOnTime.scheduledActions))
	}
	scheduledOnTimeAction := schedOnTime.scheduledActions[0]
	if !scheduledOnTimeAction.ExecuteAt.Equal(expectedOnTimeEnd) {
		t.Errorf("scheduled ExecuteAt on-time = %v, want %v", scheduledOnTimeAction.ExecuteAt, expectedOnTimeEnd)
	}
}

package contest_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/contest"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/scheduling"
	vk "github.com/witchcraze/party2re/internal/valkey"
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
	for _, table := range []string{"players", "characters", "character_depots", "character_photos", "contest_rounds", "contest_entries", "contest_votes", "contest_legends"} {
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
	expectedDelayedActionPrefix := contest.SettlementActionID(1, expectedDelayedEnd)
	if !strings.HasPrefix(scheduledDelayedAction.ID, expectedDelayedActionPrefix) {
		t.Errorf("scheduled action ID = %q, want prefix %q", scheduledDelayedAction.ID, expectedDelayedActionPrefix)
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
		ID:          scheduledDelayedAction.ID,
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
	expectedOnTimePrefix := contest.SettlementActionID(1, expectedOnTimeEnd)
	if !strings.HasPrefix(scheduledOnTimeAction.ID, expectedOnTimePrefix) {
		t.Errorf("scheduled action ID = %q, want prefix %q", scheduledOnTimeAction.ID, expectedOnTimePrefix)
	}
}

type statefulSchedRepo struct {
	mu          sync.Mutex
	actions     map[string]core_scheduling.ScheduledAction
	locks       map[string]bool
	failSched   bool
	scheduleErr error
}

func newStatefulSchedRepo() *statefulSchedRepo {
	return &statefulSchedRepo{
		actions: make(map[string]core_scheduling.ScheduledAction),
		locks:   make(map[string]bool),
	}
}

func (r *statefulSchedRepo) Schedule(_ context.Context, action core_scheduling.ScheduledAction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failSched && r.scheduleErr != nil {
		return r.scheduleErr
	}
	r.actions[action.ID] = action
	return nil
}

func (r *statefulSchedRepo) FetchDue(_ context.Context, upTo time.Time, limit int) ([]core_scheduling.ScheduledAction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var due []core_scheduling.ScheduledAction
	for _, a := range r.actions {
		if a.State == core_scheduling.StatePending && !a.ExecuteAt.After(upTo) {
			due = append(due, a)
			if len(due) >= limit {
				break
			}
		}
	}
	return due, nil
}

func (r *statefulSchedRepo) FindPendingByActorID(_ context.Context, actorID string) ([]core_scheduling.ScheduledAction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var res []core_scheduling.ScheduledAction
	for _, a := range r.actions {
		if a.ActorID == actorID && (a.State == core_scheduling.StatePending || a.State == core_scheduling.StateProcessing) {
			res = append(res, a)
		}
	}
	return res, nil
}

func (r *statefulSchedRepo) AcquireLock(_ context.Context, actionID string, _ time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.locks[actionID] {
		return false, nil
	}
	r.locks[actionID] = true
	return true, nil
}

func (r *statefulSchedRepo) Save(_ context.Context, action core_scheduling.ScheduledAction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions[action.ID] = action
	return nil
}

func (r *statefulSchedRepo) CancelByActorID(_ context.Context, actorID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for id, a := range r.actions {
		if a.ActorID == actorID {
			delete(r.actions, id)
			count++
		}
	}
	return count, nil
}

func TestContestSettlement_SchedulingFailureAndReconciliation_DatabaseIntegration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, contestRepo, charRepo, txProvider := contestIntegrationDB(t)

	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	schedRepo := newStatefulSchedRepo()
	schedService := scheduling.NewService(schedRepo)

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithServiceScheduler(schedService),
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	handler := contest.NewContestSettlementHandler(svc, contest.WithScheduler(schedService))
	worker := scheduling.NewWorker(schedRepo, time.Second, logging.Nop())
	worker.RegisterHandler(contest.ActionTypeContestSettlement, handler)

	// Setup: insert player and character into real MariaDB
	char, err := database.CreateTestCharacter(ctx, db, "Photographer")
	if err != nil {
		t.Fatalf("CreateTestCharacter failed: %v", err)
	}

	photo, err := svc.SavePhoto(ctx, char.ID, "GrandLandscape", "Forest", "http://example.com/forest.jpg", "Majestic tree", "")
	if err != nil {
		t.Fatalf("SavePhoto failed: %v", err)
	}

	// -------------------------------------------------------------
	// Step 1: EnterContest with scheduler failure injected
	// -------------------------------------------------------------
	schedRepo.failSched = true
	schedRepo.scheduleErr = errors.New("queue unavailable")

	entry, enterErr := svc.EnterContest(ctx, char.ID, photo.ID, "GrandLandscape")
	if enterErr == nil {
		t.Fatal("expected ErrScheduleFailed from EnterContest when scheduler fails, got nil")
	}
	if !errors.Is(enterErr, contest.ErrScheduleFailed) {
		t.Fatalf("expected ErrScheduleFailed, got: %v", enterErr)
	}
	if entry.ID == "" {
		t.Fatal("expected entry to be returned non-empty, got empty entry")
	}

	// MariaDB verification: entry row exists
	savedEntry, findErr := contestRepo.FindEntryByRoundAndCharacter(ctx, entry.Round, char.ID)
	if findErr != nil {
		t.Fatalf("expected entry to exist in MariaDB, got: %v", findErr)
	}
	if savedEntry.ID != entry.ID {
		t.Errorf("saved entry ID = %q, want %q", savedEntry.ID, entry.ID)
	}

	// MariaDB verification: duplicate submission rejected (prevents double entry)
	_, retryErr := svc.EnterContest(ctx, char.ID, photo.ID, "AnotherTitle")
	if !errors.Is(retryErr, contest.ErrAlreadyEntered) {
		t.Fatalf("expected ErrAlreadyEntered on retry, got: %v", retryErr)
	}

	// Queue verification: no action was enqueued due to failure
	if len(schedRepo.actions) != 0 {
		t.Fatalf("expected 0 actions in queue after failure, got %d", len(schedRepo.actions))
	}

	// -------------------------------------------------------------
	// Step 2: Settlement check execution with scheduler failure
	// Round 1 is preparing with 1 entry (< quorum 5), EndTime arrives (now)
	// -------------------------------------------------------------
	r1, err := contestRepo.GetRoundByNumber(ctx, 1)
	if err != nil {
		t.Fatalf("GetRoundByNumber failed: %v", err)
	}
	r1.EndTime = now
	if err := contestRepo.SaveRound(ctx, r1); err != nil {
		t.Fatalf("SaveRound failed: %v", err)
	}

	dueAction := core_scheduling.ScheduledAction{
		ID:          contest.SettlementActionID(1, now),
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     "system",
		ScheduledAt: now.Add(-10 * 24 * time.Hour),
		ExecuteAt:   now,
		State:       core_scheduling.StatePending,
	}
	schedRepo.actions[dueAction.ID] = dueAction

	// Worker processes action while next schedule fails
	worker.ProcessAction(ctx, dueAction)

	// MariaDB verification: round 1 was postponed and extended to now+10 days
	r1Postponed, err := contestRepo.GetRoundByNumber(ctx, 1)
	if err != nil {
		t.Fatalf("GetRoundByNumber failed: %v", err)
	}
	expectedPostponedEnd := now.Add(contest.ContestCycleDays * 24 * time.Hour)
	if !r1Postponed.EndTime.Equal(expectedPostponedEnd) {
		t.Errorf("MariaDB round 1 EndTime = %v, want %v", r1Postponed.EndTime, expectedPostponedEnd)
	}

	// Queue verification: the processed action is marked Failed (terminal state)
	finalProcessedAction := schedRepo.actions[dueAction.ID]
	if finalProcessedAction.State != core_scheduling.StateFailed {
		t.Errorf("expected due action state to be %s, got %s", core_scheduling.StateFailed, finalProcessedAction.State)
	}

	// -------------------------------------------------------------
	// Step 3: Disaster Recovery / Reconciliation
	// Scheduler recovers, ReconcileSettlementSchedule restores next reservation
	// -------------------------------------------------------------
	schedRepo.failSched = false
	schedRepo.scheduleErr = nil

	if err := svc.ReconcileSettlementSchedule(ctx); err != nil {
		t.Fatalf("ReconcileSettlementSchedule failed: %v", err)
	}

	expectedNextActionPrefix := contest.SettlementActionID(1, expectedPostponedEnd)
	var nextAction *core_scheduling.ScheduledAction
	for _, a := range schedRepo.actions {
		if strings.HasPrefix(a.ID, expectedNextActionPrefix) {
			nextAction = &a
			break
		}
	}
	if nextAction == nil {
		t.Fatalf("expected next scheduled action with prefix %q in queue, none found", expectedNextActionPrefix)
	}
	if nextAction.State != core_scheduling.StatePending {
		t.Errorf("expected next action state %s, got %s", core_scheduling.StatePending, nextAction.State)
	}
	if !nextAction.ExecuteAt.Equal(expectedPostponedEnd) {
		t.Errorf("expected next action ExecuteAt = %v, want %v", nextAction.ExecuteAt, expectedPostponedEnd)
	}

	// Reconcile must NOT duplicate entries, prizes, or alter MariaDB round
	r1AfterReconcile, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if !r1AfterReconcile.EndTime.Equal(expectedPostponedEnd) {
		t.Errorf("round 1 EndTime changed during reconcile: %v", r1AfterReconcile.EndTime)
	}
	entries, _ := contestRepo.ListEntriesByRound(ctx, 1)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry in round 1, got %d", len(entries))
	}

	// -------------------------------------------------------------
	// Step 4: Defense against replay of stale/old actions
	// Dispatching the old action (dueAction with now) again must NOT re-postpone
	// -------------------------------------------------------------
	err = handler.Handle(ctx, dueAction)
	if err != nil {
		t.Fatalf("expected stale action handling to return nil, got: %v", err)
	}
	r1AfterStale, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if !r1AfterStale.EndTime.Equal(expectedPostponedEnd) {
		t.Errorf("stale action altered round 1 EndTime: %v, want %v", r1AfterStale.EndTime, expectedPostponedEnd)
	}
}

const contestTestValkeyDB = 11

func openIsolatedContestTestValkey(t *testing.T, dbIndex int) valkey.Client {
	t.Helper()
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}

	cfg := vk.ConfigFromEnvironment()
	cfg.DB = dbIndex
	client, err := vk.NewClientWithConfig(cfg)
	if err != nil {
		t.Fatalf("open isolated valkey client (DB %d): %v", dbIndex, err)
	}
	return client
}

func TestContestSettlement_RealValkey_StateTransitionsAndReconciliation_Integration(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	ctx := context.Background()
	_, contestRepo, charRepo, txProvider := contestIntegrationDB(t)

	vkClient := openIsolatedContestTestValkey(t, contestTestValkeyDB)
	t.Cleanup(vkClient.Close)

	// Clean up ONLY the isolated test DB 11 at start and cleanup
	_ = vkClient.Do(ctx, vkClient.B().Flushdb().Build())
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_ = vkClient.Do(cleanupCtx, vkClient.B().Flushdb().Build())
	})

	schedRepo := scheduling.NewValkeyRepository(vkClient)
	schedService := scheduling.NewService(schedRepo)

	now := time.Now().UTC().Truncate(time.Second)
	roundEndTime := now.Add(7 * 24 * time.Hour)
	r1 := contest.ContestRound{
		Round:     1,
		Status:    contest.StatusPreparing,
		StartTime: now,
		EndTime:   roundEndTime,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := contestRepo.SaveRound(ctx, r1); err != nil {
		t.Fatalf("SaveRound failed: %v", err)
	}

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithServiceScheduler(schedService),
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// -------------------------------------------------------------
	// 1. Initial Reconciliation: creates Pending action in Valkey
	// -------------------------------------------------------------
	if err := svc.ReconcileSettlementSchedule(ctx); err != nil {
		t.Fatalf("ReconcileSettlementSchedule failed: %v", err)
	}

	actions, err := schedRepo.FindPendingByActorID(ctx, contest.ContestSettlementActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 pending action in Valkey, got %d", len(actions))
	}
	act1 := actions[0]
	if act1.State != core_scheduling.StatePending {
		t.Errorf("expected StatePending, got %s", act1.State)
	}
	if !act1.ExecuteAt.Equal(roundEndTime) {
		t.Errorf("expected ExecuteAt = %v, got %v", roundEndTime, act1.ExecuteAt)
	}
	if !strings.HasPrefix(act1.ID, contest.SettlementActionID(1, roundEndTime)) {
		t.Errorf("act1 ID %q missing prefix", act1.ID)
	}

	// -------------------------------------------------------------
	// 2. Idempotent Reconciliation: existing Pending action preserved, no duplicate
	// -------------------------------------------------------------
	if err := svc.ReconcileSettlementSchedule(ctx); err != nil {
		t.Fatalf("duplicate ReconcileSettlementSchedule failed: %v", err)
	}
	actions2, err := schedRepo.FindPendingByActorID(ctx, contest.ContestSettlementActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID after duplicate failed: %v", err)
	}
	if len(actions2) != 1 {
		t.Fatalf("expected still 1 action, got %d", len(actions2))
	}
	if actions2[0].ID != act1.ID {
		t.Errorf("action ID changed on idempotent reconcile: %q vs %q", actions2[0].ID, act1.ID)
	}

	// -------------------------------------------------------------
	// 2b. Concurrent Reconciliation: multiple goroutines racing simultaneously
	//     must not create duplicate actions.
	// -------------------------------------------------------------
	var wg sync.WaitGroup
	errCh := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.ReconcileSettlementSchedule(ctx); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent ReconcileSettlementSchedule error: %v", err)
	}
	actionsConcurrent, err := schedRepo.FindPendingByActorID(ctx, contest.ContestSettlementActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID after concurrent reconcile failed: %v", err)
	}
	if len(actionsConcurrent) != 1 {
		t.Fatalf("expected exactly 1 action after concurrent reconcile, got %d", len(actionsConcurrent))
	}

	// -------------------------------------------------------------
	// 2c. Legacy Actor ID ("system") compatibility:
	//     If a reservation was scheduled before upgrade under actor "system",
	//     reconciliation must recognize it and avoid creating duplicate actions.
	// -------------------------------------------------------------
	legacyEndTime := roundEndTime.Add(24 * time.Hour)
	rLegacy := contest.ContestRound{
		Round:     2,
		Status:    contest.StatusActive,
		StartTime: now,
		EndTime:   legacyEndTime,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := contestRepo.SaveRound(ctx, rLegacy); err != nil {
		t.Fatalf("SaveRound legacy failed: %v", err)
	}
	legacyActionID := contest.SettlementActionID(2, legacyEndTime)
	legacyAction := core_scheduling.ScheduledAction{
		ID:          legacyActionID,
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     "system",
		ScheduledAt: now,
		ExecuteAt:   legacyEndTime,
		State:       core_scheduling.StatePending,
	}
	if err := schedRepo.Schedule(ctx, legacyAction); err != nil {
		t.Fatalf("Schedule legacy action failed: %v", err)
	}
	if err := svc.ReconcileSettlementSchedule(ctx); err != nil {
		t.Fatalf("ReconcileSettlementSchedule with legacy action failed: %v", err)
	}
	pendingSys, err := schedRepo.FindPendingByActorID(ctx, "system")
	if err != nil {
		t.Fatalf("FindPendingByActorID system failed: %v", err)
	}
	if len(pendingSys) != 1 || pendingSys[0].ID != legacyActionID {
		t.Errorf("expected 1 legacy action under system, got %v", pendingSys)
	}
	pendingContest, err := schedRepo.FindPendingByActorID(ctx, contest.ContestSettlementActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID contest failed: %v", err)
	}
	for _, a := range pendingContest {
		if a.ExecuteAt.Equal(legacyEndTime) {
			t.Errorf("duplicate action scheduled under contest:settlement for legacy round: %s", a.ID)
		}
	}
	// Clean up legacy round so round 1 remains target
	rLegacy.Status = contest.StatusSettled
	_ = contestRepo.SaveRound(ctx, rLegacy)
	_ = vkClient.Do(ctx, vkClient.B().Del().Key("party2:scheduled:action:"+legacyActionID).Build())
	_ = vkClient.Do(ctx, vkClient.B().Del().Key("party2:scheduled:actor:system").Build())
	_ = vkClient.Do(ctx, vkClient.B().Zrem().Key("party2:scheduled:pending").Member(legacyActionID).Build())

	// -------------------------------------------------------------
	// 3. Processing State Protection: action being processed is NOT overwritten
	// -------------------------------------------------------------
	act1.State = core_scheduling.StateProcessing
	if err := schedRepo.Save(ctx, act1); err != nil {
		t.Fatalf("Save processing action failed: %v", err)
	}

	if err := svc.ReconcileSettlementSchedule(ctx); err != nil {
		t.Fatalf("ReconcileSettlementSchedule during processing failed: %v", err)
	}
	actionsProc, err := schedRepo.FindPendingByActorID(ctx, contest.ContestSettlementActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID during processing failed: %v", err)
	}
	if len(actionsProc) != 1 {
		t.Fatalf("expected 1 action during processing, got %d", len(actionsProc))
	}
	if actionsProc[0].ID != act1.ID {
		t.Errorf("processing action overwritten or replaced: %q vs %q", actionsProc[0].ID, act1.ID)
	}
	if actionsProc[0].State != core_scheduling.StateProcessing {
		t.Errorf("expected action to remain StateProcessing, got %s", actionsProc[0].State)
	}

	// -------------------------------------------------------------
	// 4. Completed / Failed Protection: terminal action is NOT resurrected to Pending,
	//    and missing reservation is safely recreated with a distinct ID.
	// -------------------------------------------------------------
	act1.State = core_scheduling.StateFailed
	act1.RetainUntil = now.Add(24 * time.Hour)
	if err := schedRepo.Save(ctx, act1); err != nil {
		t.Fatalf("Save failed action failed: %v", err)
	}

	// Valkey check: act1 key is still in StateFailed
	storedStr, err := vkClient.Do(ctx, vkClient.B().Get().Key("party2:scheduled:action:"+act1.ID).Build()).ToString()
	if err != nil {
		t.Fatalf("Get act1 action failed: %v", err)
	}
	if !strings.Contains(storedStr, string(core_scheduling.StateFailed)) {
		t.Fatalf("stored act1 payload should have StateFailed, got: %s", storedStr)
	}

	// Reconciliation should notice that there are no pending/processing actions,
	// and register a NEW reservation with fresh ID without overwriting act1
	if err := svc.ReconcileSettlementSchedule(ctx); err != nil {
		t.Fatalf("ReconcileSettlementSchedule after failure failed: %v", err)
	}

	actionsAfterFail, err := schedRepo.FindPendingByActorID(ctx, contest.ContestSettlementActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID after failure failed: %v", err)
	}
	if len(actionsAfterFail) != 1 {
		t.Fatalf("expected 1 new pending action, got %d", len(actionsAfterFail))
	}
	act2 := actionsAfterFail[0]
	if act2.ID == act1.ID {
		t.Errorf("expected fresh action ID for recovery, but got same ID %q", act2.ID)
	}
	if act2.State != core_scheduling.StatePending {
		t.Errorf("expected new action state StatePending, got %s", act2.State)
	}

	// Crucial check: old act1 was NOT resurrected back to Pending!
	storedStrOld, err := vkClient.Do(ctx, vkClient.B().Get().Key("party2:scheduled:action:"+act1.ID).Build()).ToString()
	if err != nil {
		t.Fatalf("Get old act1 action failed: %v", err)
	}
	if !strings.Contains(storedStrOld, string(core_scheduling.StateFailed)) {
		t.Errorf("old act1 was corrupted/overwritten by reconciliation! Got: %s", storedStrOld)
	}
}

func TestContestSettlement_AdminSettle_ScheduleFailureReturnsCommittedResult_Integration(t *testing.T) {
	ctx := context.Background()
	_, contestRepo, charRepo, txProvider := contestIntegrationDB(t)

	now := time.Now().UTC().Truncate(time.Second)
	schedRepo := newStatefulSchedRepo()
	schedService := scheduling.NewService(schedRepo)

	r1 := contest.ContestRound{
		Round:     1,
		Status:    contest.StatusPreparing,
		StartTime: now.Add(-10 * 24 * time.Hour),
		EndTime:   now,
		CreatedAt: now.Add(-10 * 24 * time.Hour),
		UpdatedAt: now.Add(-10 * 24 * time.Hour),
	}
	if err := contestRepo.SaveRound(ctx, r1); err != nil {
		t.Fatalf("SaveRound failed: %v", err)
	}

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithServiceScheduler(schedService),
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// Inject scheduling failure
	schedRepo.failSched = true
	schedRepo.scheduleErr = errors.New("queue unavailable")

	result, settleErr := svc.SettleContest(ctx, true)
	if settleErr == nil || !errors.Is(settleErr, contest.ErrScheduleFailed) {
		t.Fatalf("expected ErrScheduleFailed from SettleContest on schedule error, got %v", settleErr)
	}
	if result.Round != 1 {
		t.Errorf("expected result.Round = 1, got %d", result.Round)
	}

	// MariaDB verification: round was postponed/saved despite schedule error (SQL committed!)
	r1After, err := contestRepo.GetRoundByNumber(ctx, 1)
	if err != nil {
		t.Fatalf("GetRoundByNumber failed: %v", err)
	}
	if r1After.EndTime.Equal(now) {
		t.Errorf("round 1 EndTime was not updated, SQL failed to commit!")
	}
}

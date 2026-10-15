package contest_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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

func openContestTestValkey(t *testing.T) valkey.Client {
	t.Helper()
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}

	client, err := vk.NewClient()
	if err != nil {
		t.Fatalf("open valkey client: %v", err)
	}
	return client
}

func cleanupValkeyAction(ctx context.Context, client valkey.Client, actorID, actionID string) {
	_ = client.Do(ctx, client.B().Del().Key("party2:scheduled:action:"+actionID).Build())
	_ = client.Do(ctx, client.B().Del().Key("party2:scheduled:lock:"+actionID).Build())
	_ = client.Do(ctx, client.B().Zrem().Key("party2:scheduled:pending").Member(actionID).Build())
	if actorID != "" {
		_ = client.Do(ctx, client.B().Srem().Key("party2:scheduled:actor:"+actorID).Member(actionID).Build())
	}
}

func cleanupValkeyActor(ctx context.Context, client valkey.Client, actorID string, extraActionIDs ...string) {
	members, err := client.Do(ctx, client.B().Smembers().Key("party2:scheduled:actor:"+actorID).Build()).AsStrSlice()
	if err == nil {
		for _, m := range members {
			cleanupValkeyAction(ctx, client, actorID, m)
		}
	}
	for _, actID := range extraActionIDs {
		if actID != "" {
			cleanupValkeyAction(ctx, client, actorID, actID)
		}
	}
	_ = client.Do(ctx, client.B().Del().Key("party2:scheduled:actor:"+actorID).Build())
}

func TestContestSettlement_RealValkey_StateTransitionsAndReconciliation_Integration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, contestRepo, charRepo, txProvider := contestIntegrationDB(t)

	vkClient := openContestTestValkey(t)
	t.Cleanup(vkClient.Close)

	testActorID := fmt.Sprintf("contest:test:%s", id.New())
	var trackedActions []string
	var trackMu sync.Mutex
	trackAction := func(actionID string) {
		trackMu.Lock()
		defer trackMu.Unlock()
		trackedActions = append(trackedActions, actionID)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		trackMu.Lock()
		ids := append([]string(nil), trackedActions...)
		trackMu.Unlock()
		cleanupValkeyActor(cleanupCtx, vkClient, testActorID, ids...)
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
		contest.WithSettlementActorID(testActorID),
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

	actions, err := schedRepo.FindPendingByActorID(ctx, testActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 pending action in Valkey, got %d", len(actions))
	}
	act1 := actions[0]
	trackAction(act1.ID)
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
	actions2, err := schedRepo.FindPendingByActorID(ctx, testActorID)
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
	actionsConcurrent, err := schedRepo.FindPendingByActorID(ctx, testActorID)
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
	legacyActionID := fmt.Sprintf("%s:%s", contest.SettlementActionID(2, legacyEndTime), id.New())
	t.Cleanup(func() {
		cleanupValkeyAction(context.Background(), vkClient, "system", legacyActionID)
	})
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
	var foundSys bool
	for _, a := range pendingSys {
		if a.ID == legacyActionID {
			foundSys = true
			break
		}
	}
	if !foundSys {
		t.Errorf("expected legacy action %q under system, got %v", legacyActionID, pendingSys)
	}
	pendingContest, err := schedRepo.FindPendingByActorID(ctx, testActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID contest failed: %v", err)
	}
	for _, a := range pendingContest {
		if a.ExecuteAt.Equal(legacyEndTime) {
			t.Errorf("duplicate action scheduled under %s for legacy round: %s", testActorID, a.ID)
		}
	}
	// Clean up legacy round so round 1 remains target
	rLegacy.Status = contest.StatusSettled
	_ = contestRepo.SaveRound(ctx, rLegacy)
	cleanupValkeyAction(ctx, vkClient, "system", legacyActionID)

	// -------------------------------------------------------------
	// 2d. Different Round with Matching EndTime Not Confused:
	//     An existing pending action for a different round (e.g. Round 99)
	//     with matching EndTime must NOT be mistaken for Round 1!
	// -------------------------------------------------------------
	otherActionID := fmt.Sprintf("%s:%s", contest.SettlementActionID(99, roundEndTime), id.New())
	trackAction(otherActionID)
	otherAction := core_scheduling.ScheduledAction{
		ID:          otherActionID,
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     testActorID,
		ScheduledAt: now,
		ExecuteAt:   roundEndTime,
		State:       core_scheduling.StatePending,
	}
	if err := schedRepo.Schedule(ctx, otherAction); err != nil {
		t.Fatalf("Schedule other round action failed: %v", err)
	}
	// Temporarily remove act1 from Valkey so round 1 has no reservation
	cleanupValkeyAction(ctx, vkClient, testActorID, act1.ID)

	// Reconcile must recognize Round 1 needs a reservation despite Round 99's action sharing EndTime
	if err := svc.ReconcileSettlementSchedule(ctx); err != nil {
		t.Fatalf("ReconcileSettlementSchedule with different round action failed: %v", err)
	}
	actionsAfter2d, err := schedRepo.FindPendingByActorID(ctx, testActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID after step 2d failed: %v", err)
	}
	var newAct1 *core_scheduling.ScheduledAction
	for _, a := range actionsAfter2d {
		if strings.HasPrefix(a.ID, contest.SettlementActionID(1, roundEndTime)) {
			temp := a
			newAct1 = &temp
			break
		}
	}
	if newAct1 == nil {
		t.Fatalf("expected new action scheduled for round 1, but found none (mistaken for round 99!) in %v", actionsAfter2d)
	}
	trackAction(newAct1.ID)
	act1 = *newAct1
	// Clean up Round 99 action
	cleanupValkeyAction(ctx, vkClient, testActorID, otherActionID)

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
	actionsProc, err := schedRepo.FindPendingByActorID(ctx, testActorID)
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

	actionsAfterFail, err := schedRepo.FindPendingByActorID(ctx, testActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID after failure failed: %v", err)
	}
	if len(actionsAfterFail) != 1 {
		t.Fatalf("expected 1 new pending action, got %d", len(actionsAfterFail))
	}
	act2 := actionsAfterFail[0]
	trackAction(act2.ID)
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

func TestContestSettlement_IndependentServices_ConcurrencyAndDuplicateActionSafety_Integration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, contestRepo, charRepo, txProvider := contestIntegrationDB(t)

	vkClient := openContestTestValkey(t)
	t.Cleanup(vkClient.Close)

	testActorID := fmt.Sprintf("contest:test:%s", id.New())
	var trackedActions []string
	var trackMu sync.Mutex
	trackAction := func(actionID string) {
		trackMu.Lock()
		defer trackMu.Unlock()
		trackedActions = append(trackedActions, actionID)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		trackMu.Lock()
		ids := append([]string(nil), trackedActions...)
		trackMu.Unlock()
		cleanupValkeyActor(cleanupCtx, vkClient, testActorID, ids...)
	})

	schedRepo := scheduling.NewValkeyRepository(vkClient)
	schedService := scheduling.NewService(schedRepo)

	now := time.Now().UTC().Truncate(time.Second)
	round1EndTime := now.Add(-1 * time.Hour) // Past settlement deadline

	// Setup Round 1 (Active)
	r1 := contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: round1EndTime.Add(-7 * 24 * time.Hour),
		EndTime:   round1EndTime,
		CreatedAt: now.Add(-7 * 24 * time.Hour),
		UpdatedAt: now.Add(-7 * 24 * time.Hour),
	}
	if err := contestRepo.SaveRound(ctx, r1); err != nil {
		t.Fatalf("SaveRound 1 failed: %v", err)
	}

	// Setup Round 2 (Preparing) with quorum so round 1 concludes and round 2 promotes to active
	round2EndTime := round1EndTime.Add(7 * 24 * time.Hour)
	r2 := contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: round1EndTime,
		EndTime:   round2EndTime,
		CreatedAt: now.Add(-7 * 24 * time.Hour),
		UpdatedAt: now.Add(-7 * 24 * time.Hour),
	}
	if err := contestRepo.SaveRound(ctx, r2); err != nil {
		t.Fatalf("SaveRound 2 failed: %v", err)
	}

	// Create 5 contestants with initial 0 funds
	chars := make([]corecharacter.Character, 5)
	for i := 0; i < 5; i++ {
		c, err := database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("ConcHero%d_%s", i, id.New()[:6]), 0)
		if err != nil {
			t.Fatalf("CreateTestCharacterWithFunds %d failed: %v", i, err)
		}
		chars[i] = c

		// Round 1 entries (descending votes: Char 0 has 10, Char 1 has 9, Char 2 has 8...)
		entryR1 := contest.ContestEntry{
			ID:            id.New(),
			Round:         1,
			CharacterID:   c.ID,
			CharacterName: c.Name,
			Title:         fmt.Sprintf("R1 Entry %d", i),
			Votes:         10 - i,
			CreatedAt:     round1EndTime.Add(-time.Duration(10-i) * time.Hour),
			UpdatedAt:     round1EndTime.Add(-time.Duration(10-i) * time.Hour),
		}
		if err := contestRepo.SaveEntry(ctx, entryR1); err != nil {
			t.Fatalf("SaveEntry round 1 %d failed: %v", i, err)
		}

		// Round 2 entries (quorum met)
		entryR2 := contest.ContestEntry{
			ID:            id.New(),
			Round:         2,
			CharacterID:   c.ID,
			CharacterName: c.Name,
			Title:         fmt.Sprintf("R2 Entry %d", i),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := contestRepo.SaveEntry(ctx, entryR2); err != nil {
			t.Fatalf("SaveEntry round 2 %d failed: %v", i, err)
		}
	}

	// Create two completely INDEPENDENT Service instances sharing storage
	svc1, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithServiceScheduler(schedService),
		contest.WithSettlementActorID(testActorID),
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewService 1 failed: %v", err)
	}
	svc2, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithServiceScheduler(schedService),
		contest.WithSettlementActorID(testActorID),
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewService 2 failed: %v", err)
	}

	// 1. Concurrent reconcile across independent services: verify all return values
	var wg sync.WaitGroup
	reconcileErrs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		reconcileErrs <- svc1.ReconcileSettlementSchedule(ctx)
	}()
	go func() {
		defer wg.Done()
		reconcileErrs <- svc2.ReconcileSettlementSchedule(ctx)
	}()
	wg.Wait()
	close(reconcileErrs)

	for err := range reconcileErrs {
		if err != nil {
			t.Fatalf("concurrent ReconcileSettlementSchedule returned error: %v", err)
		}
	}

	// 2. Independent verification of Reconcile Success:
	// Verify that concurrent reconcile succeeded in scheduling at least 1 valid action for Round 1
	// without relying on manual injection or error swallowing.
	reconciled, err := schedRepo.FindPendingByActorID(ctx, testActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID failed: %v", err)
	}
	for _, a := range reconciled {
		trackAction(a.ID)
	}

	var round1ReconciledActions []core_scheduling.ScheduledAction
	for _, a := range reconciled {
		if strings.HasPrefix(a.ID, contest.SettlementActionID(1, round1EndTime)) {
			round1ReconciledActions = append(round1ReconciledActions, a)
		}
	}
	if len(round1ReconciledActions) == 0 {
		t.Fatalf("expected concurrent reconcile to schedule at least 1 pending action for round 1, found %d (reconcile failed!)", len(round1ReconciledActions))
	}
	for _, a := range round1ReconciledActions {
		if a.State != core_scheduling.StatePending {
			t.Errorf("reconciled action state = %s, want StatePending", a.State)
		}
		if !a.ExecuteAt.Equal(round1EndTime) {
			t.Errorf("reconciled action ExecuteAt = %v, want %v", a.ExecuteAt, round1EndTime)
		}
	}

	// 3. Test duplicate action safety independently:
	// Ensure that even when duplicate scheduled actions exist in Valkey for Round 1,
	// concurrent worker execution processes them idempotently without double-settling or double-awarding.
	round1ExecutionActions := append([]core_scheduling.ScheduledAction(nil), round1ReconciledActions...)
	if len(round1ExecutionActions) < 2 {
		extraID := fmt.Sprintf("%s:%s", contest.SettlementActionID(1, round1EndTime), id.New())
		trackAction(extraID)
		extraAct := core_scheduling.ScheduledAction{
			ID:         extraID,
			ActionType: contest.ActionTypeContestSettlement,
			ActorID:    testActorID,
			ExecuteAt:  round1EndTime,
			State:      core_scheduling.StatePending,
		}
		if err := schedRepo.Schedule(ctx, extraAct); err != nil {
			t.Fatalf("Schedule extraAct failed: %v", err)
		}
		round1ExecutionActions = append(round1ExecutionActions, extraAct)
	}

	h1 := contest.NewContestSettlementHandler(svc1, contest.WithScheduler(schedService))
	h2 := contest.NewContestSettlementHandler(svc2, contest.WithScheduler(schedService))
	worker1 := scheduling.NewWorker(schedRepo, time.Second, logging.Nop())
	worker1.RegisterHandler(contest.ActionTypeContestSettlement, h1)
	worker2 := scheduling.NewWorker(schedRepo, time.Second, logging.Nop())
	worker2.RegisterHandler(contest.ActionTypeContestSettlement, h2)

	// Execute duplicate actions concurrently through independent workers
	for i := 0; i < len(round1ExecutionActions); i += 2 {
		actA := round1ExecutionActions[i]
		hasActB := i+1 < len(round1ExecutionActions)
		wg.Add(1)
		go func(a core_scheduling.ScheduledAction) {
			defer wg.Done()
			worker1.ProcessAction(ctx, a)
		}(actA)
		if hasActB {
			actB := round1ExecutionActions[i+1]
			wg.Add(1)
			go func(b core_scheduling.ScheduledAction) {
				defer wg.Done()
				worker2.ProcessAction(ctx, b)
			}(actB)
		}
		wg.Wait()
	}

	// 4. Assertions on MariaDB state:
	// Round 1 must be settled exactly once
	r1After, err := contestRepo.GetRoundByNumber(ctx, 1)
	if err != nil {
		t.Fatalf("GetRoundByNumber 1 failed: %v", err)
	}
	if r1After.Status != contest.StatusSettled {
		t.Errorf("round 1 status = %s, want settled", r1After.Status)
	}

	// Round 2 promoted to active
	r2After, err := contestRepo.GetRoundByNumber(ctx, 2)
	if err != nil {
		t.Fatalf("GetRoundByNumber 2 failed: %v", err)
	}
	if r2After.Status != contest.StatusActive {
		t.Errorf("round 2 status = %s, want active", r2After.Status)
	}

	// Exactly 1 winner legend archived for Round 1
	legends, totalLegends, err := contestRepo.ListLegends(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListLegends failed: %v", err)
	}
	if totalLegends != 1 || len(legends) != 1 {
		t.Fatalf("expected exactly 1 legend recorded, got total=%d len=%d", totalLegends, len(legends))
	}
	if legends[0].Round != 1 || legends[0].CharacterID != chars[0].ID {
		t.Errorf("legend record mismatch: round=%d char=%s, want round=1 char=%s", legends[0].Round, legends[0].CharacterID, chars[0].ID)
	}

	// ZERO duplicate prize awards:
	// 1st place: exactly 10,000 gold and 3 small medals
	c0After, err := charRepo.FindByID(ctx, chars[0].ID)
	if err != nil {
		t.Fatalf("FindByID char 0 failed: %v", err)
	}
	if c0After.Money != contest.PrizeFirst.Gold {
		t.Errorf("char 0 money = %d, want %d (DUPLICATE REWARDS DETECTED!)", c0After.Money, contest.PrizeFirst.Gold)
	}
	if c0After.SmallMedals != contest.PrizeFirst.SmallMedals {
		t.Errorf("char 0 medals = %d, want %d", c0After.SmallMedals, contest.PrizeFirst.SmallMedals)
	}

	// 2nd place: exactly 5,000 gold and 2 medals
	c1After, err := charRepo.FindByID(ctx, chars[1].ID)
	if err != nil {
		t.Fatalf("FindByID char 1 failed: %v", err)
	}
	if c1After.Money != contest.PrizeSecond.Gold {
		t.Errorf("char 1 money = %d, want %d", c1After.Money, contest.PrizeSecond.Gold)
	}

	// 3rd place: exactly 3,000 gold and 1 medal
	c2After, err := charRepo.FindByID(ctx, chars[2].ID)
	if err != nil {
		t.Fatalf("FindByID char 2 failed: %v", err)
	}
	if c2After.Money != contest.PrizeThird.Gold {
		t.Errorf("char 2 money = %d, want %d", c2After.Money, contest.PrizeThird.Gold)
	}

	// 5. Assertions on Valkey state:
	// A. No pending actions remain for Round 1
	pendingActions, err := schedRepo.FindPendingByActorID(ctx, testActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID failed: %v", err)
	}
	for _, a := range pendingActions {
		if strings.HasPrefix(a.ID, contest.SettlementActionID(1, round1EndTime)) {
			t.Errorf("stale pending action found for round 1: %s", a.ID)
		}
	}

	// B. Valid pending reservation exists for Round 2, exactly matching latest SQL round state
	var round2Reservation *core_scheduling.ScheduledAction
	expectedRound2Prefix := contest.SettlementActionID(r2After.Round, r2After.EndTime)
	for _, a := range pendingActions {
		if strings.HasPrefix(a.ID, expectedRound2Prefix) {
			temp := a
			round2Reservation = &temp
			trackAction(a.ID)
			break
		}
	}
	if round2Reservation == nil {
		t.Fatalf("expected valid next reservation for round %d with EndTime %v in Valkey, but none found in %v", r2After.Round, r2After.EndTime, pendingActions)
	}
	if !round2Reservation.ExecuteAt.Equal(r2After.EndTime) {
		t.Errorf("next reservation ExecuteAt = %v, want %v matching SQL EndTime", round2Reservation.ExecuteAt, r2After.EndTime)
	}
	if round2Reservation.State != core_scheduling.StatePending {
		t.Errorf("next reservation State = %s, want StatePending", round2Reservation.State)
	}

	// 6. Verification: Even if an old round pending reservation survives or arrives late,
	// executing it must not disrupt automated progress, replay settlement, or overwrite Round 2 reservation.
	staleRound1ActionID := fmt.Sprintf("%s:%s", contest.SettlementActionID(1, round1EndTime), id.New())
	trackAction(staleRound1ActionID)
	staleRound1Action := core_scheduling.ScheduledAction{
		ID:         staleRound1ActionID,
		ActionType: contest.ActionTypeContestSettlement,
		ActorID:    testActorID,
		ExecuteAt:  round1EndTime,
		State:      core_scheduling.StatePending,
	}
	if err := schedRepo.Schedule(ctx, staleRound1Action); err != nil {
		t.Fatalf("Schedule stale round 1 action failed: %v", err)
	}

	worker1.ProcessAction(ctx, staleRound1Action)

	// SQL state unchanged: Round 1 still settled, Char 0 funds unchanged
	c0AfterStale, err := charRepo.FindByID(ctx, chars[0].ID)
	if err != nil {
		t.Fatalf("FindByID char 0 after stale action failed: %v", err)
	}
	if c0AfterStale.Money != contest.PrizeFirst.Gold {
		t.Errorf("char 0 money altered by stale action = %d, want %d", c0AfterStale.Money, contest.PrizeFirst.Gold)
	}

	// Valkey state unchanged: Round 2 next reservation still present and pending
	actionsAfterStale, err := schedRepo.FindPendingByActorID(ctx, testActorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID after stale action failed: %v", err)
	}
	var round2StillPresent bool
	for _, a := range actionsAfterStale {
		if strings.HasPrefix(a.ID, expectedRound2Prefix) && a.State == core_scheduling.StatePending {
			round2StillPresent = true
			break
		}
	}
	if !round2StillPresent {
		t.Fatalf("Round 2 next reservation was disrupted or overwritten by stale round 1 action!")
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

func TestContestSettlement_GetOverview_SchedulingFailureAndReconciliation_DatabaseIntegration(t *testing.T) {
	ctx := context.Background()
	_, contestRepo, charRepo, txProvider := contestIntegrationDB(t)

	now := time.Now().UTC().Truncate(time.Second)
	schedRepo := newStatefulSchedRepo()
	schedService := scheduling.NewService(schedRepo)

	targetEndTime := now.Add(time.Hour)
	r1 := contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now,
		EndTime:   targetEndTime,
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

	// 1. Inject scheduler failure: ScheduleWithID fails
	schedRepo.failSched = true
	schedRepo.scheduleErr = errors.New("queue unavailable")

	// Direct ReconcileSettlementSchedule fails with ErrScheduleFailed
	if err := svc.ReconcileSettlementSchedule(ctx); !errors.Is(err, contest.ErrScheduleFailed) {
		t.Fatalf("expected ErrScheduleFailed from ReconcileSettlementSchedule, got: %v", err)
	}

	// GetOverview must propagate the failure, returning ErrScheduleFailed
	_, overviewErr := svc.GetOverview(ctx)
	if overviewErr == nil || !errors.Is(overviewErr, contest.ErrScheduleFailed) {
		t.Fatalf("expected ErrScheduleFailed from GetOverview, got: %v", overviewErr)
	}

	// SQL round must be unchanged in MariaDB
	roundAfterFail, err := contestRepo.GetRoundByNumber(ctx, 1)
	if err != nil {
		t.Fatalf("GetRoundByNumber failed: %v", err)
	}
	if !roundAfterFail.EndTime.Equal(targetEndTime) || roundAfterFail.Status != contest.StatusActive {
		t.Fatalf("expected SQL round to remain untouched, got: %+v", roundAfterFail)
	}

	// Queue must have 0 actions
	if len(schedRepo.actions) != 0 {
		t.Fatalf("expected 0 actions scheduled during failure, got %d", len(schedRepo.actions))
	}

	// 2. Clear scheduler failure and verify recovery
	schedRepo.failSched = false
	schedRepo.scheduleErr = nil

	overview, err := svc.GetOverview(ctx)
	if err != nil {
		t.Fatalf("expected GetOverview to succeed after scheduler recovery, got: %v", err)
	}
	if overview.ActiveRound == nil || overview.ActiveRound.Round != 1 {
		t.Fatalf("expected ActiveRound 1, got %+v", overview.ActiveRound)
	}

	// 1 scheduled action must now exist in the queue
	if len(schedRepo.actions) != 1 {
		t.Fatalf("expected 1 scheduled action after recovery, got %d", len(schedRepo.actions))
	}
	expectedActionPrefix := contest.SettlementActionID(1, targetEndTime)
	var createdAction *core_scheduling.ScheduledAction
	for _, a := range schedRepo.actions {
		if strings.HasPrefix(a.ID, expectedActionPrefix) {
			createdAction = &a
			break
		}
	}
	if createdAction == nil {
		t.Fatalf("expected scheduled action with prefix %q in queue", expectedActionPrefix)
	}
	if createdAction.State != core_scheduling.StatePending {
		t.Errorf("expected action State %s, got %s", core_scheduling.StatePending, createdAction.State)
	}
	if !createdAction.ExecuteAt.Equal(targetEndTime) {
		t.Errorf("expected action ExecuteAt %v, got %v", targetEndTime, createdAction.ExecuteAt)
	}

	// 3. Subsequent GetOverview calls must NOT duplicate the scheduled reservation
	overview2, err := svc.GetOverview(ctx)
	if err != nil {
		t.Fatalf("subsequent GetOverview failed: %v", err)
	}
	if overview2.ActiveRound == nil || overview2.ActiveRound.Round != 1 {
		t.Fatalf("expected ActiveRound 1 on subsequent call, got %+v", overview2.ActiveRound)
	}
	if len(schedRepo.actions) != 1 {
		t.Fatalf("expected still 1 scheduled action (no duplicate), got %d", len(schedRepo.actions))
	}
}

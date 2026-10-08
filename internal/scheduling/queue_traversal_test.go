package scheduling_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	core "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/scheduling"
)

func TestWorker_QueueTraversal_StarvationPrevention(t *testing.T) {
	client := openValkeyClient(t)
	t.Cleanup(client.Close)

	ctx := context.Background()
	actorID := id.New()
	t.Cleanup(func() {
		client.Do(ctx, client.B().Del().Key("party2:scheduled:actor:"+actorID).Build())
	})

	fault := &terminalFaultClient{Client: client}
	repo := scheduling.NewValkeyRepository(fault)
	handler := &terminalCountingHandler{}
	worker := scheduling.NewWorker(repo, time.Minute, logging.NewJSON(io.Discard))
	worker.RegisterHandler("test:action", handler)

	// Step 1: Schedule 50 valid, uniquely identified due actions with earlier ExecuteAt values.
	// Inject error ONLY on terminal Completed SET so Processing SET and feature handling succeed,
	// leaving 50 records in StateProcessing.
	blockedIDs := make([]string, 50)
	baseTime := time.Now().Add(-10 * time.Minute)
	for i := 0; i < 50; i++ {
		actionID := fmt.Sprintf("blocked-%02d-%s", i, id.New())
		blockedIDs[i] = actionID
		action := validTestAction(actionID, baseTime.Add(time.Duration(i)*time.Second))
		action.ActorID = actorID
		if err := repo.Schedule(ctx, action); err != nil {
			t.Fatalf("Schedule blocked %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, blockedIDs...)
	})

	// Fail terminal SET when state is Completed
	fault.fail = func(cmd []string) bool {
		if cmd[0] == "SET" && len(cmd) > 2 {
			var stored core.ScheduledAction
			if err := json.Unmarshal([]byte(cmd[2]), &stored); err == nil {
				return stored.State == core.StateCompleted
			}
		}
		return false
	}

	for _, aID := range blockedIDs {
		action := validTestAction(aID, baseTime)
		action.ActorID = actorID
		worker.ProcessAction(ctx, action)
	}

	if handler.calls != 50 {
		t.Fatalf("expected 50 handler calls for initial blocked actions, got %d", handler.calls)
	}

	// Step 2: Restore storage writes. Schedule 1 healthy Pending action with a later but already due ExecuteAt.
	fault.fail = nil
	healthyID := fmt.Sprintf("healthy-%s", id.New())
	healthyAction := validTestAction(healthyID, baseTime.Add(60*time.Second))
	healthyAction.ActorID = actorID
	if err := repo.Schedule(ctx, healthyAction); err != nil {
		t.Fatalf("Schedule healthy action: %v", err)
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, healthyID)
	})

	// Verify actor discovery sees all 51 unfinished records before worker run
	unfinished, err := repo.FindPendingByActorID(ctx, actorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID: %v", err)
	}
	if len(unfinished) != 51 {
		t.Fatalf("expected 51 unfinished records, got %d", len(unfinished))
	}

	// Step 3: Run worker fetch/process iteration with production limit (50).
	// Healthy due action beyond the 50 retained Processing records must execute!
	due, err := repo.FetchDue(ctx, time.Now(), 50)
	if err != nil {
		t.Fatalf("FetchDue: %v", err)
	}
	for _, a := range due {
		worker.ProcessAction(ctx, a)
	}

	// Handler calls must increment to 51 (healthy action executed; 50 blocked records not replayed)
	if handler.calls != 51 {
		t.Fatalf("expected 51 handler calls (healthy action executed), got %d", handler.calls)
	}

	// Blocked records must remain in Processing state in storage and queue
	for _, aID := range blockedIDs {
		data, err := client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+aID).Build()).ToString()
		if err != nil {
			t.Fatalf("get blocked action %s: %v", aID, err)
		}
		var stored core.ScheduledAction
		if err := json.Unmarshal([]byte(data), &stored); err != nil {
			t.Fatal(err)
		}
		if stored.State != core.StateProcessing {
			t.Fatalf("expected action %s to remain Processing, got %s", aID, stored.State)
		}
	}

	// Actor query must now report exactly 50 unfinished records (healthy action completed)
	unfinishedAfter, err := repo.FindPendingByActorID(ctx, actorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID after: %v", err)
	}
	if len(unfinishedAfter) != 50 {
		t.Fatalf("expected 50 unfinished records remaining, got %d", len(unfinishedAfter))
	}
}

func TestWorker_QueueTraversal_MultiPage(t *testing.T) {
	client := openValkeyClient(t)
	t.Cleanup(client.Close)

	ctx := context.Background()
	actorID := id.New()
	t.Cleanup(func() {
		client.Do(ctx, client.B().Del().Key("party2:scheduled:actor:"+actorID).Build())
	})

	fault := &terminalFaultClient{Client: client}
	repo := scheduling.NewValkeyRepository(fault)
	handler := &terminalCountingHandler{}
	worker := scheduling.NewWorker(repo, time.Minute, logging.NewJSON(io.Discard))
	worker.RegisterHandler("test:action", handler)

	// 120 blocked actions (exceeds multiple pages of 50)
	const numBlocked = 120
	blockedIDs := make([]string, numBlocked)
	baseTime := time.Now().Add(-20 * time.Minute)
	for i := 0; i < numBlocked; i++ {
		actionID := fmt.Sprintf("multipage-blocked-%03d-%s", i, id.New())
		blockedIDs[i] = actionID
		action := validTestAction(actionID, baseTime.Add(time.Duration(i)*time.Second))
		action.ActorID = actorID
		if err := repo.Schedule(ctx, action); err != nil {
			t.Fatalf("Schedule %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, blockedIDs...)
	})

	fault.fail = func(cmd []string) bool {
		if cmd[0] == "SET" && len(cmd) > 2 {
			var stored core.ScheduledAction
			if err := json.Unmarshal([]byte(cmd[2]), &stored); err == nil {
				return stored.State == core.StateCompleted
			}
		}
		return false
	}

	for _, aID := range blockedIDs {
		action := validTestAction(aID, baseTime)
		action.ActorID = actorID
		worker.ProcessAction(ctx, action)
	}

	if handler.calls != numBlocked {
		t.Fatalf("expected %d calls, got %d", numBlocked, handler.calls)
	}

	fault.fail = nil
	healthyID := fmt.Sprintf("multipage-healthy-%s", id.New())
	healthyAction := validTestAction(healthyID, baseTime.Add(time.Duration(numBlocked+10)*time.Second))
	healthyAction.ActorID = actorID
	if err := repo.Schedule(ctx, healthyAction); err != nil {
		t.Fatalf("Schedule healthy: %v", err)
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, healthyID)
	})

	// Fetch with limit 50; must traverse past multiple pages (120 entries) to return the healthy action
	due, err := repo.FetchDue(ctx, time.Now(), 50)
	if err != nil {
		t.Fatalf("FetchDue: %v", err)
	}
	for _, a := range due {
		worker.ProcessAction(ctx, a)
	}

	if handler.calls != numBlocked+1 {
		t.Fatalf("expected %d handler calls, got %d", numBlocked+1, handler.calls)
	}
}

func TestWorker_QueueTraversal_HealthyContrast(t *testing.T) {
	client := openValkeyClient(t)
	t.Cleanup(client.Close)

	ctx := context.Background()
	repo := scheduling.NewValkeyRepository(client)
	handler := &terminalCountingHandler{}
	worker := scheduling.NewWorker(repo, time.Minute, logging.NewJSON(io.Discard))
	worker.RegisterHandler("test:action", handler)

	const count = 50
	ids := make([]string, count)
	baseTime := time.Now().Add(-5 * time.Minute)
	for i := 0; i < count; i++ {
		actionID := fmt.Sprintf("contrast-%02d-%s", i, id.New())
		ids[i] = actionID
		action := validTestAction(actionID, baseTime.Add(time.Duration(i)*time.Second))
		if err := repo.Schedule(ctx, action); err != nil {
			t.Fatalf("Schedule %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, ids...)
	})

	due, err := repo.FetchDue(ctx, time.Now(), 50)
	if err != nil {
		t.Fatalf("FetchDue: %v", err)
	}
	if len(due) != count {
		t.Fatalf("expected %d due actions, got %d", count, len(due))
	}
	for _, a := range due {
		worker.ProcessAction(ctx, a)
	}

	if handler.calls != count {
		t.Fatalf("expected %d handler calls, got %d", count, handler.calls)
	}

	// Verify all actions were completed and removed from queue
	for _, aID := range ids {
		score, err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(aID).Build()).ToString()
		if !valkey.IsValkeyNil(err) {
			t.Fatalf("expected %s to be removed from queue, score=%s, err=%v", aID, score, err)
		}
	}
}

func TestWorker_QueueTraversal_MixedStatesAndTerminalCleanup(t *testing.T) {
	client := openValkeyClient(t)
	t.Cleanup(client.Close)

	ctx := context.Background()
	fault := &terminalFaultClient{Client: client}
	repo := scheduling.NewValkeyRepository(fault)
	handler := &terminalCountingHandler{}
	worker := scheduling.NewWorker(repo, time.Minute, logging.NewJSON(io.Discard))
	worker.RegisterHandler("test:action", handler)

	baseTime := time.Now().Add(-15 * time.Minute)

	// 10 blocked Processing actions
	blockedIDs := make([]string, 10)
	for i := 0; i < 10; i++ {
		actionID := fmt.Sprintf("mixed-blocked-%02d-%s", i, id.New())
		blockedIDs[i] = actionID
		action := validTestAction(actionID, baseTime.Add(time.Duration(i)*time.Second))
		if err := repo.Schedule(ctx, action); err != nil {
			t.Fatalf("Schedule: %v", err)
		}
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, blockedIDs...)
	})

	// Fail terminal SET to leave them Processing
	fault.fail = func(cmd []string) bool {
		if cmd[0] == "SET" && len(cmd) > 2 {
			var stored core.ScheduledAction
			if err := json.Unmarshal([]byte(cmd[2]), &stored); err == nil {
				return stored.State == core.StateCompleted
			}
		}
		return false
	}
	for _, aID := range blockedIDs {
		worker.ProcessAction(ctx, validTestAction(aID, baseTime))
	}
	if handler.calls != 10 {
		t.Fatalf("expected 10 handler calls for blocked, got %d", handler.calls)
	}
	fault.fail = nil

	// 10 Completed terminal actions left in queue (fail ZREM on Save so they remain in queue)
	completedIDs := make([]string, 10)
	fault.fail = func(cmd []string) bool {
		return cmd[0] == "ZREM" && cmd[1] == "party2:scheduled:pending"
	}
	for i := 0; i < 10; i++ {
		actionID := fmt.Sprintf("mixed-completed-%02d-%s", i, id.New())
		completedIDs[i] = actionID
		action := validTestAction(actionID, baseTime.Add(time.Duration(10+i)*time.Second))
		if err := repo.Schedule(ctx, action); err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		if err := action.MarkProcessing(); err != nil {
			t.Fatal(err)
		}
		if err := action.MarkCompleted(time.Hour); err != nil {
			t.Fatal(err)
		}
		// Save will persist Completed but fail ZREM
		_ = repo.Save(ctx, action)
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, completedIDs...)
	})
	fault.fail = nil

	// 5 healthy Pending actions
	healthyIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		actionID := fmt.Sprintf("mixed-healthy-%02d-%s", i, id.New())
		healthyIDs[i] = actionID
		action := validTestAction(actionID, baseTime.Add(time.Duration(20+i)*time.Second))
		if err := repo.Schedule(ctx, action); err != nil {
			t.Fatalf("Schedule: %v", err)
		}
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, healthyIDs...)
	})

	// FetchDue with limit 50:
	// Should skip the 10 Processing actions, return the 10 Completed actions (for cleanup) and 5 Pending actions (for execution)
	due, err := repo.FetchDue(ctx, time.Now(), 50)
	if err != nil {
		t.Fatalf("FetchDue: %v", err)
	}
	if len(due) != 15 {
		t.Fatalf("expected 15 due actions (10 completed + 5 pending), got %d", len(due))
	}

	for _, a := range due {
		worker.ProcessAction(ctx, a)
	}

	// Handler calls must only increment by 5 (only healthy pending actions executed, completed actions not redispatched)
	if handler.calls != 15 {
		t.Fatalf("expected 15 handler calls (10 initial + 5 healthy), got %d", handler.calls)
	}

	// Verify the 10 Completed actions were cleaned up from queue
	for _, aID := range completedIDs {
		_, err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(aID).Build()).ToString()
		if !valkey.IsValkeyNil(err) {
			t.Fatalf("expected completed %s to be cleaned from queue", aID)
		}
	}

	// Verify the 10 blocked actions remain in Processing in storage, but separated from execution queue
	for _, aID := range blockedIDs {
		data, err := client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+aID).Build()).ToString()
		if err != nil {
			t.Fatalf("get blocked action %s: %v", aID, err)
		}
		var stored core.ScheduledAction
		if err := json.Unmarshal([]byte(data), &stored); err != nil {
			t.Fatal(err)
		}
		if stored.State != core.StateProcessing {
			t.Fatalf("expected action %s to remain Processing, got %s", aID, stored.State)
		}
		score, err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(aID).Build()).ToString()
		if !valkey.IsValkeyNil(err) {
			t.Fatalf("expected blocked %s to be separated from pending queue, score=%s, err=%v", aID, score, err)
		}
	}
}

func TestWorker_QueueTraversal_TransientErrorPropagation(t *testing.T) {
	client := openValkeyClient(t)
	t.Cleanup(client.Close)

	ctx := context.Background()
	fault := &terminalFaultClient{Client: client}
	repo := scheduling.NewValkeyRepository(fault)

	baseTime := time.Now().Add(-5 * time.Minute)
	// Schedule 3 actions
	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		actionID := fmt.Sprintf("transient-%02d-%s", i, id.New())
		ids[i] = actionID
		action := validTestAction(actionID, baseTime.Add(time.Duration(i)*time.Second))
		if err := repo.Schedule(ctx, action); err != nil {
			t.Fatalf("Schedule: %v", err)
		}
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, ids...)
	})

	// Inject error on GET of the second action
	targetKey := "party2:scheduled:action:" + ids[1]
	fault.fail = func(cmd []string) bool {
		return cmd[0] == "GET" && cmd[1] == targetKey
	}

	// FetchDue must fail immediately with the error and preserve all entries in queue
	actions, err := repo.FetchDue(ctx, time.Now(), 50)
	if err == nil || !errors.Is(err, terminalWriteError) {
		t.Fatalf("expected terminalWriteError from FetchDue, got err=%v, actions=%v", err, actions)
	}

	// Verify all 3 remain queued (none removed)
	for _, aID := range ids {
		score, err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(aID).Build()).ToString()
		if err != nil || score == "" {
			t.Fatalf("expected %s to remain queued after transient error", aID)
		}
	}
}

func TestWorker_QueueTraversal_ScanBudgetProgressAcrossTicks(t *testing.T) {
	client := openValkeyClient(t)
	t.Cleanup(client.Close)

	ctx := context.Background()
	actorID := id.New()
	t.Cleanup(func() {
		client.Do(ctx, client.B().Del().Key("party2:scheduled:actor:"+actorID).Build())
	})

	repo := scheduling.NewValkeyRepository(client)
	handler := &terminalCountingHandler{}
	worker := scheduling.NewWorker(repo, time.Minute, logging.NewJSON(io.Discard))
	worker.RegisterHandler("test:action", handler)

	// Step 1: Schedule 1000 distinct valid test:action records in StateProcessing with ExecuteAt=now-1h,
	// all sharing the exact same score (equal scores) and one unique test actor.
	const numBlocked = 1000
	blockedIDs := make([]string, numBlocked)
	baseTime := time.Now().Add(-time.Hour).Truncate(time.Second)
	for i := 0; i < numBlocked; i++ {
		actionID := fmt.Sprintf("budget-blocked-%04d-%s", i, id.New())
		blockedIDs[i] = actionID
		action := validTestAction(actionID, baseTime)
		action.State = core.StateProcessing
		action.ActorID = actorID
		if err := repo.Schedule(ctx, action); err != nil {
			t.Fatalf("Schedule %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, blockedIDs...)
	})

	// Step 2: Add 5 mixed stale entries into pending queue (absent payload in actionKey).
	staleIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		staleID := fmt.Sprintf("budget-stale-%d-%s", i, id.New())
		staleIDs[i] = staleID
		if err := client.Do(ctx, client.B().Zadd().Key("party2:scheduled:pending").ScoreMember().ScoreMember(float64(baseTime.Unix()), staleID).Build()).Error(); err != nil {
			t.Fatalf("Zadd stale %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		for _, sID := range staleIDs {
			client.Do(ctx, client.B().Zrem().Key("party2:scheduled:pending").Member(sID).Build())
		}
	})

	// Step 3: Add 1 terminal Completed record behind the prefix with future RetainUntil (simulating failed ZREM during terminal Save).
	terminalID := fmt.Sprintf("budget-terminal-%s", id.New())
	terminalAction := validTestAction(terminalID, baseTime.Add(30*time.Second))
	terminalAction.State = core.StateCompleted
	terminalAction.RetainUntil = time.Now().Add(24 * time.Hour)
	if err := repo.Schedule(ctx, terminalAction); err != nil {
		t.Fatalf("Schedule terminal: %v", err)
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, terminalID)
	})

	// Step 4: Schedule 1 valid Pending action for the same actor with ExecuteAt=now-59m.
	healthyID := fmt.Sprintf("budget-healthy-%s", id.New())
	healthyAction := validTestAction(healthyID, baseTime.Add(time.Minute))
	healthyAction.ActorID = actorID
	if err := repo.Schedule(ctx, healthyAction); err != nil {
		t.Fatalf("Schedule healthy: %v", err)
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, healthyID)
	})

	// Verify actor discovery sees all 1001 unfinished records before worker run (1000 Processing + 1 Pending; terminal is omitted)
	unfinished, err := repo.FindPendingByActorID(ctx, actorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID: %v", err)
	}
	if len(unfinished) != 1001 {
		t.Fatalf("expected 1001 unfinished records, got %d", len(unfinished))
	}

	// Step 5: Controlled concurrent insert & removal during traversal.
	concurrentInsertID := fmt.Sprintf("budget-concurrent-%s", id.New())
	concurrentAction := validTestAction(concurrentInsertID, baseTime.Add(2*time.Minute))
	concurrentAction.ActorID = actorID
	if err := repo.Schedule(ctx, concurrentAction); err != nil {
		t.Fatalf("Schedule concurrent: %v", err)
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, concurrentInsertID)
	})

	// Repeat FetchDue(now, 50) and Worker.ProcessAction across ticks.
	// Production limit is 50.
	var executedHealthy bool
	var executedConcurrent bool
	for tick := 0; tick < 10; tick++ {
		due, err := repo.FetchDue(ctx, time.Now(), 50)
		if err != nil {
			t.Fatalf("FetchDue tick %d: %v", tick, err)
		}
		if len(due) > 50 {
			t.Fatalf("tick %d returned %d actions, exceeding production limit 50", tick, len(due))
		}
		for _, a := range due {
			worker.ProcessAction(ctx, a)
			if a.ID == healthyID {
				executedHealthy = true
			}
			if a.ID == concurrentInsertID {
				executedConcurrent = true
			}
		}
		if executedHealthy && executedConcurrent {
			break
		}
	}

	if !executedHealthy {
		t.Fatalf("expected healthy action to be executed across repeated production-limit ticks, but was never fetched")
	}
	if !executedConcurrent {
		t.Fatalf("expected concurrent action to be executed across repeated production-limit ticks")
	}
	// Exactly 2 handler calls (healthyAction and concurrentAction; 1000 Processing and 1 Completed NEVER replayed)
	if handler.calls != 2 {
		t.Fatalf("expected exactly 2 handler calls, got %d", handler.calls)
	}

	// Verify terminal action was cleaned up from pending queue (metadata-only finalization)
	termScore, err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(terminalID).Build()).ToString()
	if !valkey.IsValkeyNil(err) {
		t.Fatalf("expected terminal action %s to be cleaned from queue, score=%s, err=%v", terminalID, termScore, err)
	}

	// Verify all stale IDs were removed from pending queue
	for _, sID := range staleIDs {
		score, err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(sID).Build()).ToString()
		if !valkey.IsValkeyNil(err) {
			t.Fatalf("expected stale %s to be cleaned from queue, score=%s, err=%v", sID, score, err)
		}
	}

	// Verify all 1000 blocked records remain discoverable and in StateProcessing
	unfinishedAfter, err := repo.FindPendingByActorID(ctx, actorID)
	if err != nil {
		t.Fatalf("FindPendingByActorID after: %v", err)
	}
	if len(unfinishedAfter) != 1000 {
		t.Fatalf("expected 1000 unfinished records remaining, got %d", len(unfinishedAfter))
	}

	// Step 6: Healthy diagnostic contrast: scheduling a fresh due action immediately fetches and executes in 1 tick
	contrastID := fmt.Sprintf("budget-contrast-%s", id.New())
	contrastAction := validTestAction(contrastID, baseTime.Add(3*time.Minute))
	contrastAction.ActorID = actorID
	if err := repo.Schedule(ctx, contrastAction); err != nil {
		t.Fatalf("Schedule contrast: %v", err)
	}
	t.Cleanup(func() {
		cleanupKeys(t, client, contrastID)
	})

	dueContrast, err := repo.FetchDue(ctx, time.Now(), 50)
	if err != nil {
		t.Fatalf("FetchDue contrast: %v", err)
	}
	if len(dueContrast) != 1 || dueContrast[0].ID != contrastID {
		t.Fatalf("expected contrast action %s returned immediately, got %+v", contrastID, dueContrast)
	}
	worker.ProcessAction(ctx, dueContrast[0])
	if handler.calls != 3 {
		t.Fatalf("expected 3 handler calls after contrast, got %d", handler.calls)
	}
}

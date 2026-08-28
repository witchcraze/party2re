package scheduling_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/scheduling"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

func openValkeyClient(t *testing.T) valkey.Client {
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

func cleanupKeys(t *testing.T, client valkey.Client, ids ...string) {
	t.Helper()
	ctx := context.Background()
	for _, id := range ids {
		client.Do(ctx, client.B().Del().Key("party2:scheduled:action:"+id).Build())
		client.Do(ctx, client.B().Del().Key("party2:scheduled:lock:"+id).Build())
		client.Do(ctx, client.B().Zrem().Key("party2:scheduled:pending").Member(id).Build())
	}
}

func validTestAction(id string, executeAt time.Time) core_scheduling.ScheduledAction {
	return core_scheduling.ScheduledAction{
		ID:          id,
		ActionType:  "test:action",
		ActorID:     "char-test",
		State:       core_scheduling.StatePending,
		ScheduledAt: time.Now().UTC(),
		ExecuteAt:   executeAt,
	}
}

func TestValkeyRepository_Schedule_StoresActionAndEnqueues(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "sched-test-01"
	defer cleanupKeys(t, client, id)

	repo := scheduling.NewValkeyRepository(client)
	ctx := context.Background()
	executeAt := time.Now().Add(time.Hour).UTC()
	action := validTestAction(id, executeAt)

	if err := repo.Schedule(ctx, action); err != nil {
		t.Fatalf("Schedule() error = %v", err)
	}

	val, err := client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+id).Build()).AsBytes()
	if err != nil {
		t.Fatalf("action key not found: %v", err)
	}
	var stored core_scheduling.ScheduledAction
	if err := json.Unmarshal(val, &stored); err != nil {
		t.Fatalf("unmarshal stored action: %v", err)
	}
	if stored.ID != id {
		t.Errorf("stored ID = %q, want %q", stored.ID, id)
	}

	score, err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(id).Build()).AsFloat64()
	if err != nil {
		t.Fatalf("action not in pending ZSET: %v", err)
	}
	if score != float64(executeAt.Unix()) {
		t.Errorf("ZSET score = %v, want %v", score, float64(executeAt.Unix()))
	}
}

func TestValkeyRepository_FetchDue_ReturnsPastActions(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "fetchdue-past-01"
	defer cleanupKeys(t, client, id)

	repo := scheduling.NewValkeyRepository(client)
	ctx := context.Background()
	if err := repo.Schedule(ctx, validTestAction(id, time.Now().Add(-time.Minute).UTC())); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	actions, err := repo.FetchDue(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("FetchDue: %v", err)
	}
	found := false
	for _, a := range actions {
		if a.ID == id {
			found = true
		}
	}
	if !found {
		t.Errorf("expected past action %q to be returned by FetchDue", id)
	}
}

func TestValkeyRepository_FetchDue_ExcludesFutureActions(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "fetchdue-future-01"
	defer cleanupKeys(t, client, id)

	repo := scheduling.NewValkeyRepository(client)
	ctx := context.Background()
	if err := repo.Schedule(ctx, validTestAction(id, time.Now().Add(time.Hour).UTC())); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	actions, err := repo.FetchDue(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("FetchDue: %v", err)
	}
	for _, a := range actions {
		if a.ID == id {
			t.Errorf("future action %q should not be returned by FetchDue", id)
		}
	}
}

func TestValkeyRepository_FetchDue_MalformedJSONRemovedFromQueue(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "fetchdue-malformed-01"
	defer cleanupKeys(t, client, id)

	ctx := context.Background()
	client.Do(ctx, client.B().Set().Key("party2:scheduled:action:"+id).Value("not-valid-json").Build())
	score := float64(time.Now().Add(-time.Minute).Unix())
	client.Do(ctx, client.B().Zadd().Key("party2:scheduled:pending").ScoreMember().ScoreMember(score, id).Build())

	repo := scheduling.NewValkeyRepository(client)
	actions, err := repo.FetchDue(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("FetchDue: %v", err)
	}
	for _, a := range actions {
		if a.ID == id {
			t.Errorf("malformed action %q should be skipped", id)
		}
	}

	members, _ := client.Do(ctx, client.B().Zrange().Key("party2:scheduled:pending").Min("-inf").Max("+inf").Byscore().Build()).AsStrSlice()
	for _, m := range members {
		if m == id {
			t.Errorf("malformed action %q should be removed from pending ZSET", id)
		}
	}
}

func TestValkeyRepository_FetchDue_InvalidValidateRemovedFromQueue(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "fetchdue-invalid-01"
	defer cleanupKeys(t, client, id)

	ctx := context.Background()
	invalid := core_scheduling.ScheduledAction{
		ID:        id,
		ActorID:   "char-test",
		State:     core_scheduling.StatePending,
		ExecuteAt: time.Now().Add(-time.Minute).UTC(),
		// ActionType intentionally empty → Validate() fails
	}
	data, _ := json.Marshal(invalid)
	client.Do(ctx, client.B().Set().Key("party2:scheduled:action:"+id).Value(string(data)).Build())
	score := float64(time.Now().Add(-time.Minute).Unix())
	client.Do(ctx, client.B().Zadd().Key("party2:scheduled:pending").ScoreMember().ScoreMember(score, id).Build())

	repo := scheduling.NewValkeyRepository(client)
	actions, err := repo.FetchDue(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("FetchDue: %v", err)
	}
	for _, a := range actions {
		if a.ID == id {
			t.Errorf("invalid action %q should be skipped by Validate()", id)
		}
	}

	members, _ := client.Do(ctx, client.B().Zrange().Key("party2:scheduled:pending").Min("-inf").Max("+inf").Byscore().Build()).AsStrSlice()
	for _, m := range members {
		if m == id {
			t.Errorf("invalid action %q should be removed from pending ZSET", id)
		}
	}
}

func TestValkeyRepository_FetchDue_StaleQueueEntryCleanedUp(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "fetchdue-stale-01"
	defer cleanupKeys(t, client, id)

	ctx := context.Background()
	score := float64(time.Now().Add(-time.Minute).Unix())
	client.Do(ctx, client.B().Zadd().Key("party2:scheduled:pending").ScoreMember().ScoreMember(score, id).Build())

	repo := scheduling.NewValkeyRepository(client)
	actions, err := repo.FetchDue(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("FetchDue: %v", err)
	}
	for _, a := range actions {
		if a.ID == id {
			t.Errorf("stale action %q should be skipped", id)
		}
	}

	members, _ := client.Do(ctx, client.B().Zrange().Key("party2:scheduled:pending").Min("-inf").Max("+inf").Byscore().Build()).AsStrSlice()
	for _, m := range members {
		if m == id {
			t.Errorf("stale queue entry %q should be removed from pending ZSET", id)
		}
	}
}

func TestValkeyRepository_AcquireLock_FirstCallerSucceeds(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "lock-test-01"
	defer cleanupKeys(t, client, id)

	repo := scheduling.NewValkeyRepository(client)
	ctx := context.Background()

	acquired, err := repo.AcquireLock(ctx, id, 5*time.Second)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if !acquired {
		t.Error("expected first caller to acquire lock")
	}
}

func TestValkeyRepository_AcquireLock_SecondCallerRejected(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "lock-test-02"
	defer cleanupKeys(t, client, id)

	repo := scheduling.NewValkeyRepository(client)
	ctx := context.Background()

	if _, err := repo.AcquireLock(ctx, id, 5*time.Second); err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	acquired, err := repo.AcquireLock(ctx, id, 5*time.Second)
	if err != nil {
		t.Fatalf("second AcquireLock: %v", err)
	}
	if acquired {
		t.Error("expected second caller to be rejected when lock is held")
	}
}

func TestValkeyRepository_AcquireLock_ExpiresAfterTTL(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "lock-ttl-01"
	defer cleanupKeys(t, client, id)

	repo := scheduling.NewValkeyRepository(client)
	ctx := context.Background()

	if _, err := repo.AcquireLock(ctx, id, time.Second); err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	time.Sleep(2 * time.Second)

	acquired, err := repo.AcquireLock(ctx, id, time.Second)
	if err != nil {
		t.Fatalf("AcquireLock after TTL: %v", err)
	}
	if !acquired {
		t.Error("expected lock to be re-acquirable after TTL expiry")
	}
}

func TestValkeyRepository_Save_CompletedRemovesFromQueueAndSetsRetainTTL(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "save-completed-01"
	defer cleanupKeys(t, client, id)

	repo := scheduling.NewValkeyRepository(client)
	ctx := context.Background()

	action := validTestAction(id, time.Now().Add(-time.Minute).UTC())
	if err := repo.Schedule(ctx, action); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	now := time.Now()
	action.State = core_scheduling.StateCompleted
	action.AttemptedAt = &now
	completed := now
	action.CompletedAt = &completed
	action.RetainUntil = now.Add(24 * time.Hour)

	if err := repo.Save(ctx, action); err != nil {
		t.Fatalf("Save(completed): %v", err)
	}

	members, _ := client.Do(ctx, client.B().Zrange().Key("party2:scheduled:pending").Min("-inf").Max("+inf").Byscore().Build()).AsStrSlice()
	for _, m := range members {
		if m == id {
			t.Errorf("completed action %q should be removed from pending ZSET", id)
		}
	}

	ttl, err := client.Do(ctx, client.B().Ttl().Key("party2:scheduled:action:"+id).Build()).AsInt64()
	if err != nil {
		t.Fatalf("TTL check: %v", err)
	}
	if ttl <= 0 {
		t.Errorf("expected positive TTL on completed action key, got %d", ttl)
	}
}

func TestValkeyRepository_Save_FailedRemovesFromQueueAndSetsRetainTTL(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "save-failed-01"
	defer cleanupKeys(t, client, id)

	repo := scheduling.NewValkeyRepository(client)
	ctx := context.Background()

	action := validTestAction(id, time.Now().Add(-time.Minute).UTC())
	if err := repo.Schedule(ctx, action); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	now := time.Now()
	action.State = core_scheduling.StateFailed
	action.AttemptedAt = &now
	completed := now
	action.CompletedAt = &completed
	action.RetainUntil = now.Add(24 * time.Hour)

	if err := repo.Save(ctx, action); err != nil {
		t.Fatalf("Save(failed): %v", err)
	}

	members, _ := client.Do(ctx, client.B().Zrange().Key("party2:scheduled:pending").Min("-inf").Max("+inf").Byscore().Build()).AsStrSlice()
	for _, m := range members {
		if m == id {
			t.Errorf("failed action %q should be removed from pending ZSET", id)
		}
	}

	ttl, err := client.Do(ctx, client.B().Ttl().Key("party2:scheduled:action:"+id).Build()).AsInt64()
	if err != nil {
		t.Fatalf("TTL check: %v", err)
	}
	if ttl <= 0 {
		t.Errorf("expected positive TTL on failed action key, got %d", ttl)
	}
}

func TestValkeyRepository_Save_ProcessingUpdatesKeyWithoutRemovingFromQueue(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()
	id := "save-processing-01"
	defer cleanupKeys(t, client, id)

	repo := scheduling.NewValkeyRepository(client)
	ctx := context.Background()

	action := validTestAction(id, time.Now().Add(-time.Minute).UTC())
	if err := repo.Schedule(ctx, action); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	now := time.Now()
	action.State = core_scheduling.StateProcessing
	action.AttemptedAt = &now

	if err := repo.Save(ctx, action); err != nil {
		t.Fatalf("Save(processing): %v", err)
	}

	members, _ := client.Do(ctx, client.B().Zrange().Key("party2:scheduled:pending").Min("-inf").Max("+inf").Byscore().Build()).AsStrSlice()
	found := false
	for _, m := range members {
		if m == id {
			found = true
		}
	}
	if !found {
		t.Errorf("processing action %q should remain in pending ZSET", id)
	}

	ttl, err := client.Do(ctx, client.B().Ttl().Key("party2:scheduled:action:"+id).Build()).AsInt64()
	if err != nil {
		t.Fatalf("TTL check: %v", err)
	}
	if ttl != -1 {
		t.Errorf("processing action should have no TTL (-1=persistent), got %d", ttl)
	}
}

func TestValkeyRepository_CancelByActorID(t *testing.T) {
	client := openValkeyClient(t)
	defer client.Close()

	id1 := "cancel-actor-01"
	id2 := "cancel-actor-02"
	id3 := "cancel-other-03"
	defer cleanupKeys(t, client, id1, id2, id3)

	repo := scheduling.NewValkeyRepository(client)
	ctx := context.Background()

	a1 := validTestAction(id1, time.Now().Add(time.Hour).UTC())
	a1.ActorID = "actor-target"
	a2 := validTestAction(id2, time.Now().Add(2*time.Hour).UTC())
	a2.ActorID = "actor-target"
	a3 := validTestAction(id3, time.Now().Add(time.Hour).UTC())
	a3.ActorID = "actor-other"

	if err := repo.Schedule(ctx, a1); err != nil {
		t.Fatalf("Schedule a1: %v", err)
	}
	if err := repo.Schedule(ctx, a2); err != nil {
		t.Fatalf("Schedule a2: %v", err)
	}
	if err := repo.Schedule(ctx, a3); err != nil {
		t.Fatalf("Schedule a3: %v", err)
	}

	if err := repo.CancelByActorID(ctx, "actor-target"); err != nil {
		t.Fatalf("CancelByActorID error: %v", err)
	}

	// Verify a1 and a2 are removed from pending queue and action keys
	_, err := client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+id1).Build()).AsBytes()
	if err == nil {
		t.Errorf("action %s should be deleted", id1)
	}
	_, err = client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+id2).Build()).AsBytes()
	if err == nil {
		t.Errorf("action %s should be deleted", id2)
	}

	// a3 should still exist
	val, err := client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+id3).Build()).AsBytes()
	if err != nil || len(val) == 0 {
		t.Errorf("action %s should still exist", id3)
	}
}

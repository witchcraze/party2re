package scheduling

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestValkeyRepository_New(t *testing.T) {
	client := valkeytest.NewMockClient()
	repo := NewValkeyRepository(client)
	if repo == nil || repo.client != client {
		t.Fatal("expected non-nil ValkeyRepository initialized with client")
	}
}

func TestValkeyRepository_Schedule(t *testing.T) {
	ctx := context.Background()

	// 1. Success with ActorID
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo := NewValkeyRepository(client)

	executeAt := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	action := core_scheduling.ScheduledAction{
		ID:          "act-01",
		ActionType:  "adventure:step",
		ActorID:     "char-01",
		State:       core_scheduling.StatePending,
		ScheduledAt: time.Now().UTC(),
		ExecuteAt:   executeAt,
	}

	if err := repo.Schedule(ctx, action); err != nil {
		t.Fatalf("unexpected Schedule error: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands recorded, got %d: %v", len(cmds), cmds)
	}

	// First command: SET action data
	if cmds[0][0] != "SET" || cmds[0][1] != actionKeyPrefix+"act-01" {
		t.Errorf("unexpected SET command: %v", cmds[0])
	}
	var storedAction core_scheduling.ScheduledAction
	if err := json.Unmarshal([]byte(cmds[0][2]), &storedAction); err != nil {
		t.Fatalf("stored payload is not valid JSON: %v", err)
	}
	if storedAction.ID != "act-01" || storedAction.ActorID != "char-01" {
		t.Errorf("stored action mismatch: %+v", storedAction)
	}

	// Second command: SADD actor index
	if cmds[1][0] != "SADD" || cmds[1][1] != actorKeyPrefix+"char-01" || cmds[1][2] != "act-01" {
		t.Errorf("unexpected SADD command: %v", cmds[1])
	}

	// Third command: ZADD pending queue
	if cmds[2][0] != "ZADD" || cmds[2][1] != pendingQueueKey {
		t.Errorf("unexpected ZADD command: %v", cmds[2])
	}

	// 2. Success without ActorID (should skip SADD)
	clientNoActor := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repoNoActor := NewValkeyRepository(clientNoActor)

	actionNoActor := core_scheduling.ScheduledAction{
		ID:         "act-02",
		ActionType: "system:cleanup",
		ActorID:    "", // empty
		State:      core_scheduling.StatePending,
		ExecuteAt:  executeAt,
	}
	if err := repoNoActor.Schedule(ctx, actionNoActor); err != nil {
		t.Fatalf("unexpected Schedule error: %v", err)
	}
	cmdsNoActor := clientNoActor.RecordedCommandStrings()
	if len(cmdsNoActor) != 2 {
		t.Fatalf("expected 2 commands without ActorID, got %d: %v", len(cmdsNoActor), cmdsNoActor)
	}

	// 3. Error on SET
	errSet := errors.New("valkey set error")
	clientSetErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "SET" {
			return valkeytest.MakeErrorResult(errSet)
		}
		return valkeytest.MakeOKResult()
	}))
	repoSetErr := NewValkeyRepository(clientSetErr)
	if err := repoSetErr.Schedule(ctx, action); !errors.Is(err, errSet) {
		t.Fatalf("expected errSet, got %v", err)
	}

	// 4. Error on ZADD
	errZadd := errors.New("valkey zadd error")
	clientZaddErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "ZADD" {
			return valkeytest.MakeErrorResult(errZadd)
		}
		return valkeytest.MakeOKResult()
	}))
	repoZaddErr := NewValkeyRepository(clientZaddErr)
	if err := repoZaddErr.Schedule(ctx, action); !errors.Is(err, errZadd) {
		t.Fatalf("expected errZadd, got %v", err)
	}
}

func TestValkeyRepository_FetchDue(t *testing.T) {
	ctx := context.Background()

	// 1. Zrangebyscore fails
	errZrange := errors.New("zrange failed")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errZrange)
	}))
	repoErr := NewValkeyRepository(clientErr)
	if _, err := repoErr.FetchDue(ctx, time.Now(), 10); !errors.Is(err, errZrange) {
		t.Fatalf("expected errZrange, got %v", err)
	}

	// 2. Empty IDs from Zrangebyscore
	clientEmpty := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringSliceResult([]string{})
	}))
	repoEmpty := NewValkeyRepository(clientEmpty)
	actionsEmpty, err := repoEmpty.FetchDue(ctx, time.Now(), 10)
	if err != nil || actionsEmpty != nil {
		t.Fatalf("expected nil actions without error, got actions=%v err=%v", actionsEmpty, err)
	}

	// 3. Mixed batch of due IDs:
	// - "valid-1": valid action
	// - "missing-2": GET returns error -> ZREM called
	// - "malformed-3": GET returns invalid JSON -> ZREM and DEL called
	// - "invalid-4": GET returns valid JSON but invalid domain invariant -> ZREM called
	validAction := core_scheduling.ScheduledAction{
		ID:         "valid-1",
		ActionType: "adventure:explore",
		ActorID:    "char-valid",
		State:      core_scheduling.StatePending,
		ExecuteAt:  time.Now().UTC(),
	}
	validJSON, _ := json.Marshal(validAction)

	invalidDomainAction := core_scheduling.ScheduledAction{
		ID:         "invalid-4",
		ActionType: "", // empty ActionType fails Validate()
		ActorID:    "char-inv",
		State:      core_scheduling.StatePending,
	}
	invalidDomainJSON, _ := json.Marshal(invalidDomainAction)

	clientMixed := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		c := cmd.Commands()
		switch c[0] {
		case "ZRANGEBYSCORE":
			return valkeytest.MakeStringSliceResult([]string{"valid-1", "missing-2", "malformed-3", "invalid-4"})
		case "GET":
			switch c[1] {
			case actionKeyPrefix + "valid-1":
				return valkeytest.MakeStringResult(string(validJSON))
			case actionKeyPrefix + "missing-2":
				return valkeytest.MakeErrorResult(errors.New("key missing"))
			case actionKeyPrefix + "malformed-3":
				return valkeytest.MakeStringResult("{invalid-json-content")
			case actionKeyPrefix + "invalid-4":
				return valkeytest.MakeStringResult(string(invalidDomainJSON))
			}
		}
		return valkeytest.MakeOKResult()
	}))

	repoMixed := NewValkeyRepository(clientMixed)
	actions, err := repoMixed.FetchDue(ctx, time.Now(), 10)
	if err != nil {
		t.Fatalf("unexpected FetchDue error: %v", err)
	}

	if len(actions) != 1 || actions[0].ID != "valid-1" {
		t.Fatalf("expected only 'valid-1' returned, got %d actions: %+v", len(actions), actions)
	}

	// Verify cleanup commands were issued for bad keys
	cleanupCmds := clientMixed.RecordedCommandStrings()
	var zrems []string
	var dels []string
	for _, cmd := range cleanupCmds {
		if cmd[0] == "ZREM" {
			zrems = append(zrems, cmd[len(cmd)-1])
		}
		if cmd[0] == "DEL" {
			dels = append(dels, cmd[len(cmd)-1])
		}
	}

	if !reflect.DeepEqual(zrems, []string{"missing-2", "malformed-3", "invalid-4"}) {
		t.Errorf("unexpected ZREM cleanup calls: %v", zrems)
	}
	if !reflect.DeepEqual(dels, []string{actionKeyPrefix + "malformed-3"}) {
		t.Errorf("unexpected DEL cleanup calls: %v", dels)
	}
}

func TestValkeyRepository_AcquireLock(t *testing.T) {
	ctx := context.Background()

	// 1. Lock acquired successfully
	clientAcquired := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repoAcquired := NewValkeyRepository(clientAcquired)
	acquired, err := repoAcquired.AcquireLock(ctx, "lock-1", 10*time.Second)
	if err != nil || !acquired {
		t.Fatalf("expected lock acquired, got acquired=%v, err=%v", acquired, err)
	}
	lastCmd := clientAcquired.LastCommandStrings()
	if lastCmd[0] != "SET" || lastCmd[1] != lockKeyPrefix+"lock-1" {
		t.Fatalf("unexpected lock command: %v", lastCmd)
	}

	// 2. Lock not acquired (already locked by another node -> nil result)
	clientNil := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeNilResult()
	}))
	repoNil := NewValkeyRepository(clientNil)
	acquired, err = repoNil.AcquireLock(ctx, "lock-2", 10*time.Second)
	if err != nil || acquired {
		t.Fatalf("expected acquired=false with nil err, got acquired=%v, err=%v", acquired, err)
	}

	// 3. Network / Valkey error
	errLock := errors.New("lock error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errLock)
	}))
	repoErr := NewValkeyRepository(clientErr)
	acquired, err = repoErr.AcquireLock(ctx, "lock-3", 10*time.Second)
	if acquired || !errors.Is(err, errLock) {
		t.Fatalf("expected acquired=false with errLock, got acquired=%v, err=%v", acquired, err)
	}

	// 4. Sub-second TTL clamped to 1 second
	clientClamp := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repoClamp := NewValkeyRepository(clientClamp)
	_, _ = repoClamp.AcquireLock(ctx, "lock-4", 200*time.Millisecond)
	clampCmd := clientClamp.LastCommandStrings()
	// Command: SET party2:scheduled:lock:lock-4 1 NX EX 1
	if len(clampCmd) < 6 || clampCmd[len(clampCmd)-1] != "1" {
		t.Fatalf("expected TTL clamped to 1s in %v", clampCmd)
	}
}

func TestValkeyRepository_Save(t *testing.T) {
	ctx := context.Background()

	// 1. Save Completed state with future RetainUntil
	clientCompleted := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repoCompleted := NewValkeyRepository(clientCompleted)

	futureRetain := time.Now().Add(time.Hour)
	completedAction := core_scheduling.ScheduledAction{
		ID:          "comp-1",
		ActionType:  "test:completed",
		ActorID:     "char-c1",
		State:       core_scheduling.StateCompleted,
		RetainUntil: futureRetain,
	}

	if err := repoCompleted.Save(ctx, completedAction); err != nil {
		t.Fatalf("unexpected Save error: %v", err)
	}

	cmdsComp := clientCompleted.RecordedCommandStrings()
	// Should do: ZREM pending, SREM actor, DEL lock, SET action with EX
	var operations []string
	for _, c := range cmdsComp {
		operations = append(operations, c[0])
	}
	expectedOps := []string{"ZREM", "SREM", "DEL", "SET"}
	if !reflect.DeepEqual(operations, expectedOps) {
		t.Fatalf("expected operations %v, got %v", expectedOps, operations)
	}

	// 2. Save Failed state with past RetainUntil (ttl <= 0 -> DEL action immediately)
	clientPast := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repoPast := NewValkeyRepository(clientPast)

	pastRetain := time.Now().Add(-10 * time.Minute)
	failedAction := core_scheduling.ScheduledAction{
		ID:          "fail-1",
		ActionType:  "test:failed",
		ActorID:     "",
		State:       core_scheduling.StateFailed,
		RetainUntil: pastRetain,
	}
	if err := repoPast.Save(ctx, failedAction); err != nil {
		t.Fatalf("unexpected Save error: %v", err)
	}
	cmdsPast := clientPast.RecordedCommandStrings()
	// ZREM pending, DEL lock, DEL action (no SREM because ActorID is empty)
	var opsPast []string
	for _, c := range cmdsPast {
		opsPast = append(opsPast, c[0])
	}
	expectedOpsPast := []string{"ZREM", "DEL", "DEL"}
	if !reflect.DeepEqual(opsPast, expectedOpsPast) {
		t.Fatalf("expected operations %v, got %v", expectedOpsPast, opsPast)
	}

	// 3. Save Processing state (simple SET without TTL, no ZREM or SREM)
	clientProc := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repoProc := NewValkeyRepository(clientProc)

	procAction := core_scheduling.ScheduledAction{
		ID:         "proc-1",
		ActionType: "test:proc",
		ActorID:    "char-p1",
		State:      core_scheduling.StateProcessing,
	}
	if err := repoProc.Save(ctx, procAction); err != nil {
		t.Fatalf("unexpected Save error: %v", err)
	}
	cmdsProc := clientProc.RecordedCommandStrings()
	if len(cmdsProc) != 1 || cmdsProc[0][0] != "SET" || strings.Contains(strings.Join(cmdsProc[0], " "), "EX") {
		t.Fatalf("expected single SET without TTL, got %v", cmdsProc)
	}
}

func TestValkeyRepository_CancelByActorID(t *testing.T) {
	ctx := context.Background()

	// 1. Empty actorID is no-op
	clientEmpty := valkeytest.NewMockClient()
	repoEmpty := NewValkeyRepository(clientEmpty)
	if err := repoEmpty.CancelByActorID(ctx, ""); err != nil {
		t.Fatalf("expected nil error for empty actorID, got %v", err)
	}
	if len(clientEmpty.RecordedCommands()) != 0 {
		t.Fatal("expected no commands recorded for empty actorID")
	}

	// 2. SMEMBERS fails with error
	errSmembers := errors.New("smembers error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errSmembers)
	}))
	repoErr := NewValkeyRepository(clientErr)
	if err := repoErr.CancelByActorID(ctx, "char-err"); !errors.Is(err, errSmembers) {
		t.Fatalf("expected errSmembers, got %v", err)
	}

	// 3. SMEMBERS returns nil (no actions for actor)
	clientNil := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeNilResult()
	}))
	repoNil := NewValkeyRepository(clientNil)
	if err := repoNil.CancelByActorID(ctx, "char-nil"); err != nil {
		t.Fatalf("expected nil error when actor key is nil, got %v", err)
	}

	// 4. Multiple actions canceled
	clientSuccess := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "SMEMBERS" {
			return valkeytest.MakeStringSliceResult([]string{"act-a", "act-b"})
		}
		return valkeytest.MakeOKResult()
	}))
	repoSuccess := NewValkeyRepository(clientSuccess)
	if err := repoSuccess.CancelByActorID(ctx, "char-multi"); err != nil {
		t.Fatalf("unexpected CancelByActorID error: %v", err)
	}

	cmds := clientSuccess.RecordedCommandStrings()
	// SMEMBERS, then for act-a (ZREM, DEL, DEL), for act-b (ZREM, DEL, DEL), then DEL actor
	if cmds[0][0] != "SMEMBERS" || cmds[0][1] != actorKeyPrefix+"char-multi" {
		t.Errorf("unexpected SMEMBERS command: %v", cmds[0])
	}
	lastCmd := cmds[len(cmds)-1]
	if lastCmd[0] != "DEL" || lastCmd[1] != actorKeyPrefix+"char-multi" {
		t.Errorf("unexpected final DEL command for actor: %v", lastCmd)
	}
}

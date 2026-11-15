package scheduling_test

import (
	"context"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	core "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestRequeuePendingPreservesStoredState(t *testing.T) {
	client := testutil.OpenValkey(t)
	ctx := context.Background()
	repo := scheduling.NewValkeyRepository(client)
	action := validTestAction("requeue:"+id.New(), time.Now().UTC().Add(-time.Hour))
	action.ActorID = action.ID
	defer cleanupKeys(t, client, action.ID)
	defer func() {
		if _, err := repo.CancelByActorID(ctx, action.ActorID); err != nil {
			t.Error(err)
		}
	}()
	if err := repo.Schedule(ctx, action); err != nil {
		t.Fatal(err)
	}
	payload, err := client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+action.ID).Build()).ToString()
	if err != nil {
		t.Fatal(err)
	}
	if acquired, err := repo.AcquireLock(ctx, action.ID, time.Minute); err != nil || !acquired {
		t.Fatalf("lock: %v, %v", acquired, err)
	}
	if err := client.Do(ctx, client.B().Zrem().Key("party2:scheduled:pending").Member(action.ID).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := repo.RequeuePending(ctx, action); err != nil {
			t.Fatal(err)
		}
	}
	score, err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(action.ID).Build()).AsFloat64()
	if err != nil || score != float64(action.ExecuteAt.Unix()) {
		t.Fatalf("restored score: %v, %v", score, err)
	}
	current, err := client.Do(ctx, client.B().Get().Key("party2:scheduled:action:"+action.ID).Build()).ToString()
	if err != nil || current != payload {
		t.Fatalf("payload changed: %v", err)
	}
	actorMembers, err := client.Do(ctx, client.B().Smembers().Key("party2:scheduled:actor:"+action.ActorID).Build()).AsStrSlice()
	if err != nil || len(actorMembers) != 1 || actorMembers[0] != action.ID {
		t.Fatalf("actor index: %v, %v", actorMembers, err)
	}
	if acquired, err := repo.AcquireLock(ctx, action.ID, time.Minute); err != nil || acquired {
		t.Fatalf("lock was replaced: %v, %v", acquired, err)
	}

	// An existing queue score wins over an earlier discovery snapshot.
	newScore := float64(action.ExecuteAt.Unix() + 60)
	if err := client.Do(ctx, client.B().Zadd().Key("party2:scheduled:pending").ScoreMember().ScoreMember(newScore, action.ID).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := repo.RequeuePending(ctx, action); err != nil {
		t.Fatal(err)
	}
	score, err = client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(action.ID).Build()).AsFloat64()
	if err != nil || score != newScore {
		t.Fatalf("existing score changed: %v, %v", score, err)
	}

	// Processing may be published after discovery; queue repair must never revert its payload.
	processing := action
	processing.State = core.StateProcessing
	if err := repo.Save(ctx, processing); err != nil {
		t.Fatal(err)
	}
	if err := client.Do(ctx, client.B().Zrem().Key("party2:scheduled:pending").Member(action.ID).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := repo.RequeuePending(ctx, processing); err != nil {
		t.Fatal(err)
	}
	if err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(action.ID).Build()).Error(); !valkey.IsValkeyNil(err) {
		t.Fatalf("Processing was enqueued: %v", err)
	}
	if err := repo.RequeuePending(ctx, action); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindPendingByActorID(ctx, action.ActorID)
	if err != nil || len(got) != 1 || got[0].State != core.StateProcessing {
		t.Fatalf("Processing payload changed: %+v, %v", got, err)
	}
	due, err := repo.FetchDue(ctx, time.Now(), 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range due {
		if candidate.ID == action.ID {
			t.Fatal("stale repair made Processing executable")
		}
	}
	if err := client.Do(ctx, client.B().Zscore().Key("party2:scheduled:pending").Member(action.ID).Build()).Error(); !valkey.IsValkeyNil(err) {
		t.Fatalf("Processing queue metadata not cleaned: %v", err)
	}
}

package playercontext

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
)

type queryReaders struct {
	character character.Character
	actions   []scheduling.ScheduledAction
	remaining time.Duration
	asleep    bool
	errAt     string
	err       error
	ctx       context.Context
	calls     []string
}

func (r *queryReaders) read(ctx context.Context, id, stage string) error {
	if ctx != r.ctx || id != r.character.ID {
		return errors.New("query did not preserve context or character ID")
	}
	r.calls = append(r.calls, stage)
	if stage == r.errAt {
		return r.err
	}
	return ctx.Err()
}

func (r *queryReaders) FindByID(ctx context.Context, id string) (character.Character, error) {
	return r.character, r.read(ctx, id, "character")
}

func (r *queryReaders) FindPendingByActorID(ctx context.Context, id string) ([]scheduling.ScheduledAction, error) {
	return r.actions, r.read(ctx, id, "actions")
}

func (r *queryReaders) GetRemainingLock(ctx context.Context, category, id string) (time.Duration, error) {
	if category != timer.CategorySleep {
		return 0, errors.New("wrong remaining timer category")
	}
	return r.remaining, r.read(ctx, id, "sleep")
}

func (r *queryReaders) IsLocked(ctx context.Context, category, id string) (bool, error) {
	if category != timer.CategoryAsleep {
		return false, errors.New("wrong asleep timer category")
	}
	return r.asleep, r.read(ctx, id, "asleep")
}

func TestQuery_ReadStateAndAvailability(t *testing.T) {
	for _, tc := range []struct {
		name         string
		remaining    time.Duration
		asleep       bool
		actions      []scheduling.ScheduledAction
		wantSleeping bool
		wantWake     bool
		wantActions  []string
	}{
		{name: "awake"},
		{name: "sleeping", remaining: time.Minute, asleep: true, wantSleeping: true, wantActions: []string{"rescue_request"}},
		{name: "penalty timer without asleep flag", remaining: time.Minute, wantSleeping: true, wantActions: []string{"rescue_request"}},
		{name: "ready to wake", asleep: true, wantSleeping: true, wantWake: true, wantActions: []string{"home_wake", "rescue_request"}},
		{name: "pending work", actions: []scheduling.ScheduledAction{{ID: "pending", ActorID: "character", State: scheduling.StatePending}}, wantActions: []string{"rescue_request"}},
		{name: "processing work", actions: []scheduling.ScheduledAction{{ID: "processing", ActorID: "character", State: scheduling.StateProcessing}}, wantActions: []string{"rescue_request"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, tc.name)
			char := healthySnapshot().Character
			char.ID = "character"
			readers := &queryReaders{character: char, actions: tc.actions, remaining: tc.remaining, asleep: tc.asleep, ctx: ctx}
			got, err := NewService(readers, readers, readers).Query(ctx, char.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := Snapshot{Character: char, OngoingActions: tc.actions, SleepRemaining: tc.remaining, Sleeping: tc.wantSleeping, CanWake: tc.wantWake, LocationID: LocationTown}
			if !reflect.DeepEqual(got.Snapshot, want) {
				t.Fatalf("snapshot=%+v, want %+v", got.Snapshot, want)
			}
			if tc.wantActions == nil {
				tc.wantActions = Evaluate(want)
			}
			if !reflect.DeepEqual(got.AvailableActions, tc.wantActions) {
				t.Fatalf("actions=%v, want %v", got.AvailableActions, tc.wantActions)
			}
			if !reflect.DeepEqual(readers.calls, []string{"character", "actions", "sleep", "asleep"}) {
				t.Fatalf("state must be read once: %v", readers.calls)
			}
		})
	}
}

func TestQuery_PropagatesReadErrors(t *testing.T) {
	stages := []string{"character", "actions", "sleep", "asleep"}
	for i, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			wantErr := errors.New(stage + " unavailable")
			readers := &queryReaders{character: character.Character{ID: "character"}, ctx: ctx, errAt: stage, err: wantErr}
			got, err := NewService(readers, readers, readers).Query(ctx, "character")
			if !errors.Is(err, wantErr) || !reflect.DeepEqual(got, Result{}) {
				t.Fatalf("got %+v, %v; want zero result and %v", got, err, wantErr)
			}
			if !reflect.DeepEqual(readers.calls, stages[:i+1]) {
				t.Fatalf("read continued after error: %v", readers.calls)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	readers := &queryReaders{character: character.Character{ID: "character"}, ctx: ctx}
	if _, err := NewService(readers, readers, readers).Query(ctx, "character"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestQuery_ExistingTimerLifecycle(t *testing.T) {
	ctx := context.Background()
	char := healthySnapshot().Character
	char.ID = "character"
	readers := &queryReaders{character: char, ctx: ctx}
	timers := timer.NewService(nil)
	service := NewService(readers, readers, timers)
	for _, category := range []string{timer.CategorySleep, timer.CategoryAsleep} {
		if err := timers.SetLock(ctx, category, char.ID, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	assertActions := func(want []string) {
		t.Helper()
		got, err := service.Query(ctx, char.ID)
		if err != nil || !reflect.DeepEqual(got.AvailableActions, want) {
			t.Fatalf("actions=%v, err=%v, want %v", got.AvailableActions, err, want)
		}
	}
	assertActions([]string{"rescue_request"})
	if err := timers.ReleaseLock(ctx, timer.CategorySleep, char.ID); err != nil {
		t.Fatal(err)
	}
	assertActions([]string{"home_wake", "rescue_request"})
	if err := timers.ReleaseLock(ctx, timer.CategoryAsleep, char.ID); err != nil {
		t.Fatal(err)
	}
	assertActions(Evaluate(healthySnapshot()))
}

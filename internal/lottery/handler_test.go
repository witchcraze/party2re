package lottery_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/scheduling"
)

type memScheduledRepo struct {
	actions map[string]core_scheduling.ScheduledAction
}

func newMemScheduledRepo() *memScheduledRepo {
	return &memScheduledRepo{actions: make(map[string]core_scheduling.ScheduledAction)}
}

func (r *memScheduledRepo) Schedule(_ context.Context, action core_scheduling.ScheduledAction) error {
	r.actions[action.ID] = action
	return nil
}

func (r *memScheduledRepo) FetchDue(_ context.Context, upTo time.Time, limit int) ([]core_scheduling.ScheduledAction, error) {
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

func (r *memScheduledRepo) AcquireLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (r *memScheduledRepo) Save(_ context.Context, action core_scheduling.ScheduledAction) error {
	r.actions[action.ID] = action
	return nil
}

func (r *memScheduledRepo) CancelByActorID(_ context.Context, actorID string) (int, error) {
	count := 0
	for id, a := range r.actions {
		if a.ActorID == actorID {
			delete(r.actions, id)
			count++
		}
	}
	return count, nil
}

func (r *memScheduledRepo) FindPendingByActorID(_ context.Context, actorID string) ([]core_scheduling.ScheduledAction, error) {
	var out []core_scheduling.ScheduledAction
	for _, a := range r.actions {
		if a.ActorID == actorID && (a.State == core_scheduling.StatePending || a.State == core_scheduling.StateProcessing) {
			out = append(out, a)
		}
	}
	return out, nil
}

type failingScheduler struct {
	err     error
	findErr error
}

func (f failingScheduler) ScheduleWithID(context.Context, string, string, string, map[string]string, time.Time) error {
	return f.err
}

func (f failingScheduler) FindPendingByActorID(context.Context, string) ([]core_scheduling.ScheduledAction, error) {
	return nil, f.findErr
}

func TestDrawHandler_OverdueAndSameIDCollision(t *testing.T) {
	origReader := rand.Reader
	rand.Reader = bytes.NewReader(make([]byte, 1024))
	defer func() { rand.Reader = origReader }()

	jst := time.FixedZone("JST", 9*60*60)
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, jst).UTC()
	expectedNext := lottery.NextDrawDateJST(now) // 2026-11-21 00:00 JST
	collidingID := lottery.DrawActionID(expectedNext)

	var createdRound lottery.TakarakujiRound
	repo := &mockLotteryRepo{
		getActiveTakarakujiRoundFn: func(context.Context) (lottery.TakarakujiRound, error) {
			return lottery.TakarakujiRound{
				RoundID:      1,
				DrawDate:     now.Add(-time.Minute),
				Prize1ItemID: "item-129",
				Prize1Amount: 1,
			}, nil
		},
		createTakarakujiRoundFn: func(_ context.Context, round lottery.TakarakujiRound) (lottery.TakarakujiRound, error) {
			round.RoundID = 2
			createdRound = round
			return round, nil
		},
	}

	svc, err := lottery.NewService(repo, lottery.WithClock(fixedClock{now: now}))
	if err != nil {
		t.Fatal(err)
	}

	schedRepo := newMemScheduledRepo()
	schedSvc := scheduling.NewService(schedRepo)
	overdueAction := core_scheduling.ScheduledAction{
		ID:          collidingID,
		ActionType:  lottery.ActionTypeTakarakujiDraw,
		ActorID:     "system",
		ScheduledAt: now.Add(-24 * time.Hour),
		ExecuteAt:   time.Date(2026, 10, 1, 0, 0, 0, 0, jst).UTC(), // stale past date
		State:       core_scheduling.StatePending,
	}
	if err := schedRepo.Schedule(context.Background(), overdueAction); err != nil {
		t.Fatal(err)
	}

	worker := scheduling.NewWorker(schedRepo, time.Second, logging.Nop())
	worker.RegisterHandler(lottery.ActionTypeTakarakujiDraw, lottery.NewDrawHandler(svc, lottery.WithScheduler(schedSvc)))
	worker.ProcessAction(context.Background(), overdueAction)

	if !createdRound.DrawDate.Equal(expectedNext) {
		t.Fatalf("next round DrawDate = %v, want %v (must anchor to current execution time, not stale ExecuteAt)", createdRound.DrawDate, expectedNext)
	}

	pending, err := schedRepo.FindPendingByActorID(context.Background(), "system")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending actions = %+v, want exactly 1 surviving pending next-round action", pending)
	}
	if !pending[0].ExecuteAt.Equal(expectedNext) || !strings.HasPrefix(pending[0].ID, collidingID) {
		t.Fatalf("unexpected pending next-round action: %+v", pending[0])
	}
	if completed, ok := schedRepo.actions[collidingID]; !ok || completed.State != core_scheduling.StateCompleted {
		t.Fatalf("original action %s state = %+v, want completed", collidingID, completed)
	}
}

func TestDrawHandler_FutureExecuteAt(t *testing.T) {
	origReader := rand.Reader
	rand.Reader = bytes.NewReader(make([]byte, 1024))
	defer func() { rand.Reader = origReader }()

	jst := time.FixedZone("JST", 9*60*60)
	futureDraw := time.Date(2099, 9, 11, 0, 0, 0, 0, jst).UTC()
	expectedNext := lottery.NextDrawDateJST(futureDraw)

	var createdRound lottery.TakarakujiRound
	repo := &mockLotteryRepo{
		getActiveTakarakujiRoundFn: func(context.Context) (lottery.TakarakujiRound, error) {
			return lottery.TakarakujiRound{
				RoundID:      4,
				DrawDate:     futureDraw,
				Prize1ItemID: "item-129",
				Prize1Amount: 1,
			}, nil
		},
		createTakarakujiRoundFn: func(_ context.Context, round lottery.TakarakujiRound) (lottery.TakarakujiRound, error) {
			round.RoundID = 5
			createdRound = round
			return round, nil
		},
	}

	svc, err := lottery.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	schedRepo := newMemScheduledRepo()
	schedSvc := scheduling.NewService(schedRepo)
	h := lottery.NewDrawHandler(svc, lottery.WithScheduler(schedSvc))

	action := core_scheduling.ScheduledAction{
		ID:         lottery.DrawActionID(futureDraw),
		ActionType: lottery.ActionTypeTakarakujiDraw,
		ActorID:    "system",
		ExecuteAt:  futureDraw,
		State:      core_scheduling.StatePending,
	}
	if err := h.Handle(context.Background(), action); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	if !createdRound.DrawDate.Equal(expectedNext) {
		t.Fatalf("next round DrawDate = %v, want %v", createdRound.DrawDate, expectedNext)
	}
	actions, err := schedRepo.FindPendingByActorID(context.Background(), "system")
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || !actions[0].ExecuteAt.Equal(expectedNext) {
		t.Fatalf("next actions = %+v, want pending at %v", actions, expectedNext)
	}
}

func TestDrawHandler_PropagatesScheduleError(t *testing.T) {
	origReader := rand.Reader
	rand.Reader = bytes.NewReader(make([]byte, 1024))
	defer func() { rand.Reader = origReader }()

	schedErr := errors.New("valkey unavailable")
	jst := time.FixedZone("JST", 9*60*60)
	drawTime := time.Date(2099, 9, 11, 0, 0, 0, 0, jst).UTC()
	repo := &mockLotteryRepo{
		getActiveTakarakujiRoundFn: func(context.Context) (lottery.TakarakujiRound, error) {
			return lottery.TakarakujiRound{
				RoundID:      1,
				DrawDate:     drawTime,
				Prize1ItemID: "item-129",
				Prize1Amount: 1,
			}, nil
		},
	}
	svc, err := lottery.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	h := lottery.NewDrawHandler(svc, lottery.WithScheduler(failingScheduler{err: schedErr}))
	err = h.Handle(context.Background(), core_scheduling.ScheduledAction{
		ID:        lottery.DrawActionID(drawTime),
		ExecuteAt: drawTime,
	})
	if !errors.Is(err, schedErr) {
		t.Fatalf("expected schedule error %v, got %v", schedErr, err)
	}
}

package lottery_test

import (
	"context"
	"errors"
	"testing"
	"time"

	core "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/scheduling"
)

func TestDrawReconcile_ReservationsAndReadErrors(t *testing.T) {
	ctx := context.Background()
	date := time.Date(2026, 11, 11, 15, 0, 0, 0, time.UTC)
	for _, state := range []core.State{core.StatePending, core.StateProcessing, core.StateFailed, core.StateCompleted} {
		t.Run(string(state), func(t *testing.T) {
			repo := &mockLotteryRepo{getActiveTakarakujiRoundFn: func(context.Context) (lottery.TakarakujiRound, error) {
				return lottery.TakarakujiRound{RoundID: 2, DrawDate: date}, nil
			}}
			svc, err := lottery.NewService(repo)
			if err != nil {
				t.Fatal(err)
			}
			queue := newMemScheduledRepo()
			old := core.ScheduledAction{ID: lottery.DrawActionID(date), ActorID: "system", ActionType: lottery.ActionTypeTakarakujiDraw, ExecuteAt: date, State: state}
			if err := queue.Save(ctx, old); err != nil {
				t.Fatal(err)
			}
			h := lottery.NewDrawHandler(svc, lottery.WithScheduler(scheduling.NewService(queue)))
			for range 2 {
				if err := h.Reconcile(ctx); err != nil {
					t.Fatal(err)
				}
			}
			actions, err := queue.FindPendingByActorID(ctx, "system")
			if err != nil {
				t.Fatal(err)
			}
			if len(actions) != 1 {
				t.Fatalf("unfinished = %+v", actions)
			}
			if queue.actions[old.ID].State != state {
				t.Fatal("existing record overwritten")
			}
		})
	}
	storageErr := errors.New("SQL unavailable")
	repo := &mockLotteryRepo{getActiveTakarakujiRoundFn: func(context.Context) (lottery.TakarakujiRound, error) { return lottery.TakarakujiRound{}, storageErr }, createTakarakujiRoundFn: func(context.Context, lottery.TakarakujiRound) (lottery.TakarakujiRound, error) {
		t.Fatal("created round after read failure")
		return lottery.TakarakujiRound{}, nil
	}}
	svc, err := lottery.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	queue := newMemScheduledRepo()
	h := lottery.NewDrawHandler(svc, lottery.WithScheduler(scheduling.NewService(queue)))
	if err := h.Reconcile(ctx); !errors.Is(err, storageErr) {
		t.Fatalf("error = %v", err)
	}
	if len(queue.actions) != 0 {
		t.Fatal("scheduled after SQL read failure")
	}
	repo.getActiveTakarakujiRoundFn = func(context.Context) (lottery.TakarakujiRound, error) {
		return lottery.TakarakujiRound{RoundID: 2, DrawDate: date}, nil
	}
	h = lottery.NewDrawHandler(svc, lottery.WithScheduler(failingScheduler{findErr: storageErr}))
	if err := h.Reconcile(ctx); !errors.Is(err, storageErr) {
		t.Fatalf("discovery error = %v", err)
	}
}

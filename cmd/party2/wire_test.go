package main

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/chapel"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/scheduling"
)

type mockSchedRepo struct {
	actions []core_scheduling.ScheduledAction
}

func (m *mockSchedRepo) Schedule(_ context.Context, a core_scheduling.ScheduledAction) error {
	m.actions = append(m.actions, a)
	return nil
}

func (m *mockSchedRepo) FetchDue(_ context.Context, _ time.Time, _ int) ([]core_scheduling.ScheduledAction, error) {
	return nil, nil
}

func (m *mockSchedRepo) AcquireLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (m *mockSchedRepo) Save(_ context.Context, _ core_scheduling.ScheduledAction) error {
	return nil
}

func (m *mockSchedRepo) CancelByActorID(_ context.Context, _ string) error {
	return nil
}

type mockChapelRepo struct {
	clearedAll bool
	clearedID  string
}

func (m *mockChapelRepo) GetBlessing(_ context.Context, _ string) (chapel.CharacterBlessing, error) {
	return chapel.CharacterBlessing{ActiveBlessing: chapel.BlessingNone}, nil
}

func (m *mockChapelRepo) SelectBlessing(_ context.Context, _ string, _ chapel.BlessingType) (chapel.CharacterBlessing, error) {
	return chapel.CharacterBlessing{}, nil
}

func (m *mockChapelRepo) ClearBlessing(_ context.Context, charID string) error {
	m.clearedID = charID
	return nil
}

func (m *mockChapelRepo) ClearAllBlessings(_ context.Context) error {
	m.clearedAll = true
	return nil
}

func TestRegisterWorkerHandlers_ChapelReset(t *testing.T) {
	repo := &mockSchedRepo{}
	worker := scheduling.NewWorker(repo, time.Second, logging.Nop())
	sched := scheduling.NewService(repo)

	soc := &socServices{
		worker: worker,
		sched:  sched,
	}

	chapelRepo := &mockChapelRepo{}
	chapelService, err := chapel.NewService(chapelRepo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	soc.registerWorkerHandlers(nil, chapelService)

	// Dispatch chapel_reset action via worker
	ctx := context.Background()
	action := core_scheduling.ScheduledAction{
		ID:          "act-reset-test",
		ActionType:  chapel.ActionTypeChapelReset,
		ActorID:     "system",
		ScheduledAt: time.Now(),
		ExecuteAt:   time.Now(),
		State:       core_scheduling.StatePending,
	}

	worker.ProcessAction(ctx, action)

	if !chapelRepo.clearedAll {
		t.Errorf("expected ClearAllBlessings to be called on chapel_reset")
	}
}

func TestWireChapelDailyReset(t *testing.T) {
	repo := &mockSchedRepo{}
	sched := scheduling.NewService(repo)

	wireChapelDailyReset(sched)

	if len(repo.actions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(repo.actions))
	}

	action := repo.actions[0]
	if action.ActionType != chapel.ActionTypeChapelReset {
		t.Errorf("expected ActionType %s, got %s", chapel.ActionTypeChapelReset, action.ActionType)
	}
	if action.ActorID != "system" {
		t.Errorf("expected ActorID system, got %s", action.ActorID)
	}
	if !action.ExecuteAt.After(time.Now()) {
		t.Errorf("expected ExecuteAt to be in the future, got %v", action.ExecuteAt)
	}
}

package chapel_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/chapel"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

type mockScheduler struct {
	scheduledActions []struct {
		id         string
		actionType string
		actorID    string
		params     map[string]string
		executeAt  time.Time
	}
}

func (m *mockScheduler) ScheduleWithID(_ context.Context, id, actionType, actorID string, params map[string]string, executeAt time.Time) error {
	m.scheduledActions = append(m.scheduledActions, struct {
		id         string
		actionType string
		actorID    string
		params     map[string]string
		executeAt  time.Time
	}{
		id:         id,
		actionType: actionType,
		actorID:    actorID,
		params:     params,
		executeAt:  executeAt,
	})
	return nil
}

func TestResetHandler_HandleResidualAction_PreservesBlessings(t *testing.T) {
	ctx := context.Background()
	repo := &mockChapelRepo{
		blessing: chapel.CharacterBlessing{
			CharacterID:    "char1",
			ActiveBlessing: chapel.BlessingExp,
			PrayedAt:       time.Now().UTC(),
		},
	}
	svc, err := chapel.NewService(repo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	sched := &mockScheduler{}
	handler := chapel.NewResetHandler(svc, chapel.WithScheduler(sched))

	action := core_scheduling.ScheduledAction{
		ID:         "act-reset-all",
		ActionType: chapel.ActionTypeChapelReset,
		ActorID:    "system",
		Params:     map[string]string{"character_id": "all"},
	}

	if err := handler.Handle(ctx, action); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	// Residual action must be a no-op: blessing is preserved
	b, err := svc.GetBlessing(ctx, "char1")
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingExp {
		t.Errorf("expected blessing to be preserved as BlessingExp, got %v", b.ActiveBlessing)
	}

	// No rescheduled action enqueued
	if len(sched.scheduledActions) != 0 {
		t.Errorf("expected 0 rescheduled actions, got %d", len(sched.scheduledActions))
	}
}

func TestResetHandler_HandleResidualSingle_PreservesBlessings(t *testing.T) {
	ctx := context.Background()
	repo := &mockChapelRepo{
		blessing: chapel.CharacterBlessing{
			CharacterID:    "char-specific",
			ActiveBlessing: chapel.BlessingGold,
			PrayedAt:       time.Now().UTC(),
		},
	}
	svc, err := chapel.NewService(repo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	handler := chapel.NewResetHandler(svc)

	action := core_scheduling.ScheduledAction{
		ID:         "act-reset-single",
		ActionType: chapel.ActionTypeChapelReset,
		ActorID:    "char-specific",
	}

	if err := handler.Handle(ctx, action); err != nil {
		t.Fatalf("Handle single failed: %v", err)
	}

	b, err := svc.GetBlessing(ctx, "char-specific")
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingGold {
		t.Errorf("expected blessing to be preserved as BlessingGold, got %v", b.ActiveBlessing)
	}
}

func TestNextMidnightJST_And_DailyResetActionID(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	// 2026-09-09 15:30:00 JST
	refTime := time.Date(2026, 9, 9, 15, 30, 0, 0, jst)
	next := chapel.NextMidnightJST(refTime)

	expected := time.Date(2026, 9, 10, 0, 0, 0, 0, jst)
	if !next.Equal(expected) {
		t.Errorf("NextMidnightJST = %v, want %v", next, expected)
	}

	actionID := chapel.DailyResetActionID(next)
	expectedID := "chapel_reset:2026-09-10"
	if actionID != expectedID {
		t.Errorf("DailyResetActionID = %s, want %s", actionID, expectedID)
	}
}

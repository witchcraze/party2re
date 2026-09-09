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

func TestResetHandler_HandleAll(t *testing.T) {
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

	handler := chapel.NewResetHandler(svc)

	action := core_scheduling.ScheduledAction{
		ID:         "act-reset-all",
		ActionType: chapel.ActionTypeChapelReset,
		ActorID:    "system",
		Params:     map[string]string{"character_id": "all"},
	}

	if err := handler.Handle(ctx, action); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	b, err := svc.GetBlessing(ctx, "char1")
	if err != nil {
		t.Fatalf("GetBlessing failed: %v", err)
	}
	if b.ActiveBlessing != chapel.BlessingNone {
		t.Errorf("expected BlessingNone, got %v", b.ActiveBlessing)
	}
}

func TestResetHandler_HandleSingle(t *testing.T) {
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

	// Single character reset via ActorID
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
	if b.ActiveBlessing != chapel.BlessingNone {
		t.Errorf("expected BlessingNone, got %v", b.ActiveBlessing)
	}
}

func TestResetHandler_AutoReschedule(t *testing.T) {
	ctx := context.Background()
	repo := &mockChapelRepo{
		blessing: chapel.CharacterBlessing{
			CharacterID:    "char1",
			ActiveBlessing: chapel.BlessingMonster,
		},
	}
	svc, err := chapel.NewService(repo)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	sched := &mockScheduler{}
	handler := chapel.NewResetHandler(svc, chapel.WithScheduler(sched))

	action := core_scheduling.ScheduledAction{
		ID:         "chapel_reset:2026-09-09",
		ActionType: chapel.ActionTypeChapelReset,
		ActorID:    "system",
	}

	if err := handler.Handle(ctx, action); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(sched.scheduledActions) != 1 {
		t.Fatalf("expected 1 rescheduled action, got %d", len(sched.scheduledActions))
	}

	rescheduled := sched.scheduledActions[0]
	if rescheduled.actionType != chapel.ActionTypeChapelReset {
		t.Errorf("expected actionType %s, got %s", chapel.ActionTypeChapelReset, rescheduled.actionType)
	}
	if rescheduled.actorID != "system" {
		t.Errorf("expected actorID system, got %s", rescheduled.actorID)
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

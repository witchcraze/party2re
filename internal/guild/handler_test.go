package guild_test

import (
	"context"
	"testing"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/guild"
)

type mockScheduler struct {
	scheduledActions []struct {
		ID         string
		ActionType string
		ActorID    string
		Params     map[string]string
		ExecuteAt  time.Time
	}
}

func (m *mockScheduler) ScheduleWithID(ctx context.Context, id, actionType, actorID string, params map[string]string, executeAt time.Time) error {
	m.scheduledActions = append(m.scheduledActions, struct {
		ID         string
		ActionType string
		ActorID    string
		Params     map[string]string
		ExecuteAt  time.Time
	}{id, actionType, actorID, params, executeAt})
	return nil
}

func TestInactivityCheckHandler(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)

	disbandedCount := 0
	repo := &mockGuildRepo{
		listInactiveGuildsFn: func(ctx context.Context, cutoff time.Time, limit int) ([]guild.Guild, error) {
			return []guild.Guild{
				{ID: "dead-1"},
				{ID: "dead-2"},
			}, nil
		},
		disbandGuildFn: func(ctx context.Context, guildID string) error {
			disbandedCount++
			return nil
		},
	}

	svc, err := guild.NewService(repo)
	if err != nil {
		t.Fatalf("NewService error: %v", err)
	}

	sched := &mockScheduler{}
	handler := guild.NewInactivityCheckHandler(svc,
		guild.WithScheduler(sched),
	)

	action := core_scheduling.ScheduledAction{
		ID:         "test-action",
		ActionType: guild.ActionTypeGuildInactivityCheck,
	}

	err = handler.Handle(ctx, action)
	if err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	if disbandedCount != 2 {
		t.Errorf("expected 2 guilds disbanded, got %d", disbandedCount)
	}

	if len(sched.scheduledActions) != 1 {
		t.Fatalf("expected 1 rescheduled action, got %d", len(sched.scheduledActions))
	}
	nextAction := sched.scheduledActions[0]
	if nextAction.ActionType != guild.ActionTypeGuildInactivityCheck {
		t.Errorf("expected action type %q, got %q", guild.ActionTypeGuildInactivityCheck, nextAction.ActionType)
	}

	_ = now
}

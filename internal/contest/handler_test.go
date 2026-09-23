package contest_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
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

func (m *mockScheduler) ScheduleWithID(_ context.Context, id, actionType, actorID string, params map[string]string, executeAt time.Time) error {
	m.scheduledActions = append(m.scheduledActions, struct {
		ID         string
		ActionType string
		ActorID    string
		Params     map[string]string
		ExecuteAt  time.Time
	}{id, actionType, actorID, params, executeAt})
	return nil
}

func TestSettlementActionID(t *testing.T) {
	targetTime := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	expected := fmt.Sprintf("contest_settlement:3:%d", targetTime.Unix())
	actual := contest.SettlementActionID(3, targetTime)
	if actual != expected {
		t.Fatalf("expected %q, got %q", expected, actual)
	}
}

func TestContestSettlementHandler_SettleSuccess(t *testing.T) {
	ctx := context.Background()
	startTime := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(contest.ContestCycleDays * 24 * time.Hour)
	now := endTime.Add(time.Minute)

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	// Seed 5 characters and entries for round 1
	for i := 1; i <= 5; i++ {
		charID := fmt.Sprintf("char-%d", i)
		charRepo.characters[charID] = corecharacter.Character{
			ID:   charID,
			Name: fmt.Sprintf("Player%d", i),
		}
		contestRepo.entries[fmt.Sprintf("entry-%d", i)] = contest.ContestEntry{
			ID:            fmt.Sprintf("entry-%d", i),
			Round:         1,
			CharacterID:   charID,
			CharacterName: fmt.Sprintf("Player%d", i),
			Title:         fmt.Sprintf("Photo Title %d", i),
			PhotoID:       fmt.Sprintf("photo-%d", i),
			Votes:         10 - i,
			CreatedAt:     startTime.Add(time.Duration(i) * time.Hour),
		}
	}

	contestRepo.rounds[1] = contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: startTime,
		EndTime:   endTime,
	}

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create contest service: %v", err)
	}

	sched := &mockScheduler{}
	handler := contest.NewContestSettlementHandler(svc, contest.WithScheduler(sched))

	action := core_scheduling.ScheduledAction{
		ID:          contest.SettlementActionID(1, endTime),
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     "system",
		ScheduledAt: startTime,
		ExecuteAt:   endTime,
		State:       core_scheduling.StatePending,
	}

	if err := handler.Handle(ctx, action); err != nil {
		t.Fatalf("expected handler to succeed, got: %v", err)
	}

	// Verify round 1 is settled
	r1 := contestRepo.rounds[1]
	if r1.Status != contest.StatusSettled {
		t.Errorf("expected round 1 to be settled, got %s", r1.Status)
	}

	// Verify next round 2 is active
	r2 := contestRepo.rounds[2]
	if r2.Status != contest.StatusActive {
		t.Errorf("expected round 2 to be active, got %s", r2.Status)
	}

	// Verify scheduler scheduled round 2 settlement
	if len(sched.scheduledActions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(sched.scheduledActions))
	}
	scheduled := sched.scheduledActions[0]
	if scheduled.ActionType != contest.ActionTypeContestSettlement {
		t.Errorf("expected action type %q, got %q", contest.ActionTypeContestSettlement, scheduled.ActionType)
	}
	expectedID := contest.SettlementActionID(2, r2.EndTime)
	if scheduled.ID != expectedID {
		t.Errorf("expected action ID %q, got %q", expectedID, scheduled.ID)
	}
	if !scheduled.ExecuteAt.Equal(r2.EndTime) {
		t.Errorf("expected executeAt %v, got %v", r2.EndTime, scheduled.ExecuteAt)
	}
}

func TestContestSettlementHandler_Postponed(t *testing.T) {
	ctx := context.Background()
	startTime := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(contest.ContestCycleDays * 24 * time.Hour)
	now := endTime.Add(time.Minute)

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	// Only 2 entries (less than minimum 5)
	for i := 1; i <= 2; i++ {
		charID := fmt.Sprintf("char-%d", i)
		charRepo.characters[charID] = corecharacter.Character{
			ID:   charID,
			Name: fmt.Sprintf("Player%d", i),
		}
		contestRepo.entries[fmt.Sprintf("entry-%d", i)] = contest.ContestEntry{
			ID:            fmt.Sprintf("entry-%d", i),
			Round:         1,
			CharacterID:   charID,
			CharacterName: fmt.Sprintf("Player%d", i),
			Title:         fmt.Sprintf("Photo Title %d", i),
			PhotoID:       fmt.Sprintf("photo-%d", i),
			CreatedAt:     startTime.Add(time.Duration(i) * time.Hour),
		}
	}

	contestRepo.rounds[1] = contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: startTime,
		EndTime:   endTime,
	}

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create contest service: %v", err)
	}

	sched := &mockScheduler{}
	handler := contest.NewContestSettlementHandler(svc, contest.WithScheduler(sched))

	action := core_scheduling.ScheduledAction{
		ID:          contest.SettlementActionID(1, endTime),
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     "system",
		ScheduledAt: startTime,
		ExecuteAt:   endTime,
		State:       core_scheduling.StatePending,
	}

	if err := handler.Handle(ctx, action); err != nil {
		t.Fatalf("expected handler to succeed, got: %v", err)
	}

	// Verify round 1 remains active and is extended by 10 days
	r1 := contestRepo.rounds[1]
	if r1.Status != contest.StatusActive {
		t.Errorf("expected round 1 to remain active, got %s", r1.Status)
	}
	expectedEnd := endTime.Add(contest.ContestCycleDays * 24 * time.Hour)
	if !r1.EndTime.Equal(expectedEnd) {
		t.Errorf("expected extended end time %v, got %v", expectedEnd, r1.EndTime)
	}

	// Verify next action scheduled for round 1 at extended end time
	if len(sched.scheduledActions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(sched.scheduledActions))
	}
	scheduled := sched.scheduledActions[0]
	expectedID := contest.SettlementActionID(1, expectedEnd)
	if scheduled.ID != expectedID {
		t.Errorf("expected action ID %q, got %q", expectedID, scheduled.ID)
	}
	if !scheduled.ExecuteAt.Equal(expectedEnd) {
		t.Errorf("expected executeAt %v, got %v", expectedEnd, scheduled.ExecuteAt)
	}
}

func TestContestSettlementHandler_NotReadyToSettle(t *testing.T) {
	ctx := context.Background()
	startTime := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(contest.ContestCycleDays * 24 * time.Hour)
	now := startTime.Add(2 * 24 * time.Hour) // Only 2 days passed

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	contestRepo.rounds[1] = contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: startTime,
		EndTime:   endTime,
	}

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create contest service: %v", err)
	}

	sched := &mockScheduler{}
	handler := contest.NewContestSettlementHandler(svc, contest.WithScheduler(sched))

	action := core_scheduling.ScheduledAction{
		ID:          contest.SettlementActionID(1, endTime),
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     "system",
		ScheduledAt: startTime,
		ExecuteAt:   endTime,
		State:       core_scheduling.StatePending,
	}

	err = handler.Handle(ctx, action)
	if !errors.Is(err, contest.ErrContestNotReadyToSettle) {
		t.Fatalf("expected ErrContestNotReadyToSettle, got %v", err)
	}
	if len(sched.scheduledActions) != 0 {
		t.Errorf("expected no scheduled actions on error, got %d", len(sched.scheduledActions))
	}
}

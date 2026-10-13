package contest_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	scheduleErr    error
	pendingActions []core_scheduling.ScheduledAction
	findPendingErr error
}

func (m *mockScheduler) ScheduleWithID(_ context.Context, id, actionType, actorID string, params map[string]string, executeAt time.Time) error {
	if m.scheduleErr != nil {
		return m.scheduleErr
	}
	m.scheduledActions = append(m.scheduledActions, struct {
		ID         string
		ActionType string
		ActorID    string
		Params     map[string]string
		ExecuteAt  time.Time
	}{id, actionType, actorID, params, executeAt})
	return nil
}

func (m *mockScheduler) FindPendingByActorID(_ context.Context, actorID string) ([]core_scheduling.ScheduledAction, error) {
	if m.findPendingErr != nil {
		return nil, m.findPendingErr
	}
	var res []core_scheduling.ScheduledAction
	for _, a := range m.pendingActions {
		if a.ActorID == actorID {
			res = append(res, a)
		}
	}
	for _, a := range m.scheduledActions {
		if a.ActorID == actorID {
			res = append(res, core_scheduling.ScheduledAction{
				ID:         a.ID,
				ActionType: a.ActionType,
				ActorID:    a.ActorID,
				ExecuteAt:  a.ExecuteAt,
				State:      core_scheduling.StatePending,
			})
		}
	}
	return res, nil
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
	contestRepo.rounds[2] = contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: endTime,
		EndTime:   endTime.Add(contest.ContestCycleDays * 24 * time.Hour),
	}
	for i := 1; i <= 5; i++ {
		contestRepo.entries[fmt.Sprintf("entry-r2-%d", i)] = contest.ContestEntry{
			ID:          fmt.Sprintf("entry-r2-%d", i),
			Round:       2,
			CharacterID: fmt.Sprintf("char-r2-%d", i),
			Title:       fmt.Sprintf("Next Photo %d", i),
		}
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
	if !strings.HasPrefix(scheduled.ID, expectedID) {
		t.Errorf("expected action ID with prefix %q, got %q", expectedID, scheduled.ID)
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

	// Verify round 1 remains active and is extended by 10 days from current check time
	r1 := contestRepo.rounds[1]
	if r1.Status != contest.StatusActive {
		t.Errorf("expected round 1 to remain active, got %s", r1.Status)
	}
	expectedEnd := now.Add(contest.ContestCycleDays * 24 * time.Hour)
	if !r1.EndTime.Equal(expectedEnd) {
		t.Errorf("expected extended end time %v, got %v", expectedEnd, r1.EndTime)
	}

	// Verify next action scheduled for round 1 at extended end time
	if len(sched.scheduledActions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(sched.scheduledActions))
	}
	scheduled := sched.scheduledActions[0]
	expectedID := contest.SettlementActionID(1, expectedEnd)
	if !strings.HasPrefix(scheduled.ID, expectedID) {
		t.Errorf("expected action ID with prefix %q, got %q", expectedID, scheduled.ID)
	}

	if !scheduled.ExecuteAt.Equal(expectedEnd) {
		t.Errorf("expected executeAt %v, got %v", expectedEnd, scheduled.ExecuteAt)
	}
}

func TestContestSettlementHandler_Postponed_DelayedExecution(t *testing.T) {
	ctx := context.Background()
	startTime := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(contest.ContestCycleDays * 24 * time.Hour) // 2026-09-11
	now := endTime.Add(25 * 24 * time.Hour)                             // 2026-10-06 (25 days delayed)
	expectedEnd := now.Add(contest.ContestCycleDays * 24 * time.Hour)   // 2026-10-16 (future!)

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

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

	r1 := contestRepo.rounds[1]
	if !r1.EndTime.Equal(expectedEnd) {
		t.Errorf("expected extended end time %v, got %v", expectedEnd, r1.EndTime)
	}

	if len(sched.scheduledActions) != 1 {
		t.Fatalf("expected 1 scheduled action, got %d", len(sched.scheduledActions))
	}
	scheduled := sched.scheduledActions[0]
	expectedID := contest.SettlementActionID(1, expectedEnd)
	if !strings.HasPrefix(scheduled.ID, expectedID) {
		t.Errorf("expected action ID with prefix %q, got %q", expectedID, scheduled.ID)
	}

	if !scheduled.ExecuteAt.Equal(expectedEnd) {
		t.Errorf("expected executeAt %v, got %v", expectedEnd, scheduled.ExecuteAt)
	}
	if !scheduled.ExecuteAt.After(now) {
		t.Errorf("expected executeAt %v to be strictly in the future after check time %v", scheduled.ExecuteAt, now)
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

func TestParseSettlementActionID(t *testing.T) {
	targetTime := time.Date(2026, 10, 13, 15, 30, 0, 0, time.UTC)
	validID := contest.SettlementActionID(42, targetTime)

	round, parsedTime, ok := contest.ParseSettlementActionID(validID)
	if !ok {
		t.Fatalf("expected ParseSettlementActionID to succeed for %q", validID)
	}
	if round != 42 {
		t.Errorf("expected round 42, got %d", round)
	}
	if !parsedTime.Equal(targetTime) {
		t.Errorf("expected time %v, got %v", targetTime, parsedTime)
	}

	invalidIDs := []string{
		"",
		"invalid_action_id",
		"contest_settlement:abc:123",
		"contest_settlement:1:invalid",
		"other_action:1:12345",
	}
	for _, id := range invalidIDs {
		_, _, ok := contest.ParseSettlementActionID(id)
		if ok {
			t.Errorf("expected ParseSettlementActionID to fail for %q", id)
		}
	}
}

func TestContestSettlementHandler_ScheduleFailureReturnsError(t *testing.T) {
	ctx := context.Background()
	startTime := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	endTime := startTime.Add(contest.ContestCycleDays * 24 * time.Hour)
	now := endTime.Add(time.Minute)

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

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

	sched := &mockScheduler{
		scheduleErr: errors.New("queue unavailable"),
	}
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
	if err == nil {
		t.Fatal("expected error from handler when scheduling fails, got nil")
	}
	if !errors.Is(err, contest.ErrScheduleFailed) {
		t.Fatalf("expected ErrScheduleFailed, got %v", err)
	}

	// SQL round was committed despite scheduling failure
	r1, rErr := contestRepo.GetRoundByNumber(ctx, 1)
	if rErr != nil {
		t.Fatalf("GetRoundByNumber failed: %v", rErr)
	}
	if r1.Status != contest.StatusSettled {
		t.Errorf("expected round 1 to be settled in SQL, got %s", r1.Status)
	}
}

func TestContestSettlementHandler_StaleSettledRound_NoDoubleSettlement(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	r2End := now.Add(5 * 24 * time.Hour)

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	// Round 1 is already settled in SQL
	contestRepo.rounds[1] = contest.ContestRound{
		Round:     1,
		Status:    contest.StatusSettled,
		StartTime: now.Add(-15 * 24 * time.Hour),
		EndTime:   now.Add(-5 * 24 * time.Hour),
	}
	// Round 2 is currently active
	contestRepo.rounds[2] = contest.ContestRound{
		Round:     2,
		Status:    contest.StatusActive,
		StartTime: now.Add(-5 * 24 * time.Hour),
		EndTime:   r2End,
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

	// Old action arriving for Round 1
	actionRound1 := core_scheduling.ScheduledAction{
		ID:          contest.SettlementActionID(1, contestRepo.rounds[1].EndTime),
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     "system",
		ScheduledAt: contestRepo.rounds[1].StartTime,
		ExecuteAt:   contestRepo.rounds[1].EndTime,
		State:       core_scheduling.StatePending,
	}

	err = handler.Handle(ctx, actionRound1)
	if err != nil {
		t.Fatalf("expected nil error on stale round handling, got: %v", err)
	}

	// Round 2 must remain active and NOT settled prematurely
	r2, _ := contestRepo.GetRoundByNumber(ctx, 2)
	if r2.Status != contest.StatusActive {
		t.Errorf("expected round 2 still active, got %s", r2.Status)
	}

	// Active round 2 schedule must be ensured
	if len(sched.scheduledActions) != 1 {
		t.Fatalf("expected 1 scheduled action for round 2, got %d", len(sched.scheduledActions))
	}
	if !strings.HasPrefix(sched.scheduledActions[0].ID, contest.SettlementActionID(2, r2End)) {
		t.Errorf("expected schedule ID for round 2 with prefix, got %q", sched.scheduledActions[0].ID)
	}
}

func TestContestSettlementHandler_AlreadyPostponedRound_NoDoublePostpone(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	oldEndTime := now.Add(-time.Hour)
	newEndTime := now.Add(10 * 24 * time.Hour)

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	// Round 1 preparing was already postponed to newEndTime
	contestRepo.rounds[1] = contest.ContestRound{
		Round:     1,
		Status:    contest.StatusPreparing,
		StartTime: now.Add(-10 * 24 * time.Hour),
		EndTime:   newEndTime,
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

	// Old action with oldEndTime
	actionOld := core_scheduling.ScheduledAction{
		ID:          contest.SettlementActionID(1, oldEndTime),
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     "system",
		ScheduledAt: contestRepo.rounds[1].StartTime,
		ExecuteAt:   oldEndTime,
		State:       core_scheduling.StatePending,
	}

	err = handler.Handle(ctx, actionOld)
	if err != nil {
		t.Fatalf("expected nil error on already-postponed round handling, got: %v", err)
	}

	// EndTime must NOT be extended again
	r1, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if !r1.EndTime.Equal(newEndTime) {
		t.Errorf("expected EndTime %v unchanged, got %v", newEndTime, r1.EndTime)
	}

	// Schedule for newEndTime must be ensured
	if len(sched.scheduledActions) != 1 {
		t.Fatalf("expected 1 scheduled action for round 1, got %d", len(sched.scheduledActions))
	}
	if !strings.HasPrefix(sched.scheduledActions[0].ID, contest.SettlementActionID(1, newEndTime)) {
		t.Errorf("expected schedule ID for round 1 with newEndTime prefix, got %q", sched.scheduledActions[0].ID)
	}
}

func TestContestSettlementHandler_GetRoundByNumberStorageError_PropagatesError(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	dbErr := errors.New("database disk I/O failure")

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()
	contestRepo.getRoundByNumberErr = dbErr

	svc, err := contest.NewService(charRepo, contestRepo, contest.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	sched := &mockScheduler{}
	handler := contest.NewContestSettlementHandler(svc, contest.WithScheduler(sched))

	action := core_scheduling.ScheduledAction{
		ID:          contest.SettlementActionID(1, now),
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     contest.ContestSettlementActorID,
		ScheduledAt: now.Add(-time.Hour),
		ExecuteAt:   now,
		State:       core_scheduling.StatePending,
	}

	err = handler.Handle(ctx, action)
	if err == nil {
		t.Fatal("expected error when GetRoundByNumber fails, got nil")
	}
	if !strings.Contains(err.Error(), dbErr.Error()) {
		t.Fatalf("expected error containing dbErr, got %v", err)
	}
	if len(sched.scheduledActions) != 0 {
		t.Fatalf("expected no scheduled actions on DB error, got %d", len(sched.scheduledActions))
	}
}

func TestContestSettlementHandler_RoundNotFound_ReconcilesWithoutSettling(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	futureEnd := now.Add(10 * 24 * time.Hour)

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()
	// Round 2 is currently active in SQL
	contestRepo.rounds[2] = contest.ContestRound{
		Round:     2,
		Status:    contest.StatusActive,
		StartTime: now,
		EndTime:   futureEnd,
	}

	svc, err := contest.NewService(charRepo, contestRepo, contest.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	sched := &mockScheduler{}
	handler := contest.NewContestSettlementHandler(svc, contest.WithScheduler(sched))

	// Phantom action for round 999 which does NOT exist in SQL
	action := core_scheduling.ScheduledAction{
		ID:          contest.SettlementActionID(999, now),
		ActionType:  contest.ActionTypeContestSettlement,
		ActorID:     contest.ContestSettlementActorID,
		ScheduledAt: now.Add(-time.Hour),
		ExecuteAt:   now,
		State:       core_scheduling.StatePending,
	}

	err = handler.Handle(ctx, action)
	if err != nil {
		t.Fatalf("expected nil error on nonexistent round reconciliation, got: %v", err)
	}

	// Active round 2 must remain active (NOT settled prematurely)
	r2, _ := contestRepo.GetRoundByNumber(ctx, 2)
	if r2.Status != contest.StatusActive {
		t.Errorf("expected round 2 still active, got %s", r2.Status)
	}

	// Valid round 2 schedule must be ensured
	if len(sched.scheduledActions) != 1 {
		t.Fatalf("expected 1 scheduled action for active round 2, got %d", len(sched.scheduledActions))
	}
	if !strings.HasPrefix(sched.scheduledActions[0].ID, contest.SettlementActionID(2, futureEnd)) {
		t.Errorf("expected schedule prefix for round 2, got %s", sched.scheduledActions[0].ID)
	}
}

func TestContestSettlementHandler_ExistingPendingReservation_NotDuplicated(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	endTime := now.Add(10 * 24 * time.Hour)

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()
	contestRepo.rounds[1] = contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now,
		EndTime:   endTime,
	}

	svc, err := contest.NewService(charRepo, contestRepo, contest.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	sched := &mockScheduler{
		pendingActions: []core_scheduling.ScheduledAction{
			{
				ID:         contest.SettlementActionID(1, endTime) + ":existing-nonce",
				ActionType: contest.ActionTypeContestSettlement,
				ActorID:    contest.ContestSettlementActorID,
				ExecuteAt:  endTime,
				State:      core_scheduling.StatePending,
			},
		},
	}
	svc.SetScheduler(sched)

	// Calling ReconcileSettlementSchedule when an action is already pending for endTime
	err = svc.ReconcileSettlementSchedule(ctx)
	if err != nil {
		t.Fatalf("ReconcileSettlementSchedule failed: %v", err)
	}

	// Must NOT add another action
	if len(sched.scheduledActions) != 0 {
		t.Errorf("expected 0 new scheduled actions (existing pending preserved), got %d", len(sched.scheduledActions))
	}
}

func TestContestSettlementHandler_ProcessingReservation_NotOverwritten(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	endTime := now.Add(10 * 24 * time.Hour)

	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()
	contestRepo.rounds[1] = contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now,
		EndTime:   endTime,
	}

	svc, err := contest.NewService(charRepo, contestRepo, contest.WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	sched := &mockScheduler{
		pendingActions: []core_scheduling.ScheduledAction{
			{
				ID:         contest.SettlementActionID(1, endTime) + ":processing-nonce",
				ActionType: contest.ActionTypeContestSettlement,
				ActorID:    contest.ContestSettlementActorID,
				ExecuteAt:  endTime,
				State:      core_scheduling.StateProcessing,
			},
		},
	}
	svc.SetScheduler(sched)

	// Calling ReconcileSettlementSchedule while worker is processing
	err = svc.ReconcileSettlementSchedule(ctx)
	if err != nil {
		t.Fatalf("ReconcileSettlementSchedule failed: %v", err)
	}

	// Must NOT overwrite or add a duplicate action
	if len(sched.scheduledActions) != 0 {
		t.Errorf("expected 0 new scheduled actions (processing protected), got %d", len(sched.scheduledActions))
	}
}

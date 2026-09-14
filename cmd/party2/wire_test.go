package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/chapel"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/eventplaza"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/tavern"
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

	soc.registerWorkerHandlers(nil, chapelService, nil)

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

type mockTavernService struct {
	resetFullnessCalled bool
	claimedCharID       string
}

func (m *mockTavernService) ResetFullness(ctx context.Context, characterID string) error {
	m.resetFullnessCalled = true
	return nil
}

func (m *mockTavernService) ClaimDelivery(ctx context.Context, characterID string) (tavern.OrderResult, error) {
	m.claimedCharID = characterID
	return tavern.OrderResult{}, tavern.ErrNoActiveDelivery
}

func TestWirePostAdventureHook_ResetsFullnessAndClaimsDelivery(t *testing.T) {
	tavernMock := &mockTavernService{}
	postAdventureHook := func(ctx context.Context, characterID string) error {
		_ = tavernMock.ResetFullness(ctx, characterID)
		_, err := tavernMock.ClaimDelivery(ctx, characterID)
		if errors.Is(err, tavern.ErrNoActiveDelivery) || errors.Is(err, tavern.ErrInsufficientFunds) {
			return nil
		}
		return err
	}

	err := postAdventureHook(context.Background(), "char-42")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !tavernMock.resetFullnessCalled {
		t.Errorf("expected ResetFullness to be called")
	}
	if tavernMock.claimedCharID != "char-42" {
		t.Errorf("expected ClaimDelivery for char-42, got %s", tavernMock.claimedCharID)
	}
}

type mockPlazaService struct {
	banquetRecorded   bool
	presenceRecorded  bool
	recordedAttendees int
	recordedDuration  time.Duration
}

func (m *mockPlazaService) RecordVictoryBanquet(ctx context.Context, bossID, bossName, slayerID, slayerName string, tier int) (eventplaza.CelebrationBanquet, error) {
	m.banquetRecorded = true
	attendees := eventplaza.BanquetAttendeesForTier(tier)
	_ = m.RecordBanquetPresence(ctx, "banquet-1", attendees, time.Hour)
	return eventplaza.CelebrationBanquet{
		ID:   "banquet-1",
		Tier: tier,
	}, nil
}

func (m *mockPlazaService) RecordBanquetPresence(ctx context.Context, banquetID string, count int, duration time.Duration) error {
	m.presenceRecorded = true
	m.recordedAttendees = count
	m.recordedDuration = duration
	return nil
}

func TestWireBossVictoryBanquetHook_RecordsPresence(t *testing.T) {
	plazaMock := &mockPlazaService{}
	hook := func(ctx context.Context, bossID, bossName, slayerID, slayerName string, tier int) error {
		_, err := plazaMock.RecordVictoryBanquet(ctx, bossID, bossName, slayerID, slayerName, tier)
		return err
	}

	for _, tier := range []int{1, 2, 3} {
		plazaMock.banquetRecorded = false
		plazaMock.presenceRecorded = false
		plazaMock.recordedAttendees = 0

		err := hook(context.Background(), "boss-1", "King", "slayer-1", "Hero", tier)
		if err != nil {
			t.Fatalf("tier %d: expected nil error, got %v", tier, err)
		}
		if !plazaMock.banquetRecorded {
			t.Errorf("tier %d: expected banquet to be recorded", tier)
		}
		if !plazaMock.presenceRecorded {
			t.Errorf("tier %d: expected presence to be recorded", tier)
		}
		expectedAttendees := eventplaza.BanquetAttendeesForTier(tier)
		if plazaMock.recordedAttendees != expectedAttendees {
			t.Errorf("tier %d: expected %d attendees, got %d", tier, expectedAttendees, plazaMock.recordedAttendees)
		}
		if plazaMock.recordedDuration != time.Hour {
			t.Errorf("tier %d: expected duration 1h, got %v", tier, plazaMock.recordedDuration)
		}
	}
}

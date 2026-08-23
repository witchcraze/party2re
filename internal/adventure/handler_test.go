package adventure

import (
	"context"
	"testing"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

func TestAdventureCompletionHandler_Handle_ResolvesBattleAndStoresResult(t *testing.T) {
	service, clock, repository, characters := newTestService(t)
	handler := NewAdventureCompletionHandler(service)

	adv, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}

	clock.now = adv.AvailableAt

	action := core_scheduling.ScheduledAction{
		Params: map[string]string{"adventure_id": adv.ID},
	}
	if err := handler.Handle(context.Background(), action); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if !repository.value.Claimed || !repository.value.Resolved {
		t.Errorf("expected adventure to be claimed and resolved, got claimed=%v, resolved=%v", repository.value.Claimed, repository.value.Resolved)
	}
	if characters.value.Experience != adv.ExperienceReward {
		t.Errorf("expected experience = %d, got %d", adv.ExperienceReward, characters.value.Experience)
	}
	if repository.value.BattleResult.WinnerID != characters.value.ID {
		t.Errorf("expected WinnerID = %s, got %s", characters.value.ID, repository.value.BattleResult.WinnerID)
	}
}

func TestAdventureCompletionHandler_Handle_IdempotentIfAlreadyClaimed(t *testing.T) {
	service, clock, _, characters := newTestService(t)
	handler := NewAdventureCompletionHandler(service)

	adv, err := service.Start(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = adv.AvailableAt

	// Manual claim first
	if _, err := service.Claim(context.Background(), adv.ID); err != nil {
		t.Fatal(err)
	}

	action := core_scheduling.ScheduledAction{
		Params: map[string]string{"adventure_id": adv.ID},
	}
	// Handler should return nil (idempotent)
	if err := handler.Handle(context.Background(), action); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
}

func TestAdventureCompletionHandler_Handle_MissingParam(t *testing.T) {
	service, _, _, _ := newTestService(t)
	handler := NewAdventureCompletionHandler(service)

	action := core_scheduling.ScheduledAction{
		Params: map[string]string{},
	}
	err := handler.Handle(context.Background(), action)
	if err == nil {
		t.Fatal("expected error for missing adventure_id, got nil")
	}
}

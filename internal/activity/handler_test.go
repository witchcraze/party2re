package activity

import (
	"context"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"testing"
)

func TestTrainingHandler_Handle_AppliesReward(t *testing.T) {
	service, clock, activities, characters := newTestService(t)
	handler := NewTrainingHandler(service)

	activity, err := service.StartTraining(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}

	clock.now = activity.AvailableAt

	action := core_scheduling.ScheduledAction{
		Params: map[string]string{"activity_id": activity.ID},
	}
	if err := handler.Handle(context.Background(), action); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	if !activities.value.Claimed {
		t.Error("expected activity to be claimed")
	}
	if characters.value.Experience != TrainingReward {
		t.Errorf("expected experience = %d, got %d", TrainingReward, characters.value.Experience)
	}
}

func TestTrainingHandler_Handle_IdempotentIfAlreadyClaimed(t *testing.T) {
	service, clock, _, characters := newTestService(t)
	handler := NewTrainingHandler(service)

	activity, err := service.StartTraining(context.Background(), characters.value.ID)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = activity.AvailableAt

	// Manual claim first
	if _, err := service.Claim(context.Background(), activity.ID); err != nil {
		t.Fatal(err)
	}

	action := core_scheduling.ScheduledAction{
		Params: map[string]string{"activity_id": activity.ID},
	}
	// Handler should return nil (idempotent)
	if err := handler.Handle(context.Background(), action); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
}

func TestTrainingHandler_Handle_MissingParam(t *testing.T) {
	service, _, _, _ := newTestService(t)
	handler := NewTrainingHandler(service)

	action := core_scheduling.ScheduledAction{
		Params: map[string]string{},
	}
	err := handler.Handle(context.Background(), action)
	if err == nil {
		t.Fatal("expected error for missing activity_id, got nil")
	}
}

package scheduling

import (
	"context"
	"testing"
	"time"
)

func TestService_Schedule(t *testing.T) {
	repo := newMockRepository()
	service := NewService(repo)

	executeAt := time.Now().Add(1 * time.Hour)
	params := map[string]string{"key": "value"}

	id, err := service.Schedule(context.Background(), "test_action", "actor_1", params, executeAt)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if id == "" {
		t.Error("expected non-empty ID")
	}

	if len(repo.actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(repo.actions))
	}

	saved := repo.actions[0]
	if saved.ID != id {
		t.Errorf("expected ID %s, got %s", id, saved.ID)
	}
	if saved.ActionType != "test_action" {
		t.Errorf("expected ActionType test_action, got %s", saved.ActionType)
	}
	if saved.ActorID != "actor_1" {
		t.Errorf("expected ActorID actor_1, got %s", saved.ActorID)
	}
	if saved.Params["key"] != "value" {
		t.Errorf("expected param key=value, got %v", saved.Params)
	}
}

func TestService_CancelByActorID(t *testing.T) {
	repo := newMockRepository()
	service := NewService(repo)
	ctx := context.Background()

	_, err := service.Schedule(ctx, "action_1", "actor_1", nil, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("Schedule error: %v", err)
	}
	_, err = service.Schedule(ctx, "action_2", "actor_2", nil, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("Schedule error: %v", err)
	}
	_, err = service.Schedule(ctx, "action_3", "actor_1", nil, time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	if len(repo.actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(repo.actions))
	}

	if err := service.CancelByActorID(ctx, "actor_1"); err != nil {
		t.Fatalf("CancelByActorID error: %v", err)
	}

	if len(repo.actions) != 1 {
		t.Fatalf("expected 1 action remaining, got %d", len(repo.actions))
	}
	if repo.actions[0].ActorID != "actor_2" {
		t.Errorf("expected remaining action for actor_2, got %s", repo.actions[0].ActorID)
	}
}

func TestService_ClearActiveActions(t *testing.T) {
	repo := newMockRepository()
	service := NewService(repo)
	ctx := context.Background()

	_, _ = service.Schedule(ctx, "training", "char-stuck", nil, time.Now().Add(1*time.Hour))
	_, _ = service.Schedule(ctx, "adventure", "char-stuck", nil, time.Now().Add(2*time.Hour))
	_, _ = service.Schedule(ctx, "other", "char-ok", nil, time.Now().Add(1*time.Hour))

	if err := service.ClearActiveActions(ctx, "char-stuck"); err != nil {
		t.Fatalf("ClearActiveActions failed: %v", err)
	}

	if len(repo.actions) != 1 {
		t.Fatalf("expected 1 remaining action, got %d", len(repo.actions))
	}
	if repo.actions[0].ActorID != "char-ok" {
		t.Errorf("expected char-ok remaining, got %s", repo.actions[0].ActorID)
	}
}

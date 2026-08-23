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

package scheduling

import (
	"fmt"
	"testing"
	"time"
)

func TestScheduledAction(t *testing.T) {
	t.Run("valid transition to processing", func(t *testing.T) {
		action := ScheduledAction{
			ID:          "1",
			State:       StatePending,
			ScheduledAt: time.Now(),
		}

		err := action.MarkProcessing()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if action.State != StateProcessing {
			t.Errorf("expected state to be %s, got %s", StateProcessing, action.State)
		}
		if action.AttemptedAt == nil {
			t.Error("expected AttemptedAt to be set")
		}
	})

	t.Run("invalid transition to processing", func(t *testing.T) {
		action := ScheduledAction{
			ID:    "2",
			State: StateCompleted,
		}

		err := action.MarkProcessing()
		if err == nil {
			t.Fatal("expected error on invalid transition")
		}
	})

	t.Run("valid transition to completed", func(t *testing.T) {
		action := ScheduledAction{
			ID:    "3",
			State: StateProcessing,
		}

		err := action.MarkCompleted(24 * time.Hour)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if action.State != StateCompleted {
			t.Errorf("expected state to be %s, got %s", StateCompleted, action.State)
		}
		if action.CompletedAt == nil {
			t.Error("expected CompletedAt to be set")
		}
		if action.RetainUntil.IsZero() {
			t.Error("expected RetainUntil to be set")
		}
	})

	t.Run("valid transition to failed", func(t *testing.T) {
		action := ScheduledAction{
			ID:    "4",
			State: StateProcessing,
		}

		err := action.MarkFailed(24 * time.Hour)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if action.State != StateFailed {
			t.Errorf("expected state to be %s, got %s", StateFailed, action.State)
		}
		if action.RetainUntil.IsZero() {
			t.Error("expected RetainUntil to be set for failed actions")
		}
	})
}

func validAction() ScheduledAction {
	return ScheduledAction{
		ID:         "abc123",
		ActionType: "training_complete",
		ActorID:    "char-1",
		State:      StatePending,
		ExecuteAt:  time.Now().Add(time.Hour),
	}
}

func TestScheduledAction_Validate(t *testing.T) {
	t.Run("valid action passes", func(t *testing.T) {
		a := validAction()
		if err := a.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("empty ID rejected", func(t *testing.T) {
		a := validAction()
		a.ID = ""
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for empty ID")
		}
	})

	t.Run("oversized ID rejected", func(t *testing.T) {
		a := validAction()
		a.ID = string(make([]byte, MaxIDLength+1))
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for oversized ID")
		}
	})

	t.Run("empty ActionType rejected", func(t *testing.T) {
		a := validAction()
		a.ActionType = ""
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for empty ActionType")
		}
	})

	t.Run("oversized ActionType rejected", func(t *testing.T) {
		a := validAction()
		a.ActionType = string(make([]byte, MaxActionTypeLength+1))
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for oversized ActionType")
		}
	})

	t.Run("empty ActorID rejected", func(t *testing.T) {
		a := validAction()
		a.ActorID = ""
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for empty ActorID")
		}
	})

	t.Run("unknown state rejected", func(t *testing.T) {
		a := validAction()
		a.State = "bogus_state_from_valkey"
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for unknown state")
		}
	})

	t.Run("zero ExecuteAt rejected", func(t *testing.T) {
		a := validAction()
		a.ExecuteAt = time.Time{}
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for zero ExecuteAt")
		}
	})

	t.Run("too many params rejected", func(t *testing.T) {
		a := validAction()
		a.Params = make(map[string]string, MaxParamCount+1)
		for i := range MaxParamCount + 1 {
			a.Params[fmt.Sprintf("k%d", i)] = "v"
		}
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for too many params")
		}
	})

	t.Run("oversized param key rejected", func(t *testing.T) {
		a := validAction()
		a.Params = map[string]string{string(make([]byte, MaxParamKeyLength+1)): "v"}
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for oversized param key")
		}
	})

	t.Run("oversized param value rejected", func(t *testing.T) {
		a := validAction()
		a.Params = map[string]string{"k": string(make([]byte, MaxParamValueLength+1))}
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for oversized param value")
		}
	})
}

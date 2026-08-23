package scheduling

import (
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
		
		err := action.MarkFailed()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		
		if action.State != StateFailed {
			t.Errorf("expected state to be %s, got %s", StateFailed, action.State)
		}
	})
}

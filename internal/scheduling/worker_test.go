package scheduling

import (
	"context"
	"log/slog"
	"testing"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

type mockRepository struct {
	actions []core_scheduling.ScheduledAction
	locked  map[string]bool
	saved   map[string]core_scheduling.ScheduledAction
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		locked: make(map[string]bool),
		saved:  make(map[string]core_scheduling.ScheduledAction),
	}
}

func (m *mockRepository) Schedule(ctx context.Context, action core_scheduling.ScheduledAction) error {
	m.actions = append(m.actions, action)
	return nil
}

func (m *mockRepository) FetchDue(ctx context.Context, upTo time.Time, limit int) ([]core_scheduling.ScheduledAction, error) {
	return m.actions, nil
}

func (m *mockRepository) AcquireLock(ctx context.Context, actionID string, lockTTL time.Duration) (bool, error) {
	if m.locked[actionID] {
		return false, nil
	}
	m.locked[actionID] = true
	return true, nil
}

func (m *mockRepository) Save(ctx context.Context, action core_scheduling.ScheduledAction) error {
	m.saved[action.ID] = action
	return nil
}

type mockHandler struct {
	handled bool
	err     error
}

func (h *mockHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	h.handled = true
	return h.err
}

type mockLogger struct{}

func (m *mockLogger) Info(ctx context.Context, msg string, attrs ...slog.Attr) {}
func (m *mockLogger) Warn(ctx context.Context, msg string, attrs ...slog.Attr) {}
func (m *mockLogger) Error(ctx context.Context, msg string, err error, attrs ...slog.Attr) {}

func TestWorker(t *testing.T) {
	repo := newMockRepository()
	logger := &mockLogger{}
	worker := NewWorker(repo, 1*time.Millisecond, logger)

	handler := &mockHandler{}
	worker.RegisterHandler("test_action", handler)

	// Add a pending action (all required fields set so Validate passes)
	action := core_scheduling.ScheduledAction{
		ID:         "1",
		ActionType: "test_action",
		ActorID:    "char-1",
		State:      core_scheduling.StatePending,
		ExecuteAt:  time.Now().Add(-1 * time.Hour), // Past due
	}
	repo.actions = append(repo.actions, action)

	// Run processAction directly for testing
	worker.processAction(context.Background(), action)

	if !handler.handled {
		t.Error("expected handler to be called")
	}

	saved, ok := repo.saved["1"]
	if !ok {
		t.Fatal("expected action to be saved")
	}

	if saved.State != core_scheduling.StateCompleted {
		t.Errorf("expected state to be %s, got %s", core_scheduling.StateCompleted, saved.State)
	}
}

func TestWorker_AlreadyLocked(t *testing.T) {
	repo := newMockRepository()
	logger := &mockLogger{}
	worker := NewWorker(repo, 1*time.Millisecond, logger)

	handler := &mockHandler{}
	worker.RegisterHandler("test_action", handler)

	action := core_scheduling.ScheduledAction{
		ID:         "2",
		ActionType: "test_action",
		ActorID:    "char-1",
		State:      core_scheduling.StatePending,
		ExecuteAt:  time.Now(),
	}

	// Pre-lock
	repo.locked["2"] = true

	worker.processAction(context.Background(), action)

	if handler.handled {
		t.Error("expected handler NOT to be called when locked")
	}
}

func TestWorker_InvalidActionRejected(t *testing.T) {
	repo := newMockRepository()
	logger := &mockLogger{}
	worker := NewWorker(repo, 1*time.Millisecond, logger)

	handler := &mockHandler{}
	worker.RegisterHandler("test_action", handler)

	// Action with empty ActionType fails Validate: should be skipped before lock/dispatch.
	action := core_scheduling.ScheduledAction{
		ID:         "bad-1",
		ActionType: "", // invalid: empty
		ActorID:    "char-1",
		State:      core_scheduling.StatePending,
		ExecuteAt:  time.Now(),
	}

	worker.processAction(context.Background(), action)

	if handler.handled {
		t.Error("expected handler NOT to be called for invalid action")
	}
	if repo.locked["bad-1"] {
		t.Error("expected lock NOT to be acquired for invalid action")
	}
}


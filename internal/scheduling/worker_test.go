package scheduling

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

type mockRepository struct {
	actions        []core_scheduling.ScheduledAction
	locked         map[string]bool
	saved          map[string]core_scheduling.ScheduledAction
	fetchDueErr    error
	acquireLockErr error
	saveErr        error
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
	if m.fetchDueErr != nil {
		return nil, m.fetchDueErr
	}
	return m.actions, nil
}

func (m *mockRepository) AcquireLock(ctx context.Context, actionID string, lockTTL time.Duration) (bool, error) {
	if m.acquireLockErr != nil {
		return false, m.acquireLockErr
	}
	if m.locked[actionID] {
		return false, nil
	}
	m.locked[actionID] = true
	return true, nil
}

func (m *mockRepository) Save(ctx context.Context, action core_scheduling.ScheduledAction) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved[action.ID] = action
	return nil
}

func (m *mockRepository) CancelByActorID(ctx context.Context, actorID string) error {
	var remaining []core_scheduling.ScheduledAction
	for _, a := range m.actions {
		if a.ActorID != actorID {
			remaining = append(remaining, a)
		} else {
			delete(m.locked, a.ID)
			delete(m.saved, a.ID)
		}
	}
	m.actions = remaining
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

func (m *mockLogger) Info(ctx context.Context, msg string, attrs ...slog.Attr)             {}
func (m *mockLogger) Warn(ctx context.Context, msg string, attrs ...slog.Attr)             {}
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

func TestWorker_Run_ContextCancel(t *testing.T) {
	repo := newMockRepository()
	logger := &mockLogger{}
	worker := NewWorker(repo, 50*time.Millisecond, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		worker.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Succeeded in stopping
	case <-time.After(500 * time.Millisecond):
		t.Fatal("worker.Run did not stop after context cancellation")
	}
}

func TestWorker_Run_TickerExecution(t *testing.T) {
	repo := newMockRepository()
	logger := &mockLogger{}
	worker := NewWorker(repo, 10*time.Millisecond, logger)

	handler := &mockHandler{}
	worker.RegisterHandler("tick_action", handler)

	action := core_scheduling.ScheduledAction{
		ID:         "tick-1",
		ActionType: "tick_action",
		ActorID:    "char-tick",
		State:      core_scheduling.StatePending,
		ExecuteAt:  time.Now().Add(-1 * time.Minute),
	}
	repo.actions = append(repo.actions, action)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go worker.Run(ctx)

	// Wait for ticker to trigger processActions
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if handler.handled {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if !handler.handled {
		t.Fatal("expected handler to be invoked by worker ticker")
	}
}

func TestWorker_processActions_FetchDueError(t *testing.T) {
	repo := newMockRepository()
	repo.fetchDueErr = errors.New("simulated fetch error")
	logger := &mockLogger{}
	worker := NewWorker(repo, time.Minute, logger)

	handler := &mockHandler{}
	worker.RegisterHandler("action", handler)

	// Should not panic, logs error and returns
	worker.processActions(context.Background())

	if handler.handled {
		t.Error("handler should not have been called when FetchDue fails")
	}
}

func TestWorker_processAction_AcquireLockError(t *testing.T) {
	repo := newMockRepository()
	repo.acquireLockErr = errors.New("lock backend failure")
	logger := &mockLogger{}
	worker := NewWorker(repo, time.Minute, logger)

	handler := &mockHandler{}
	worker.RegisterHandler("action", handler)

	action := core_scheduling.ScheduledAction{
		ID:         "lock-err-1",
		ActionType: "action",
		ActorID:    "char-1",
		State:      core_scheduling.StatePending,
		ExecuteAt:  time.Now(),
	}

	worker.processAction(context.Background(), action)

	if handler.handled {
		t.Error("handler should not have been called when AcquireLock errors")
	}
}

func TestWorker_processAction_SaveProcessingError(t *testing.T) {
	repo := newMockRepository()
	repo.saveErr = errors.New("save backend failure")
	logger := &mockLogger{}
	worker := NewWorker(repo, time.Minute, logger)

	handler := &mockHandler{}
	worker.RegisterHandler("action", handler)

	action := core_scheduling.ScheduledAction{
		ID:         "save-err-1",
		ActionType: "action",
		ActorID:    "char-1",
		State:      core_scheduling.StatePending,
		ExecuteAt:  time.Now(),
	}

	worker.processAction(context.Background(), action)

	if handler.handled {
		t.Error("handler should not have been called when Save fails")
	}
}

func TestWorker_processAction_UnregisteredActionType(t *testing.T) {
	repo := newMockRepository()
	logger := &mockLogger{}
	worker := NewWorker(repo, time.Minute, logger)

	action := core_scheduling.ScheduledAction{
		ID:         "unreg-1",
		ActionType: "unknown_type",
		ActorID:    "char-1",
		State:      core_scheduling.StatePending,
		ExecuteAt:  time.Now(),
	}

	worker.processAction(context.Background(), action)

	saved, ok := repo.saved["unreg-1"]
	if !ok {
		t.Fatal("expected action to be saved")
	}
	if saved.State != core_scheduling.StateFailed {
		t.Errorf("expected state to be %s, got %s", core_scheduling.StateFailed, saved.State)
	}
}

func TestWorker_processAction_HandlerError(t *testing.T) {
	repo := newMockRepository()
	logger := &mockLogger{}
	worker := NewWorker(repo, time.Minute, logger)

	handler := &mockHandler{err: errors.New("execution failed")}
	worker.RegisterHandler("failing_action", handler)

	action := core_scheduling.ScheduledAction{
		ID:         "fail-1",
		ActionType: "failing_action",
		ActorID:    "char-1",
		State:      core_scheduling.StatePending,
		ExecuteAt:  time.Now(),
	}

	worker.processAction(context.Background(), action)

	if !handler.handled {
		t.Fatal("expected handler to be called")
	}

	saved, ok := repo.saved["fail-1"]
	if !ok {
		t.Fatal("expected action to be saved")
	}
	if saved.State != core_scheduling.StateFailed {
		t.Errorf("expected state to be %s, got %s", core_scheduling.StateFailed, saved.State)
	}
}

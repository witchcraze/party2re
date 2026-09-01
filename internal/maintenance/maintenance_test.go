package maintenance_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/maintenance"
)

type memoryMaintenanceRepo struct {
	mu     sync.RWMutex
	status maintenance.Status
}

func (m *memoryMaintenanceRepo) GetStatus(ctx context.Context) (maintenance.Status, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status, nil
}

func (m *memoryMaintenanceRepo) SetStatus(ctx context.Context, status maintenance.Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = status
	return nil
}

func TestMaintenanceService(t *testing.T) {
	repo := &memoryMaintenanceRepo{
		status: maintenance.Status{
			Enabled:   false,
			Message:   "System is operating normally.",
			UpdatedAt: time.Now().UTC(),
		},
	}

	svc, err := maintenance.NewService(repo)
	if err != nil {
		t.Fatalf("unexpected error creating service: %v", err)
	}

	ctx := context.Background()

	t.Run("default status is disabled", func(t *testing.T) {
		status, err := svc.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if status.Enabled {
			t.Errorf("expected maintenance to be disabled, got true")
		}
		if svc.IsEnabled(ctx) {
			t.Errorf("expected IsEnabled to return false")
		}
	})

	t.Run("enable maintenance mode", func(t *testing.T) {
		endTime := time.Now().Add(2 * time.Hour).UTC()
		status, err := svc.SetMaintenance(ctx, true, "Emergency upgrade in progress", &endTime)
		if err != nil {
			t.Fatalf("SetMaintenance failed: %v", err)
		}
		if !status.Enabled {
			t.Errorf("expected status.Enabled to be true")
		}
		if status.Message != "Emergency upgrade in progress" {
			t.Errorf("unexpected message: %s", status.Message)
		}
		if status.EstimatedEndTime == nil || *status.EstimatedEndTime != endTime {
			t.Errorf("unexpected estimated end time: %v", status.EstimatedEndTime)
		}
		if !svc.IsEnabled(ctx) {
			t.Errorf("expected IsEnabled to return true")
		}
	})

	t.Run("disable maintenance mode", func(t *testing.T) {
		status, err := svc.SetMaintenance(ctx, false, "", nil)
		if err != nil {
			t.Fatalf("SetMaintenance failed: %v", err)
		}
		if status.Enabled {
			t.Errorf("expected status.Enabled to be false")
		}
		if status.Message != "System is operating normally." {
			t.Errorf("expected default normal message, got: %s", status.Message)
		}
		if status.EstimatedEndTime != nil {
			t.Errorf("expected nil estimated end time, got: %v", status.EstimatedEndTime)
		}
		if svc.IsEnabled(ctx) {
			t.Errorf("expected IsEnabled to return false")
		}
	})

	t.Run("reject message exceeding 500 characters", func(t *testing.T) {
		longMsg := strings.Repeat("A", 501)
		_, err := svc.SetMaintenance(ctx, true, longMsg, nil)
		if err == nil {
			t.Fatalf("expected error for overly long message, got nil")
		}
		if err != maintenance.ErrInvalidMessage {
			t.Errorf("expected ErrInvalidMessage, got: %v", err)
		}
	})

	t.Run("nil repository error", func(t *testing.T) {
		_, err := maintenance.NewService(nil)
		if err == nil {
			t.Fatalf("expected error for nil repo, got nil")
		}
	})
}

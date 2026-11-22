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

	t.Run("accept multibyte and mixed-width messages up to 500 characters after trim", func(t *testing.T) {
		mixed500 := strings.Repeat("A", 200) + strings.Repeat("あ", 250) + strings.Repeat("🎉", 50)
		cases := []struct {
			name    string
			input   string
			wantMsg string
		}{
			{
				name:    "167 Japanese characters (501 UTF-8 bytes)",
				input:   strings.Repeat("あ", 167),
				wantMsg: strings.Repeat("あ", 167),
			},
			{
				name:    "500 ASCII characters with surrounding whitespace",
				input:   "  " + strings.Repeat("A", 500) + "\n\t",
				wantMsg: strings.Repeat("A", 500),
			},
			{
				name:    "500 Japanese characters",
				input:   strings.Repeat("あ", 500),
				wantMsg: strings.Repeat("あ", 500),
			},
			{
				name:    "500 mixed-width characters with surrounding whitespace",
				input:   "  " + mixed500 + "  ",
				wantMsg: mixed500,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				status, err := svc.SetMaintenance(ctx, true, tc.input, nil)
				if err != nil {
					t.Fatalf("SetMaintenance failed: %v", err)
				}
				if status.Message != tc.wantMsg {
					t.Errorf("message mismatch: got len=%d, want len=%d", len(status.Message), len(tc.wantMsg))
				}
			})
		}
	})

	t.Run("reject message exceeding 500 characters without updating repository", func(t *testing.T) {
		sentinel := "persisted sentinel message"
		if _, err := svc.SetMaintenance(ctx, false, sentinel, nil); err != nil {
			t.Fatalf("setup SetMaintenance failed: %v", err)
		}

		cases := []struct {
			name  string
			input string
		}{
			{
				name:  "501 ASCII characters",
				input: strings.Repeat("A", 501),
			},
			{
				name:  "501 Japanese characters",
				input: strings.Repeat("あ", 501),
			},
			{
				name:  "501 mixed-width characters",
				input: strings.Repeat("A", 200) + strings.Repeat("あ", 250) + strings.Repeat("🎉", 51),
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := svc.SetMaintenance(ctx, true, tc.input, nil)
				if err != maintenance.ErrInvalidMessage {
					t.Fatalf("expected ErrInvalidMessage, got: %v", err)
				}
				got, err := repo.GetStatus(ctx)
				if err != nil {
					t.Fatalf("GetStatus failed: %v", err)
				}
				if got.Enabled || got.Message != sentinel {
					t.Errorf("repository mutated on validation failure: got=%+v", got)
				}
			})
		}
	})

	t.Run("nil repository error", func(t *testing.T) {
		_, err := maintenance.NewService(nil)
		if err == nil {
			t.Fatalf("expected error for nil repo, got nil")
		}
	})
}

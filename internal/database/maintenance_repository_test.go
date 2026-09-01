package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/maintenance"
)

func TestMaintenanceRepository_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo, err := NewMaintenanceRepository(db)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	ctx := context.Background()

	t.Run("GetStatus returns valid status", func(t *testing.T) {
		status, err := repo.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if status.Message == "" {
			t.Errorf("expected non-empty message")
		}
	})

	t.Run("SetStatus updates and persists status", func(t *testing.T) {
		endTime := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Second)
		expected := maintenance.Status{
			Enabled:          true,
			Message:          "Integration test maintenance",
			EstimatedEndTime: &endTime,
			UpdatedAt:        time.Now().UTC().Truncate(time.Second),
		}

		if err := repo.SetStatus(ctx, expected); err != nil {
			t.Fatalf("SetStatus failed: %v", err)
		}

		actual, err := repo.GetStatus(ctx)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}

		if actual.Enabled != expected.Enabled {
			t.Errorf("expected Enabled=%v, got %v", expected.Enabled, actual.Enabled)
		}
		if actual.Message != expected.Message {
			t.Errorf("expected Message=%s, got %s", expected.Message, actual.Message)
		}
		if actual.EstimatedEndTime == nil || !actual.EstimatedEndTime.Equal(endTime) {
			t.Errorf("expected EstimatedEndTime=%v, got %v", endTime, actual.EstimatedEndTime)
		}

		// Reset back to disabled
		resetStatus := maintenance.Status{
			Enabled:   false,
			Message:   "System is operating normally.",
			UpdatedAt: time.Now().UTC(),
		}
		_ = repo.SetStatus(ctx, resetStatus)
	})
}

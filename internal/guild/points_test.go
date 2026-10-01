package guild_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/guild"
)

func TestCalculateDecayedPoints(t *testing.T) {
	tests := []struct {
		name     string
		points   int64
		factor   float64
		expected int64
	}{
		{"100 GP with 0.8 factor", 100, 0.8, 80},
		{"9 GP with 0.8 factor", 9, 0.8, 7},
		{"1 GP with 0.8 factor", 1, 0.8, 0},
		{"0 GP with 0.8 factor", 0, 0.8, 0},
		{"10 GP with 0.8 factor", 10, 0.8, 8},
		{"5 GP with 0.8 factor", 5, 0.8, 4},
		{"1000 GP with 0.8 factor", 1000, 0.8, 800},
		{"negative points should return 0", -50, 0.8, 0},
		{"zero factor should return 0", 100, 0.0, 0},
		{"negative factor should return 0", 100, -0.2, 0},
		{"half factor (0.5)", 100, 0.5, 50},
		{"half factor on odd (9, 0.5)", 9, 0.5, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := guild.CalculateDecayedPoints(tc.points, tc.factor)
			if got != tc.expected {
				t.Errorf("CalculateDecayedPoints(%d, %f) = %d; want %d", tc.points, tc.factor, got, tc.expected)
			}
		})
	}
}

func TestService_DecayGuildPoints(t *testing.T) {
	ctx := context.Background()

	t.Run("successful decay with specified factor", func(t *testing.T) {
		var receivedFactor float64
		repo := &mockGuildRepo{
			decayGuildPointsFn: func(ctx context.Context, factor float64) error {
				receivedFactor = factor
				return nil
			},
		}

		svc, err := guild.NewService(repo)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		if err := svc.DecayGuildPoints(ctx, 0.75); err != nil {
			t.Fatalf("DecayGuildPoints failed: %v", err)
		}

		if receivedFactor != 0.75 {
			t.Errorf("expected factor 0.75, got %f", receivedFactor)
		}
	})

	t.Run("default factor applied when factor <= 0", func(t *testing.T) {
		var receivedFactor float64
		repo := &mockGuildRepo{
			decayGuildPointsFn: func(ctx context.Context, factor float64) error {
				receivedFactor = factor
				return nil
			},
		}

		svc, err := guild.NewService(repo)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		if err := svc.DecayGuildPoints(ctx, 0.0); err != nil {
			t.Fatalf("DecayGuildPoints failed: %v", err)
		}

		if receivedFactor != guild.DefaultPointDecayFactor {
			t.Errorf("expected default factor %f, got %f", guild.DefaultPointDecayFactor, receivedFactor)
		}
	})

	t.Run("factor > 1.0 returns ErrInvalidDecayFactor", func(t *testing.T) {
		repo := &mockGuildRepo{}
		svc, err := guild.NewService(repo)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		err = svc.DecayGuildPoints(ctx, 1.05)
		if !errors.Is(err, guild.ErrInvalidDecayFactor) {
			t.Errorf("expected ErrInvalidDecayFactor, got %v", err)
		}
	})

	t.Run("repository error is propagated", func(t *testing.T) {
		expectedErr := errors.New("db error")
		repo := &mockGuildRepo{
			decayGuildPointsFn: func(ctx context.Context, factor float64) error {
				return expectedErr
			},
		}

		svc, err := guild.NewService(repo)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		err = svc.DecayGuildPoints(ctx, 0.8)
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	})
}

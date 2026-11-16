package http_test

import (
	"context"
	"time"

	"github.com/witchcraze/party2re/internal/rescue"
)

type mockRescueService struct {
	emergencyRescueFn func(ctx context.Context, characterID, reason string, now time.Time) (rescue.RescueRecord, error)
	unstuckFn         func(ctx context.Context, characterID string) error
}

func (m *mockRescueService) EmergencyRescue(ctx context.Context, characterID, reason string, now time.Time) (rescue.RescueRecord, error) {
	if m.emergencyRescueFn != nil {
		return m.emergencyRescueFn(ctx, characterID, reason, now)
	}
	return rescue.RescueRecord{
		ID:             "rec-1",
		CharacterID:    characterID,
		Reason:         reason,
		PenaltySeconds: 600,
		CreatedAt:      now,
	}, nil
}

func (m *mockRescueService) Unstuck(ctx context.Context, characterID string) error {
	if m.unstuckFn != nil {
		return m.unstuckFn(ctx, characterID)
	}
	return nil
}

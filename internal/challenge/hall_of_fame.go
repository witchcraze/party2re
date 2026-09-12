package challenge

import (
	"context"
	"strings"
)

// GetHallOfFame retrieves the reigning Hall of Fame party record for the specified challenge tier (vs_challenge.cgi).
func (s *Service) GetHallOfFame(ctx context.Context, tierID string) (*HallOfFameEntry, error) {
	if strings.TrimSpace(tierID) == "" {
		return nil, ErrTierNotFound
	}
	return s.repo.GetHallOfFame(ctx, tierID)
}

// ListHallOfFame lists all Hall of Fame party entries across tiers.
func (s *Service) ListHallOfFame(ctx context.Context) ([]HallOfFameEntry, error) {
	return s.repo.ListHallOfFame(ctx)
}

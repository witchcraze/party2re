package guild

import (
	"context"
	"math"
)

// CalculateDecayedPoints calculates the new points after applying the decay factor.
// Formula: floor(points * factor). Returns 0 if points or factor is <= 0.
// Authentic legacy parity: login.cgi:448 ($gpoint = int($gpoint * 0.8)).
func CalculateDecayedPoints(points int64, factor float64) int64 {
	if points <= 0 || factor <= 0 {
		return 0
	}
	decayed := math.Floor(float64(points) * factor)
	if decayed < 0 {
		return 0
	}
	return int64(decayed)
}

// DecayGuildPoints applies point decay across all guilds with positive points.
// Uses DefaultPointDecayFactor (0.8) if factor <= 0.
func (s *Service) DecayGuildPoints(ctx context.Context, factor float64) error {
	if factor <= 0 {
		factor = DefaultPointDecayFactor
	}
	if factor > 1.0 {
		return ErrInvalidDecayFactor
	}
	return s.repo.DecayGuildPoints(ctx, factor)
}

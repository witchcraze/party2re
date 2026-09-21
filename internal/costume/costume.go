package costume

import (
	"context"
	"time"
)

// ActiveCostume models a character's currently active rented daily costume.
type ActiveCostume struct {
	CharacterID string    `json:"character_id"`
	ItemNo      int       `json:"item_no"`
	ItemName    string    `json:"item_name"`
	Icon        string    `json:"icon"`
	RentedAt    time.Time `json:"rented_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// IsActive reports whether the rented costume has not expired yet.
func (c ActiveCostume) IsActive(now time.Time) bool {
	return !c.ExpiresAt.IsZero() && now.Before(c.ExpiresAt)
}

// Repository defines storage operations for character active costume rentals.
type Repository interface {
	GetActiveCostume(ctx context.Context, characterID string) (*ActiveCostume, error)
	SaveActiveCostume(ctx context.Context, costume ActiveCostume, ttl time.Duration) error
	ClearActiveCostume(ctx context.Context, characterID string) error
}

// CostumeRepository is an alias for Repository for domain readability.
type CostumeRepository = Repository

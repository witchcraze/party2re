package costume

import (
	"context"
	"time"
)

// ActiveCostume models a character's currently active costume appearance.
type ActiveCostume struct {
	CharacterID string    `json:"character_id"`
	ItemNo      int       `json:"item_no"`
	ItemName    string    `json:"item_name"`
	Icon        string    `json:"icon"`
	RentedAt    time.Time `json:"rented_at"`
}

// Repository defines storage operations for character active costume state.
type Repository interface {
	GetActiveCostume(ctx context.Context, characterID string) (*ActiveCostume, error)
	SaveActiveCostume(ctx context.Context, costume ActiveCostume) error
	ClearActiveCostume(ctx context.Context, characterID string) error
	ClearActiveCostumeWithClaim(ctx context.Context, characterID, claimCategory, claimToken string) (bool, error)
	// RestoreActiveCostume undoes a save: only while applied is still the stored
	// value, it puts previous back (or clears when nil); later writes are kept.
	RestoreActiveCostume(ctx context.Context, applied ActiveCostume, previous *ActiveCostume) (bool, error)
}

// CostumeRepository is an alias for Repository for domain readability.
type CostumeRepository = Repository

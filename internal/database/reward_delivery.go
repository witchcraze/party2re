package database

import (
	"context"
	"database/sql"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

// deliverRewardItems validates and delivers reward item instances for a character within an ambient transaction.
// It delegates to the centralized depot.DeliverRewardItems helper enforcing Rank 3 -> Rank 5 lock ordering
// and legacy drop rules (_npc_action.cgi:74-75).
func deliverRewardItems(
	ctx context.Context,
	db *sql.DB,
	char corecharacter.Character,
	items []coreitem.Instance,
) error {
	_, err := DeliverRewardItems(ctx, db, char, items, depot.PolicyTreatOverflowAsLost)
	return err
}

// DeliverRewardItems delivers reward items for a character in the database using the specified policy.
func DeliverRewardItems(
	ctx context.Context,
	db *sql.DB,
	char corecharacter.Character,
	items []coreitem.Instance,
	policy depot.DeliveryPolicy,
) ([]depot.DeliveryResult, error) {
	if len(items) == 0 {
		return nil, nil
	}
	invRepo := &InventoryRepository{db: db}
	depotRepo := &DepotRepository{db: db}
	return depot.DeliverRewardItems(ctx, invRepo, depotRepo, char, items, policy)
}

package database

import (
	"context"
	"database/sql"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

// deliverRewardItems validates and delivers reward item instances for a character within an ambient transaction.
// It enforces the Rank 3 (Inventory) -> Rank 5 (Depot) lock hierarchy:
// 1. Items are delivered to the character's inventory up to coreinventory.DefaultMaxCapacity.
// 2. Any overflow items are routed to the character's depot.
// 3. If the depot is also full, overflow items are treated as lost drops (_npc_action.cgi:74-75).
func deliverRewardItems(
	ctx context.Context,
	db *sql.DB,
	char corecharacter.Character,
	items []coreitem.Instance,
) error {
	if len(items) == 0 {
		return nil
	}

	invRepo := &InventoryRepository{db: db}
	depotRepo := &DepotRepository{db: db}

	// Rank 3: Lock and load inventory
	inv, err := invRepo.FindByCharacterIDForUpdate(ctx, char.ID)
	if err != nil {
		return err
	}

	var overflow []coreitem.Instance
	invChanged := false

	for _, item := range items {
		if !inv.IsFull() {
			if err := inv.Add(item); err == nil {
				invChanged = true
				continue
			}
		}
		overflow = append(overflow, item)
	}

	if invChanged {
		if err := invRepo.Save(ctx, inv); err != nil {
			return err
		}
	}

	if len(overflow) == 0 {
		return nil
	}

	// Rank 5: Lock and load depot
	dep, err := depotRepo.FindByCharacterIDForUpdate(ctx, char.ID)
	if errors.Is(err, depot.ErrNotFound) {
		dep, err = depot.NewDepotWithCapacity(char.ID, char.JobLevel, 0, char.OverDepot)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	depotChanged := false
	for _, item := range overflow {
		if err := dep.AddItem(item); err == nil {
			depotChanged = true
		}
		// If dep.AddItem fails (e.g. depot is full), it is a lost drop matching legacy rules.
	}

	if depotChanged {
		if err := depotRepo.Save(ctx, dep); err != nil {
			return err
		}
	}

	return nil
}

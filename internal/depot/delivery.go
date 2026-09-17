package depot

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

// DeliveryPolicy dictates how item overflow is handled when the character's depot is at capacity.
type DeliveryPolicy int

const (
	// PolicyTreatOverflowAsLost treats items that exceed depot capacity as lost drops without failing the transaction.
	// Matches legacy Party2 Perl CGI combat and exploration rules (_npc_action.cgi:74-75).
	PolicyTreatOverflowAsLost DeliveryPolicy = iota

	// PolicyAbortOnDepotFull returns ErrDepotFull when an item cannot fit into the depot,
	// causing the enclosing transaction to abort/rollback (e.g. quests, purchases, wishes).
	PolicyAbortOnDepotFull
)

// DeliveredTo represents the destination of a delivered reward item.
type DeliveredTo string

const (
	DeliveredToInventory DeliveredTo = "inventory"
	DeliveredToDepot     DeliveredTo = "depot"
	DeliveredToLost      DeliveredTo = "lost"
)

// DeliveryResult records the destination and instance of a processed reward item.
type DeliveryResult struct {
	DeliveredTo DeliveredTo
	Item        item.Instance
}

// InventoryDeliveryRepository defines the minimal persistence operations on inventory needed for reward delivery.
type InventoryDeliveryRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inventory coreinventory.Inventory) error
}

// DepotDeliveryRepository defines the minimal persistence operations on depot needed for reward delivery.
type DepotDeliveryRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (Depot, error)
	Save(ctx context.Context, value Depot) error
}

// DeliverRewardItem delivers a single reward item for a character using the specified delivery policy.
// It constructs an item instance from itemDefID and quantity, then delegates to DeliverRewardItems.
func DeliverRewardItem(
	ctx context.Context,
	invRepo InventoryDeliveryRepository,
	depotRepo DepotDeliveryRepository,
	char corecharacter.Character,
	itemDefID string,
	quantity int,
	policy DeliveryPolicy,
) (DeliveryResult, error) {
	inst, err := item.NewInstance(itemDefID, quantity)
	if err != nil {
		return DeliveryResult{}, err
	}
	results, err := DeliverRewardItems(ctx, invRepo, depotRepo, char, []item.Instance{inst}, policy)
	if err != nil {
		return DeliveryResult{}, err
	}
	if len(results) == 0 {
		return DeliveryResult{}, errors.New("no delivery result returned")
	}
	return results[0], nil
}

// DeliverRewardItemInstance delivers a pre-constructed item instance for a character using the specified delivery policy.
func DeliverRewardItemInstance(
	ctx context.Context,
	invRepo InventoryDeliveryRepository,
	depotRepo DepotDeliveryRepository,
	char corecharacter.Character,
	inst item.Instance,
	policy DeliveryPolicy,
) (DeliveryResult, error) {
	results, err := DeliverRewardItems(ctx, invRepo, depotRepo, char, []item.Instance{inst}, policy)
	if err != nil {
		return DeliveryResult{}, err
	}
	if len(results) == 0 {
		return DeliveryResult{}, errors.New("no delivery result returned")
	}
	return results[0], nil
}

// DeliverRewardItems delivers a list of reward item instances for a character using the configured delivery policy.
// It strictly enforces the lock hierarchy: Rank 3 (Inventory) -> Rank 5 (Depot).
//
// Lifecycle:
// 1. Deliver items to the character's inventory up to coreinventory.DefaultMaxCapacity (if invRepo is provided).
// 2. Any items overflowing inventory are routed to the character's depot.
// 3. Load or initialize the depot, refreshing capacity via dep.RefreshCapacity(char.JobLevel, char.OverDepot).
// 4. If the depot has space, add items and persist the depot.
// 5. If the depot is full:
//   - PolicyTreatOverflowAsLost: mark remaining items as DeliveredToLost.
//   - PolicyAbortOnDepotFull: return ErrDepotFull to abort/rollback the ambient transaction.
func DeliverRewardItems(
	ctx context.Context,
	invRepo InventoryDeliveryRepository,
	depotRepo DepotDeliveryRepository,
	char corecharacter.Character,
	items []item.Instance,
	policy DeliveryPolicy,
) ([]DeliveryResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	results := make([]DeliveryResult, 0, len(items))
	var overflow []item.Instance

	// Rank 3: Inventory check and delivery
	if invRepo != nil {
		inv, err := invRepo.FindByCharacterIDForUpdate(ctx, char.ID)
		if err != nil {
			return nil, err
		}

		invChanged := false
		for _, inst := range items {
			if !inv.IsFull() {
				if err := inv.Add(inst); err == nil {
					invChanged = true
					results = append(results, DeliveryResult{
						DeliveredTo: DeliveredToInventory,
						Item:        inst,
					})
					continue
				}
			}
			overflow = append(overflow, inst)
		}

		if invChanged {
			if err := invRepo.Save(ctx, inv); err != nil {
				return nil, err
			}
		}
	} else {
		overflow = append(overflow, items...)
	}

	if len(overflow) == 0 {
		return results, nil
	}

	// Rank 5: Depot overflow routing
	if depotRepo == nil {
		if policy == PolicyAbortOnDepotFull {
			return nil, ErrDepotFull
		}
		for _, inst := range overflow {
			results = append(results, DeliveryResult{
				DeliveredTo: DeliveredToLost,
				Item:        inst,
			})
		}
		return results, nil
	}

	dep, err := depotRepo.FindByCharacterIDForUpdate(ctx, char.ID)
	if errors.Is(err, ErrNotFound) {
		dep, err = NewDepotWithCapacity(char.ID, char.JobLevel, 0, char.OverDepot)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	dep.RefreshCapacity(char.JobLevel, char.OverDepot)

	depotChanged := false
	for _, inst := range overflow {
		if err := dep.AddItem(inst); err == nil {
			depotChanged = true
			results = append(results, DeliveryResult{
				DeliveredTo: DeliveredToDepot,
				Item:        inst,
			})
		} else {
			if policy == PolicyAbortOnDepotFull {
				return nil, ErrDepotFull
			}
			results = append(results, DeliveryResult{
				DeliveredTo: DeliveredToLost,
				Item:        inst,
			})
		}
	}

	if depotChanged {
		if err := depotRepo.Save(ctx, dep); err != nil {
			return nil, err
		}
	}

	return results, nil
}

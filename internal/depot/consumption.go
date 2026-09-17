package depot

import (
	"context"
	"errors"
	"fmt"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

// StorageLocation represents where an item is located: Inventory or Depot.
type StorageLocation string

const (
	LocationInventory StorageLocation = "inventory"
	LocationDepot     StorageLocation = "depot"
)

// SearchPriority defines which storage container is searched first when an item could exist in both.
type SearchPriority int

const (
	PriorityInventoryFirst SearchPriority = iota
	PriorityDepotFirst
)

// ItemQuery specifies criteria for finding an item in storage.
// At least one of InstanceID, DefinitionID, or Match must be provided.
type ItemQuery struct {
	InstanceID   string
	DefinitionID string
	Match        func(inst item.Instance) bool
}

// QueryByInstanceID constructs an ItemQuery matching a specific instance ID.
func QueryByInstanceID(instanceID string) ItemQuery {
	return ItemQuery{InstanceID: instanceID}
}

// QueryByDefinitionID constructs an ItemQuery matching an item definition ID.
func QueryByDefinitionID(defID string) ItemQuery {
	return ItemQuery{DefinitionID: defID}
}

// QueryByMatch constructs an ItemQuery with a custom predicate function.
func QueryByMatch(match func(inst item.Instance) bool) ItemQuery {
	return ItemQuery{Match: match}
}

// Matches tests whether an item instance satisfies the query criteria.
func (q ItemQuery) Matches(inst item.Instance) bool {
	if q.InstanceID != "" && inst.ID != q.InstanceID {
		return false
	}
	if q.DefinitionID != "" && inst.DefinitionID != q.DefinitionID {
		return false
	}
	if q.Match != nil && !q.Match(inst) {
		return false
	}
	return q.InstanceID != "" || q.DefinitionID != "" || q.Match != nil
}

// ResolvedItem represents an item found in storage along with its location.
type ResolvedItem struct {
	Location StorageLocation
	Item     item.Instance
}

// ConsumptionResult represents the outcome of consuming an item from storage.
type ConsumptionResult struct {
	Location StorageLocation
	Item     item.Instance
}

// ResolveItem finds an item matching query across Inventory and Depot using the specified priority.
func ResolveItem(inv *coreinventory.Inventory, dep *Depot, query ItemQuery, priority SearchPriority) (ResolvedItem, error) {
	if priority == PriorityDepotFirst {
		if res, ok := findInDepot(dep, query); ok {
			return ResolvedItem{Location: LocationDepot, Item: res}, nil
		}
		if res, ok := findInInventory(inv, query); ok {
			return ResolvedItem{Location: LocationInventory, Item: res}, nil
		}
		return ResolvedItem{}, ErrItemNotFound
	}

	// PriorityInventoryFirst (default)
	if res, ok := findInInventory(inv, query); ok {
		return ResolvedItem{Location: LocationInventory, Item: res}, nil
	}
	if res, ok := findInDepot(dep, query); ok {
		return ResolvedItem{Location: LocationDepot, Item: res}, nil
	}
	return ResolvedItem{}, ErrItemNotFound
}

// ConsumeItem resolves and consumes quantity from Inventory or Depot using the specified priority.
// It modifies the source container in-place and returns the ConsumptionResult.
func ConsumeItem(inv *coreinventory.Inventory, dep *Depot, query ItemQuery, priority SearchPriority, quantity int) (ConsumptionResult, error) {
	if quantity <= 0 {
		return ConsumptionResult{}, ErrInvalidQuantity
	}

	resolved, err := ResolveItem(inv, dep, query, priority)
	if err != nil {
		return ConsumptionResult{}, err
	}

	if resolved.Location == LocationInventory {
		if inv == nil {
			return ConsumptionResult{}, ErrItemNotFound
		}
		consumed, err := inv.ConsumeItem(resolved.Item.ID, quantity)
		if err != nil {
			if errors.Is(err, coreinventory.ErrInvalidQuantity) {
				return ConsumptionResult{}, ErrInvalidQuantity
			}
			return ConsumptionResult{}, err
		}
		return ConsumptionResult{Location: LocationInventory, Item: consumed}, nil
	}

	if dep == nil {
		return ConsumptionResult{}, ErrItemNotFound
	}
	consumed, err := dep.Consume(resolved.Item.ID, quantity)
	if err != nil {
		return ConsumptionResult{}, err
	}
	return ConsumptionResult{Location: LocationDepot, Item: consumed}, nil
}

// SaveConsumptionResult persists the modified storage container (Inventory or Depot) based on res.Location.
func SaveConsumptionResult(
	ctx context.Context,
	invRepo InventoryDeliveryRepository,
	depotRepo DepotDeliveryRepository,
	res ConsumptionResult,
	inv coreinventory.Inventory,
	dep Depot,
) error {
	switch res.Location {
	case LocationInventory:
		if invRepo == nil {
			return errors.New("inventory repository is nil")
		}
		return invRepo.Save(ctx, inv)
	case LocationDepot:
		if depotRepo == nil {
			return errors.New("depot repository is nil")
		}
		return depotRepo.Save(ctx, dep)
	default:
		return fmt.Errorf("invalid storage location: %q", res.Location)
	}
}

// ConsumeDualSource locks Inventory and Depot in deterministic Rank 3 -> Rank 5 order,
// consumes the requested item, and saves the modified storage container.
func ConsumeDualSource(
	ctx context.Context,
	invRepo InventoryDeliveryRepository,
	depotRepo DepotDeliveryRepository,
	characterID string,
	query ItemQuery,
	priority SearchPriority,
	quantity int,
) (ConsumptionResult, error) {
	if characterID == "" {
		return ConsumptionResult{}, ErrInvalidCharacterID
	}
	if quantity <= 0 {
		return ConsumptionResult{}, ErrInvalidQuantity
	}

	var inv coreinventory.Inventory
	if invRepo != nil {
		var err error
		inv, err = invRepo.FindByCharacterIDForUpdate(ctx, characterID)
		if err != nil {
			return ConsumptionResult{}, fmt.Errorf("lock inventory: %w", err)
		}
	}

	var dep Depot
	if depotRepo != nil {
		var err error
		dep, err = depotRepo.FindByCharacterIDForUpdate(ctx, characterID)
		if err != nil {
			return ConsumptionResult{}, fmt.Errorf("lock depot: %w", err)
		}
	}

	res, err := ConsumeItem(&inv, &dep, query, priority, quantity)
	if err != nil {
		return ConsumptionResult{}, err
	}

	if err := SaveConsumptionResult(ctx, invRepo, depotRepo, res, inv, dep); err != nil {
		return ConsumptionResult{}, err
	}

	return res, nil
}

func findInInventory(inv *coreinventory.Inventory, query ItemQuery) (item.Instance, bool) {
	if inv == nil {
		return item.Instance{}, false
	}
	for _, inst := range inv.Items {
		if query.Matches(inst) {
			return inst, true
		}
	}
	return item.Instance{}, false
}

func findInDepot(dep *Depot, query ItemQuery) (item.Instance, bool) {
	if dep == nil {
		return item.Instance{}, false
	}
	for _, inst := range dep.Items {
		if query.Matches(inst) {
			return inst, true
		}
	}
	return item.Instance{}, false
}

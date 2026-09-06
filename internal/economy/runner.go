package economy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/event"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

// ResourceCost specifies currency and inventory item costs required to execute a transaction.
type ResourceCost struct {
	Gold              int
	SmallMedals       int
	ItemInstanceID    string
	ItemInstanceQty   int
	ItemDefinitionID  string
	ItemDefinitionQty int
}

// ResourceGrant specifies currency and inventory item rewards granted upon successful execution.
type ResourceGrant struct {
	Gold             int
	SmallMedals      int
	ItemDefinitionID string
	ItemQuantity     int
}

// TxContext provides transactional state to domain callbacks within the lock boundary.
type TxContext struct {
	context.Context
	Character corecharacter.Character
	Inventory coreinventory.Inventory
	grants    []ResourceGrant
	events    []event.Event
}

// AddGrant stages an additional resource grant to be applied before transaction commit.
func (tc *TxContext) AddGrant(grant ResourceGrant) {
	tc.grants = append(tc.grants, grant)
}

// EmitEvent records a domain event to be dispatched upon transaction completion.
func (tc *TxContext) EmitEvent(evt event.Event) {
	if evt != nil {
		tc.events = append(tc.events, evt)
	}
}

// Events returns a copy of all domain events emitted within this transaction context.
func (tc *TxContext) Events() []event.Event {
	return append([]event.Event(nil), tc.events...)
}

// TransactionRequest describes the parameters for a cross-domain atomic transaction.
type TransactionRequest struct {
	CharacterID   string
	Cost          ResourceCost
	CostFunc      func(char corecharacter.Character) (ResourceCost, error)
	Grant         ResourceGrant
	LockInventory bool
}

// TransactionResult contains the outcome of an executed transaction.
type TransactionResult struct {
	Character   corecharacter.Character
	Inventory   coreinventory.Inventory
	GrantedItem *coreitem.Instance
}

// TransactionCallback is invoked within the exclusive row-lock boundary.
type TransactionCallback func(tc *TxContext) error

// TransactionRunner defines the standard interface for executing cross-domain transactional operations.
type TransactionRunner interface {
	ExecuteTransaction(ctx context.Context, req TransactionRequest, fn TransactionCallback) (*TransactionResult, error)
}

// ExecuteTransaction executes a cross-domain transaction enforcing deterministic row-lock order (Rank 2 -> Rank 3).
func (s *Service) ExecuteTransaction(ctx context.Context, req TransactionRequest, fn TransactionCallback) (*TransactionResult, error) {
	charID := strings.TrimSpace(req.CharacterID)
	if charID == "" {
		return nil, ErrInvalidCharacterID
	}

	var result *TransactionResult
	var committedEvents []event.Event

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock Character first (Rank 2: Deterministic lock order)
		char, err := s.characters.FindByIDForUpdate(txCtx, charID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		// 2. Resolve cost (either dynamic CostFunc or static Cost)
		cost := req.Cost
		if req.CostFunc != nil {
			dynCost, err := req.CostFunc(char)
			if err != nil {
				return err
			}
			cost = dynCost
		}

		// 3. Validate amounts
		if cost.Gold < 0 || cost.SmallMedals < 0 {
			return ErrInvalidAmount
		}
		if cost.ItemInstanceQty < 0 || cost.ItemDefinitionQty < 0 {
			return ErrInvalidQuantity
		}

		// 4. Validate and deduct currency costs
		if cost.Gold > 0 {
			if char.Money < cost.Gold {
				return ErrInsufficientGold
			}
			if err := char.DeductMoney(cost.Gold); err != nil {
				return ErrInsufficientGold
			}
		}
		if cost.SmallMedals > 0 {
			if char.SmallMedals < cost.SmallMedals {
				return ErrInsufficientMedals
			}
			if err := char.DeductSmallMedals(cost.SmallMedals); err != nil {
				return ErrInsufficientMedals
			}
		}

		// 5. Determine if inventory locking is needed (Rank 3)
		needsInventory := req.LockInventory ||
			(cost.ItemInstanceID != "" && cost.ItemInstanceQty > 0) ||
			(cost.ItemDefinitionID != "" && cost.ItemDefinitionQty > 0) ||
			(req.Grant.ItemDefinitionID != "" && req.Grant.ItemQuantity > 0)

		var inv coreinventory.Inventory
		inventoryLoaded := false

		if needsInventory {
			inv, err = s.findInventory(txCtx, charID)
			if err != nil {
				return err
			}
			inventoryLoaded = true

			// Deduct item instance if requested
			if cost.ItemInstanceID != "" && cost.ItemInstanceQty > 0 {
				inst, found := inv.Find(cost.ItemInstanceID)
				if !found {
					return ErrItemNotFound
				}
				if inst.Quantity < cost.ItemInstanceQty {
					return ErrInsufficientItemQuantity
				}
				if err := inv.Consume(cost.ItemInstanceID, cost.ItemInstanceQty); err != nil {
					return err
				}
			}

			// Deduct item definition if requested
			if cost.ItemDefinitionID != "" && cost.ItemDefinitionQty > 0 {
				if inv.Quantity(cost.ItemDefinitionID) < cost.ItemDefinitionQty {
					return ErrInsufficientItemQuantity
				}
				remaining := cost.ItemDefinitionQty
				for _, inst := range inv.Items {
					if inst.DefinitionID == cost.ItemDefinitionID && inst.Quantity > 0 {
						toTake := inst.Quantity
						if toTake > remaining {
							toTake = remaining
						}
						_ = inv.Consume(inst.ID, toTake)
						remaining -= toTake
						if remaining <= 0 {
							break
						}
					}
				}
			}
		}

		// 6. Invoke domain callback within row-lock boundary
		tc := &TxContext{
			Context:   txCtx,
			Character: char,
			Inventory: inv,
		}

		if fn != nil {
			if err := fn(tc); err != nil {
				return err
			}
		}

		// 7. Apply resource grants
		allGrants := make([]ResourceGrant, 0, 1+len(tc.grants))
		if req.Grant.Gold > 0 || req.Grant.SmallMedals > 0 || (req.Grant.ItemDefinitionID != "" && req.Grant.ItemQuantity > 0) {
			allGrants = append(allGrants, req.Grant)
		}
		allGrants = append(allGrants, tc.grants...)

		var lastGrantedItem *coreitem.Instance
		for _, g := range allGrants {
			if g.Gold < 0 || g.SmallMedals < 0 {
				return ErrInvalidAmount
			}
			if g.ItemQuantity < 0 {
				return ErrInvalidQuantity
			}
			if g.Gold > 0 {
				_ = tc.Character.AddMoney(g.Gold)
			}
			if g.SmallMedals > 0 {
				_ = tc.Character.AddSmallMedals(g.SmallMedals)
			}
			if g.ItemDefinitionID != "" && g.ItemQuantity > 0 {
				if !inventoryLoaded {
					inv, err = s.findInventory(txCtx, charID)
					if err != nil {
						return err
					}
					tc.Inventory = inv
					inventoryLoaded = true
				}
				newInst, err := coreitem.NewInstance(g.ItemDefinitionID, g.ItemQuantity)
				if err != nil {
					return err
				}
				if err := tc.Inventory.Add(newInst); err != nil {
					return fmt.Errorf("%w: %v", ErrInventoryFull, err)
				}
				lastGrantedItem = &newInst
			}
		}

		// 8. Save modified entities in deterministic order (Rank 2 -> Rank 3)
		if err := s.characters.Update(txCtx, tc.Character); err != nil {
			return err
		}
		if inventoryLoaded {
			if err := s.inventories.Save(txCtx, tc.Inventory); err != nil {
				return err
			}
		}

		// 9. Dispatch synchronous in-tx domain events
		if s.dispatcher != nil && len(tc.events) > 0 {
			for _, evt := range tc.events {
				if err := s.dispatcher.PublishSync(txCtx, evt); err != nil {
					return err
				}
			}
		}

		committedEvents = tc.events
		result = &TransactionResult{
			Character:   tc.Character,
			Inventory:   tc.Inventory,
			GrantedItem: lastGrantedItem,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 10. Dispatch post-commit asynchronous events
	if s.dispatcher != nil && len(committedEvents) > 0 {
		for _, evt := range committedEvents {
			s.dispatcher.PublishAsync(ctx, evt)
		}
	}

	return result, nil
}

// Run executes a typed transaction callback and returns the result and transaction outcome.
func Run[T any](ctx context.Context, s *Service, req TransactionRequest, fn func(tc *TxContext) (T, error)) (T, *TransactionResult, error) {
	var val T
	res, err := s.ExecuteTransaction(ctx, req, func(tc *TxContext) error {
		var callbackErr error
		val, callbackErr = fn(tc)
		return callbackErr
	})
	if err != nil {
		var zero T
		return zero, nil, err
	}
	return val, res, nil
}

package economy

import (
	"context"
	"fmt"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

// GrantItem creates a new item instance and saves it into the character's inventory under an exclusive lock.
func (s *Service) GrantItem(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (coreinventory.Inventory, coreitem.Instance, error) {
	if strings.TrimSpace(characterID) == "" {
		return coreinventory.Inventory{}, coreitem.Instance{}, ErrInvalidCharacterID
	}
	if strings.TrimSpace(itemDefinitionID) == "" {
		return coreinventory.Inventory{}, coreitem.Instance{}, ErrItemNotFound
	}
	if quantity <= 0 {
		return coreinventory.Inventory{}, coreitem.Instance{}, ErrInvalidQuantity
	}

	var resInv coreinventory.Inventory
	var resInst coreitem.Instance

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		inst, err := coreitem.NewInstance(itemDefinitionID, quantity)
		if err != nil {
			return err
		}

		if err := inv.Add(inst); err != nil {
			return fmt.Errorf("%w: %v", ErrInventoryFull, err)
		}

		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}

		resInv = inv
		resInst = inst
		return nil
	})
	if err != nil {
		return coreinventory.Inventory{}, coreitem.Instance{}, err
	}
	return resInv, resInst, nil
}

// ConsumeItemInstance consumes quantity from a specific inventory item instance ID.
func (s *Service) ConsumeItemInstance(ctx context.Context, characterID string, itemInstanceID string, quantity int) (coreinventory.Inventory, error) {
	if strings.TrimSpace(characterID) == "" {
		return coreinventory.Inventory{}, ErrInvalidCharacterID
	}
	if strings.TrimSpace(itemInstanceID) == "" {
		return coreinventory.Inventory{}, ErrItemNotFound
	}
	if quantity <= 0 {
		return coreinventory.Inventory{}, ErrInvalidQuantity
	}

	var resInv coreinventory.Inventory
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		inst, found := inv.Find(itemInstanceID)
		if !found {
			return ErrItemNotFound
		}
		if inst.Quantity < quantity {
			return ErrInsufficientItemQuantity
		}

		if err := inv.Consume(itemInstanceID, quantity); err != nil {
			return err
		}

		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}

		resInv = inv
		return nil
	})
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	return resInv, nil
}

// ConsumeItemDefinition consumes quantity across any item instances matching the definition ID.
func (s *Service) ConsumeItemDefinition(ctx context.Context, characterID string, itemDefinitionID string, quantity int) (coreinventory.Inventory, error) {
	if strings.TrimSpace(characterID) == "" {
		return coreinventory.Inventory{}, ErrInvalidCharacterID
	}
	if strings.TrimSpace(itemDefinitionID) == "" {
		return coreinventory.Inventory{}, ErrItemNotFound
	}
	if quantity <= 0 {
		return coreinventory.Inventory{}, ErrInvalidQuantity
	}

	var resInv coreinventory.Inventory
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		inv, err := s.findInventory(txCtx, characterID)
		if err != nil {
			return err
		}

		if inv.Quantity(itemDefinitionID) < quantity {
			return ErrInsufficientItemQuantity
		}

		remaining := quantity
		for _, inst := range inv.Items {
			if inst.DefinitionID == itemDefinitionID && inst.Quantity > 0 {
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

		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}

		resInv = inv
		return nil
	})
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	return resInv, nil
}

// ExchangeRequest describes a compound atomic economic exchange.
type ExchangeRequest struct {
	CharacterID         string
	DeductGold          int
	AddGold             int
	DeductMedals        int
	AddMedals           int
	ConsumeInstanceID   string
	ConsumeInstanceQty  int
	ConsumeDefinitionID string
	ConsumeDefQty       int
	GrantDefinitionID   string
	GrantQuantity       int
}

// ExchangeResult describes the outcome of a compound economic exchange.
type ExchangeResult struct {
	Character   corecharacter.Character
	Inventory   coreinventory.Inventory
	GrantedItem *coreitem.Instance
}

// Exchange performs a compound atomic currency and inventory exchange following strict lock hierarchy.
func (s *Service) Exchange(ctx context.Context, req ExchangeRequest) (*ExchangeResult, error) {
	if strings.TrimSpace(req.CharacterID) == "" {
		return nil, ErrInvalidCharacterID
	}
	if req.DeductGold < 0 || req.AddGold < 0 || req.DeductMedals < 0 || req.AddMedals < 0 {
		return nil, ErrInvalidAmount
	}
	if req.ConsumeInstanceQty < 0 || req.ConsumeDefQty < 0 || req.GrantQuantity < 0 {
		return nil, ErrInvalidQuantity
	}

	var result *ExchangeResult

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock Character first (Deterministic lock order: characters -> inventory_items)
		char, err := s.findCharacter(txCtx, req.CharacterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// Validate gold and medal deductions
		if req.DeductGold > 0 {
			if char.Money < req.DeductGold {
				return ErrInsufficientGold
			}
			if err := char.DeductMoney(req.DeductGold); err != nil {
				return ErrInsufficientGold
			}
		}
		if req.DeductMedals > 0 {
			if char.SmallMedals < req.DeductMedals {
				return ErrInsufficientMedals
			}
			if err := char.DeductSmallMedals(req.DeductMedals); err != nil {
				return ErrInsufficientMedals
			}
		}
		if req.AddGold > 0 {
			_ = char.AddMoney(req.AddGold)
		}
		if req.AddMedals > 0 {
			_ = char.AddSmallMedals(req.AddMedals)
		}

		// 2. Lock Inventory next
		needsInventory := req.ConsumeInstanceID != "" || req.ConsumeDefinitionID != "" || req.GrantDefinitionID != ""
		var inv coreinventory.Inventory
		var grantedInst *coreitem.Instance

		if needsInventory {
			inv, err = s.findInventory(txCtx, req.CharacterID)
			if err != nil {
				return err
			}

			// Validate and consume item instance if requested
			if req.ConsumeInstanceID != "" && req.ConsumeInstanceQty > 0 {
				inst, found := inv.Find(req.ConsumeInstanceID)
				if !found {
					return ErrItemNotFound
				}
				if inst.Quantity < req.ConsumeInstanceQty {
					return ErrInsufficientItemQuantity
				}
				if err := inv.Consume(req.ConsumeInstanceID, req.ConsumeInstanceQty); err != nil {
					return err
				}
			}

			// Validate and consume item definition if requested
			if req.ConsumeDefinitionID != "" && req.ConsumeDefQty > 0 {
				if inv.Quantity(req.ConsumeDefinitionID) < req.ConsumeDefQty {
					return ErrInsufficientItemQuantity
				}
				remaining := req.ConsumeDefQty
				for _, inst := range inv.Items {
					if inst.DefinitionID == req.ConsumeDefinitionID && inst.Quantity > 0 {
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

			// Grant item if requested
			if req.GrantDefinitionID != "" && req.GrantQuantity > 0 {
				newInst, err := coreitem.NewInstance(req.GrantDefinitionID, req.GrantQuantity)
				if err != nil {
					return err
				}
				if err := inv.Add(newInst); err != nil {
					return fmt.Errorf("%w: %v", ErrInventoryFull, err)
				}
				grantedInst = &newInst
			}

			if err := s.inventories.Save(txCtx, inv); err != nil {
				return err
			}
		}

		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}

		result = &ExchangeResult{
			Character:   char,
			Inventory:   inv,
			GrantedItem: grantedInst,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

package depot

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

// effectiveJobLevel returns the job-change count used for depot capacity.
func effectiveJobLevel(char corecharacter.Character) int {
	return char.JobLevel
}

func (s *Service) findOrCreateDepot(ctx context.Context, characterID string, char corecharacter.Character) (Depot, error) {
	dep, err := s.depotRepo.FindByCharacterIDForUpdate(ctx, characterID)
	if err != nil && errors.Is(err, ErrNotFound) {
		return NewDepotWithCapacity(characterID, effectiveJobLevel(char), 0, char.OverDepot)
	}
	if err != nil {
		return Depot{}, err
	}
	dep.RefreshCapacity(char.JobLevel, char.OverDepot)
	return dep, nil
}

func (s *Service) saveDepot(ctx context.Context, dep Depot) error {
	if err := s.depotRepo.Save(ctx, dep); err != nil {
		return fmt.Errorf("save depot: %w", err)
	}
	return nil
}

func validateItemOp(characterID string, itemInstanceID string) error {
	if strings.TrimSpace(characterID) == "" {
		return ErrInvalidCharacterID
	}
	if strings.TrimSpace(itemInstanceID) == "" {
		return ErrInvalidItemInstanceID
	}
	return nil
}

func mapEconomyError(err error) error {
	if errors.Is(err, economy.ErrInsufficientGold) {
		return ErrInsufficientFunds
	}
	if errors.Is(err, economy.ErrCharacterNotFound) {
		return corecharacter.ErrNotFound
	}
	return err
}

func (s *Service) itemKind(defID string) int {
	if s.itemDefs == nil {
		return 3
	}
	def, err := s.itemDefs.FindByID(defID)
	if err != nil {
		return 3
	}
	switch def.Slot {
	case item.SlotMainHand:
		return 1 // Weapon
	case item.SlotOffHand, item.SlotBody, item.SlotAccessory:
		return 2 // Armor / Shield / Accessory
	default:
		return 3 // Item / Consumable / Material
	}
}

// GetDepot returns the current depot state for the given character, computing dynamic capacity.
func (s *Service) GetDepot(ctx context.Context, characterID string) (Depot, error) {
	if strings.TrimSpace(characterID) == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	char, err := s.charRepo.FindByID(ctx, characterID)
	if err != nil {
		return Depot{}, err
	}
	depot, err := s.depotRepo.FindByCharacterID(ctx, characterID)
	if err != nil && errors.Is(err, ErrNotFound) {
		return NewDepotWithCapacity(characterID, effectiveJobLevel(char), 0, char.OverDepot)
	}
	if err != nil {
		return Depot{}, err
	}
	depot.RefreshCapacity(char.JobLevel, char.OverDepot)
	return depot, nil
}

// DepositItem deposits an item from character inventory into the depot.
func (s *Service) DepositItem(ctx context.Context, characterID string, itemInstanceID string) (Depot, error) {
	if err := validateItemOp(characterID, itemInstanceID); err != nil {
		return Depot{}, err
	}
	var resultDepot Depot
	req := economy.TransactionRequest{CharacterID: characterID, LockInventory: true}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		dep, err := s.findOrCreateDepot(tc.Context, characterID, tc.Character)
		if err != nil {
			return err
		}
		itemInstance, found := tc.Inventory.Find(itemInstanceID)
		if !found {
			return ErrItemNotFound
		}
		if err := dep.AddItem(itemInstance); err != nil {
			return err
		}
		if err := tc.Inventory.Consume(itemInstanceID, itemInstance.Quantity); err != nil {
			return err
		}
		if err := s.saveDepot(tc.Context, dep); err != nil {
			return err
		}
		resultDepot = dep
		return nil
	})
	if err != nil {
		return Depot{}, mapEconomyError(err)
	}
	return resultDepot, nil
}

// WithdrawItem withdraws an item from the depot into the character inventory.
// Upon successful withdrawal, automatically registers item in Collection book if configured.
func (s *Service) WithdrawItem(ctx context.Context, characterID string, itemInstanceID string) (Depot, error) {
	if err := validateItemOp(characterID, itemInstanceID); err != nil {
		return Depot{}, err
	}
	var resultDepot Depot
	req := economy.TransactionRequest{CharacterID: characterID, LockInventory: true}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		dep, err := s.depotRepo.FindByCharacterIDForUpdate(tc.Context, characterID)
		if err != nil {
			return err
		}
		dep.RefreshCapacity(tc.Character.JobLevel, tc.Character.OverDepot)
		itemInstance, err := dep.RemoveItem(itemInstanceID)
		if err != nil {
			return err
		}
		if err := tc.Inventory.Add(itemInstance); err != nil {
			return ErrInventoryFull
		}
		if err := s.saveDepot(tc.Context, dep); err != nil {
			return err
		}

		if s.collector != nil && s.itemDefs != nil {
			if def, defErr := s.itemDefs.FindByID(itemInstance.DefinitionID); defErr == nil {
				cat := string(def.Slot)
				if cat == "" {
					cat = "consumable"
				}
				_ = s.collector.RecordItemDiscovered(tc.Context, characterID, def.ID, def.Name, cat)
			}
		}

		resultDepot = dep
		return nil
	})
	if err != nil {
		return Depot{}, mapEconomyError(err)
	}
	return resultDepot, nil
}

// SellItem sells a single item from the depot for 50% of its base catalog price.
func (s *Service) SellItem(ctx context.Context, characterID string, itemInstanceID string) (Depot, int, error) {
	if err := validateItemOp(characterID, itemInstanceID); err != nil {
		return Depot{}, 0, err
	}
	var resultDepot Depot
	var goldEarned int
	req := economy.TransactionRequest{CharacterID: characterID}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		dep, err := s.findOrCreateDepot(tc.Context, characterID, tc.Character)
		if err != nil {
			return err
		}
		itemInstance, err := dep.RemoveItem(itemInstanceID)
		if err != nil {
			return err
		}
		unitPrice := 0
		if s.itemDefs != nil {
			if def, defErr := s.itemDefs.FindByID(itemInstance.DefinitionID); defErr == nil {
				unitPrice = def.Price
			}
		}
		sellPricePerUnit := int(float64(unitPrice) * 0.5)
		goldEarned = sellPricePerUnit * itemInstance.Quantity
		tc.AddGrant(economy.ResourceGrant{Gold: goldEarned})
		if err := s.saveDepot(tc.Context, dep); err != nil {
			return err
		}
		resultDepot = dep
		return nil
	})
	if err != nil {
		return Depot{}, 0, mapEconomyError(err)
	}
	return resultDepot, goldEarned, nil
}

// SellItems sells multiple items from the depot in an atomic batch.
func (s *Service) SellItems(ctx context.Context, characterID string, itemInstanceIDs []string) (Depot, int, error) {
	if strings.TrimSpace(characterID) == "" {
		return Depot{}, 0, ErrInvalidCharacterID
	}
	if len(itemInstanceIDs) == 0 {
		return Depot{}, 0, ErrEmptyItemList
	}
	var resultDepot Depot
	var totalGoldEarned int
	req := economy.TransactionRequest{CharacterID: characterID}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		dep, err := s.findOrCreateDepot(tc.Context, characterID, tc.Character)
		if err != nil {
			return err
		}
		// Verify all items exist first before mutating
		for _, targetID := range itemInstanceIDs {
			found := false
			for _, existing := range dep.Items {
				if existing.ID == targetID {
					found = true
					break
				}
			}
			if !found {
				return ErrItemNotFound
			}
		}

		for _, targetID := range itemInstanceIDs {
			itemInstance, _ := dep.RemoveItem(targetID)
			unitPrice := 0
			if s.itemDefs != nil {
				if def, defErr := s.itemDefs.FindByID(itemInstance.DefinitionID); defErr == nil {
					unitPrice = def.Price
				}
			}
			sellPricePerUnit := int(float64(unitPrice) * 0.5)
			totalGoldEarned += sellPricePerUnit * itemInstance.Quantity
		}

		tc.AddGrant(economy.ResourceGrant{Gold: totalGoldEarned})
		if err := s.saveDepot(tc.Context, dep); err != nil {
			return err
		}
		resultDepot = dep
		return nil
	})
	if err != nil {
		return Depot{}, 0, mapEconomyError(err)
	}
	return resultDepot, totalGoldEarned, nil
}

// SortItems sorts depot items by legacy kind order:
// Kind 1: Weapon (main-hand)
// Kind 2: Armor / Shield / Accessory (off-hand, body, accessory)
// Kind 3: Items / Consumables / Materials (none)
// Tie-breaker: DefinitionID ascending.
func (s *Service) SortItems(ctx context.Context, characterID string) (Depot, error) {
	if strings.TrimSpace(characterID) == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	var resultDepot Depot
	req := economy.TransactionRequest{CharacterID: characterID}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		dep, err := s.findOrCreateDepot(tc.Context, characterID, tc.Character)
		if err != nil {
			return err
		}
		sort.SliceStable(dep.Items, func(i, j int) bool {
			kindI := s.itemKind(dep.Items[i].DefinitionID)
			kindJ := s.itemKind(dep.Items[j].DefinitionID)
			if kindI != kindJ {
				return kindI < kindJ
			}
			return dep.Items[i].DefinitionID < dep.Items[j].DefinitionID
		})
		if err := s.saveDepot(tc.Context, dep); err != nil {
			return err
		}
		resultDepot = dep
		return nil
	})
	if err != nil {
		return Depot{}, mapEconomyError(err)
	}
	return resultDepot, nil
}

// Expand expands depot capacity by 5 slots, consuming tiered gold cost up to 20 times.
func (s *Service) Expand(ctx context.Context, characterID string) (Depot, error) {
	if strings.TrimSpace(characterID) == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	var resultDepot Depot
	req := economy.TransactionRequest{CharacterID: characterID}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		dep, err := s.findOrCreateDepot(tc.Context, characterID, tc.Character)
		if err != nil {
			return err
		}
		if dep.ExDepot >= MaxExDepot {
			return ErrDepotMaxExpanded
		}
		cost, err := ExpansionCost(dep.ExDepot)
		if err != nil {
			return err
		}
		if tc.Character.Money < cost {
			return ErrInsufficientFunds
		}
		if err := tc.Character.DeductMoney(cost); err != nil {
			return ErrInsufficientFunds
		}
		if err := s.charRepo.Update(tc.Context, tc.Character); err != nil {
			return err
		}
		dep.ExDepot++
		dep.RefreshCapacity(tc.Character.JobLevel, tc.Character.OverDepot)
		if err := s.saveDepot(tc.Context, dep); err != nil {
			return err
		}
		resultDepot = dep
		return nil
	})
	if err != nil {
		return Depot{}, mapEconomyError(err)
	}
	return resultDepot, nil
}

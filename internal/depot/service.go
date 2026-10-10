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
	dep, err := FindOrCreate(ctx, s.depotRepo, char)
	if err != nil {
		return Depot{}, err
	}
	dep.ItemDefs = s.itemDefs
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

type legacySortItem struct {
	instance item.Instance
	kind, no int
}

// withLegacySortKeys resolves each instance's stored kind/number through the
// item catalog. Missing definitions or non-canonical identities are errors
// rather than guessed classifications.
func (s *Service) withLegacySortKeys(items []item.Instance) ([]legacySortItem, error) {
	keyed := make([]legacySortItem, len(items))
	for i, inst := range items {
		if s.itemDefs == nil {
			return nil, fmt.Errorf("depot sort requires item definitions")
		}
		def, err := s.itemDefs.FindByID(inst.DefinitionID)
		if err != nil {
			return nil, fmt.Errorf("sort depot item %s: %w", inst.DefinitionID, err)
		}
		kind, no, err := def.LegacyStorageKey()
		if err != nil {
			return nil, fmt.Errorf("sort depot item %s: %w", inst.DefinitionID, err)
		}
		keyed[i] = legacySortItem{instance: inst, kind: kind, no: no}
	}
	return keyed, nil
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
		newDep, err := NewDepotWithCapacity(characterID, effectiveJobLevel(char), 0, char.OverDepot)
		if err != nil {
			return Depot{}, err
		}
		newDep.ItemDefs = s.itemDefs
		return newDep, nil
	}
	if err != nil {
		return Depot{}, err
	}
	depot.RefreshCapacity(char.JobLevel, char.OverDepot)
	depot.ItemDefs = s.itemDefs
	return depot, nil
}

// DepositItem deposits an item from character inventory into the depot.
// Transaction: ExecuteTransaction.
// Lock Order: characters(2) -> inventory_items/equipment_slots(3) -> character_depots/depot_items(5).
func (s *Service) DepositItem(ctx context.Context, characterID string, itemInstanceID string) (Depot, error) {
	if err := validateItemOp(characterID, itemInstanceID); err != nil {
		return Depot{}, err
	}
	var resultDepot Depot
	req := economy.TransactionRequest{CharacterID: characterID, LockInventory: true}
	_, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		itemInstance, found := tc.Inventory.Find(itemInstanceID)
		if !found {
			return ErrItemNotFound
		}
		if _, err := s.detachEquipment(tc.Context, &tc.Character, itemInstanceID); err != nil {
			return err
		}
		dep, err := s.findOrCreateDepot(tc.Context, characterID, tc.Character)
		if err != nil {
			return err
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
		if tc.Inventory.IsFull() {
			return ErrInventoryFull
		}
		itemInstance, err := dep.PurgeSlot(itemInstanceID)
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
				//lint:ignore error-swallow best-effort collection discovery
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
// Transaction: delegates to SellItems (ExecuteTransaction).
// Lock Order: characters(2) -> character_depots/depot_items(5).
func (s *Service) SellItem(ctx context.Context, characterID string, itemInstanceID string) (Depot, int, error) {
	if err := validateItemOp(characterID, itemInstanceID); err != nil {
		return Depot{}, 0, err
	}
	return s.SellItems(ctx, characterID, []string{itemInstanceID})
}

// SellItems sells multiple items from the depot in an atomic batch.
// Transaction: ExecuteTransaction.
// Lock Order: characters(2) -> character_depots/depot_items(5).
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
		// Validate every target and its valuation before deleting any assets.
		seen := make(map[string]bool, len(itemInstanceIDs))
		for _, targetID := range itemInstanceIDs {
			if seen[targetID] {
				return ErrItemNotFound
			}
			seen[targetID] = true
			found := false
			for _, existing := range dep.Items {
				if existing.ID == targetID {
					if s.itemDefs == nil {
						return fmt.Errorf("depot sale requires item definitions")
					}
					def, err := s.itemDefs.FindByID(existing.DefinitionID)
					if err != nil {
						return fmt.Errorf("value depot item %s: %w", existing.DefinitionID, err)
					}
					if def.Price < 0 {
						return ErrInvalidAmount
					}
					if existing.Quantity <= 0 {
						return ErrInvalidQuantity
					}
					price, err := economy.SafeMultiply(def.Price/2, existing.Quantity)
					if err != nil {
						return err
					}
					totalGoldEarned, err = economy.SafeAdd(totalGoldEarned, price)
					if err != nil {
						return err
					}
					found = true
					break
				}
			}
			if !found {
				return ErrItemNotFound
			}
		}

		for _, targetID := range itemInstanceIDs {
			if _, err := dep.PurgeSlot(targetID); err != nil {
				return err
			}
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

// SortItems orders depot items by the original stored kind (1 weapon, 2 armor,
// 3 item-catalog entry including shields and accessories) and then by numeric
// item number (depot.cgi seiton). Equal keys keep their existing order.
// A definition without a resolvable key fails the sort and leaves the depot unchanged.
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
		keyed, err := s.withLegacySortKeys(dep.Items)
		if err != nil {
			return err
		}
		sort.SliceStable(keyed, func(i, j int) bool {
			if keyed[i].kind != keyed[j].kind {
				return keyed[i].kind < keyed[j].kind
			}
			return keyed[i].no < keyed[j].no
		})
		for i := range keyed {
			dep.Items[i] = keyed[i].instance
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

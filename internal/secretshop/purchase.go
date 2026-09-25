package secretshop

import (
	"context"
	"errors"
	"fmt"
	"strings"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

// PurchaseResult contains outcome of a secret shop purchase.
type PurchaseResult struct {
	CharacterID         string `json:"character_id"`
	Item                Item   `json:"item"`
	Quantity            int    `json:"quantity"`
	TotalPrice          int    `json:"total_price"`
	RemainingGold       int    `json:"remaining_gold"`
	InventoryInstanceID string `json:"inventory_instance_id"`
	TransferredToDepot  bool   `json:"transferred_to_depot"`
	NPCMessage          string `json:"npc_message"`
}

// PurchaseItem purchases rare items from the secret shop with transactional protection.
// If inventory consumable slot is empty and quantity == 1, it is placed in character inventory.
// If inventory is occupied or quantity > 1, it is automatically transferred to depot.
func (s *Service) PurchaseItem(
	ctx context.Context,
	characterID string,
	itemID string,
	quantity int,
) (*PurchaseResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	if quantity <= 0 || quantity > MaxPurchaseQuantity {
		return nil, ErrInvalidQuantity
	}

	var result *PurchaseResult

	operation := func(txCtx context.Context) error {
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		if !CheckEligibility(char) {
			return ErrAccessDenied
		}

		shopItem, ok := s.catalog.FindByID(itemID)
		if !ok {
			return ErrItemNotFound
		}

		// Check helper quest exclusion filter if configured
		if s.helperFilter != nil {
			activeHelperItemIDs, err := s.helperFilter.GetActiveHelperItemIDs(txCtx)
			if err != nil {
				return err
			}
			for _, helperDefID := range activeHelperItemIDs {
				if helperDefID == shopItem.ItemDefinitionID {
					return ErrItemUnavailableInHelperQuest
				}
			}
		}

		totalPrice, err := safeMultiply(shopItem.Price, quantity)
		if err != nil {
			return err
		}

		if char.Money < totalPrice {
			return ErrInsufficientFunds
		}

		inv, err := s.inventoryRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		occupied := false
		for _, inst := range inv.Items {
			if s.itemDefProvider != nil {
				def, err := s.itemDefProvider.FindByID(inst.DefinitionID)
				if err == nil && (def.Slot == coreitem.SlotNone || def.Slot == "") {
					occupied = true
					break
				}
			} else {
				if _, ok := s.catalog.FindByDefinitionID(inst.DefinitionID); ok || strings.HasPrefix(inst.DefinitionID, "item-") {
					occupied = true
					break
				}
			}
		}

		if !occupied && quantity == 1 {
			res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
				CharacterID:       characterID,
				DeductGold:        totalPrice,
				GrantDefinitionID: shopItem.ItemDefinitionID,
				GrantQuantity:     1,
			})
			if err != nil {
				if errors.Is(err, economy.ErrInsufficientGold) {
					return ErrInsufficientFunds
				}
				if errors.Is(err, economy.ErrCharacterNotFound) {
					return ErrCharacterNotFound
				}
				return err
			}

			if s.collectionRecorder != nil {
				category := shopItem.Category
				if s.itemDefProvider != nil {
					if def, err := s.itemDefProvider.FindByID(shopItem.ItemDefinitionID); err == nil {
						category = def.Category()
					}
				}
				//lint:ignore error-swallow best-effort collection discovery
				_ = s.collectionRecorder.RecordItemDiscovered(txCtx, characterID, shopItem.ItemDefinitionID, shopItem.Name, category)
			}

			result = &PurchaseResult{
				CharacterID:         char.ID,
				Item:                shopItem,
				Quantity:            1,
				TotalPrice:          totalPrice,
				RemainingGold:       res.Character.Money,
				InventoryInstanceID: res.GrantedItem.ID,
				TransferredToDepot:  false,
				NPCMessage:          fmt.Sprintf("%sメェ〜。持ってけメェ〜", shopItem.Name),
			}
			return nil
		}

		// Consumable slot is occupied or quantity > 1: route to depot
		if s.depotRepo == nil {
			return ErrDepotNotConfigured
		}

		dep, err := depot.FindOrCreate(txCtx, s.depotRepo, char)
		if err != nil {
			return err
		}

		inst, err := coreitem.NewInstance(shopItem.ItemDefinitionID, quantity)
		if err != nil {
			return err
		}
		if err := dep.AddItem(inst); err != nil {
			if errors.Is(err, depot.ErrDepotFull) {
				return ErrDepotFull
			}
			return err
		}

		res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
			CharacterID: characterID,
			DeductGold:  totalPrice,
		})
		if err != nil {
			if errors.Is(err, economy.ErrInsufficientGold) {
				return ErrInsufficientFunds
			}
			if errors.Is(err, economy.ErrCharacterNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		if err := s.depotRepo.Save(txCtx, dep); err != nil {
			return err
		}

		result = &PurchaseResult{
			CharacterID:         char.ID,
			Item:                shopItem,
			Quantity:            quantity,
			TotalPrice:          totalPrice,
			RemainingGold:       res.Character.Money,
			InventoryInstanceID: inst.ID,
			TransferredToDepot:  true,
			NPCMessage:          fmt.Sprintf("%sは%sメェ〜の預かり所の方に投げましたメェ〜", shopItem.Name, char.Name),
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, operation); err != nil {
			return nil, err
		}
	} else {
		if err := operation(ctx); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func safeMultiply(price, qty int) (int, error) {
	if price < 0 || qty < 0 {
		return 0, ErrInvalidQuantity
	}
	return economy.SafeMultiply(price, qty)
}

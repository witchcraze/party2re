package eventplaza

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

// BazaarPurchaseResult represents the result of buying goods from the traveling merchant.
type BazaarPurchaseResult struct {
	CharacterID         string     `json:"character_id"`
	Item                BazaarItem `json:"item"`
	Quantity            int        `json:"quantity"`
	TotalPrice          int        `json:"total_price"`
	RemainingGold       int        `json:"remaining_gold"`
	InventoryInstanceID string     `json:"inventory_instance_id"`
	TransferredToDepot  bool       `json:"transferred_to_depot"`
	NPCMessage          string     `json:"npc_message"`
}

func (s *Service) PurchaseBazaarItem(
	ctx context.Context,
	characterID string,
	itemID string,
	quantity int,
) (BazaarPurchaseResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return BazaarPurchaseResult{}, ErrCharacterNotFound
	}
	if quantity <= 0 || quantity > MaxPurchaseQuantity {
		return BazaarPurchaseResult{}, ErrInvalidQuantity
	}

	cutoff := s.clock.Now().Add(-PresenceWindow)
	participants, err := s.countParticipants(ctx, cutoff)
	if err != nil {
		return BazaarPurchaseResult{}, fmt.Errorf("failed to count active participants: %w", err)
	}

	currentTier, _, _ := CalculateMerchantTier(participants)
	if currentTier <= 0 {
		return BazaarPurchaseResult{}, ErrMerchantNotPresent
	}

	var targetItem *BazaarItem
	for i := range s.bazaarCatalog {
		if s.bazaarCatalog[i].ID == itemID || s.bazaarCatalog[i].ItemDefinitionID == itemID {
			targetItem = &s.bazaarCatalog[i]
			break
		}
	}
	if targetItem == nil {
		return BazaarPurchaseResult{}, ErrItemNotFound
	}

	if targetItem.TierRequired != currentTier {
		return BazaarPurchaseResult{}, ErrItemTierLocked
	}

	if s.helper != nil {
		activeHelperIDs, err := s.helper.GetActiveHelperItemIDs(ctx, s.clock.Now())
		if err != nil {
			return BazaarPurchaseResult{}, fmt.Errorf("failed to get active helper items: %w", err)
		}
		for _, helperID := range activeHelperIDs {
			if helperID == targetItem.ItemDefinitionID {
				return BazaarPurchaseResult{}, ErrItemUnavailable
			}
		}
	}

	if targetItem.Price > math.MaxInt/quantity {
		return BazaarPurchaseResult{}, ErrPriceOverflow
	}
	totalCost := targetItem.Price * quantity

	var result BazaarPurchaseResult

	operation := func(txCtx context.Context) error {
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		if char.Money < totalCost {
			return ErrInsufficientGold
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
				if strings.HasPrefix(inst.DefinitionID, "item-") {
					occupied = true
					break
				}
			}
		}

		if !occupied && quantity == 1 {
			res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
				CharacterID:       characterID,
				DeductGold:        totalCost,
				GrantDefinitionID: targetItem.ItemDefinitionID,
				GrantQuantity:     1,
			})
			if err != nil {
				if errors.Is(err, economy.ErrInsufficientGold) {
					return ErrInsufficientGold
				}
				if errors.Is(err, economy.ErrCharacterNotFound) {
					return ErrCharacterNotFound
				}
				return err
			}

			if s.recorder != nil {
				_ = s.recorder.RecordItemDiscovered(txCtx, characterID, targetItem.ItemDefinitionID, targetItem.Name, targetItem.Category)
			}

			_ = s.recordPresence(txCtx, characterID, s.clock.Now())

			result = BazaarPurchaseResult{
				CharacterID:         characterID,
				Item:                *targetItem,
				Quantity:            1,
				TotalPrice:          totalCost,
				RemainingGold:       res.Character.Money,
				InventoryInstanceID: res.GrantedItem.ID,
				TransferredToDepot:  false,
				NPCMessage:          fmt.Sprintf("はい、%sです", targetItem.Name),
			}
			return nil
		}

		if s.depotRepo == nil {
			return ErrDepotNotConfigured
		}

		dep, err := s.depotRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			if !errors.Is(err, depot.ErrNotFound) {
				return err
			}
			dep, err = depot.NewDepotWithCapacity(characterID, char.JobLevel, 0, char.OverDepot)
			if err != nil {
				return err
			}
		}
		dep.RefreshCapacity(char.JobLevel, char.OverDepot)

		inst, err := coreitem.NewInstance(targetItem.ItemDefinitionID, quantity)
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
			DeductGold:  totalCost,
		})
		if err != nil {
			if errors.Is(err, economy.ErrInsufficientGold) {
				return ErrInsufficientGold
			}
			if errors.Is(err, economy.ErrCharacterNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		if err := s.depotRepo.Save(txCtx, dep); err != nil {
			return err
		}

		if s.recorder != nil {
			_ = s.recorder.RecordItemDiscovered(txCtx, characterID, targetItem.ItemDefinitionID, targetItem.Name, targetItem.Category)
		}

		_ = s.recordPresence(txCtx, characterID, s.clock.Now())

		result = BazaarPurchaseResult{
			CharacterID:         characterID,
			Item:                *targetItem,
			Quantity:            quantity,
			TotalPrice:          totalCost,
			RemainingGold:       res.Character.Money,
			InventoryInstanceID: inst.ID,
			TransferredToDepot:  true,
			NPCMessage:          fmt.Sprintf("%sは%sさんの預かり所に送っておきましたよ", targetItem.Name, char.Name),
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, operation); err != nil {
			return BazaarPurchaseResult{}, err
		}
	} else {
		if err := operation(ctx); err != nil {
			return BazaarPurchaseResult{}, err
		}
	}

	return result, nil
}

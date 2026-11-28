package secretshop

import (
	"context"
	"errors"
	"slices"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

// PurchaseResult contains outcome of a secret shop purchase.
type PurchaseResult struct {
	CharacterID         string `json:"character_id"`
	Item                Item   `json:"item"`
	TotalPrice          int    `json:"total_price"`
	RemainingGold       int    `json:"remaining_gold"`
	InventoryInstanceID string `json:"inventory_instance_id"`
	TransferredToDepot  bool   `json:"transferred_to_depot"`
}

// PurchaseItem buys one unit, as legacy ＠かう does (secret.cgi kau): into the
// empty tool hand, otherwise to the Depot.
func (s *Service) PurchaseItem(ctx context.Context, characterID string, itemID string) (*PurchaseResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}

	var result *PurchaseResult

	operation := func(txCtx context.Context) error {
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
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
			if slices.Contains(activeHelperItemIDs, shopItem.ItemDefinitionID) {
				return ErrItemUnavailableInHelperQuest
			}
		}

		totalPrice := shopItem.Price
		if char.Money < totalPrice {
			return ErrInsufficientFunds
		}

		inv, err := s.inventoryRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		occupied := inv.IsFull(shopItem.ItemDefinitionID)

		if !occupied {
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
				if err := s.collectionRecorder.RecordItemDiscovered(txCtx, characterID, shopItem.ItemDefinitionID, shopItem.Name, category); err != nil {
					return err
				}
			}

			result = &PurchaseResult{
				CharacterID:         char.ID,
				Item:                shopItem,
				TotalPrice:          totalPrice,
				RemainingGold:       res.Character.Money,
				InventoryInstanceID: res.GrantedItem.ID,
				TransferredToDepot:  false,
			}
			return nil
		}

		// Tool hand occupied: route to depot
		if s.depotRepo == nil {
			return ErrDepotNotConfigured
		}

		dep, err := depot.FindOrCreate(txCtx, s.depotRepo, char)
		if err != nil {
			return err
		}

		inst, err := coreitem.NewInstance(shopItem.ItemDefinitionID, 1)
		if err != nil {
			return err
		}
		dep.Deliver(inst)

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
			TotalPrice:          totalPrice,
			RemainingGold:       res.Character.Money,
			InventoryInstanceID: inst.ID,
			TransferredToDepot:  true,
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

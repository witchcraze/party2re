package store

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

// BuyItem purchases a gold-listed item from another player's store.
// Transaction: RunInTx
// Lock Order: store_sales(0) -> characters(2, ascending) -> depots(5, ascending)
func (s *Service) BuyItem(ctx context.Context, buyerCharacterID, saleID string) error {
	return s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		// Rank 0: shared sale lock
		sale, err := s.repo.GetSaleByIDForUpdate(txCtx, saleID)
		if err != nil {
			return err
		}
		if sale.SaleType != SaleTypeGold {
			return ErrListingNotFound
		}
		if sale.CharacterID == buyerCharacterID {
			return ErrCannotBuyOwnItem
		}
		// Legacy visitors can only enter a standing store.
		if err := s.checkSaleStoreActive(txCtx, sale); err != nil {
			return err
		}

		sellerID := sale.CharacterID

		// Rank 2: characters locked ascending
		c1, c2 := buyerCharacterID, sellerID
		if c1 > c2 {
			c1, c2 = c2, c1
		}
		char1, err := s.charRepo.FindByIDForUpdate(txCtx, c1)
		if err != nil {
			return err
		}
		char2, err := s.charRepo.FindByIDForUpdate(txCtx, c2)
		if err != nil {
			return err
		}

		if err := s.checkMutation(txCtx, buyerCharacterID); err != nil {
			return err
		}

		var buyerChar, sellerChar *corecharacter.Character
		if char1.ID == buyerCharacterID {
			buyerChar, sellerChar = &char1, &char2
		} else {
			buyerChar, sellerChar = &char2, &char1
		}

		if buyerChar.Money < sale.Price {
			return ErrInsufficientFunds
		}

		// Rank 5: depots locked ascending
		dep1, err := depot.FindOrCreate(txCtx, s.depotRepo, char1)
		if err != nil {
			return err
		}
		dep2, err := depot.FindOrCreate(txCtx, s.depotRepo, char2)
		if err != nil {
			return err
		}

		var buyerDepot *depot.Depot
		if dep1.CharacterID == buyerCharacterID {
			buyerDepot = &dep1
		} else {
			buyerDepot = &dep2
		}

		if err := buyerChar.DeductMoney(sale.Price); err != nil {
			return ErrInsufficientFunds
		}
		if err := s.charRepo.Update(txCtx, *buyerChar); err != nil {
			return err
		}

		buyerDepot.Deliver(coreitem.Instance{
			ID:               s.idGen(),
			DefinitionID:     sale.ItemDefinitionID,
			Quantity:         sale.Quantity,
			EnhancementLevel: sale.EnhancementLevel,
		})
		if err := s.depotRepo.Save(txCtx, *buyerDepot); err != nil {
			return err
		}

		if err := s.repo.DeleteSale(txCtx, saleID); err != nil {
			return err
		}

		if s.delivery == nil {
			return errors.New("store delivery service is required")
		}
		return s.delivery.HoldPayment(txCtx, sellerChar.ID, buyerChar.Name, sale.Price)
	})
}

// TradeItem exchanges an item from the customer's depot with a barter-listed item.
// Transaction: RunInTx
// Lock Order: store_sales(0) -> characters(2, ascending) -> depots(5, ascending)
func (s *Service) TradeItem(ctx context.Context, buyerCharacterID, saleID, customerDepotItemInstanceID string) error {
	return s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		// Rank 0: shared sale lock
		sale, err := s.repo.GetSaleByIDForUpdate(txCtx, saleID)
		if err != nil {
			return err
		}
		if sale.SaleType != SaleTypeBarter {
			return ErrListingNotFound
		}
		if sale.CharacterID == buyerCharacterID {
			return ErrCannotTradeOwnItem
		}
		// Legacy visitors can only enter a standing store.
		if err := s.checkSaleStoreActive(txCtx, sale); err != nil {
			return err
		}

		sellerID := sale.CharacterID

		// Rank 2: characters locked ascending
		c1, c2 := buyerCharacterID, sellerID
		if c1 > c2 {
			c1, c2 = c2, c1
		}
		char1, err := s.charRepo.FindByIDForUpdate(txCtx, c1)
		if err != nil {
			return err
		}
		char2, err := s.charRepo.FindByIDForUpdate(txCtx, c2)
		if err != nil {
			return err
		}

		if err := s.checkMutation(txCtx, buyerCharacterID); err != nil {
			return err
		}

		// Rank 5: depots locked ascending
		dep1, err := depot.FindOrCreate(txCtx, s.depotRepo, char1)
		if err != nil {
			return err
		}
		dep2, err := depot.FindOrCreate(txCtx, s.depotRepo, char2)
		if err != nil {
			return err
		}

		var buyerDepot, sellerDepot *depot.Depot
		if dep1.CharacterID == buyerCharacterID {
			buyerDepot, sellerDepot = &dep1, &dep2
		} else {
			buyerDepot, sellerDepot = &dep2, &dep1
		}

		// Find desired item in buyer depot
		bIdx := -1
		for i, it := range buyerDepot.Items {
			if it.ID == customerDepotItemInstanceID {
				bIdx = i
				break
			}
		}
		if bIdx == -1 {
			return ErrTradeItemMissing
		}

		tradedInst := buyerDepot.Items[bIdx]
		matchedName := tradedInst.DefinitionID
		if s.itemCatalog != nil {
			if def, err := s.itemCatalog.FindByID(tradedInst.DefinitionID); err == nil {
				matchedName = def.Name
			}
		}
		if matchedName != sale.WishItemName {
			return ErrTradeItemMissing
		}

		// Remove 1 from buyer depot
		tradedItem, err := buyerDepot.ConsumeOne(customerDepotItemInstanceID)
		if err != nil {
			return ErrTradeItemMissing
		}

		// Add traded item to seller depot
		sellerDepot.Deliver(coreitem.Instance{
			ID:               s.idGen(),
			DefinitionID:     tradedItem.DefinitionID,
			Quantity:         1,
			EnhancementLevel: tradedItem.EnhancementLevel,
		})

		// Add sale item to buyer depot
		buyerDepot.Deliver(coreitem.Instance{
			ID:               s.idGen(),
			DefinitionID:     sale.ItemDefinitionID,
			Quantity:         sale.Quantity,
			EnhancementLevel: sale.EnhancementLevel,
		})

		if err := s.depotRepo.Save(txCtx, *buyerDepot); err != nil {
			return err
		}
		if err := s.depotRepo.Save(txCtx, *sellerDepot); err != nil {
			return err
		}

		if err := s.repo.DeleteSale(txCtx, saleID); err != nil {
			return err
		}

		if s.delivery == nil {
			return errors.New("store delivery service is required")
		}
		buyer, seller := char1, char2
		if buyer.ID != buyerCharacterID {
			buyer, seller = seller, buyer
		}
		if err := s.delivery.NotifyItem(txCtx, buyer.ID, seller.Name, sale.ItemDefinitionID); err != nil {
			return err
		}
		return s.delivery.NotifyItem(txCtx, seller.ID, buyer.Name, tradedItem.DefinitionID)
	})
}

func (s *Service) checkSaleStoreActive(ctx context.Context, sale Sale) error {
	st, err := s.repo.GetStoreByID(ctx, sale.StoreID)
	if err != nil {
		return err
	}
	if !st.IsActive(s.nowFunc().UTC()) {
		return ErrStoreExpired
	}
	return nil
}

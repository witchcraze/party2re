package store

import (
	"context"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

// ListGoldItem lists an item from the character's depot for gold sale.
func (s *Service) ListGoldItem(ctx context.Context, characterID, depotItemInstanceID string, price int) (*Sale, error) {
	if price < 1 || price > 999999 {
		return nil, ErrInvalidPrice
	}

	now := s.nowFunc().UTC()
	var createdSale *Sale

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}

		dep, err := depot.FindOrCreate(txCtx, s.depotRepo, char)
		if err != nil {
			return err
		}

		st, err := s.repo.GetStoreByCharacterID(txCtx, characterID)
		if err != nil {
			return err
		}
		if !st.IsActive(now) {
			return ErrStoreExpired
		}

		sales, err := s.repo.GetSalesByStoreID(txCtx, st.ID)
		if err != nil {
			return err
		}
		if len(sales) >= MaxListings(char.OverStore) {
			return ErrMaxListingsReached
		}

		// Withdraw 1 from depot
		itemInst, err := dep.ConsumeOne(depotItemInstanceID)
		if err != nil {
			return ErrItemNotFound
		}

		itemName := itemInst.DefinitionID
		if s.itemCatalog != nil {
			if def, err := s.itemCatalog.FindByID(itemInst.DefinitionID); err == nil {
				itemName = def.Name
			}
		}

		if err := s.depotRepo.Save(txCtx, dep); err != nil {
			return err
		}

		// Find next available slot number
		usedSlots := make(map[int]bool)
		for _, sale := range sales {
			usedSlots[sale.SlotNumber] = true
		}
		slotNumber := 1
		for usedSlots[slotNumber] {
			slotNumber++
		}

		saleObj := Sale{
			ID:               s.idGen(),
			StoreID:          st.ID,
			CharacterID:      characterID,
			SlotNumber:       slotNumber,
			ItemDefinitionID: itemInst.DefinitionID,
			ItemName:         itemName,
			Quantity:         1,
			EnhancementLevel: itemInst.EnhancementLevel,
			SaleType:         SaleTypeGold,
			Price:            price,
			WishItemName:     "",
			CreatedAt:        now,
		}
		if err := s.repo.SaveSale(txCtx, saleObj); err != nil {
			return err
		}

		createdSale = &saleObj
		return nil
	})
	if err != nil {
		return nil, err
	}
	return createdSale, nil
}

// ListBarterItem lists an item from the character's depot for barter exchange with a desired item.
func (s *Service) ListBarterItem(ctx context.Context, characterID, depotItemInstanceID, wishItemName string) (*Sale, error) {
	cleanWish := strings.TrimSpace(wishItemName)
	if cleanWish == "" {
		return nil, ErrItemNotFound
	}
	var resolvedWishName string
	if s.itemCatalog != nil {
		def, err := s.itemCatalog.FindByName(cleanWish)
		if err != nil {
			return nil, ErrItemNotFound
		}
		resolvedWishName = def.Name
	} else {
		resolvedWishName = cleanWish
	}

	now := s.nowFunc().UTC()
	var createdSale *Sale

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}

		dep, err := depot.FindOrCreate(txCtx, s.depotRepo, char)
		if err != nil {
			return err
		}

		st, err := s.repo.GetStoreByCharacterID(txCtx, characterID)
		if err != nil {
			return err
		}
		if !st.IsActive(now) {
			return ErrStoreExpired
		}

		sales, err := s.repo.GetSalesByStoreID(txCtx, st.ID)
		if err != nil {
			return err
		}
		if len(sales) >= MaxListings(char.OverStore) {
			return ErrMaxListingsReached
		}

		// Withdraw 1 from depot
		itemInst, err := dep.ConsumeOne(depotItemInstanceID)
		if err != nil {
			return ErrItemNotFound
		}

		itemName := itemInst.DefinitionID
		if s.itemCatalog != nil {
			if def, err := s.itemCatalog.FindByID(itemInst.DefinitionID); err == nil {
				itemName = def.Name
			}
		}

		if err := s.depotRepo.Save(txCtx, dep); err != nil {
			return err
		}

		usedSlots := make(map[int]bool)
		for _, sale := range sales {
			usedSlots[sale.SlotNumber] = true
		}
		slotNumber := 1
		for usedSlots[slotNumber] {
			slotNumber++
		}

		saleObj := Sale{
			ID:               s.idGen(),
			StoreID:          st.ID,
			CharacterID:      characterID,
			SlotNumber:       slotNumber,
			ItemDefinitionID: itemInst.DefinitionID,
			ItemName:         itemName,
			Quantity:         1,
			EnhancementLevel: itemInst.EnhancementLevel,
			SaleType:         SaleTypeBarter,
			Price:            0,
			WishItemName:     resolvedWishName,
			CreatedAt:        now,
		}
		if err := s.repo.SaveSale(txCtx, saleObj); err != nil {
			return err
		}

		createdSale = &saleObj
		return nil
	})
	if err != nil {
		return nil, err
	}
	return createdSale, nil
}

// WithdrawListing withdraws a listed item back to the owner's depot.
func (s *Service) WithdrawListing(ctx context.Context, characterID, saleID string) error {
	return s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		// Rank 0: shared sale lock
		sale, err := s.repo.GetSaleByIDForUpdate(txCtx, saleID)
		if err != nil {
			return err
		}
		if sale.CharacterID != characterID {
			return ErrListingNotFound
		}

		// Rank 2: character lock
		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}

		// Owner commands need an active store, as legacy owners act inside it.
		st, err := s.repo.GetStoreByCharacterID(txCtx, characterID)
		if err != nil {
			return err
		}
		if !st.IsActive(s.nowFunc().UTC()) {
			return ErrStoreExpired
		}

		// Rank 5: depot lock
		dep, err := depot.FindOrCreate(txCtx, s.depotRepo, char)
		if err != nil {
			return err
		}

		dep.Deliver(coreitem.Instance{
			ID:               s.idGen(),
			DefinitionID:     sale.ItemDefinitionID,
			Quantity:         sale.Quantity,
			EnhancementLevel: sale.EnhancementLevel,
		})
		if err := s.depotRepo.Save(txCtx, dep); err != nil {
			return err
		}

		return s.repo.DeleteSale(txCtx, saleID)
	})
}

// BuyItem purchases a gold-listed item from another player's store.
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
		if err := sellerChar.AddMoney(sale.Price); err != nil {
			return err
		}

		if err := s.charRepo.Update(txCtx, *buyerChar); err != nil {
			return err
		}
		if err := s.charRepo.Update(txCtx, *sellerChar); err != nil {
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

		return nil
	})
}

// TradeItem exchanges an item from the customer's depot with a barter-listed item.
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

		return nil
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

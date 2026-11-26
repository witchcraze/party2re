package fleamarket

import (
	"context"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/id"
)

// PurchaseListing allows a buyer character to purchase a listed item.
func (s *Service) PurchaseListing(
	ctx context.Context,
	buyerCharacterID string,
	listingID string,
	now time.Time,
) (PurchaseResult, error) {
	if strings.TrimSpace(buyerCharacterID) == "" || strings.TrimSpace(listingID) == "" {
		return PurchaseResult{}, ErrInvalidInput
	}

	var result PurchaseResult

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock listing first (CAS / P2P shared entity lock, Rank 0)
		listing, err := s.repo.GetListingByIDForUpdate(txCtx, listingID)
		if err != nil {
			return err
		}
		if listing.Status != StatusActive {
			return ErrListingNotActive
		}
		if listing.SellerCharacterID == buyerCharacterID {
			return ErrCannotBuyOwnListing
		}

		// 2. Deterministic lock acquisition order for characters (ascending ID order, Rank 2)
		firstID, secondID := id.Sort2(listing.SellerCharacterID, buyerCharacterID)

		firstChar, err := s.lockCharacter(txCtx, firstID)
		if err != nil {
			return err
		}
		secondChar, err := s.lockCharacter(txCtx, secondID)
		if err != nil {
			return err
		}

		if err := s.checkMutation(txCtx, buyerCharacterID); err != nil {
			return err
		}

		var buyerChar, sellerChar corecharacter.Character
		if firstChar.ID == buyerCharacterID {
			buyerChar = firstChar
			sellerChar = secondChar
		} else {
			buyerChar = secondChar
			sellerChar = firstChar
		}

		// 3. Verify buyer funds
		if buyerChar.Money < listing.Price {
			return ErrInsufficientGold
		}

		// 4. Lock buyer depot (Rank 5)
		buyerDepot, err := depot.FindOrCreate(txCtx, s.depotRepo, buyerChar)
		if err != nil {
			return err
		}

		// 5. Deduct Gold from Buyer (seller receives a pending remittance below)
		if err := buyerChar.DeductMoney(listing.Price); err != nil {
			return ErrInsufficientGold
		}

		// 6. Deliver Item to Buyer Depot (legacy &send_item)
		itemInst, err := coreitem.NewInstanceWithEnhancement(listing.ItemID, 1, listing.EnhancementLevel)
		if err != nil {
			return err
		}
		buyerDepot.Deliver(itemInst)

		// 7. Update listing status
		listing.Status = StatusSold
		listing.SoldAt = &now
		listing.BuyerCharacterID = &buyerCharacterID
		buyerName := buyerChar.Name
		listing.BuyerName = &buyerName

		// 8. Persist mutations and hold seller proceeds with the Home money notice
		if err := s.charRepo.Update(txCtx, buyerChar); err != nil {
			return err
		}
		if err := s.depotRepo.Save(txCtx, buyerDepot); err != nil {
			return err
		}
		if err := s.repo.UpdateListing(txCtx, listing); err != nil {
			return err
		}
		if s.payment == nil {
			return errors.New("fleamarket payment holder is required")
		}
		if err := s.payment.HoldPayment(txCtx, sellerChar.ID, buyerChar.Name, listing.Price); err != nil {
			return err
		}

		result = PurchaseResult{
			Listing:      listing,
			BuyerGold:    buyerChar.Money,
			ItemInstance: itemInst,
		}

		return nil
	})

	if err != nil {
		return PurchaseResult{}, err
	}
	return result, nil
}

// CancelListing allows the seller to cancel an active listing and retrieve their item.
func (s *Service) CancelListing(
	ctx context.Context,
	sellerCharacterID string,
	listingID string,
) (Listing, error) {
	if strings.TrimSpace(sellerCharacterID) == "" || strings.TrimSpace(listingID) == "" {
		return Listing{}, ErrInvalidInput
	}

	var cancelled Listing

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock listing (Rank 0)
		listing, err := s.repo.GetListingByIDForUpdate(txCtx, listingID)
		if err != nil {
			return err
		}
		if listing.SellerCharacterID != sellerCharacterID {
			return ErrUnauthorizedSeller
		}
		if listing.Status != StatusActive {
			return ErrListingNotActive
		}

		// 2. Lock seller character (Rank 2)
		sellerChar, err := s.lockCharacter(txCtx, sellerCharacterID)
		if err != nil {
			return err
		}

		if err := s.checkMutation(txCtx, sellerCharacterID); err != nil {
			return err
		}

		// 3. Lock seller depot (Rank 5)
		sellerDepot, err := s.depotRepo.FindByCharacterIDForUpdate(txCtx, sellerCharacterID)
		if err != nil {
			return err
		}
		sellerDepot.RefreshCapacity(sellerChar.JobLevel, sellerChar.OverDepot)

		// 4. Return item instance to seller depot (legacy &send_item)
		itemInst, err := coreitem.NewInstanceWithEnhancement(listing.ItemID, 1, listing.EnhancementLevel)
		if err != nil {
			return err
		}
		sellerDepot.Deliver(itemInst)
		if err := s.depotRepo.Save(txCtx, sellerDepot); err != nil {
			return err
		}

		// 5. Update listing status
		listing.Status = StatusCancelled
		if err := s.repo.UpdateListing(txCtx, listing); err != nil {
			return err
		}

		cancelled = listing
		return nil
	})

	if err != nil {
		return Listing{}, err
	}
	return cancelled, nil
}

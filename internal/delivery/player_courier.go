package delivery

import (
	"context"
	"strings"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
)

// SendParcel sends an item and/or gold package to another character via town courier.
func (s *Service) SendParcel(
	ctx context.Context,
	senderID string,
	req SendParcelRequest,
	now time.Time,
) (*Parcel, error) {
	if strings.TrimSpace(senderID) == "" || strings.TrimSpace(req.RecipientCharacterID) == "" {
		return nil, ErrInvalidInput
	}
	if senderID == req.RecipientCharacterID {
		return nil, ErrSelfParcelNotAllowed
	}
	if req.GoldAmount < 0 {
		return nil, ErrInvalidInput
	}
	if req.GoldAmount == 0 && req.ItemInstanceID == "" {
		return nil, ErrInvalidParcelPayload
	}

	var parcel *Parcel

	action := func(txCtx context.Context) error {
		// 1. Fetch sender with lock
		sender, err := s.charRepo.FindByIDForUpdate(txCtx, senderID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// 2. Fetch recipient
		recipient, err := s.charRepo.FindByID(txCtx, req.RecipientCharacterID)
		if err != nil {
			return ErrRecipientNotFound
		}

		// 3. Verify sender funds
		totalGoldNeeded := req.GoldAmount + DefaultCourierFee
		if sender.Money < totalGoldNeeded {
			return ErrInsufficientFunds
		}

		var itemID, itemName string
		var itemQty int

		// 4. If sending an item, verify and consume from sender inventory
		if req.ItemInstanceID != "" {
			if req.ItemQuantity <= 0 {
				req.ItemQuantity = 1
			}
			inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, senderID)
			if err != nil {
				return err
			}

			inst, found := inv.Find(req.ItemInstanceID)
			if !found || inst.Quantity < req.ItemQuantity {
				return ErrInsufficientItems
			}

			itemID = inst.DefinitionID
			itemName = inst.DefinitionID
			if s.itemDefs != nil {
				if def, defErr := s.itemDefs.FindByID(inst.DefinitionID); defErr == nil {
					itemName = def.Name
				}
			}
			itemQty = req.ItemQuantity

			if err := inv.Consume(inst.ID, req.ItemQuantity); err != nil {
				return err
			}
			if err := s.invRepo.Save(txCtx, inv); err != nil {
				return err
			}
		}

		// 5. Deduct funds from sender
		if err := sender.DeductMoney(totalGoldNeeded); err != nil {
			return ErrInsufficientFunds
		}
		if err := s.charRepo.Update(txCtx, sender); err != nil {
			return err
		}

		// 6. Create and save Parcel
		p := &Parcel{
			ID:                   id.New(),
			SenderCharacterID:    senderID,
			SenderCharacterName:  sender.Name,
			RecipientCharacterID: recipient.ID,
			ItemID:               itemID,
			ItemName:             itemName,
			ItemQuantity:         itemQty,
			GoldAmount:           req.GoldAmount,
			Message:              strings.TrimSpace(req.Message),
			CourierFee:           DefaultCourierFee,
			Status:               ParcelStatusPending,
			CreatedAt:            now,
		}

		if err := s.repo.SaveParcel(txCtx, p); err != nil {
			return err
		}

		parcel = p
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, action); err != nil {
			return nil, err
		}
	} else {
		if err := action(ctx); err != nil {
			return nil, err
		}
	}

	return parcel, nil
}

// ClaimParcel deposits the parcel's gold and items into the recipient character's possession.
func (s *Service) ClaimParcel(
	ctx context.Context,
	recipientID string,
	parcelID string,
	now time.Time,
) (*ParcelClaimResult, error) {
	if strings.TrimSpace(recipientID) == "" || strings.TrimSpace(parcelID) == "" {
		return nil, ErrInvalidInput
	}

	var result *ParcelClaimResult

	action := func(txCtx context.Context) error {
		// 1. Fetch parcel with pessimistic lock (Tier 8) and check status & ownership FIRST
		parcel, err := s.repo.GetParcelByIDForUpdate(txCtx, parcelID)
		if err != nil {
			return err
		}
		if parcel.RecipientCharacterID != recipientID {
			return ErrForbidden
		}
		if parcel.Status != ParcelStatusPending {
			return ErrParcelAlreadyClaimed
		}

		// 2. Fetch recipient with lock (Tier 2)
		recipient, err := s.charRepo.FindByIDForUpdate(txCtx, recipientID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// 3. Fetch recipient inventory with lock (Tier 3)
		inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, recipientID)
		if err != nil {
			return err
		}

		// 4. Credit Gold
		if parcel.GoldAmount > 0 {
			_ = recipient.AddMoney(parcel.GoldAmount)
			if err := s.charRepo.Update(txCtx, recipient); err != nil {
				return err
			}
		}

		// 5. Add item if present
		if parcel.ItemID != "" && parcel.ItemQuantity > 0 {
			inst, err := coreitem.NewInstance(parcel.ItemID, parcel.ItemQuantity)
			if err != nil {
				return err
			}
			if err := inv.Add(inst); err != nil {
				return err
			}
			if err := s.invRepo.Save(txCtx, inv); err != nil {
				return err
			}
		}

		// 6. Update parcel status with CAS guard
		parcel.Status = ParcelStatusClaimed
		parcel.ClaimedAt = &now
		if err := s.repo.UpdateParcel(txCtx, parcel); err != nil {
			return err
		}

		result = &ParcelClaimResult{
			ParcelID:     parcel.ID,
			SenderName:   parcel.SenderCharacterName,
			ItemID:       parcel.ItemID,
			ItemName:     parcel.ItemName,
			ItemQuantity: parcel.ItemQuantity,
			GoldAmount:   parcel.GoldAmount,
			CurrentGold:  recipient.Money,
		}

		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, action); err != nil {
			return nil, err
		}
	} else {
		if err := action(ctx); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// CancelParcel cancels a pending parcel and returns items/gold to sender.
func (s *Service) CancelParcel(ctx context.Context, senderID string, parcelID string) error {
	if strings.TrimSpace(senderID) == "" || strings.TrimSpace(parcelID) == "" {
		return ErrInvalidInput
	}

	action := func(txCtx context.Context) error {
		// 1. Lock parcel with pessimistic lock (Tier 8) and check status & sender ownership FIRST
		parcel, err := s.repo.GetParcelByIDForUpdate(txCtx, parcelID)
		if err != nil {
			return err
		}
		if parcel.SenderCharacterID != senderID {
			return ErrForbidden
		}
		if parcel.Status != ParcelStatusPending {
			return ErrParcelAlreadyClaimed
		}

		// 2. Lock sender character (Tier 2)
		sender, err := s.charRepo.FindByIDForUpdate(txCtx, senderID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// 3. Lock sender inventory (Tier 3)
		inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, senderID)
		if err != nil {
			return err
		}

		// 4. Return gold payload (fee is non-refundable)
		if parcel.GoldAmount > 0 {
			_ = sender.AddMoney(parcel.GoldAmount)
			if err := s.charRepo.Update(txCtx, sender); err != nil {
				return err
			}
		}

		// 5. Return item if present
		if parcel.ItemID != "" && parcel.ItemQuantity > 0 {
			inst, err := coreitem.NewInstance(parcel.ItemID, parcel.ItemQuantity)
			if err != nil {
				return err
			}
			if err := inv.Add(inst); err != nil {
				return err
			}
			if err := s.invRepo.Save(txCtx, inv); err != nil {
				return err
			}
		}

		// 6. Update parcel status with CAS guard
		parcel.Status = ParcelStatusCancelled
		return s.repo.UpdateParcel(txCtx, parcel)
	}

	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, action)
	}
	return action(ctx)
}

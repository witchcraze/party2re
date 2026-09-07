package depot

import (
	"context"
	"errors"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

// SendMoney sends gold from sender to recipient using 2-character ascending row locks.
func (s *Service) SendMoney(ctx context.Context, fromCharacterID, toCharacterID string, amount int) (Depot, error) {
	fromCharacterID = strings.TrimSpace(fromCharacterID)
	toCharacterID = strings.TrimSpace(toCharacterID)
	if fromCharacterID == "" || toCharacterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	if fromCharacterID == toCharacterID {
		return Depot{}, ErrSelfTransferNotAllowed
	}
	if amount <= 0 {
		return Depot{}, ErrInvalidAmount
	}

	firstID, secondID := id.Sort2(fromCharacterID, toCharacterID)
	var senderDepot Depot

	runInTx := s.txProvider.RunInTx
	err := runInTx(ctx, func(txCtx context.Context) error {
		c1, err := s.charRepo.FindByIDForUpdate(txCtx, firstID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrRecipientNotFound
			}
			return err
		}
		c2, err := s.charRepo.FindByIDForUpdate(txCtx, secondID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrRecipientNotFound
			}
			return err
		}

		var sender, recipient corecharacter.Character
		if c1.ID == fromCharacterID {
			sender, recipient = c1, c2
		} else {
			sender, recipient = c2, c1
		}

		if sender.Money < amount {
			return ErrInsufficientFunds
		}
		if err := sender.DeductMoney(amount); err != nil {
			return ErrInsufficientFunds
		}
		_ = recipient.AddMoney(amount)

		if err := s.charRepo.Update(txCtx, sender); err != nil {
			return err
		}
		if err := s.charRepo.Update(txCtx, recipient); err != nil {
			return err
		}

		sDep, err := s.findOrCreateDepot(txCtx, fromCharacterID, sender)
		if err != nil {
			return err
		}
		senderDepot = sDep
		return nil
	})
	if err != nil {
		return Depot{}, err
	}
	return senderDepot, nil
}

// SendItem transfers an item from sender's inventory to recipient's depot using strict lock hierarchy:
// Rank 2 (characters asc) -> Rank 3 (sender inventory) -> Rank 5 (recipient depot).
func (s *Service) SendItem(ctx context.Context, fromCharacterID, toCharacterID, itemInstanceID string) (Depot, error) {
	fromCharacterID = strings.TrimSpace(fromCharacterID)
	toCharacterID = strings.TrimSpace(toCharacterID)
	itemInstanceID = strings.TrimSpace(itemInstanceID)

	if fromCharacterID == "" || toCharacterID == "" {
		return Depot{}, ErrInvalidCharacterID
	}
	if fromCharacterID == toCharacterID {
		return Depot{}, ErrSelfTransferNotAllowed
	}
	if itemInstanceID == "" {
		return Depot{}, ErrInvalidItemInstanceID
	}

	firstID, secondID := id.Sort2(fromCharacterID, toCharacterID)
	var senderDepot Depot

	runInTx := s.txProvider.RunInTx
	err := runInTx(ctx, func(txCtx context.Context) error {
		// Rank 2: Characters in ascending order
		c1, err := s.charRepo.FindByIDForUpdate(txCtx, firstID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrRecipientNotFound
			}
			return err
		}
		c2, err := s.charRepo.FindByIDForUpdate(txCtx, secondID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrRecipientNotFound
			}
			return err
		}

		var sender, recipient corecharacter.Character
		if c1.ID == fromCharacterID {
			sender, recipient = c1, c2
		} else {
			sender, recipient = c2, c1
		}

		// Rank 3: Sender inventory
		senderInv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, fromCharacterID)
		if err != nil {
			return err
		}
		itemInst, found := senderInv.Find(itemInstanceID)
		if !found {
			return ErrItemNotFound
		}

		// Rank 5: Recipient depot
		recDepot, err := s.findOrCreateDepot(txCtx, toCharacterID, recipient)
		if err != nil {
			return err
		}
		if err := recDepot.AddItem(itemInst); err != nil {
			return ErrRecipientDepotFull
		}

		if err := senderInv.Consume(itemInstanceID, itemInst.Quantity); err != nil {
			return err
		}
		if err := s.invRepo.Save(txCtx, senderInv); err != nil {
			return err
		}
		if err := s.saveDepot(txCtx, recDepot); err != nil {
			return err
		}

		sDep, err := s.findOrCreateDepot(txCtx, fromCharacterID, sender)
		if err != nil {
			return err
		}
		senderDepot = sDep
		return nil
	})
	if err != nil {
		return Depot{}, err
	}
	return senderDepot, nil
}

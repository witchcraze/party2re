package depot

import (
	"context"
	"errors"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

// SendMoney moves gold from the sender's wallet into a remittance held for the
// recipient, under 2-character ascending row locks, then Depot(5) and notice rows(8).
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
	if s.remittances == nil {
		return Depot{}, errRemittancesUnconfigured
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

		if err := s.checkMutation(txCtx, fromCharacterID); err != nil {
			return err
		}

		var sender, recipient corecharacter.Character
		if c1.ID == fromCharacterID {
			sender, recipient = c1, c2
		} else {
			sender, recipient = c2, c1
		}

		// okuru rejects a full recipient Depot before gold too (depot.cgi:349-353).
		recDepot, err := s.findOrCreateDepot(txCtx, toCharacterID, recipient)
		if err != nil {
			return err
		}
		if recDepot.IsFull() {
			return ErrRecipientDepotFull
		}
		if sender.Money < amount {
			return ErrInsufficientFunds
		}
		if err := sender.DeductMoney(amount); err != nil {
			return ErrInsufficientFunds
		}
		if err := s.charRepo.Update(txCtx, sender); err != nil {
			return err
		}
		// send_money holds the gold until the recipient collects it (system.cgi:842–855).
		if err := s.remittances.AddRemittance(txCtx, Remittance{ID: id.New(), CharacterID: toCharacterID,
			Amount: amount, Source: RemittanceDepotTransfer, SenderName: sender.Name}); err != nil {
			return err
		}

		sDep, err := s.findOrCreateDepot(txCtx, fromCharacterID, sender)
		if err != nil {
			return err
		}
		senderDepot = sDep
		// Notice rows (Rank 8) follow every Depot lock (Rank 5).
		return s.notifyMoneySent(txCtx, toCharacterID)
	})
	if err != nil {
		return Depot{}, err
	}
	return senderDepot, nil
}

// SendItem transfers an item from sender's inventory to recipient's depot using strict lock hierarchy:
// Transaction: RunInTx.
// Lock Order: characters(2, ascending IDs) -> sender inventory/equipment(3) -> recipient/sender Depot(5) -> notice insert.
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

		if err := s.checkMutation(txCtx, fromCharacterID); err != nil {
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
		detached, err := s.detachEquipment(txCtx, &sender, itemInstanceID)
		if err != nil {
			return err
		}

		// Rank 5: Recipient depot
		recDepot, err := s.findOrCreateDepot(txCtx, toCharacterID, recipient)
		if err != nil {
			return err
		}
		// An existing stack does not bypass the full check (depot.cgi:349-353).
		if recDepot.IsFull() {
			return ErrRecipientDepotFull
		}
		recDepot.Deliver(itemInst)

		if err := senderInv.Consume(itemInstanceID, itemInst.Quantity); err != nil {
			return err
		}
		if detached {
			if err := s.charRepo.Update(txCtx, sender); err != nil {
				return err
			}
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
		// Notice rows (Rank 8) follow every Depot lock (Rank 5).
		return s.notifyItemSent(txCtx, toCharacterID, sender.Name, itemInst)
	})
	if err != nil {
		return Depot{}, err
	}
	return senderDepot, nil
}

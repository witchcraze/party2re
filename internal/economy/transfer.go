package economy

import (
	"context"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

// TransferGold atomically transfers gold between two characters using deterministic lock order.
func (s *Service) TransferGold(ctx context.Context, fromCharacterID, toCharacterID string, amount int) (fromChar, toChar corecharacter.Character, err error) {
	fromCharacterID = strings.TrimSpace(fromCharacterID)
	toCharacterID = strings.TrimSpace(toCharacterID)

	if fromCharacterID == "" || toCharacterID == "" {
		return corecharacter.Character{}, corecharacter.Character{}, ErrInvalidCharacterID
	}
	if fromCharacterID == toCharacterID {
		return corecharacter.Character{}, corecharacter.Character{}, ErrSelfTransferNotAllowed
	}
	if amount <= 0 {
		return corecharacter.Character{}, corecharacter.Character{}, ErrInvalidAmount
	}

	firstID, secondID := id.Sort2(fromCharacterID, toCharacterID)

	err = s.runInTx(ctx, func(txCtx context.Context) error {
		c1, err := s.characters.FindByIDForUpdate(txCtx, firstID)
		if err != nil {
			return ErrCharacterNotFound
		}
		c2, err := s.characters.FindByIDForUpdate(txCtx, secondID)
		if err != nil {
			return ErrCharacterNotFound
		}

		var sender, recipient corecharacter.Character
		if c1.ID == fromCharacterID {
			sender = c1
			recipient = c2
		} else {
			sender = c2
			recipient = c1
		}

		if sender.Money < amount {
			return ErrInsufficientGold
		}
		if err := sender.DeductMoney(amount); err != nil {
			return ErrInsufficientGold
		}
		_ = recipient.AddMoney(amount)

		if err := s.characters.Update(txCtx, sender); err != nil {
			return err
		}
		if err := s.characters.Update(txCtx, recipient); err != nil {
			return err
		}

		fromChar = sender
		toChar = recipient
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, corecharacter.Character{}, err
	}
	return fromChar, toChar, nil
}

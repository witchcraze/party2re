package gemstore

import (
	"context"
	"strings"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
)

// SendGem transfers a gem from sender's Gem Box to recipient's Gem Box with deterministic deadlock-free locking.
func (s *Service) SendGem(ctx context.Context, senderID, recipientID, itemInstanceOrDefID string) (SendResult, error) {
	senderID = strings.TrimSpace(senderID)
	recipientID = strings.TrimSpace(recipientID)
	itemInstanceOrDefID = strings.TrimSpace(itemInstanceOrDefID)

	if senderID == "" || recipientID == "" {
		return SendResult{}, ErrInvalidCharacterID
	}
	if senderID == recipientID {
		return SendResult{}, ErrCannotSendToSelf
	}
	if itemInstanceOrDefID == "" {
		return SendResult{}, ErrInvalidGemID
	}

	firstID, secondID := id.Sort2(senderID, recipientID)

	var res SendResult
	run := func(txCtx context.Context) error {
		// 1. Lock characters in deterministic order (Rank 2)
		char1, err := s.characters.FindByIDForUpdate(txCtx, firstID)
		if err != nil {
			return err
		}
		char2, err := s.characters.FindByIDForUpdate(txCtx, secondID)
		if err != nil {
			return err
		}

		senderChar := char1
		recipientChar := char2
		if senderID == secondID {
			senderChar = char2
			recipientChar = char1
		}

		// 2. Lock gem boxes in deterministic order (Rank 8)
		box1, err := s.getOrCreateGemBoxForUpdate(txCtx, char1)
		if err != nil {
			return err
		}
		box2, err := s.getOrCreateGemBoxForUpdate(txCtx, char2)
		if err != nil {
			return err
		}

		senderBox := box1
		recipientBox := box2
		if senderID == secondID {
			senderBox = box2
			recipientBox = box1
		}

		if recipientBox.IsFull() {
			return ErrRecipientGemBoxFull
		}

		targetInst, ok := senderBox.FindItem(itemInstanceOrDefID)
		if !ok {
			return ErrItemNotOwned
		}

		gem, ok := s.catalog.FindGemByID(targetInst.DefinitionID)
		if !ok {
			itemName := resolveItemName(targetInst.DefinitionID, s.catalog, s.items)
			gem, ok = s.catalog.FindGemByName(itemName)
			if !ok {
				return ErrGemNotFound
			}
		}

		if _, err := senderBox.RemoveItem(targetInst.ID); err != nil {
			return err
		}

		transferInstance, err := coreitem.NewInstance(gem.ID, 1)
		if err != nil {
			return err
		}

		if err := recipientBox.AddItem(transferInstance); err != nil {
			return err
		}

		if err := s.gemBoxes.Save(txCtx, senderBox); err != nil {
			return err
		}
		if err := s.gemBoxes.Save(txCtx, recipientBox); err != nil {
			return err
		}

		res = SendResult{
			SenderCharacter:    senderChar,
			RecipientCharacter: recipientChar,
			SenderGemBox:       senderBox,
			RecipientGemBox:    recipientBox,
			Gem:                gem,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return SendResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return SendResult{}, err
		}
	}

	return res, nil
}

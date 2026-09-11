package gemstore

import (
	"context"
	"strings"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

// BuyGem purchases a gem from the gem store and places it into the character's dedicated Gem Box.
func (s *Service) BuyGem(ctx context.Context, characterID, gemID string) (BuyResult, error) {
	characterID = strings.TrimSpace(characterID)
	gemID = strings.TrimSpace(gemID)
	if characterID == "" {
		return BuyResult{}, ErrInvalidCharacterID
	}
	if gemID == "" {
		return BuyResult{}, ErrInvalidGemID
	}

	gem, ok := s.catalog.FindGemByID(gemID)
	if !ok {
		gem, ok = s.catalog.FindGemByName(gemID)
		if !ok {
			return BuyResult{}, ErrGemNotFound
		}
	}

	price := gem.Price * ShopPriceMultiplier

	var res BuyResult
	run := func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Unlock requirement is based on job_lv (transfer count)
		if gem.RequiredJobLevel > 1 && char.JobLevel < gem.RequiredJobLevel {
			return ErrLevelTooLow
		}

		box, err := s.getOrCreateGemBoxForUpdate(txCtx, char)
		if err != nil {
			return err
		}

		if box.IsFull() {
			return ErrGemBoxFull
		}

		if err := char.DeductMoney(price); err != nil {
			return ErrInsufficientFunds
		}
		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}

		inst, err := coreitem.NewInstance(gem.ID, 1)
		if err != nil {
			return err
		}

		if err := box.AddItem(inst); err != nil {
			return err
		}

		if err := s.gemBoxes.Save(txCtx, box); err != nil {
			return err
		}

		res = BuyResult{
			Character:    char,
			GemBox:       box,
			Gem:          gem,
			Cost:         price,
			ItemInstance: inst,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return BuyResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return BuyResult{}, err
		}
	}

	return res, nil
}

// SellGem sells a gem from the character's Gem Box for 50% of its base price.
func (s *Service) SellGem(ctx context.Context, characterID, itemInstanceOrDefID string) (SellResult, error) {
	characterID = strings.TrimSpace(characterID)
	itemInstanceOrDefID = strings.TrimSpace(itemInstanceOrDefID)
	if characterID == "" {
		return SellResult{}, ErrInvalidCharacterID
	}
	if itemInstanceOrDefID == "" {
		return SellResult{}, ErrInvalidGemID
	}

	var res SellResult
	run := func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		box, err := s.getOrCreateGemBoxForUpdate(txCtx, char)
		if err != nil {
			return err
		}

		targetInst, ok := box.FindItem(itemInstanceOrDefID)
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

		sellPrice := int(float64(gem.Price) * 0.5)
		if sellPrice < 1 {
			sellPrice = 1
		}

		if err := char.AddMoney(sellPrice); err != nil {
			return err
		}
		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}

		if _, err := box.RemoveItem(targetInst.ID); err != nil {
			return err
		}

		if err := s.gemBoxes.Save(txCtx, box); err != nil {
			return err
		}

		res = SellResult{
			Character: char,
			GemBox:    box,
			Gem:       gem,
			Payout:    sellPrice,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return SellResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return SellResult{}, err
		}
	}

	return res, nil
}

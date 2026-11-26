package god

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/depot"
)

func (s *Service) executeUnderworldWish(
	ctx context.Context,
	char *corecharacter.Character,
	wishID string,
	res *WishResult,
) error {
	tier := map[string]*int{
		WishExpandDepot:      &char.OverDepot,
		WishExpandMonster:    &char.OverMonster,
		WishExpandJobMemory:  &char.OverFuture,
		WishExpandFleaMarket: &char.OverFlea,
		WishExpandShopStore:  &char.OverStore,
	}[wishID]
	if tier == nil {
		return ErrWishNotFound
	}
	if *tier >= MaxLimitBreakTier {
		refuse(char, catalogWish(buildUnderworldWishes(*char), wishID), res)
		return nil
	}
	*tier++

	if wishID == WishExpandDepot && s.depots != nil {
		dep, err := s.depots.FindByCharacterIDForUpdate(ctx, char.ID)
		if errors.Is(err, depot.ErrNotFound) {
			dep, err = depot.NewDepotWithCapacity(char.ID, char.JobLevel, 0, char.OverDepot)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		dep.RefreshCapacity(char.JobLevel, char.OverDepot)
		if err := s.depots.Save(ctx, dep); err != nil {
			return err
		}
	}

	if err := s.characters.Update(ctx, *char); err != nil {
		return err
	}
	*res = WishResult{Character: *char, Wish: catalogWish(buildUnderworldWishes(*char), wishID)}
	return nil
}

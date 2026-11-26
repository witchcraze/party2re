package home

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

const (
	sealedKeyName  = "異封の鍵"
	sealedBoxID    = "item-258"
	sealedBoxPrize = 59 // always delivered first; slot 0 of the pool, so never drawn
)

// sealedBoxPool follows lib/_data.cgi item 259: 2..4 draws from slots 1..N−1.
var sealedBoxPool = rewardBag{
	slots: joinSlots([]int{sealedBoxPrize, 150, 150, 243, 243, 254, 254, 254, 253, 253, 253, 255, 215, 215, 207, 207, 125, 125, 126},
		numberRange(60, 65), numberRange(60, 65), numberRange(104, 107), []int{129},
		numberRange(152, 157), numberRange(152, 156), []int{168, 168, 216, 216}),
	min: 2, max: 4,
}

// openSealedBox removes the held box, else the first Depot box, and delivers
// its rewards to the Depot regardless of capacity. Without a box it has no
// effect; the caller's D4 rule still consumes a Depot key.
func (s *Service) openSealedBox(tc *economy.TxContext, _ item.Definition) (HomeItemOutcome, error) {
	if s.depotMgr == nil || s.catalog == nil {
		return HomeItemOutcome{}, ErrCannotUseHere
	}
	if i := slices.IndexFunc(tc.Inventory.Items, func(it item.Instance) bool { return it.DefinitionID == sealedBoxID }); i >= 0 {
		if err := tc.Inventory.Consume(tc.Inventory.Items[i].ID, 1); err != nil {
			return HomeItemOutcome{}, err
		}
	} else {
		dp, err := s.depotMgr.FindByCharacterID(tc.Context, tc.Character.ID)
		if err != nil {
			return HomeItemOutcome{}, err
		}
		j := slices.IndexFunc(dp.Items, func(it item.Instance) bool { return it.DefinitionID == sealedBoxID })
		if j < 0 {
			return HomeItemOutcome{Effect: HomeItemEffectNoSealedBox}, errNoEffect
		}
		if err := s.depotMgr.ConsumeOne(tc.Context, tc.Character.ID, dp.Items[j].ID); err != nil {
			return HomeItemOutcome{}, err
		}
	}

	names, err := s.deliverItemNumbers(tc.Context, tc.Character.ID, append([]int{sealedBoxPrize}, sealedBoxPool.draw(s.randomInt)...))
	if err != nil {
		return HomeItemOutcome{}, err
	}
	return HomeItemOutcome{Effect: HomeItemEffectSealedBoxOpened, Items: names}, nil
}

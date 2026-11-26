package home

import (
	"context"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
)

// rewardBag describes a Home bag/treasure (items 125/126/199): a weighted slot
// pool of legacy item numbers and the inclusive reward-count range.
type rewardBag struct {
	slots    []int
	min, max int
}

func numberRange(from, to int) []int {
	out := make([]int, 0, to-from+1)
	for n := from; n <= to; n++ {
		out = append(out, n)
	}
	return out
}

func joinSlots(parts ...[]int) []int {
	var out []int
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// Pools follow lib/_data.cgi:1479–1505,1809–1820; repeated numbers are weights.
var rewardBags = map[string]rewardBag{
	"福袋": {
		slots: joinSlots([]int{1, 1, 1, 1, 1, 1, 7}, numberRange(7, 10), numberRange(1, 26),
			numberRange(41, 56), numberRange(72, 87), numberRange(101, 103), numberRange(125, 128),
			[]int{130, 130, 130}, numberRange(130, 134)),
		min: 1, max: 1,
	},
	"幸福袋": {
		slots: joinSlots(numberRange(128, 134), numberRange(128, 136)),
		min:   1, max: 1,
	},
	"太古の財宝": {
		slots: []int{23, 23, 23, 23, 24, 24, 24, 24, 25, 25, 25, 25, 26, 26, 26, 105, 106, 134, 134,
			12, 12, 15, 15, 43, 147, 147, 150, 151, 151, 168, 193, 193, 193, 196, 215, 215, 238, 249},
		min: 2, max: 5,
	},
}

// draw picks slots 1..N−1 uniformly; legacy never reaches slot 0.
func (b rewardBag) draw(intn func(int) int) []int {
	count := b.min + intn(b.max-b.min+1)
	out := make([]int, count)
	for i := range out {
		out[i] = b.slots[1+intn(len(b.slots)-1)]
	}
	return out
}

// applyRewardBag delivers every reward to the owner's Depot as separate
// instances, ignoring capacity, inside the caller's transaction.
func (s *Service) applyRewardBag(ctx context.Context, char *corecharacter.Character, bag rewardBag) (HomeItemOutcome, error) {
	if s.depotMgr == nil || s.catalog == nil {
		return HomeItemOutcome{}, ErrCannotUseHere
	}
	names, err := s.deliverItemNumbers(ctx, char.ID, bag.draw(s.randomInt))
	if err != nil {
		return HomeItemOutcome{}, err
	}
	return HomeItemOutcome{Effect: HomeItemEffectRewardItems, Items: names}, nil
}

// deliverItemNumbers appends one instance per legacy item number to the
// Depot, ignoring capacity, and returns their names in order.
func (s *Service) deliverItemNumbers(ctx context.Context, characterID string, numbers []int) ([]string, error) {
	rewards := make([]item.Instance, 0, len(numbers))
	names := make([]string, 0, len(numbers))
	for _, n := range numbers {
		def, err := s.catalog.FindByID(fmt.Sprintf("item-%03d", n))
		if err != nil {
			return nil, err
		}
		inst, err := def.NewInstance(1)
		if err != nil {
			return nil, err
		}
		rewards = append(rewards, inst)
		names = append(names, def.Name)
	}
	if err := s.depotMgr.AppendDeliveries(ctx, characterID, rewards); err != nil {
		return nil, err
	}
	return names, nil
}

package battle

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

// BuildBattleMaxima captures quest-entry maxima without rolling equipment stats.
func (s *Service) BuildBattleMaxima(ctx context.Context, char corecharacter.Character) (hp, mp int, err error) {
	hp, mp = char.Stats.MaxHP, char.Stats.MaxMP
	if s.invRepo == nil {
		return hp, mp, nil
	}
	inv, err := s.invRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		return 0, 0, err
	}
	hp, mp = HeldVitalityMaxima(inv, hp, mp)
	return hp, mp, nil
}

// HeldVitalityMaxima applies the category-4 return to the battle line
// (quest.cgi:1187–1189). The flat cap also lowers a maximum above 999.
func HeldVitalityMaxima(inv coreinventory.Inventory, hp, mp int) (int, int) {
	var held string
	for _, inst := range inv.Items {
		if inst.Quantity <= 0 || item.LegacyHandKindByID(inst.DefinitionID) != 3 {
			continue
		}
		// shortcut: multiple tool rows leave the held item unknown, until #1329 binds the instance.
		if held != "" {
			return hp, mp
		}
		held = inst.DefinitionID
	}
	switch held {
	case "item-113":
		hp = min(hp+100, 999)
	case "item-114":
		mp = min(mp+100, 999)
	}
	return hp, mp
}

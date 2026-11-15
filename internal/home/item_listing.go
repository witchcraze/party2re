package home

import (
	"context"

	"github.com/witchcraze/party2re/internal/core/item"
)

// ListHomeItems lists the held and Depot items ＠つかう can inspect or use.
// A failed inventory or Depot read fails the listing.
func (s *Service) ListHomeItems(ctx context.Context, characterID string) ([]HomeUsableItem, error) {
	results := []HomeUsableItem{}
	if s.invMgr != nil {
		inv, err := s.invMgr.FindByCharacterID(ctx, characterID)
		if err != nil {
			return nil, err
		}
		for _, inst := range inv.Items {
			results = append(results, s.homeUsableItem(inst, "inventory"))
		}
	}
	if s.depotMgr != nil {
		dp, err := s.depotMgr.FindByCharacterID(ctx, characterID)
		if err != nil {
			return nil, err
		}
		for _, inst := range dp.Items {
			results = append(results, s.homeUsableItem(inst, "depot"))
		}
	}
	return results, nil
}

func (s *Service) homeUsableItem(inst item.Instance, source string) HomeUsableItem {
	out := HomeUsableItem{InstanceID: inst.ID, DefinitionID: inst.DefinitionID, Name: inst.DefinitionID, Kind: 3, Quantity: inst.Quantity, Source: source}
	def, err := s.catalog.FindByID(inst.DefinitionID)
	if err != nil {
		return out
	}
	out.Name, out.Slot, out.Price, out.Kind = def.Name, def.Slot, def.Price, def.Kind()
	switch out.Kind {
	case 1:
		out.Attack, out.Weight = def.Power, def.Weight
		if out.Attack == 0 && out.Weight == 0 {
			out.Attack, out.Weight = item.NominalWeaponStats(inst.DefinitionID)
		}
	case 2:
		out.Defense, out.Weight = def.Power, def.Weight
		if out.Defense == 0 && out.Weight == 0 {
			out.Defense, out.Weight = item.NominalArmorStats(inst.DefinitionID)
		}
	}
	return out
}

package home

import (
	"context"

	"github.com/witchcraze/party2re/internal/core/item"
)

// ListHomeItems retrieves all usable and inspectable items from inventory and depot.
func (s *Service) ListHomeItems(ctx context.Context, characterID string) ([]HomeUsableItem, error) {
	var results []HomeUsableItem

	if s.invMgr != nil {
		inv, err := s.invMgr.FindByCharacterID(ctx, characterID)
		if err == nil {
			for _, inst := range inv.Items {
				def, err := s.catalog.FindByID(inst.DefinitionID)
				name := inst.DefinitionID
				var slot item.Slot
				price := 0
				kind := 3
				if err == nil {
					name = def.Name
					slot = def.Slot
					price = def.Price
					kind = def.Kind()
				}
				attack := 0
				defense := 0
				weight := 0
				if kind == 1 {
					attack, weight = def.Power, def.Weight
					if attack == 0 && weight == 0 {
						attack, weight = item.NominalWeaponStats(inst.DefinitionID)
					}
				} else if kind == 2 {
					defense, weight = def.Power, def.Weight
					if defense == 0 && weight == 0 {
						defense, weight = item.NominalArmorStats(inst.DefinitionID)
					}
				}
				results = append(results, HomeUsableItem{
					InstanceID:   inst.ID,
					DefinitionID: inst.DefinitionID,
					Name:         name,
					Kind:         kind,
					Slot:         slot,
					Attack:       attack,
					Defense:      defense,
					Weight:       weight,
					Price:        price,
					Quantity:     inst.Quantity,
					Source:       "inventory",
				})
			}
		}
	}

	if s.depotMgr != nil {
		dp, err := s.depotMgr.FindByCharacterID(ctx, characterID)
		if err == nil {
			for _, inst := range dp.Items {
				def, err := s.catalog.FindByID(inst.DefinitionID)
				name := inst.DefinitionID
				var slot item.Slot
				price := 0
				kind := 3
				if err == nil {
					name = def.Name
					slot = def.Slot
					price = def.Price
					kind = def.Kind()
				}
				attack := 0
				defense := 0
				weight := 0
				if kind == 1 {
					attack, weight = def.Power, def.Weight
					if attack == 0 && weight == 0 {
						attack, weight = item.NominalWeaponStats(inst.DefinitionID)
					}
				} else if kind == 2 {
					defense, weight = def.Power, def.Weight
					if defense == 0 && weight == 0 {
						defense, weight = item.NominalArmorStats(inst.DefinitionID)
					}
				}
				results = append(results, HomeUsableItem{
					InstanceID:   inst.ID,
					DefinitionID: inst.DefinitionID,
					Name:         name,
					Kind:         kind,
					Slot:         slot,
					Attack:       attack,
					Defense:      defense,
					Weight:       weight,
					Price:        price,
					Quantity:     inst.Quantity,
					Source:       "depot",
				})
			}
		}
	}

	return results, nil
}

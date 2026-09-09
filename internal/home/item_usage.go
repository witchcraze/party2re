package home

import (
	"context"
	"fmt"
	"strings"

	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type DepotManager interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	RemoveItem(ctx context.Context, characterID, itemInstanceID string) error
}

type InventoryManager interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Consume(ctx context.Context, characterID, instanceID string, quantity int) (coreinventory.Inventory, error)
}

type ItemCatalog interface {
	FindByID(id string) (item.Definition, error)
}

func itemKindFromSlot(slot item.Slot) int {
	switch slot {
	case item.SlotMainHand:
		return 1 // Weapon
	case item.SlotOffHand, item.SlotBody, item.SlotAccessory:
		return 2 // Armor/Shield/Accessory
	default:
		return 3 // Consumable / Other
	}
}

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
					kind = itemKindFromSlot(def.Slot)
				}
				results = append(results, HomeUsableItem{
					InstanceID:   inst.ID,
					DefinitionID: inst.DefinitionID,
					Name:         name,
					Kind:         kind,
					Slot:         slot,
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
					kind = itemKindFromSlot(def.Slot)
				}
				results = append(results, HomeUsableItem{
					InstanceID:   inst.ID,
					DefinitionID: inst.DefinitionID,
					Name:         name,
					Kind:         kind,
					Slot:         slot,
					Price:        price,
					Quantity:     inst.Quantity,
					Source:       "depot",
				})
			}
		}
	}

	return results, nil
}

// UseHomeItem inspects or consumes an item from either the character's inventory or depot.
func (s *Service) UseHomeItem(ctx context.Context, characterID, instanceID, source string) (*UseHomeItemResult, error) {
	char, err := s.charReader.FindByID(ctx, characterID)
	if err != nil {
		return nil, err
	}

	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "inventory"
	}

	var definitionID string
	var itemInst item.Instance
	var found bool

	if source == "inventory" && s.invMgr != nil {
		inv, err := s.invMgr.FindByCharacterID(ctx, characterID)
		if err == nil {
			itemInst, found = inv.Find(instanceID)
			if found {
				definitionID = itemInst.DefinitionID
			}
		}
	} else if source == "depot" && s.depotMgr != nil {
		dp, err := s.depotMgr.FindByCharacterID(ctx, characterID)
		if err == nil {
			for _, it := range dp.Items {
				if it.ID == instanceID {
					itemInst = it
					definitionID = it.DefinitionID
					found = true
					break
				}
			}
		}
	}

	if !found {
		return nil, ErrItemNotFound
	}

	var def item.Definition
	if s.catalog != nil {
		def, err = s.catalog.FindByID(definitionID)
		if err != nil {
			def = item.Definition{ID: definitionID, Name: definitionID, Price: 0}
		}
	} else {
		def = item.Definition{ID: definitionID, Name: definitionID, Price: 0}
	}

	kind := itemKindFromSlot(def.Slot)

	// Inspection of weapons
	if kind == 1 {
		power := def.Price/10 + 1
		msg := fmt.Sprintf("武器名：%s / 強さ：%d / 価格：%dG", def.Name, power, def.Price)
		return &UseHomeItemResult{
			Action:    "inspect",
			Message:   msg,
			ItemName:  def.Name,
			Kind:      1,
			Consumed:  false,
			Character: &char,
		}, nil
	}

	// Inspection of armors/accessories
	if kind == 2 {
		defense := def.Price/10 + 1
		msg := fmt.Sprintf("防具名：%s / 強さ：%d / 価格：%dG", def.Name, defense, def.Price)
		return &UseHomeItemResult{
			Action:    "inspect",
			Message:   msg,
			ItemName:  def.Name,
			Kind:      2,
			Consumed:  false,
			Character: &char,
		}, nil
	}

	// Consumable item logic
	var msg string
	isUsable := true

	switch def.Name {
	case "命の木の実":
		v := s.randomInt(4) + 3
		char.Stats.MaxHP += v
		char.Stats.HP += v
		msg = fmt.Sprintf("%sのHPが %d あがった！", char.Name, v)
	case "不思議な木の実":
		v := s.randomInt(4) + 3
		char.Stats.MaxMP += v
		char.Stats.MP += v
		msg = fmt.Sprintf("%sのMPが %d あがった！", char.Name, v)
	case "力の種":
		v := s.randomInt(6) + 1
		char.Stats.Attack += v
		msg = fmt.Sprintf("%sの攻撃力が %d あがった！", char.Name, v)
	case "守りの種":
		v := s.randomInt(6) + 1
		char.Stats.Defense += v
		msg = fmt.Sprintf("%sの守備力が %d あがった！", char.Name, v)
	case "素早さの種":
		v := s.randomInt(6) + 1
		char.Stats.Agility += v
		msg = fmt.Sprintf("%sの素早さが %d あがった！", char.Name, v)
	case "スキルの種":
		v := s.randomInt(3) + 1
		if err := char.AddSP(v); err != nil {
			return nil, err
		}
		msg = fmt.Sprintf("%sのSPが %d あがった！", char.Name, v)
	case "小さなメダル":
		if err := char.AddSmallMedals(1); err != nil {
			return nil, err
		}
		msg = "メダル王にメダルを１枚献上しました"
	case "薬草":
		heal := 40
		char.Stats.HP += heal
		if char.Stats.HP > char.Stats.MaxHP {
			char.Stats.HP = char.Stats.MaxHP
		}
		msg = fmt.Sprintf("%sをつかった！HPが回復した！", def.Name)
	case "上薬草":
		heal := 100
		char.Stats.HP += heal
		if char.Stats.HP > char.Stats.MaxHP {
			char.Stats.HP = char.Stats.MaxHP
		}
		msg = fmt.Sprintf("%sをつかった！HPが大幅に回復した！", def.Name)
	case "特薬草":
		heal := 250
		char.Stats.HP += heal
		if char.Stats.HP > char.Stats.MaxHP {
			char.Stats.HP = char.Stats.MaxHP
		}
		msg = fmt.Sprintf("%sをつかった！HPが超回復した！", def.Name)
	case "世界樹のしずく":
		char.Stats.HP = char.Stats.MaxHP
		msg = fmt.Sprintf("%sをつかった！HPが全回復した！", def.Name)
	case "魔法の聖水":
		heal := 40
		char.Stats.MP += heal
		if char.Stats.MP > char.Stats.MaxMP {
			char.Stats.MP = char.Stats.MaxMP
		}
		msg = fmt.Sprintf("%sをつかった！MPが回復した！", def.Name)
	case "祈りの指輪":
		heal := 100
		char.Stats.MP += heal
		if char.Stats.MP > char.Stats.MaxMP {
			char.Stats.MP = char.Stats.MaxMP
		}
		msg = fmt.Sprintf("%sをつかった！MPが大幅に回復した！", def.Name)
	case "エルフの飲み薬":
		char.Stats.MP = char.Stats.MaxMP
		msg = fmt.Sprintf("%sをつかった！MPが全回復した！", def.Name)
	default:
		isUsable = false
	}

	if !isUsable {
		return &UseHomeItemResult{
			Action:    "cannot_use",
			Message:   fmt.Sprintf("%sはここでは使えません", def.Name),
			ItemName:  def.Name,
			Kind:      3,
			Consumed:  false,
			Character: &char,
		}, nil
	}

	// Consume 1 item
	if source == "inventory" && s.invMgr != nil {
		if _, err := s.invMgr.Consume(ctx, characterID, instanceID, 1); err != nil {
			return nil, err
		}
	} else if source == "depot" && s.depotMgr != nil {
		if err := s.depotMgr.RemoveItem(ctx, characterID, instanceID); err != nil {
			return nil, err
		}
	}

	// Update character state
	if err := s.charUpdater.Update(ctx, char); err != nil {
		return nil, err
	}

	return &UseHomeItemResult{
		Action:    "consumed",
		Message:   msg,
		ItemName:  def.Name,
		Kind:      3,
		Consumed:  true,
		Character: &char,
	}, nil
}

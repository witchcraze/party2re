package home

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

// errNoEffect is a sentinel returned by applyConsumableEffect when the item
// has no effect and must not be consumed (e.g. raw crystal with no weapon seal).
var errNoEffect = errors.New("item had no effect")

type DepotManager interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	Consume(ctx context.Context, characterID, itemInstanceID string, quantity int) error
	ConsumeOne(ctx context.Context, characterID, itemInstanceID string) error
}

type InventoryManager interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Consume(ctx context.Context, characterID, instanceID string, quantity int) (coreinventory.Inventory, error)
}

type ItemCatalog interface {
	FindByID(id string) (item.Definition, error)
}

type homeRNGAdapter struct {
	gen random.Generator
}

func (a homeRNGAdapter) Intn(n int) (int, error) {
	if a.gen == nil || n <= 0 {
		return 0, nil
	}
	return a.gen.Intn(n), nil
}

func (s *Service) randomSource() corecharacter.RandomSource {
	if s == nil || s.rng == nil {
		return nil
	}
	return homeRNGAdapter{gen: s.rng}
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

	kind := def.Kind()

	// Inspection of weapons
	if kind == 1 {
		power, weight := def.Power, def.Weight
		if power == 0 && weight == 0 {
			power, weight = item.CalculateWeaponStats(def.ID, char, s.randomSource(), false)
		}
		msg := fmt.Sprintf("武器名：%s / 強さ：%d / 重さ：%d / 価格：%dG", def.Name, power, weight, def.Price)
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
		defense, weight := def.Power, def.Weight
		if defense == 0 && weight == 0 {
			defense, weight = item.CalculateArmorStats(def.ID, char, s.randomSource())
		}
		msg := fmt.Sprintf("防具名：%s / 強さ：%d / 重さ：%d / 価格：%dG", def.Name, defense, weight, def.Price)
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
	if def.UsageCategory == item.UsageCategoryCombatOnly {
		return nil, fmt.Errorf("%w: %sは戦闘中でしか使えません", ErrCannotUseHere, def.Name)
	}
	if def.UsageCategory != item.UsageCategoryNone && !def.UsageCategory.IsUsableAtHome() {
		return nil, fmt.Errorf("%w: %sはここでは使えません", ErrCannotUseHere, def.Name)
	}

	// Validate supported consumable items before transaction
	if !isCostumeItem(def.Name) && !isRecipeScrollItem(def.Name) {
		switch def.Name {
		case "命の木の実", "不思議な木の実", "力の種", "守りの種", "素早さの種", "スキルの種", "幸せの種", "ファイト一発", "気合の霊薬", "小さなメダル", "水晶の原石":
		default:
			return nil, fmt.Errorf("%w: %sはここでは使えません", ErrCannotUseHere, def.Name)
		}
	}

	if s.runner != nil {
		var resMsg string
		if source == "inventory" {
			req := economy.TransactionRequest{
				CharacterID:   characterID,
				LockInventory: true,
				Cost: economy.ResourceCost{
					ItemInstanceID:  instanceID,
					ItemInstanceQty: 1,
				},
			}
			txRes, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
				msg, err := s.applyConsumableEffect(tc.Context, &tc.Character, def)
				resMsg = msg // capture even when err != nil (e.g. errNoEffect carries the message)
				if err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				if errors.Is(err, errNoEffect) {
					// Item had no effect; transaction was rolled back, nothing consumed.
					return &UseHomeItemResult{
						Action:    "no_effect",
						Message:   resMsg,
						ItemName:  def.Name,
						Kind:      3,
						Consumed:  false,
						Character: &char,
					}, nil
				}
				if errors.Is(err, economy.ErrItemNotFound) || errors.Is(err, economy.ErrInsufficientItemQuantity) {
					return nil, ErrItemNotFound
				}
				if errors.Is(err, economy.ErrCharacterNotFound) {
					return nil, ErrCharacterNotFound
				}
				return nil, err
			}
			return &UseHomeItemResult{
				Action:    "consumed",
				Message:   resMsg,
				ItemName:  def.Name,
				Kind:      3,
				Consumed:  true,
				Character: &txRes.Character,
			}, nil
		}

		// source == "depot"
		req := economy.TransactionRequest{
			CharacterID:   characterID,
			LockInventory: false,
		}
		txRes, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
			if s.depotMgr == nil {
				return ErrItemNotFound
			}
			// Apply effect before consuming so that a no-effect sentinel aborts
			// the transaction without touching the depot.
			msg, err := s.applyConsumableEffect(tc.Context, &tc.Character, def)
			resMsg = msg // capture even when err != nil (e.g. errNoEffect carries the message)
			if err != nil {
				return err
			}
			if err := s.depotMgr.ConsumeOne(tc.Context, characterID, instanceID); err != nil {
				if errors.Is(err, depot.ErrItemNotFound) || errors.Is(err, depot.ErrNotFound) {
					return ErrItemNotFound
				}
				return err
			}
			resMsg = msg
			return nil
		})
		if err != nil {
			if errors.Is(err, errNoEffect) {
				// Item had no effect; transaction was rolled back, nothing consumed.
				return &UseHomeItemResult{
					Action:    "no_effect",
					Message:   resMsg,
					ItemName:  def.Name,
					Kind:      3,
					Consumed:  false,
					Character: &char,
				}, nil
			}
			if errors.Is(err, economy.ErrItemNotFound) || errors.Is(err, ErrItemNotFound) {
				return nil, ErrItemNotFound
			}
			if errors.Is(err, economy.ErrCharacterNotFound) {
				return nil, ErrCharacterNotFound
			}
			return nil, err
		}
		return &UseHomeItemResult{
			Action:    "consumed",
			Message:   resMsg,
			ItemName:  def.Name,
			Kind:      3,
			Consumed:  true,
			Character: &txRes.Character,
		}, nil
	}

	// Fallback path for unit test mocks without runner configured
	msg, err := s.applyConsumableEffect(ctx, &char, def)
	if err != nil {
		if errors.Is(err, errNoEffect) {
			// Item had no effect; skip consumption and return early.
			return &UseHomeItemResult{
				Action:    "no_effect",
				Message:   msg,
				ItemName:  def.Name,
				Kind:      3,
				Consumed:  false,
				Character: &char,
			}, nil
		}
		return nil, err
	}

	// Consume 1 item
	if source == "inventory" && s.invMgr != nil {
		if _, err := s.invMgr.Consume(ctx, characterID, instanceID, 1); err != nil {
			return nil, err
		}
	} else if source == "depot" && s.depotMgr != nil {
		if err := s.depotMgr.ConsumeOne(ctx, characterID, instanceID); err != nil {
			return nil, err
		}
	}

	// Update character state
	if s.charUpdater != nil {
		if err := s.charUpdater.Update(ctx, char); err != nil {
			return nil, err
		}
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

func (s *Service) applyConsumableEffect(ctx context.Context, char *corecharacter.Character, def item.Definition) (string, error) {
	if isRecipeScrollItem(def.Name) {
		return s.applyRecipeScrollConsumable(ctx, char, def.Name)
	}
	if costumeMsg, ok, err := s.applyCostumeConsumable(ctx, char, def.Name); ok {
		return costumeMsg, err
	}

	var msg string

	switch def.Name {
	case "命の木の実":
		v := s.randomInt(4) + 3
		if char.OverLevel {
			v = 0
		}
		char.Stats.MaxHP += v
		char.Stats.Clamp(char.OverLevel, char.Level)
		msg = fmt.Sprintf("%sのHPが %d あがった！", char.Name, v)
	case "不思議な木の実":
		v := s.randomInt(4) + 3
		if char.OverLevel {
			v = 0
		}
		char.Stats.MaxMP += v
		char.Stats.Clamp(char.OverLevel, char.Level)
		msg = fmt.Sprintf("%sのMPが %d あがった！", char.Name, v)
	case "力の種":
		v := s.randomInt(6) + 1
		if char.OverLevel {
			v = 0
		}
		char.Stats.Attack += v
		char.Stats.Clamp(char.OverLevel, char.Level)
		msg = fmt.Sprintf("%sの攻撃力が %d あがった！", char.Name, v)
	case "守りの種":
		v := s.randomInt(6) + 1
		if char.OverLevel {
			v = 0
		}
		char.Stats.Defense += v
		char.Stats.Clamp(char.OverLevel, char.Level)
		msg = fmt.Sprintf("%sの守備力が %d あがった！", char.Name, v)
	case "素早さの種":
		v := s.randomInt(6) + 1
		if char.OverLevel {
			v = 0
		}
		char.Stats.Agility += v
		char.Stats.Clamp(char.OverLevel, char.Level)
		msg = fmt.Sprintf("%sの素早さが %d あがった！", char.Name, v)
	case "スキルの種":
		v := s.randomInt(3) + 1
		if err := char.AddSP(v); err != nil {
			return "", err
		}
		msg = fmt.Sprintf("%sのSPが %d あがった！", char.Name, v)
	case "幸せの種":
		if err := progression.ApplyHappySeed(char); err != nil {
			return "", err
		}
		msg = "次のクエスト時にレベルアップ！"
	case "ファイト一発", "気合の霊薬":
		char.ResetTired()
		msg = fmt.Sprintf("元気全快！%sの疲労が回復した！", char.Name)
	case "小さなメダル":
		if err := char.AddSmallMedals(1); err != nil {
			return "", err
		}
		msg = "メダル王にメダルを１枚献上しました"
	case "水晶の原石":
		if char.WeaponSeal <= 0 {
			// No seal to remove: return the no-effect sentinel so the caller
			// knows to skip consumption and rollback the transaction.
			return "しかし、何も起こらなかった…", errNoEffect
		} else {
			refund := sealRefundCrystals(char.WeaponSeal)
			char.WeaponSeal = 0
			if err := char.AddCrystal(refund); err != nil {
				return "", err
			}
			msg = fmt.Sprintf("武器に施された刻印が剥がれ落ちた！刻印晶を %d 個回収した！", refund)
		}
	default:
		return "", fmt.Errorf("%w: %sはここでは使えません", ErrCannotUseHere, def.Name)
	}

	return msg, nil
}

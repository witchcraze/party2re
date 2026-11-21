package home

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

const (
	heavenReinsName     = "天馬のたづな"
	underworldReinsName = "黒い天馬のたづな"
	// Items 113/114: Home calls their callbacks without stats and discards the
	// returned hash, so use consumes one without any stat change.
	lifeBraceletName = "命のブレスレット"
	sorceryRingName  = "ソーサリーリング"
	sacredManaName   = "聖なるマナ"
)

// orbItems are the legacy items 60..65 (_data.cgi); using one offers it to
// the Altar of Rebirth from either Home source.
var orbItems = map[string]rune{
	"シルバーオーブ": corecharacter.OrbSilver, "レッドオーブ": corecharacter.OrbRed, "ブルーオーブ": corecharacter.OrbBlue,
	"グリーンオーブ": corecharacter.OrbGreen, "イエローオーブ": corecharacter.OrbYellow, "パープルオーブ": corecharacter.OrbPurple,
}

type InventoryManager interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
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
		if err != nil {
			return nil, err
		}
		itemInst, found = inv.Find(instanceID)
		definitionID = itemInst.DefinitionID
	} else if source == "depot" && s.depotMgr != nil {
		dp, err := s.depotMgr.FindByCharacterID(ctx, characterID)
		if err != nil {
			return nil, err
		}
		for _, it := range dp.Items {
			if it.ID == instanceID {
				itemInst, definitionID, found = it, it.DefinitionID, true
				break
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
	effect, ok := homeEffectFor(def.Name)
	if !ok || (def.UsageCategory != item.UsageCategoryNone && !def.UsageCategory.IsUsableAtHome()) {
		return nil, fmt.Errorf("%w: %sはここでは使えません", ErrCannotUseHere, def.Name)
	}
	if s.runner == nil {
		return nil, ErrTransactionRunnerRequired
	}

	var resMsg string
	var kept bool
	req := economy.TransactionRequest{CharacterID: characterID, LockInventory: source == "inventory"}
	if source == "inventory" {
		req.CostFunc = func(c corecharacter.Character) (economy.ResourceCost, error) {
			if homeItemKept(def, c) {
				return economy.ResourceCost{}, nil
			}
			return economy.ResourceCost{ItemInstanceID: instanceID, ItemInstanceQty: 1}, nil
		}
	}
	txRes, err := s.runner.ExecuteTransaction(ctx, req, func(tc *economy.TxContext) error {
		if kept = homeItemKept(def, tc.Character); kept {
			if err := s.requireHeld(tc, source, instanceID); err != nil {
				return err
			}
		}
		// The effect runs before Depot consumption so that a no-effect sentinel
		// aborts the transaction without touching the Depot.
		msg, err := effect(s, tc.Context, &tc.Character, def)
		resMsg = msg // errNoEffect carries the message
		if err != nil || kept || source != "depot" {
			return err
		}
		if err := s.depotMgr.ConsumeOne(tc.Context, characterID, instanceID); err != nil {
			if errors.Is(err, depot.ErrItemNotFound) || errors.Is(err, depot.ErrNotFound) {
				return ErrItemNotFound
			}
			return err
		}
		return nil
	})
	switch {
	case err == nil:
		return committedHomeItemResult(def, resMsg, kept, &txRes.Character), nil
	case errors.Is(err, errNoEffect):
		// Rolled back: nothing consumed or changed.
		return &UseHomeItemResult{Action: "no_effect", Message: resMsg, ItemName: def.Name, Kind: 3, Character: &char}, nil
	case errors.Is(err, economy.ErrItemNotFound), errors.Is(err, economy.ErrInsufficientItemQuantity), errors.Is(err, ErrItemNotFound):
		return nil, ErrItemNotFound
	case errors.Is(err, economy.ErrCharacterNotFound):
		return nil, ErrCharacterNotFound
	default:
		return nil, err
	}
}

// requireHeld rechecks, under the transaction's locks, that a kept item is still held.
func (s *Service) requireHeld(tc *economy.TxContext, source, instanceID string) error {
	if source == "inventory" {
		if _, ok := tc.Inventory.Find(instanceID); !ok {
			return ErrItemNotFound
		}
		return nil
	}
	dp, err := s.depotMgr.FindByCharacterID(tc.Context, tc.Character.ID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(dp.Items, func(it item.Instance) bool { return it.ID == instanceID }) {
		return ErrItemNotFound
	}
	return nil
}

// committedHomeItemResult reports a committed callback; a kept item reports
// no_effect even though the callback's mutations were saved.
func committedHomeItemResult(def item.Definition, msg string, kept bool, char *corecharacter.Character) *UseHomeItemResult {
	action := "consumed"
	if kept {
		action = "no_effect"
	}
	return &UseHomeItemResult{
		Action:    action,
		Message:   msg,
		ItemName:  def.Name,
		Kind:      3,
		Consumed:  !kept,
		Character: char,
	}
}

// homeEffect is one Home callback, run inside the item transaction.
type homeEffect func(s *Service, ctx context.Context, char *corecharacter.Character, def item.Definition) (string, error)

// homeEffectFor is the only list of items Home dispatches; anything else is
// rejected before a transaction starts.
func homeEffectFor(name string) (homeEffect, bool) {
	if isRecipeScrollItem(name) {
		return func(s *Service, ctx context.Context, c *corecharacter.Character, d item.Definition) (string, error) {
			return s.applyRecipeScrollConsumable(ctx, c, d.Name)
		}, true
	}
	if bag, ok := rewardBags[name]; ok {
		return func(s *Service, ctx context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
			return s.applyRewardBag(ctx, c, bag)
		}, true
	}
	if isCostumeItem(name) {
		return func(s *Service, ctx context.Context, c *corecharacter.Character, d item.Definition) (string, error) {
			return s.applyCostumeConsumable(ctx, c, d.Name)
		}, true
	}
	if orb, ok := orbItems[name]; ok {
		return func(_ *Service, _ context.Context, c *corecharacter.Character, d item.Definition) (string, error) {
			added, err := c.AddOrb(orb)
			if err != nil {
				return "", err
			}
			if !added {
				return "すでにささげられています", errNoEffect
			}
			return d.Name + "を復活の祭壇にささげた！", nil
		}, true
	}
	e, ok := homeEffects[name]
	return e, ok
}

// statGain is a seed: base+U[0,spread) to one stat, drawn even when over-level
// nullifies it.
func statGain(spread, base int, label string, stat func(*corecharacter.Stats) *int) homeEffect {
	return func(s *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
		v := s.randomInt(spread) + base
		if c.OverLevel {
			v = 0
		}
		*stat(&c.Stats) += v
		c.Stats.Clamp(c.OverLevel, c.Level)
		return fmt.Sprintf("%sの%sが %d あがった！", c.Name, label, v), nil
	}
}

// spGain credits SP, ignoring over-level.
func spGain(amount func(*Service) int) homeEffect {
	return func(s *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
		v := amount(s)
		if err := c.AddSP(v); err != nil {
			return "", err
		}
		return fmt.Sprintf("%sのSPが %d あがった！", c.Name, v), nil
	}
}

func resetTired(_ *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
	c.ResetTired()
	return fmt.Sprintf("元気全快！%sの疲労が回復した！", c.Name), nil
}

func godReins(visit, ride string) homeEffect {
	return func(_ *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
		if c.GodVisit != "" {
			return noEffectMessage, errNoEffect // already admitted; keep the reins
		}
		c.GodVisit = visit
		return fmt.Sprintf("%sは、%s", c.Name, ride), nil
	}
}

func noStatChange(*Service, context.Context, *corecharacter.Character, item.Definition) (string, error) {
	return "", nil
}

var homeEffects = map[string]homeEffect{
	"命の木の実":        statGain(4, 3, "HP", func(st *corecharacter.Stats) *int { return &st.MaxHP }),
	"不思議な木の実":      statGain(4, 3, "MP", func(st *corecharacter.Stats) *int { return &st.MaxMP }),
	"力の種":          statGain(6, 1, "攻撃力", func(st *corecharacter.Stats) *int { return &st.Attack }),
	"守りの種":         statGain(6, 1, "守備力", func(st *corecharacter.Stats) *int { return &st.Defense }),
	"素早さの種":        statGain(6, 1, "素早さ", func(st *corecharacter.Stats) *int { return &st.Agility }),
	"スキルの種":        spGain(func(s *Service) int { return s.randomInt(3) + 1 }),
	sacredManaName: spGain(func(*Service) int { return 200 }),
	"幸せの種": func(_ *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
		if err := progression.ApplyHappySeed(c); err != nil {
			return "", err
		}
		return "次のクエスト時にレベルアップ！", nil
	},
	"ファイト一発":            resetTired,
	"気合の霊薬":             resetTired,
	heavenReinsName:     godReins(corecharacter.GodVisitHeaven, "天馬に乗り天界へと導かれた！"),
	underworldReinsName: godReins(corecharacter.GodVisitUnderworld, "黒い天馬に乗り天界?へと導かれた！"),
	lifeBraceletName:    noStatChange,
	sorceryRingName:     noStatChange,
	"小さなメダル": func(_ *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
		if err := c.AddSmallMedals(1); err != nil {
			return "", err
		}
		return "メダル伯爵にメダルを１枚献上しました", nil
	},
	"水晶の原石": func(_ *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
		if c.WeaponSeal <= 0 {
			return noEffectMessage, errNoEffect // no seal to remove; keep the crystal
		}
		refund := sealRefundCrystals(c.WeaponSeal)
		c.WeaponSeal = 0
		if err := c.AddCrystal(refund); err != nil {
			return "", err
		}
		return fmt.Sprintf("武器に施された刻印が剥がれ落ちた！刻印晶を %d 個回収した！", refund), nil
	},
	"未来のカケラ": func(s *Service, ctx context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
		return s.applyFutureMemoryConsumable(ctx, *c)
	},
	veteranRewardName: func(s *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
		return s.applyVeteranReward(c)
	},
	personalityBookName: func(s *Service, ctx context.Context, c *corecharacter.Character, _ item.Definition) (string, error) {
		return s.applyPersonalityBook(ctx, c)
	},
}

package home

import (
	"context"
	"errors"
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
		return &UseHomeItemResult{
			Action:     "inspect",
			ItemName:   def.Name,
			Kind:       1,
			Inspection: &HomeItemInspection{Power: power, Weight: weight, Price: def.Price},
			Character:  &char,
		}, nil
	}

	// Inspection of armors/accessories
	if kind == 2 {
		defense, weight := def.Power, def.Weight
		if defense == 0 && weight == 0 {
			defense, weight = item.CalculateArmorStats(def.ID, char, s.randomSource())
		}
		return &UseHomeItemResult{
			Action:     "inspect",
			ItemName:   def.Name,
			Kind:       2,
			Inspection: &HomeItemInspection{Power: defense, Weight: weight, Price: def.Price},
			Character:  &char,
		}, nil
	}

	// Consumable item logic. Legacy words only the held combat-only refusal
	// differently; a Depot one gets the generic refusal (lib/home.cgi:283–293,334–335).
	effect, ok := homeEffectFor(def.Name)
	if def.UsageCategory == item.UsageCategoryCombatOnly || !ok || (def.UsageCategory != item.UsageCategoryNone && !def.UsageCategory.IsUsableAtHome()) {
		return nil, ItemRefusal{ItemName: def.Name, CombatOnly: def.UsageCategory == item.UsageCategoryCombatOnly && source == "inventory"}
	}
	if s.runner == nil {
		return nil, ErrTransactionRunnerRequired
	}

	var outcome HomeItemOutcome
	var kept bool
	deferred := source == "depot" && def.UsageCategory == item.UsageCategoryDepotAfterAction
	// The inventory is locked for both sources: a Depot callback may take a held item.
	req := economy.TransactionRequest{CharacterID: characterID, LockInventory: true}
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
		var err error
		outcome, err = effect(s, tc, def) // errNoEffect carries its outcome
		// D4: legacy never checks mes after a Depot category-4 callback, so the
		// item is consumed even when the callback had no effect.
		if errors.Is(err, errNoEffect) && deferred {
			err = nil
		}
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
		return committedHomeItemResult(def, outcome, kept, deferred, &txRes.Character), nil
	case errors.Is(err, errNoEffect):
		// Rolled back: nothing consumed or changed.
		return &UseHomeItemResult{Action: "no_effect", ItemName: def.Name, Kind: 3, Outcome: &outcome, Character: &char}, nil
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
func committedHomeItemResult(def item.Definition, outcome HomeItemOutcome, kept, deferred bool, char *corecharacter.Character) *UseHomeItemResult {
	action := "consumed"
	if kept {
		action = "no_effect"
	}
	return &UseHomeItemResult{
		Action:    action,
		ItemName:  def.Name,
		Kind:      3,
		Consumed:  !kept,
		Deferred:  deferred,
		Outcome:   &outcome,
		Character: char,
	}
}

// homeEffect is one Home callback, run inside the item transaction. It mutates
// tc.Character and, for held items, tc.Inventory.
type homeEffect func(s *Service, tc *economy.TxContext, def item.Definition) (HomeItemOutcome, error)

// charEffect adapts a callback that only touches the character.
func charEffect(f func(s *Service, ctx context.Context, c *corecharacter.Character, def item.Definition) (HomeItemOutcome, error)) homeEffect {
	return func(s *Service, tc *economy.TxContext, def item.Definition) (HomeItemOutcome, error) {
		return f(s, tc.Context, &tc.Character, def)
	}
}

// homeEffectFor is the only list of items Home dispatches; anything else is
// rejected before a transaction starts.
func homeEffectFor(name string) (homeEffect, bool) {
	if isRecipeScrollItem(name) {
		return charEffect(func(s *Service, ctx context.Context, c *corecharacter.Character, d item.Definition) (HomeItemOutcome, error) {
			return s.applyRecipeScrollConsumable(ctx, c, d.Name)
		}), true
	}
	if bag, ok := rewardBags[name]; ok {
		return charEffect(func(s *Service, ctx context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
			return s.applyRewardBag(ctx, c, bag)
		}), true
	}
	if isCostumeItem(name) {
		return charEffect(func(s *Service, ctx context.Context, c *corecharacter.Character, d item.Definition) (HomeItemOutcome, error) {
			return s.applyCostumeConsumable(ctx, c, d.Name)
		}), true
	}
	if orb, ok := orbItems[name]; ok {
		return charEffect(func(_ *Service, _ context.Context, c *corecharacter.Character, d item.Definition) (HomeItemOutcome, error) {
			added, err := c.AddOrb(orb)
			if err != nil {
				return HomeItemOutcome{}, err
			}
			if !added {
				return HomeItemOutcome{Effect: HomeItemEffectOrbAlreadyOffered}, errNoEffect
			}
			return HomeItemOutcome{Effect: HomeItemEffectOrbOffered}, nil
		}), true
	}
	e, ok := homeEffects[name]
	return e, ok
}

// statGain is a seed: base+U[0,spread) to one stat, drawn even when over-level
// nullifies it.
func statGain(spread, base int, key string, stat func(*corecharacter.Stats) *int) homeEffect {
	return charEffect(func(s *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
		v := s.randomInt(spread) + base
		if c.OverLevel {
			v = 0
		}
		*stat(&c.Stats) += v
		c.Stats.Clamp(c.OverLevel, c.Level)
		return HomeItemOutcome{Effect: HomeItemEffectStatGain, Stat: key, Amount: v}, nil
	})
}

// spGain credits SP, ignoring over-level.
func spGain(amount func(*Service) int) homeEffect {
	return charEffect(func(s *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
		v := amount(s)
		if err := c.AddSP(v); err != nil {
			return HomeItemOutcome{}, err
		}
		return HomeItemOutcome{Effect: HomeItemEffectStatGain, Stat: HomeStatSP, Amount: v}, nil
	})
}

var resetTired = charEffect(func(_ *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
	c.ResetTired()
	return HomeItemOutcome{Effect: HomeItemEffectTiredReset}, nil
})

func godReins(visit string) homeEffect {
	return charEffect(func(_ *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
		if c.GodVisit != "" {
			return noEffect, errNoEffect // already admitted; keep the reins
		}
		c.GodVisit = visit
		return HomeItemOutcome{Effect: HomeItemEffectGodVisit, GodVisit: visit}, nil
	})
}

func noStatChange(*Service, *economy.TxContext, item.Definition) (HomeItemOutcome, error) {
	return HomeItemOutcome{Effect: HomeItemEffectNone}, nil
}

var homeEffects = map[string]homeEffect{
	"命の木の実":        statGain(4, 3, HomeStatMaxHP, func(st *corecharacter.Stats) *int { return &st.MaxHP }),
	"不思議な木の実":      statGain(4, 3, HomeStatMaxMP, func(st *corecharacter.Stats) *int { return &st.MaxMP }),
	"力の種":          statGain(6, 1, HomeStatAttack, func(st *corecharacter.Stats) *int { return &st.Attack }),
	"守りの種":         statGain(6, 1, HomeStatDefense, func(st *corecharacter.Stats) *int { return &st.Defense }),
	"素早さの種":        statGain(6, 1, HomeStatAgility, func(st *corecharacter.Stats) *int { return &st.Agility }),
	"スキルの種":        spGain(func(s *Service) int { return s.randomInt(3) + 1 }),
	sacredManaName: spGain(func(*Service) int { return 200 }),
	"幸せの種": charEffect(func(_ *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
		if err := progression.ApplyHappySeed(c); err != nil {
			return HomeItemOutcome{}, err
		}
		return HomeItemOutcome{Effect: HomeItemEffectLevelUpReserved}, nil
	}),
	"ファイト一発":            resetTired,
	"気合の霊薬":             resetTired,
	heavenReinsName:     godReins(corecharacter.GodVisitHeaven),
	underworldReinsName: godReins(corecharacter.GodVisitUnderworld),
	lifeBraceletName:    noStatChange,
	sorceryRingName:     noStatChange,
	sealedKeyName:       (*Service).openSealedBox,
	"小さなメダル": charEffect(func(_ *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
		if err := c.AddSmallMedals(1); err != nil {
			return HomeItemOutcome{}, err
		}
		return HomeItemOutcome{Effect: HomeItemEffectSmallMedal}, nil
	}),
	"水晶の原石": charEffect(func(_ *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
		if c.WeaponSeal <= 0 {
			return noEffect, errNoEffect // no seal to remove; keep the crystal
		}
		refund := sealRefundCrystals(c.WeaponSeal)
		c.WeaponSeal = 0
		if err := c.AddCrystal(refund); err != nil {
			return HomeItemOutcome{}, err
		}
		return HomeItemOutcome{Effect: HomeItemEffectSealRemoved, Amount: refund}, nil
	}),
	futureMemoryName: charEffect(func(s *Service, ctx context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
		return s.applyFutureMemoryConsumable(ctx, *c)
	}),
	veteranRewardName: charEffect(func(s *Service, _ context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
		return s.applyVeteranReward(c)
	}),
	personalityBookName: charEffect(func(s *Service, ctx context.Context, c *corecharacter.Character, _ item.Definition) (HomeItemOutcome, error) {
		return s.applyPersonalityBook(ctx, c)
	}),
}

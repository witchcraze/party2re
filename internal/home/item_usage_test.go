package home

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

type mockInventoryManager struct {
	invs map[string]coreinventory.Inventory
}

func (m *mockInventoryManager) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	inv, ok := m.invs[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

type mockDepotManager struct {
	depots map[string]depot.Depot
}

func (m *mockDepotManager) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	dp, ok := m.depots[characterID]
	if !ok {
		return depot.NewDepot(characterID)
	}
	return dp, nil
}

func (m *mockDepotManager) Consume(ctx context.Context, characterID, itemInstanceID string, quantity int) error {
	dp, ok := m.depots[characterID]
	if !ok {
		return depot.ErrNotFound
	}
	if _, err := dp.Consume(itemInstanceID, quantity); err != nil {
		return err
	}
	m.depots[characterID] = dp
	return nil
}

func (m *mockDepotManager) AppendDeliveries(ctx context.Context, characterID string, items []coreitem.Instance) error {
	dp, err := m.FindByCharacterID(ctx, characterID)
	if err != nil {
		return err
	}
	for _, inst := range items {
		dp.AppendDelivery(inst)
	}
	m.depots[characterID] = dp
	return nil
}

func (m *mockDepotManager) ConsumeOne(ctx context.Context, characterID, itemInstanceID string) error {
	return m.Consume(ctx, characterID, itemInstanceID, 1)
}

type mockCatalog struct {
	defs map[string]coreitem.Definition
}

func (m *mockCatalog) FindByID(id string) (coreitem.Definition, error) {
	d, ok := m.defs[id]
	if !ok {
		return coreitem.Definition{}, coreitem.ErrDefinitionNotFound
	}
	return d, nil
}

func TestItemUsage(t *testing.T) {
	ctx := context.Background()

	chars := map[string]corecharacter.Character{
		"char-1": {
			ID:         "char-1",
			PlayerID:   "p1",
			Name:       "Hero",
			Level:      5,
			Experience: 100,
			Tired:      50,
			Stats: corecharacter.Stats{
				MaxHP:   100,
				HP:      50,
				MaxMP:   50,
				MP:      20,
				Attack:  20,
				Defense: 15,
				Agility: 10,
			},
			SP:          5,
			SmallMedals: 0,
		},
		"char-over": {
			ID:         "char-over",
			PlayerID:   "p2",
			Name:       "OverHero",
			Level:      100,
			Experience: 100000,
			Tired:      0,
			OverLevel:  true,
			Stats: corecharacter.Stats{
				MaxHP:   500,
				HP:      500,
				MaxMP:   300,
				MP:      300,
				Attack:  200,
				Defense: 150,
				Agility: 120,
			},
			SP:          10,
			SmallMedals: 0,
		},
	}
	charReader := &mockCharReader{chars: chars}
	charUpdater := &mockCharUpdater{chars: chars}
	repo := newMockHomeRepo(chars)

	invMgr := &mockInventoryManager{invs: make(map[string]coreinventory.Inventory)}
	depotMgr := &mockDepotManager{depots: make(map[string]depot.Depot)}
	catalog := &mockCatalog{
		defs: map[string]coreitem.Definition{
			"weapon-08":   {ID: "weapon-08", Name: "銅の剣", Price: 100, Slot: coreitem.SlotMainHand, Power: 14, Weight: 9},
			"item-arm":    {ID: "item-arm", Name: "革の鎧", Price: 150, Slot: coreitem.SlotBody, Power: 12, Weight: 4},
			"item-seed1":  {ID: "item-seed1", Name: "命の木の実", Price: 50, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryAnytime},
			"item-seed2":  {ID: "item-seed2", Name: "不思議な木の実", Price: 50, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryAnytime},
			"item-seed3":  {ID: "item-seed3", Name: "力の種", Price: 50, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryAnytime},
			"item-seed4":  {ID: "item-seed4", Name: "守りの種", Price: 50, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryAnytime},
			"item-seed5":  {ID: "item-seed5", Name: "素早さの種", Price: 50, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryAnytime},
			"item-seed6":  {ID: "item-seed6", Name: "スキルの種", Price: 50, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryAnytime},
			"item-happy":  {ID: "item-happy", Name: "幸せの種", Price: 3000, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryAnytime},
			"item-fight":  {ID: "item-fight", Name: "ファイト一発", Price: 3000, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryAnytime},
			"item-fight2": {ID: "item-fight2", Name: "気合の霊薬", Price: 3000, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryAnytime},
			"item-medal":  {ID: "item-medal", Name: "小さなメダル", Price: 0, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryAnytime},
			"item-herb":   {ID: "item-herb", Name: "薬草", Price: 8, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryCombatOnly},
			"item-potion": {ID: "item-potion", Name: "特薬草", Price: 250, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryCombatOnly},
			"item-drop":   {ID: "item-drop", Name: "世界樹のしずく", Price: 5000, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryCombatOnly},
			"item-water":  {ID: "item-water", Name: "魔法の聖水", Price: 500, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryCombatOnly},
			"item-other":  {ID: "item-other", Name: "爆弾岩の破片", Price: 200, Slot: coreitem.SlotNone, UsageCategory: coreitem.UsageCategoryNone},
		},
	}

	weapon := coreitem.Instance{ID: "inst-wea", DefinitionID: "weapon-08", Quantity: 1}
	seedHand := func(t *testing.T, charID, instanceID string) {
		t.Helper()
		inv, _ := coreinventory.New(charID)
		if charID == "char-1" {
			if err := inv.Add(weapon); err != nil {
				t.Fatal(err)
			}
		}
		definitionID := "item-" + strings.TrimPrefix(strings.TrimPrefix(instanceID, "inst-"), "over-")
		if err := inv.Add(coreitem.Instance{ID: instanceID, DefinitionID: definitionID, Quantity: 1}); err != nil {
			t.Fatal(err)
		}
		invMgr.invs[charID] = inv
	}
	seedHand(t, "char-1", "inst-seed1")

	dp, _ := depot.NewDepot("char-1")
	dp.Items = append(dp.Items,
		coreitem.Instance{ID: "inst-arm", DefinitionID: "item-arm", Quantity: 1},
		coreitem.Instance{ID: "inst-seed2", DefinitionID: "item-seed2", Quantity: 1},
		coreitem.Instance{ID: "inst-medal", DefinitionID: "item-medal", Quantity: 1},
	)
	depotMgr.depots["char-1"] = dp

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithTransactionRunner(&mockTransactionRunner{chars: charReader.chars, invs: invMgr.invs}),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(catalog),
		WithNowFunc(func() time.Time { return time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC) }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	t.Run("list home items includes weight", func(t *testing.T) {
		items, err := svc.ListHomeItems(ctx, "char-1")
		if err != nil {
			t.Fatalf("ListHomeItems failed: %v", err)
		}
		if len(items) != 5 {
			t.Errorf("expected 5 items total, got %d", len(items))
		}
		for _, item := range items {
			if item.Kind == 1 || item.Kind == 2 {
				if item.Weight <= 0 {
					t.Errorf("expected positive weight for equipment %s, got %d", item.Name, item.Weight)
				}
			}
		}
	})

	t.Run("inspect weapon and armor includes weight", func(t *testing.T) {
		resWea, err := svc.UseHomeItem(ctx, "char-1", "inst-wea", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem weapon failed: %v", err)
		}
		if resWea.Action != "inspect" || resWea.Consumed {
			t.Errorf("expected inspect and not consumed, got %+v", resWea)
		}
		if resWea.Inspection == nil || resWea.Outcome != nil {
			t.Errorf("weapon inspect lost its inspection facts: %+v", resWea)
		}

		resArm, err := svc.UseHomeItem(ctx, "char-1", "inst-arm", "depot")
		if err != nil {
			t.Fatalf("UseHomeItem armor failed: %v", err)
		}
		if resArm.Action != "inspect" || resArm.Consumed {
			t.Errorf("expected inspect and not consumed, got %+v", resArm)
		}
		if resArm.Inspection == nil || resArm.Outcome != nil {
			t.Errorf("armor inspect lost its inspection facts: %+v", resArm)
		}
	})

	t.Run("reject combat-only recovery items", func(t *testing.T) {
		combatItemInstances := []string{"inst-herb", "inst-potion", "inst-drop", "inst-water"}
		for _, instID := range combatItemInstances {
			oldHP := chars["char-1"].Stats.HP
			oldMP := chars["char-1"].Stats.MP
			seedHand(t, "char-1", instID)
			res, err := svc.UseHomeItem(ctx, "char-1", instID, "inventory")
			if !errors.Is(err, ErrCannotUseHere) {
				t.Fatalf("expected ErrCannotUseHere for instance %s, got res=%+v, err=%v", instID, res, err)
			}
			if chars["char-1"].Stats.HP != oldHP || chars["char-1"].Stats.MP != oldMP {
				t.Errorf("HP or MP changed when using combat item %s", instID)
			}
		}
	})

	t.Run("consume seeds and medals", func(t *testing.T) {
		oldMaxHP := chars["char-1"].Stats.MaxHP
		oldHP := chars["char-1"].Stats.HP
		seedHand(t, "char-1", "inst-seed1")
		resSeed, err := svc.UseHomeItem(ctx, "char-1", "inst-seed1", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed failed: %v", err)
		}
		if resSeed.Action != "consumed" || !resSeed.Consumed {
			t.Errorf("expected consumed, got %+v", resSeed)
		}
		if chars["char-1"].Stats.MaxHP <= oldMaxHP {
			t.Errorf("expected MaxHP to increase from %d, got %d", oldMaxHP, chars["char-1"].Stats.MaxHP)
		}
		if chars["char-1"].Stats.HP != oldHP {
			t.Errorf("expected HP to remain unchanged at %d, got %d", oldHP, chars["char-1"].Stats.HP)
		}

		// Consume 不思議な木の実 from depot
		oldMaxMP := chars["char-1"].Stats.MaxMP
		oldMP := chars["char-1"].Stats.MP
		resMPSeed, err := svc.UseHomeItem(ctx, "char-1", "inst-seed2", "depot")
		if err != nil {
			t.Fatalf("UseHomeItem seed2 failed: %v", err)
		}
		if resMPSeed.Action != "consumed" || !resMPSeed.Consumed {
			t.Errorf("expected consumed, got %+v", resMPSeed)
		}
		if chars["char-1"].Stats.MaxMP <= oldMaxMP {
			t.Errorf("expected MaxMP to increase from %d, got %d", oldMaxMP, chars["char-1"].Stats.MaxMP)
		}
		if chars["char-1"].Stats.MP != oldMP {
			t.Errorf("expected MP to remain unchanged at %d, got %d", oldMP, chars["char-1"].Stats.MP)
		}

		// Consume small medal from depot
		resMedal, err := svc.UseHomeItem(ctx, "char-1", "inst-medal", "depot")
		if err != nil {
			t.Fatalf("UseHomeItem medal failed: %v", err)
		}
		if resMedal.Action != "consumed" {
			t.Errorf("expected consumed, got %+v", resMedal)
		}
		if chars["char-1"].SmallMedals != 1 {
			t.Errorf("expected 1 small medal, got %d", chars["char-1"].SmallMedals)
		}
	})

	t.Run("consume 幸せの種 sets exp to level * level * 10", func(t *testing.T) {
		// Hero is level 5, level * level * 10 = 250
		seedHand(t, "char-1", "inst-happy")
		res, err := svc.UseHomeItem(ctx, "char-1", "inst-happy", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem 幸せの種 failed: %v", err)
		}
		if res.Action != "consumed" || !res.Consumed {
			t.Errorf("expected consumed, got %+v", res)
		}
		if chars["char-1"].Experience != 250 {
			t.Errorf("expected 250 experience, got %d", chars["char-1"].Experience)
		}
		if got := outcomeOf(res); got.Effect != HomeItemEffectLevelUpReserved {
			t.Errorf("unexpected outcome: %+v", got)
		}
	})

	t.Run("consume ファイト一発 resets fatigue", func(t *testing.T) {
		if chars["char-1"].Tired == 0 {
			t.Fatal("expected non-zero fatigue before using fight item")
		}
		seedHand(t, "char-1", "inst-fight")
		res, err := svc.UseHomeItem(ctx, "char-1", "inst-fight", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem ファイト一発 failed: %v", err)
		}
		if res.Action != "consumed" || !res.Consumed {
			t.Errorf("expected consumed, got %+v", res)
		}
		if chars["char-1"].Tired != 0 {
			t.Errorf("expected fatigue 0, got %d", chars["char-1"].Tired)
		}
		if got := outcomeOf(res); got.Effect != HomeItemEffectTiredReset {
			t.Errorf("unexpected outcome: %+v", got)
		}

		// Also verify 気合の霊薬
		charHero := chars["char-1"]
		charHero.Tired = 75
		chars["char-1"] = charHero
		seedHand(t, "char-1", "inst-fight2")
		res2, err := svc.UseHomeItem(ctx, "char-1", "inst-fight2", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem 気合の霊薬 failed: %v", err)
		}
		if chars["char-1"].Tired != 0 {
			t.Errorf("expected fatigue 0 after 気合の霊薬, got %d", chars["char-1"].Tired)
		}
		if got := outcomeOf(res2); got.Effect != HomeItemEffectTiredReset {
			t.Errorf("unexpected outcome: %+v", got)
		}
	})

	t.Run("overlevel characters receive 0 stat increase from stat seeds", func(t *testing.T) {
		oldStats := chars["char-over"].Stats

		// 命の木の実
		seedHand(t, "char-over", "inst-over-seed1")
		resHP, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed1", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed1 failed: %v", err)
		}
		if chars["char-over"].Stats.MaxHP != oldStats.MaxHP {
			t.Errorf("expected MaxHP unchanged, got %d (was %d)", chars["char-over"].Stats.MaxHP, oldStats.MaxHP)
		}
		if got := outcomeOf(resHP); !reflect.DeepEqual(got, HomeItemOutcome{Effect: HomeItemEffectStatGain, Stat: HomeStatMaxHP}) {
			t.Errorf("expected zero HomeStatMaxHP gain, got %+v", got)
		}

		// 不思議な木の実
		seedHand(t, "char-over", "inst-over-seed2")
		resMP, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed2", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed2 failed: %v", err)
		}
		if chars["char-over"].Stats.MaxMP != oldStats.MaxMP {
			t.Errorf("expected MaxMP unchanged, got %d (was %d)", chars["char-over"].Stats.MaxMP, oldStats.MaxMP)
		}
		if got := outcomeOf(resMP); !reflect.DeepEqual(got, HomeItemOutcome{Effect: HomeItemEffectStatGain, Stat: HomeStatMaxMP}) {
			t.Errorf("expected zero HomeStatMaxMP gain, got %+v", got)
		}

		// 力の種
		seedHand(t, "char-over", "inst-over-seed3")
		resAtk, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed3", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed3 failed: %v", err)
		}
		if chars["char-over"].Stats.Attack != oldStats.Attack {
			t.Errorf("expected Attack unchanged, got %d (was %d)", chars["char-over"].Stats.Attack, oldStats.Attack)
		}
		if got := outcomeOf(resAtk); !reflect.DeepEqual(got, HomeItemOutcome{Effect: HomeItemEffectStatGain, Stat: HomeStatAttack}) {
			t.Errorf("expected zero HomeStatAttack gain, got %+v", got)
		}

		// 守りの種
		seedHand(t, "char-over", "inst-over-seed4")
		resDef, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed4", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed4 failed: %v", err)
		}
		if chars["char-over"].Stats.Defense != oldStats.Defense {
			t.Errorf("expected Defense unchanged, got %d (was %d)", chars["char-over"].Stats.Defense, oldStats.Defense)
		}
		if got := outcomeOf(resDef); !reflect.DeepEqual(got, HomeItemOutcome{Effect: HomeItemEffectStatGain, Stat: HomeStatDefense}) {
			t.Errorf("expected zero HomeStatDefense gain, got %+v", got)
		}

		// 素早さの種
		seedHand(t, "char-over", "inst-over-seed5")
		resAgi, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed5", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed5 failed: %v", err)
		}
		if chars["char-over"].Stats.Agility != oldStats.Agility {
			t.Errorf("expected Agility unchanged, got %d (was %d)", chars["char-over"].Stats.Agility, oldStats.Agility)
		}
		if got := outcomeOf(resAgi); !reflect.DeepEqual(got, HomeItemOutcome{Effect: HomeItemEffectStatGain, Stat: HomeStatAgility}) {
			t.Errorf("expected zero HomeStatAgility gain, got %+v", got)
		}

		// スキルの種 is NOT clamped by OverLevel
		oldSP := chars["char-over"].SP
		seedHand(t, "char-over", "inst-over-seed6")
		resSP, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed6", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed6 failed: %v", err)
		}
		if chars["char-over"].SP <= oldSP {
			t.Errorf("expected SP to increase from %d, got %d", oldSP, chars["char-over"].SP)
		}
		if got := outcomeOf(resSP); got.Stat != HomeStatSP || got.Amount == 0 {
			t.Errorf("expected nonzero SP gain outcome, got %+v", got)
		}
	})

	t.Run("cannot use combat-only items in home", func(t *testing.T) {
		combatInstances := []struct {
			instID string
			name   string
		}{
			{"inst-herb", "薬草"},
			{"inst-potion", "特薬草"},
			{"inst-drop", "世界樹のしずく"},
			{"inst-water", "魔法の聖水"},
		}

		for _, itemCase := range combatInstances {
			t.Run(itemCase.name, func(t *testing.T) {
				seedHand(t, "char-1", itemCase.instID)
				res, err := svc.UseHomeItem(ctx, "char-1", itemCase.instID, "inventory")
				if err != (ItemRefusal{ItemName: itemCase.name, CombatOnly: true}) || !errors.Is(err, ErrCannotUseHere) {
					t.Fatalf("expected held combat-only refusal, got res=%+v, err=%v", res, err)
				}
			})
		}
	})

	t.Run("Depot combat-only item gets the generic refusal", func(t *testing.T) {
		dp := depotMgr.depots["char-1"]
		dp.Items = append(dp.Items, coreitem.Instance{ID: "inst-depot-herb", DefinitionID: "item-herb", Quantity: 1})
		depotMgr.depots["char-1"] = dp
		res, err := svc.UseHomeItem(ctx, "char-1", "inst-depot-herb", "depot")
		if err != (ItemRefusal{ItemName: "薬草"}) {
			t.Fatalf("expected generic refusal, got res=%+v, err=%v", res, err)
		}
	})

	t.Run("cannot use non-usable item in home", func(t *testing.T) {
		seedHand(t, "char-1", "inst-other")
		res, err := svc.UseHomeItem(ctx, "char-1", "inst-other", "inventory")
		if err != (ItemRefusal{ItemName: "爆弾岩の破片"}) {
			t.Fatalf("expected generic refusal, got res=%+v, err=%v", res, err)
		}
	})
}

func TestUseHomeItem_AuthenticCatalogMatrix(t *testing.T) {
	ctx := context.Background()
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("InitialCatalog failed: %v", err)
	}

	char := corecharacter.Character{
		ID:    "char-matrix",
		Name:  "Tester",
		Level: 10,
	}
	chars := map[string]corecharacter.Character{char.ID: char}
	charReader := &mockCharReader{chars: chars}
	charUpdater := &mockCharUpdater{chars: chars}
	repo := newMockHomeRepo(chars)

	// Verify all combat-only definitions are rejected with authentic message
	consumableCombatCount := 0
	equipmentCombatCount := 0
	for _, def := range cat.Definitions() {
		def := def
		if def.UsageCategory == coreitem.UsageCategoryCombatOnly {
			if def.Slot == coreitem.SlotNone {
				consumableCombatCount++
				t.Run("consumable_combat_only_"+def.ID, func(t *testing.T) {
					inv, err := coreinventory.New(char.ID)
					if err != nil {
						t.Fatalf("New inventory failed: %v", err)
					}
					_ = inv.Add(coreitem.Instance{ID: "inst-" + def.ID, DefinitionID: def.ID, Quantity: 1})
					invMgr := &mockInventoryManager{invs: map[string]coreinventory.Inventory{char.ID: inv}}
					depotMgr := &mockDepotManager{depots: make(map[string]depot.Depot)}

					svc, err := NewService(
						repo,
						charReader,
						WithCharacterUpdater(charUpdater),
						WithTransactionRunner(&mockTransactionRunner{chars: charReader.chars, invs: invMgr.invs}),
						WithInventoryManager(invMgr),
						WithDepotManager(depotMgr),
						WithItemCatalog(cat),
					)
					if err != nil {
						t.Fatalf("NewService failed: %v", err)
					}

					res, err := svc.UseHomeItem(ctx, char.ID, "inst-"+def.ID, "inventory")
					if err != (ItemRefusal{ItemName: def.Name, CombatOnly: true}) {
						t.Fatalf("[%s] expected held combat-only refusal, got res=%+v, err=%v", def.ID, res, err)
					}
					// Verify item was not consumed
					currentInv, _ := invMgr.FindByCharacterID(ctx, char.ID)
					if inst, found := currentInv.Find("inst-" + def.ID); !found || inst.Quantity != 1 {
						t.Errorf("[%s] item should not have been consumed from inventory: found=%v, quantity=%d", def.ID, found, inst.Quantity)
					}
				})
			} else {
				equipmentCombatCount++
				t.Run("equipment_combat_only_"+def.ID, func(t *testing.T) {
					inv, err := coreinventory.New(char.ID)
					if err != nil {
						t.Fatalf("New inventory failed: %v", err)
					}
					_ = inv.Add(coreitem.Instance{ID: "inst-" + def.ID, DefinitionID: def.ID, Quantity: 1})
					invMgr := &mockInventoryManager{invs: map[string]coreinventory.Inventory{char.ID: inv}}
					depotMgr := &mockDepotManager{depots: make(map[string]depot.Depot)}

					svc, err := NewService(
						repo,
						charReader,
						WithCharacterUpdater(charUpdater),
						WithTransactionRunner(&mockTransactionRunner{chars: charReader.chars, invs: invMgr.invs}),
						WithInventoryManager(invMgr),
						WithDepotManager(depotMgr),
						WithItemCatalog(cat),
					)
					if err != nil {
						t.Fatalf("NewService failed: %v", err)
					}

					res, err := svc.UseHomeItem(ctx, char.ID, "inst-"+def.ID, "inventory")
					if err != nil {
						t.Fatalf("[%s] unexpected error inspecting equipment: %v", def.ID, err)
					}
					if res.Action != "inspect" || res.Consumed {
						t.Errorf("[%s] expected inspect action without consumption, got %+v", def.ID, res)
					}
				})
			}
		}
	}

	if consumableCombatCount != 50 {
		t.Fatalf("expected exactly 50 combat-only consumable items in authentic catalog, got %d", consumableCombatCount)
	}
	if equipmentCombatCount != 3 {
		t.Fatalf("expected exactly 3 combat-only equipment items in authentic catalog, got %d", equipmentCombatCount)
	}
}

type mockTransactionRunner struct {
	chars          map[string]corecharacter.Character
	invs           map[string]coreinventory.Inventory
	failOnCallback bool
	failOnCommit   bool
	calls          int
	lastReq        economy.TransactionRequest
}

func (m *mockTransactionRunner) ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
	m.calls++
	m.lastReq = req
	char, ok := m.chars[req.CharacterID]
	if !ok {
		return nil, economy.ErrCharacterNotFound
	}
	charCopy := char
	if req.CostFunc != nil {
		cost, err := req.CostFunc(charCopy)
		if err != nil {
			return nil, err
		}
		req.Cost = cost
	}

	if req.Cost.Gold > 0 {
		if charCopy.Money < req.Cost.Gold {
			return nil, economy.ErrInsufficientGold
		}
		if err := charCopy.DeductMoney(req.Cost.Gold); err != nil {
			return nil, err
		}
	}

	var inv coreinventory.Inventory
	var invCopy coreinventory.Inventory
	if req.LockInventory {
		var found bool
		inv, found = m.invs[req.CharacterID]
		if !found {
			var err error
			inv, err = coreinventory.New(req.CharacterID)
			if err != nil {
				return nil, err
			}
		}
		invCopy = inv
		if req.Cost.ItemInstanceID != "" && req.Cost.ItemInstanceQty > 0 {
			inst, found := invCopy.Find(req.Cost.ItemInstanceID)
			if !found {
				return nil, economy.ErrItemNotFound
			}
			if inst.Quantity < req.Cost.ItemInstanceQty {
				return nil, economy.ErrInsufficientItemQuantity
			}
			if err := invCopy.Consume(req.Cost.ItemInstanceID, req.Cost.ItemInstanceQty); err != nil {
				return nil, err
			}
		}
	}

	tc := &economy.TxContext{
		Context:   ctx,
		Character: charCopy,
		Inventory: invCopy,
	}

	if m.failOnCallback {
		return nil, errors.New("simulated callback failure")
	}

	if err := fn(tc); err != nil {
		return nil, err
	}

	if m.failOnCommit {
		return nil, errors.New("simulated commit failure")
	}

	m.chars[req.CharacterID] = tc.Character
	if req.LockInventory {
		if m.invs == nil {
			m.invs = map[string]coreinventory.Inventory{}
		}
		m.invs[req.CharacterID] = tc.Inventory
	}
	return &economy.TransactionResult{
		Character: tc.Character,
		Inventory: tc.Inventory,
	}, nil
}

type transactionalMockDepotManager struct {
	depots map[string]depot.Depot
}

func (m *transactionalMockDepotManager) FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error) {
	dp, ok := m.depots[characterID]
	if !ok {
		return depot.NewDepot(characterID)
	}
	return dp, nil
}

func (m *transactionalMockDepotManager) Consume(ctx context.Context, characterID, itemInstanceID string, quantity int) error {
	dp, ok := m.depots[characterID]
	if !ok {
		return depot.ErrNotFound
	}
	if _, err := dp.Consume(itemInstanceID, quantity); err != nil {
		return err
	}
	m.depots[characterID] = dp
	return nil
}

func (m *transactionalMockDepotManager) AppendDeliveries(ctx context.Context, characterID string, items []coreitem.Instance) error {
	dp, err := m.FindByCharacterID(ctx, characterID)
	if err != nil {
		return err
	}
	for _, inst := range items {
		dp.AppendDelivery(inst)
	}
	m.depots[characterID] = dp
	return nil
}

func (m *transactionalMockDepotManager) ConsumeOne(ctx context.Context, characterID, itemInstanceID string) error {
	return m.Consume(ctx, characterID, itemInstanceID, 1)
}

func TestUseHomeItem_TransactionalSuccess(t *testing.T) {
	ctx := context.Background()
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("InitialCatalog failed: %v", err)
	}

	charID := "char-tx-success"
	char := corecharacter.Character{
		ID:    charID,
		Name:  "TxHero",
		Level: 10,
		Stats: corecharacter.Stats{
			Attack:  20,
			Defense: 15,
		},
	}
	chars := map[string]corecharacter.Character{charID: char}

	inv, _ := coreinventory.New(charID)
	_ = inv.Add(coreitem.Instance{ID: "inst-atk-seed", DefinitionID: "item-018", Quantity: 1})
	invs := map[string]coreinventory.Inventory{charID: inv}

	dp, _ := depot.NewDepot(charID)
	dp.Items = append(dp.Items, coreitem.Instance{ID: "inst-def-seed", DefinitionID: "item-019", Quantity: 1})
	depots := map[string]depot.Depot{charID: dp}

	runner := &mockTransactionRunner{chars: chars, invs: invs}
	invMgr := &mockInventoryManager{invs: invs}
	depotMgr := &transactionalMockDepotManager{depots: depots}
	charReader := &mockCharReader{chars: chars}
	repo := newMockHomeRepo(chars)

	svc, err := NewService(
		repo,
		charReader,
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(cat),
		WithTransactionRunner(runner),
		WithRNG(random.NewDeterministic(1)),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Consume from inventory
	resInv, err := svc.UseHomeItem(ctx, charID, "inst-atk-seed", "inventory")
	if err != nil {
		t.Fatalf("UseHomeItem inventory failed: %v", err)
	}
	if !resInv.Consumed || resInv.Action != "consumed" {
		t.Fatalf("expected consumed action, got %+v", resInv)
	}
	if runner.chars[charID].Stats.Attack <= 20 {
		t.Errorf("expected attack to increase from 20, got %d", runner.chars[charID].Stats.Attack)
	}
	invAfter := runner.invs[charID]
	if _, found := invAfter.Find("inst-atk-seed"); found {
		t.Errorf("expected inst-atk-seed to be consumed from inventory")
	}
	if !runner.lastReq.LockInventory {
		t.Errorf("expected LockInventory=true for inventory item usage")
	}

	// 2. Consume from depot
	resDepot, err := svc.UseHomeItem(ctx, charID, "inst-def-seed", "depot")
	if err != nil {
		t.Fatalf("UseHomeItem depot failed: %v", err)
	}
	if !resDepot.Consumed || resDepot.Action != "consumed" {
		t.Fatalf("expected consumed action, got %+v", resDepot)
	}
	if runner.chars[charID].Stats.Defense <= 15 {
		t.Errorf("expected defense to increase from 15, got %d", runner.chars[charID].Stats.Defense)
	}
	if len(depotMgr.depots[charID].Items) != 0 {
		t.Errorf("expected depot item to be consumed, got %d items", len(depotMgr.depots[charID].Items))
	}
}

func TestUseHomeItem_TransactionalRollback(t *testing.T) {
	ctx := context.Background()
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("InitialCatalog failed: %v", err)
	}

	t.Run("inventory commit failure rolls back item and stats", func(t *testing.T) {
		charID := "char-rollback-inv"
		char := corecharacter.Character{
			ID:    charID,
			Name:  "RollbackHero",
			Level: 10,
			Stats: corecharacter.Stats{
				Attack: 20,
			},
		}
		chars := map[string]corecharacter.Character{charID: char}

		inv, _ := coreinventory.New(charID)
		_ = inv.Add(coreitem.Instance{ID: "inst-seed3", DefinitionID: "item-018", Quantity: 1})
		invs := map[string]coreinventory.Inventory{charID: inv}

		runner := &mockTransactionRunner{
			chars:        chars,
			invs:         invs,
			failOnCommit: true,
		}
		invMgr := &mockInventoryManager{invs: invs}
		charReader := &mockCharReader{chars: chars}
		repo := newMockHomeRepo(chars)

		svc, err := NewService(
			repo,
			charReader,
			WithInventoryManager(invMgr),
			WithItemCatalog(cat),
			WithTransactionRunner(runner),
		)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		res, err := svc.UseHomeItem(ctx, charID, "inst-seed3", "inventory")
		if err == nil {
			t.Fatalf("expected error on commit failure, got nil result: %+v", res)
		}

		// Verify state was rolled back
		if runner.chars[charID].Stats.Attack != 20 {
			t.Errorf("expected attack to remain 20 after rollback, got %d", runner.chars[charID].Stats.Attack)
		}
		invAfter := runner.invs[charID]
		itemInst, found := invAfter.Find("inst-seed3")
		if !found || itemInst.Quantity != 1 {
			t.Errorf("expected inst-seed3 to remain in inventory with qty 1, found=%v, qty=%d", found, itemInst.Quantity)
		}
	})

	t.Run("depot callback failure rolls back depot item and stats", func(t *testing.T) {
		charID := "char-rollback-depot"
		char := corecharacter.Character{
			ID:    charID,
			Name:  "RollbackDepotHero",
			Level: 10,
			Stats: corecharacter.Stats{
				Defense: 15,
			},
		}
		chars := map[string]corecharacter.Character{charID: char}

		dp, _ := depot.NewDepot(charID)
		dp.Items = append(dp.Items, coreitem.Instance{ID: "inst-seed4", DefinitionID: "item-019", Quantity: 1})
		depots := map[string]depot.Depot{charID: dp}

		runner := &mockTransactionRunner{
			chars:          chars,
			failOnCallback: true,
		}
		depotMgr := &transactionalMockDepotManager{depots: depots}
		charReader := &mockCharReader{chars: chars}
		repo := newMockHomeRepo(chars)

		svc, err := NewService(
			repo,
			charReader,
			WithDepotManager(depotMgr),
			WithItemCatalog(cat),
			WithTransactionRunner(runner),
		)
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		res, err := svc.UseHomeItem(ctx, charID, "inst-seed4", "depot")
		if err == nil {
			t.Fatalf("expected error on callback failure, got nil result: %+v", res)
		}

		// Verify stats were not saved
		if runner.chars[charID].Stats.Defense != 15 {
			t.Errorf("expected defense to remain 15 after rollback, got %d", runner.chars[charID].Stats.Defense)
		}
	})
}

func TestUseHomeItem_Depot_StackedItem(t *testing.T) {
	ctx := context.Background()
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("InitialCatalog failed: %v", err)
	}

	charID := "char-stacked-depot"
	char := corecharacter.Character{
		ID:    charID,
		Name:  "StackedDepotHero",
		Level: 10,
		Stats: corecharacter.Stats{
			Defense: 10,
		},
	}
	chars := map[string]corecharacter.Character{charID: char}

	dp, err := depot.NewDepot(charID)
	if err != nil {
		t.Fatalf("NewDepot failed: %v", err)
	}
	// item-019 is 守りの種 (Defense Seed)
	dp.Items = append(dp.Items, coreitem.Instance{
		ID:           "inst-seed-stack",
		DefinitionID: "item-019",
		Quantity:     3,
	})
	depots := map[string]depot.Depot{charID: dp}

	runner := &mockTransactionRunner{chars: chars}
	depotMgr := &transactionalMockDepotManager{depots: depots}
	charReader := &mockCharReader{chars: chars}
	repo := newMockHomeRepo(chars)

	svc, err := NewService(
		repo,
		charReader,
		WithDepotManager(depotMgr),
		WithItemCatalog(cat),
		WithTransactionRunner(runner),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// First use: Quantity 3 -> 2
	res1, err := svc.UseHomeItem(ctx, charID, "inst-seed-stack", "depot")
	if err != nil {
		t.Fatalf("UseHomeItem 1 failed: %v", err)
	}
	if !res1.Consumed {
		t.Errorf("expected item to be consumed, got %+v", res1)
	}
	currentDepot, err := depotMgr.FindByCharacterID(ctx, charID)
	if err != nil {
		t.Fatalf("FindByCharacterID failed: %v", err)
	}
	if len(currentDepot.Items) != 1 {
		t.Fatalf("expected 1 item slot remaining in depot, got %d", len(currentDepot.Items))
	}
	if currentDepot.Items[0].Quantity != 2 {
		t.Errorf("expected quantity 2 remaining in depot, got %d", currentDepot.Items[0].Quantity)
	}

	// Second use: Quantity 2 -> 1
	res2, err := svc.UseHomeItem(ctx, charID, "inst-seed-stack", "depot")
	if err != nil {
		t.Fatalf("UseHomeItem 2 failed: %v", err)
	}
	if !res2.Consumed {
		t.Errorf("expected item to be consumed, got %+v", res2)
	}
	currentDepot, _ = depotMgr.FindByCharacterID(ctx, charID)
	if len(currentDepot.Items) != 1 || currentDepot.Items[0].Quantity != 1 {
		t.Fatalf("expected 1 item with quantity 1, got %+v", currentDepot.Items)
	}

	// Third use: Quantity 1 -> 0 (slot removed)
	res3, err := svc.UseHomeItem(ctx, charID, "inst-seed-stack", "depot")
	if err != nil {
		t.Fatalf("UseHomeItem 3 failed: %v", err)
	}
	if !res3.Consumed {
		t.Errorf("expected item to be consumed, got %+v", res3)
	}
	currentDepot, _ = depotMgr.FindByCharacterID(ctx, charID)
	if len(currentDepot.Items) != 0 {
		t.Errorf("expected depot to be empty after consuming final unit, got %+v", currentDepot.Items)
	}

	// Fourth use: should fail with ErrItemNotFound
	_, err = svc.UseHomeItem(ctx, charID, "inst-seed-stack", "depot")
	if !errors.Is(err, ErrItemNotFound) {
		t.Errorf("expected ErrItemNotFound, got %v", err)
	}
}

func TestUseHomeItem_CrystalOre(t *testing.T) {
	ctx := context.Background()

	chars := map[string]corecharacter.Character{
		"char-seal1": {
			ID:         "char-seal1",
			Name:       "SealedHero",
			WeaponSeal: 1, // Seal 1 (爪): 50 crystals cost -> 25 refund
			Crystal:    10,
		},
		"char-seal3": {
			ID:         "char-seal3",
			Name:       "MaxHero",
			WeaponSeal: 3, // Seal 3 (竜): 5000 crystals cost -> 2500 refund
			Crystal:    998000,
		},
		"char-noseal": {
			ID:         "char-noseal",
			Name:       "PlainHero",
			WeaponSeal: 0,
			Crystal:    50,
		},
	}
	charReader := &mockCharReader{chars: chars}
	charUpdater := &mockCharUpdater{chars: chars}
	invMgr := &mockInventoryManager{invs: make(map[string]coreinventory.Inventory)}
	catalog := &mockCatalog{
		defs: map[string]coreitem.Definition{
			"item-257": {
				ID:            "item-257",
				Name:          "水晶の原石",
				UsageCategory: coreitem.UsageCategoryAnytime,
			},
		},
	}

	for id := range chars {
		inv, _ := coreinventory.New(id)
		_ = inv.Add(coreitem.Instance{ID: "inst-" + id, DefinitionID: "item-257", Quantity: 1})
		invMgr.invs[id] = inv
	}

	repo := newMockHomeRepo(chars)
	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithTransactionRunner(&mockTransactionRunner{chars: charReader.chars, invs: invMgr.invs}),
		WithInventoryManager(invMgr),
		WithItemCatalog(catalog),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// Case 1: WeaponSeal == 1 -> refund 25, WeaponSeal reset to 0
	res1, err := svc.UseHomeItem(ctx, "char-seal1", "inst-char-seal1", "inventory")
	if err != nil {
		t.Fatalf("UseHomeItem for char-seal1 failed: %v", err)
	}
	if !res1.Consumed {
		t.Errorf("expected item to be consumed")
	}
	if got := outcomeOf(res1); !reflect.DeepEqual(got, HomeItemOutcome{Effect: HomeItemEffectSealRemoved, Amount: 25}) {
		t.Errorf("unexpected outcome: %+v", got)
	}
	if chars["char-seal1"].WeaponSeal != 0 {
		t.Errorf("expected WeaponSeal reset to 0, got %d", chars["char-seal1"].WeaponSeal)
	}
	if chars["char-seal1"].Crystal != 35 {
		t.Errorf("expected Crystal 35 (10 + 25), got %d", chars["char-seal1"].Crystal)
	}

	// Case 2: WeaponSeal == 3 -> refund 2500, clamped to 999999
	res2, err := svc.UseHomeItem(ctx, "char-seal3", "inst-char-seal3", "inventory")
	if err != nil {
		t.Fatalf("UseHomeItem for char-seal3 failed: %v", err)
	}
	if !res2.Consumed {
		t.Errorf("expected item to be consumed")
	}
	if chars["char-seal3"].WeaponSeal != 0 {
		t.Errorf("expected WeaponSeal reset to 0, got %d", chars["char-seal3"].WeaponSeal)
	}
	if chars["char-seal3"].Crystal != 999999 {
		t.Errorf("expected Crystal clamped to 999999, got %d", chars["char-seal3"].Crystal)
	}

	// Case 3: WeaponSeal == 0 -> nothing happened message, item NOT consumed
	res3, err := svc.UseHomeItem(ctx, "char-noseal", "inst-char-noseal", "inventory")
	if err != nil {
		t.Fatalf("UseHomeItem for char-noseal failed: %v", err)
	}
	if res3.Consumed {
		t.Errorf("expected item NOT to be consumed when no seal is engraved")
	}
	if res3.Action != "no_effect" || outcomeOf(res3).Effect != HomeItemEffectNoEffect {
		t.Errorf("expected no-effect outcome, got %+v", res3)
	}
	if chars["char-noseal"].Crystal != 50 {
		t.Errorf("expected Crystal unchanged at 50, got %d", chars["char-noseal"].Crystal)
	}
}

func TestItemUsage_AuthenticEquipmentAppraisal(t *testing.T) {
	ctx := context.Background()

	cat, err := coreitem.DefaultCatalog()
	if err != nil {
		t.Fatalf("DefaultCatalog failed: %v", err)
	}

	charID := "char-appraise"
	chars := map[string]corecharacter.Character{
		charID: {
			ID:       charID,
			PlayerID: "player-appraise",
			Name:     "鑑定士",
			Level:    10,
		},
	}
	charReader := &mockCharReader{chars: chars}
	charUpdater := &mockCharUpdater{chars: chars}
	repo := newMockHomeRepo(chars)

	invMgr := &mockInventoryManager{invs: make(map[string]coreinventory.Inventory)}
	depotMgr := &mockDepotManager{depots: make(map[string]depot.Depot)}

	dp, _ := depot.NewDepot(charID)
	// Add authentic armors from party2/lib/_data.cgi:506-613
	dp.Items = append(dp.Items,
		coreitem.Instance{ID: "inst-cloth", DefinitionID: "armor-01", Quantity: 1},
		coreitem.Instance{ID: "inst-leather", DefinitionID: "armor-04", Quantity: 1},
		coreitem.Instance{ID: "inst-chain", DefinitionID: "armor-07", Quantity: 1},
		coreitem.Instance{ID: "inst-iron-a", DefinitionID: "armor-11", Quantity: 1},
		coreitem.Instance{ID: "inst-steel-a", DefinitionID: "armor-16", Quantity: 1},
		coreitem.Instance{ID: "inst-hagure-a", DefinitionID: "armor-40", Quantity: 1},
	)
	depotMgr.depots[charID] = dp

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
		WithTransactionRunner(&mockTransactionRunner{chars: charReader.chars, invs: invMgr.invs}),
		WithInventoryManager(invMgr),
		WithDepotManager(depotMgr),
		WithItemCatalog(cat),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	weaponTests := []struct {
		instID       string
		definitionID string
		name         string
		want         HomeItemInspection
	}{
		{
			instID:       "inst-bamboo",
			definitionID: "weapon-02",
			name:         "竹の槍", want: HomeItemInspection{Power: 4, Weight: 2, Price: 30},
		},
		{
			instID:       "inst-copper",
			definitionID: "weapon-08",
			name:         "銅の剣", want: HomeItemInspection{Power: 14, Weight: 9, Price: 400},
		},
		{
			instID:       "inst-steel",
			definitionID: "weapon-13",
			name:         "鋼鉄の剣", want: HomeItemInspection{Power: 30, Weight: 20, Price: 1800},
		},
		{
			instID:       "inst-zombie",
			definitionID: "weapon-15",
			name:         "不死殺しの剣", want: HomeItemInspection{Power: 44, Weight: 24, Price: 3500},
		},
		{
			instID:       "inst-hagure-w",
			definitionID: "weapon-40",
			name:         "流銀の剣", want: HomeItemInspection{Power: 150, Weight: 75, Price: 30000},
		},
	}

	for _, tt := range weaponTests {
		t.Run("weapon_"+tt.instID, func(t *testing.T) {
			inv, _ := coreinventory.New(charID)
			if err := inv.Add(coreitem.Instance{ID: tt.instID, DefinitionID: tt.definitionID, Quantity: 1}); err != nil {
				t.Fatal(err)
			}
			invMgr.invs[charID] = inv
			res, err := svc.UseHomeItem(ctx, charID, tt.instID, "inventory")
			if err != nil {
				t.Fatalf("UseHomeItem failed: %v", err)
			}
			if res.Action != "inspect" || res.Consumed {
				t.Errorf("expected inspect action without consumption, got %+v", res)
			}
			if res.ItemName != tt.name || res.Inspection == nil || *res.Inspection != tt.want {
				t.Errorf("inspection = %s %+v, want %s %+v", res.ItemName, res.Inspection, tt.name, tt.want)
			}
		})
	}

	armorTests := []struct {
		instID string
		name   string
		want   HomeItemInspection
	}{
		{
			instID: "inst-cloth",
			name:   "布の服", want: HomeItemInspection{Power: 3, Weight: 0, Price: 20},
		},
		{
			instID: "inst-leather",
			name:   "皮の鎧", want: HomeItemInspection{Power: 12, Weight: 4, Price: 150},
		},
		{
			instID: "inst-chain",
			name:   "鎖かたびら", want: HomeItemInspection{Power: 24, Weight: 8, Price: 540},
		},
		{
			instID: "inst-iron-a",
			name:   "鉄の鎧", want: HomeItemInspection{Power: 43, Weight: 17, Price: 1200},
		},
		{
			instID: "inst-steel-a",
			name:   "鋼鉄の鎧", want: HomeItemInspection{Power: 52, Weight: 20, Price: 3700},
		},
		{
			instID: "inst-hagure-a",
			name:   "流銀の鎧", want: HomeItemInspection{Power: 150, Weight: 45, Price: 30000},
		},
	}

	for _, tt := range armorTests {
		t.Run("armor_"+tt.instID, func(t *testing.T) {
			res, err := svc.UseHomeItem(ctx, charID, tt.instID, "depot")
			if err != nil {
				t.Fatalf("UseHomeItem failed: %v", err)
			}
			if res.Action != "inspect" || res.Consumed {
				t.Errorf("expected inspect action without consumption, got %+v", res)
			}
			if res.ItemName != tt.name || res.Inspection == nil || *res.Inspection != tt.want {
				t.Errorf("inspection = %s %+v, want %s %+v", res.ItemName, res.Inspection, tt.name, tt.want)
			}
		})
	}
}

type fixedDeterministicRNG struct {
	val int
}

func (f *fixedDeterministicRNG) Intn(n int) int   { return f.val }
func (f *fixedDeterministicRNG) IntN(n int) int   { return f.val }
func (f *fixedDeterministicRNG) Float64() float64 { return float64(f.val) }

func TestUseHomeItem_Seeds_ParityAndVitality(t *testing.T) {
	ctx := context.Background()
	cat, err := coreitem.InitialCatalog()
	if err != nil {
		t.Fatalf("InitialCatalog failed: %v", err)
	}

	testCases := []struct {
		name          string
		defID         string
		source        string
		roll          int // 0 -> +3, 3 -> +6
		expectedGain  int
		overLevel     bool
		initialMaxHP  int
		initialHP     int
		initialMaxMP  int
		initialMP     int
		expectedMaxHP int
		expectedHP    int
		expectedMaxMP int
		expectedMP    int
		expectedStat  string
	}{
		{
			name:          "命の木の実 inventory min roll (3) increases MaxHP only",
			defID:         "item-016",
			source:        "inventory",
			roll:          0,
			expectedGain:  3,
			initialMaxHP:  100,
			initialHP:     50,
			initialMaxMP:  50,
			initialMP:     20,
			expectedMaxHP: 103,
			expectedHP:    50,
			expectedMaxMP: 50,
			expectedMP:    20,
			expectedStat:  HomeStatMaxHP,
		},
		{
			name:          "命の木の実 depot max roll (6) increases MaxHP only",
			defID:         "item-016",
			source:        "depot",
			roll:          3,
			expectedGain:  6,
			initialMaxHP:  100,
			initialHP:     50,
			initialMaxMP:  50,
			initialMP:     20,
			expectedMaxHP: 106,
			expectedHP:    50,
			expectedMaxMP: 50,
			expectedMP:    20,
			expectedStat:  HomeStatMaxHP,
		},
		{
			name:          "命の木の実 OverLevel clamp gives 0 increase and preserves vitality",
			defID:         "item-016",
			source:        "inventory",
			roll:          3,
			expectedGain:  0,
			overLevel:     true,
			initialMaxHP:  500,
			initialHP:     200,
			initialMaxMP:  300,
			initialMP:     100,
			expectedMaxHP: 500,
			expectedHP:    200,
			expectedMaxMP: 300,
			expectedMP:    100,
			expectedStat:  HomeStatMaxHP,
		},
		{
			name:          "命の木の実 ceiling clamp at 999 preserves vitality",
			defID:         "item-016",
			source:        "inventory",
			roll:          3, // +6, 998 + 6 = 1004 -> clamped to 999
			expectedGain:  6,
			initialMaxHP:  998,
			initialHP:     500,
			initialMaxMP:  50,
			initialMP:     20,
			expectedMaxHP: 999,
			expectedHP:    500,
			expectedMaxMP: 50,
			expectedMP:    20,
			expectedStat:  HomeStatMaxHP,
		},
		{
			name:          "不思議な木の実 inventory min roll (3) increases MaxMP only",
			defID:         "item-017",
			source:        "inventory",
			roll:          0,
			expectedGain:  3,
			initialMaxHP:  100,
			initialHP:     50,
			initialMaxMP:  50,
			initialMP:     20,
			expectedMaxHP: 100,
			expectedHP:    50,
			expectedMaxMP: 53,
			expectedMP:    20,
			expectedStat:  HomeStatMaxMP,
		},
		{
			name:          "不思議な木の実 depot max roll (6) increases MaxMP only",
			defID:         "item-017",
			source:        "depot",
			roll:          3,
			expectedGain:  6,
			initialMaxHP:  100,
			initialHP:     50,
			initialMaxMP:  50,
			initialMP:     20,
			expectedMaxHP: 100,
			expectedHP:    50,
			expectedMaxMP: 56,
			expectedMP:    20,
			expectedStat:  HomeStatMaxMP,
		},
		{
			name:          "不思議な木の実 OverLevel clamp gives 0 increase and preserves vitality",
			defID:         "item-017",
			source:        "inventory",
			roll:          3,
			expectedGain:  0,
			overLevel:     true,
			initialMaxHP:  500,
			initialHP:     200,
			initialMaxMP:  300,
			initialMP:     100,
			expectedMaxHP: 500,
			expectedHP:    200,
			expectedMaxMP: 300,
			expectedMP:    100,
			expectedStat:  HomeStatMaxMP,
		},
		{
			name:          "不思議な木の実 ceiling clamp at 999 preserves vitality",
			defID:         "item-017",
			source:        "depot",
			roll:          3, // +6, 998 + 6 = 1004 -> clamped to 999
			expectedGain:  6,
			initialMaxHP:  100,
			initialHP:     50,
			initialMaxMP:  998,
			initialMP:     300,
			expectedMaxHP: 100,
			expectedHP:    50,
			expectedMaxMP: 999,
			expectedMP:    300,
			expectedStat:  HomeStatMaxMP,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			charID := "char-parity-test"
			char := corecharacter.Character{
				ID:        charID,
				Name:      "Tester",
				Level:     10,
				OverLevel: tc.overLevel,
				Stats: corecharacter.Stats{
					MaxHP: tc.initialMaxHP,
					HP:    tc.initialHP,
					MaxMP: tc.initialMaxMP,
					MP:    tc.initialMP,
				},
			}
			chars := map[string]corecharacter.Character{charID: char}

			inv, _ := coreinventory.New(charID)
			dp, _ := depot.NewDepot(charID)

			instID := "inst-seed-test"
			if tc.source == "inventory" {
				_ = inv.Add(coreitem.Instance{ID: instID, DefinitionID: tc.defID, Quantity: 1})
			} else {
				dp.Items = append(dp.Items, coreitem.Instance{ID: instID, DefinitionID: tc.defID, Quantity: 1})
			}

			invs := map[string]coreinventory.Inventory{charID: inv}
			depots := map[string]depot.Depot{charID: dp}

			runner := &mockTransactionRunner{chars: chars, invs: invs}
			invMgr := &mockInventoryManager{invs: invs}
			depotMgr := &transactionalMockDepotManager{depots: depots}
			charReader := &mockCharReader{chars: chars}
			repo := newMockHomeRepo(chars)

			rng := &fixedDeterministicRNG{val: tc.roll}

			svc, err := NewService(
				repo,
				charReader,
				WithInventoryManager(invMgr),
				WithDepotManager(depotMgr),
				WithItemCatalog(cat),
				WithTransactionRunner(runner),
				WithRNG(rng),
			)
			if err != nil {
				t.Fatalf("NewService failed: %v", err)
			}

			res, err := svc.UseHomeItem(ctx, charID, instID, tc.source)
			if err != nil {
				t.Fatalf("UseHomeItem failed: %v", err)
			}
			if !res.Consumed || res.Action != "consumed" {
				t.Fatalf("expected consumed action, got %+v", res)
			}
			if got := outcomeOf(res); !reflect.DeepEqual(got, HomeItemOutcome{Effect: HomeItemEffectStatGain, Stat: tc.expectedStat, Amount: tc.expectedGain}) {
				t.Errorf("unexpected outcome: %+v", got)
			}

			// Verify character stats
			updatedChar := runner.chars[charID]
			if updatedChar.Stats.MaxHP != tc.expectedMaxHP {
				t.Errorf("MaxHP = %d, want %d", updatedChar.Stats.MaxHP, tc.expectedMaxHP)
			}
			if updatedChar.Stats.HP != tc.expectedHP {
				t.Errorf("HP = %d, want %d (vitality changed!)", updatedChar.Stats.HP, tc.expectedHP)
			}
			if updatedChar.Stats.MaxMP != tc.expectedMaxMP {
				t.Errorf("MaxMP = %d, want %d", updatedChar.Stats.MaxMP, tc.expectedMaxMP)
			}
			if updatedChar.Stats.MP != tc.expectedMP {
				t.Errorf("MP = %d, want %d (vitality changed!)", updatedChar.Stats.MP, tc.expectedMP)
			}

			// Verify 1 item consumed
			if tc.source == "inventory" {
				currentInv := runner.invs[charID]
				if _, found := currentInv.Find(instID); found {
					t.Errorf("expected item %s to be consumed from inventory", instID)
				}
			} else {
				currentDepot := depotMgr.depots[charID]
				if len(currentDepot.Items) != 0 {
					t.Errorf("expected item %s to be consumed from depot", instID)
				}
			}
		})
	}
}

// Item use needs the transaction runner, and unsupported items are rejected
// before any transaction starts.
func TestUseHomeItem_RejectsWithoutRunnerOrDispatch(t *testing.T) {
	chars := map[string]corecharacter.Character{"c": {ID: "c", Name: "c"}}
	inv, _ := coreinventory.New("c")
	_ = inv.Add(coreitem.Instance{ID: "seed", DefinitionID: "item-seed6", Quantity: 1})
	invs := map[string]coreinventory.Inventory{"c": inv}
	catalog := &mockCatalog{defs: map[string]coreitem.Definition{
		"item-seed6": {ID: "item-seed6", Name: "スキルの種", UsageCategory: coreitem.UsageCategoryAnytime},
		"item-x":     {ID: "item-x", Name: "未対応の品", UsageCategory: coreitem.UsageCategoryAnytime},
	}}
	newSvc := func(opts ...ServiceOption) *Service {
		opts = append(opts, WithInventoryManager(&mockInventoryManager{invs: invs}), WithItemCatalog(catalog))
		svc, err := NewService(newMockHomeRepo(chars), &mockCharReader{chars: chars}, opts...)
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}

	if _, err := newSvc().UseHomeItem(context.Background(), "c", "seed", "inventory"); !errors.Is(err, ErrTransactionRunnerRequired) {
		t.Fatalf("want ErrTransactionRunnerRequired, got %v", err)
	}
	if got := invs["c"]; len(got.Items) != 1 || chars["c"].SP != 0 {
		t.Fatalf("state changed without a runner: %+v SP=%d", got.Items, chars["c"].SP)
	}

	inv, _ = coreinventory.New("c")
	if err := inv.Add(coreitem.Instance{ID: "x", DefinitionID: "item-x", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	invs["c"] = inv
	runner := &mockTransactionRunner{chars: chars, invs: invs}
	if _, err := newSvc(WithTransactionRunner(runner)).UseHomeItem(context.Background(), "c", "x", "inventory"); !errors.Is(err, ErrCannotUseHere) || runner.calls != 0 {
		t.Fatalf("want rejection before the runner, got %v (calls=%d)", err, runner.calls)
	}
}

// outcomeOf returns the callback outcome, or the zero outcome for inspections.
func outcomeOf(res *UseHomeItemResult) HomeItemOutcome {
	if res == nil || res.Outcome == nil {
		return HomeItemOutcome{}
	}
	return *res.Outcome
}

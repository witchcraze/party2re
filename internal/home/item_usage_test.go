package home

import (
	"context"
	"errors"
	"fmt"
	mrand "math/rand"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
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

func (m *mockInventoryManager) Consume(ctx context.Context, characterID, instanceID string, quantity int) (coreinventory.Inventory, error) {
	inv, _ := m.FindByCharacterID(ctx, characterID)
	_ = inv.Consume(instanceID, quantity)
	m.invs[characterID] = inv
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

func (m *mockDepotManager) RemoveItem(ctx context.Context, characterID, itemInstanceID string) error {
	dp, ok := m.depots[characterID]
	if !ok {
		return depot.ErrNotFound
	}
	for i, it := range dp.Items {
		if it.ID == itemInstanceID {
			dp.Items = append(dp.Items[:i], dp.Items[i+1:]...)
			m.depots[characterID] = dp
			return nil
		}
	}
	return depot.ErrItemNotFound
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
			"item-wea":    {ID: "item-wea", Name: "銅の剣", Price: 100, Slot: coreitem.SlotMainHand},
			"item-arm":    {ID: "item-arm", Name: "革の鎧", Price: 150, Slot: coreitem.SlotBody},
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

	inv, _ := coreinventory.New("char-1")
	_ = inv.Add(coreitem.Instance{ID: "inst-wea", DefinitionID: "item-wea", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-seed1", DefinitionID: "item-seed1", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-seed3", DefinitionID: "item-seed3", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-seed4", DefinitionID: "item-seed4", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-seed5", DefinitionID: "item-seed5", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-seed6", DefinitionID: "item-seed6", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-happy", DefinitionID: "item-happy", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-fight", DefinitionID: "item-fight", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-fight2", DefinitionID: "item-fight2", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-herb", DefinitionID: "item-herb", Quantity: 2})
	_ = inv.Add(coreitem.Instance{ID: "inst-potion", DefinitionID: "item-potion", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-drop", DefinitionID: "item-drop", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-water", DefinitionID: "item-water", Quantity: 1})
	_ = inv.Add(coreitem.Instance{ID: "inst-other", DefinitionID: "item-other", Quantity: 1})
	invMgr.invs["char-1"] = inv

	dp, _ := depot.NewDepot("char-1")
	dp.Items = append(dp.Items,
		coreitem.Instance{ID: "inst-arm", DefinitionID: "item-arm", Quantity: 1},
		coreitem.Instance{ID: "inst-seed2", DefinitionID: "item-seed2", Quantity: 1},
		coreitem.Instance{ID: "inst-medal", DefinitionID: "item-medal", Quantity: 1},
	)
	depotMgr.depots["char-1"] = dp

	invOver, _ := coreinventory.New("char-over")
	_ = invOver.Add(coreitem.Instance{ID: "inst-over-seed1", DefinitionID: "item-seed1", Quantity: 1})
	_ = invOver.Add(coreitem.Instance{ID: "inst-over-seed2", DefinitionID: "item-seed2", Quantity: 1})
	_ = invOver.Add(coreitem.Instance{ID: "inst-over-seed3", DefinitionID: "item-seed3", Quantity: 1})
	_ = invOver.Add(coreitem.Instance{ID: "inst-over-seed4", DefinitionID: "item-seed4", Quantity: 1})
	_ = invOver.Add(coreitem.Instance{ID: "inst-over-seed5", DefinitionID: "item-seed5", Quantity: 1})
	_ = invOver.Add(coreitem.Instance{ID: "inst-over-seed6", DefinitionID: "item-seed6", Quantity: 1})
	invMgr.invs["char-over"] = invOver

	svc, err := NewService(
		repo,
		charReader,
		WithCharacterUpdater(charUpdater),
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
		if len(items) != 17 {
			t.Errorf("expected 17 items total, got %d", len(items))
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
		if !strings.Contains(resWea.Message, "重さ：") || !strings.Contains(resWea.Message, "強さ：") || !strings.Contains(resWea.Message, "価格：") {
			t.Errorf("unexpected weapon inspect message format: %s", resWea.Message)
		}

		resArm, err := svc.UseHomeItem(ctx, "char-1", "inst-arm", "depot")
		if err != nil {
			t.Fatalf("UseHomeItem armor failed: %v", err)
		}
		if resArm.Action != "inspect" || resArm.Consumed {
			t.Errorf("expected inspect and not consumed, got %+v", resArm)
		}
		if !strings.Contains(resArm.Message, "重さ：") || !strings.Contains(resArm.Message, "強さ：") || !strings.Contains(resArm.Message, "価格：") {
			t.Errorf("unexpected armor inspect message format: %s", resArm.Message)
		}
	})

	t.Run("reject combat-only recovery items", func(t *testing.T) {
		combatItemInstances := []string{"inst-herb", "inst-potion", "inst-drop", "inst-water"}
		for _, instID := range combatItemInstances {
			oldHP := chars["char-1"].Stats.HP
			oldMP := chars["char-1"].Stats.MP
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
		if res.Message != "次のクエスト時にレベルアップ！" {
			t.Errorf("unexpected message: %s", res.Message)
		}
	})

	t.Run("consume ファイト一発 resets fatigue", func(t *testing.T) {
		if chars["char-1"].Tired == 0 {
			t.Fatal("expected non-zero fatigue before using fight item")
		}
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
		expectedMsg := "元気全快！Heroの疲労が回復した！"
		if res.Message != expectedMsg {
			t.Errorf("expected message %q, got %q", expectedMsg, res.Message)
		}

		// Also verify 気合の霊薬
		charHero := chars["char-1"]
		charHero.Tired = 75
		chars["char-1"] = charHero
		res2, err := svc.UseHomeItem(ctx, "char-1", "inst-fight2", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem 気合の霊薬 failed: %v", err)
		}
		if chars["char-1"].Tired != 0 {
			t.Errorf("expected fatigue 0 after 気合の霊薬, got %d", chars["char-1"].Tired)
		}
		if res2.Message != expectedMsg {
			t.Errorf("expected message %q, got %q", expectedMsg, res2.Message)
		}
	})

	t.Run("overlevel characters receive 0 stat increase from stat seeds", func(t *testing.T) {
		oldStats := chars["char-over"].Stats

		// 命の木の実
		resHP, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed1", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed1 failed: %v", err)
		}
		if chars["char-over"].Stats.MaxHP != oldStats.MaxHP {
			t.Errorf("expected MaxHP unchanged, got %d (was %d)", chars["char-over"].Stats.MaxHP, oldStats.MaxHP)
		}
		if !strings.Contains(resHP.Message, "0 あがった！") {
			t.Errorf("expected 0 increase message, got %s", resHP.Message)
		}

		// 不思議な木の実
		resMP, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed2", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed2 failed: %v", err)
		}
		if chars["char-over"].Stats.MaxMP != oldStats.MaxMP {
			t.Errorf("expected MaxMP unchanged, got %d (was %d)", chars["char-over"].Stats.MaxMP, oldStats.MaxMP)
		}
		if !strings.Contains(resMP.Message, "0 あがった！") {
			t.Errorf("expected 0 increase message, got %s", resMP.Message)
		}

		// 力の種
		resAtk, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed3", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed3 failed: %v", err)
		}
		if chars["char-over"].Stats.Attack != oldStats.Attack {
			t.Errorf("expected Attack unchanged, got %d (was %d)", chars["char-over"].Stats.Attack, oldStats.Attack)
		}
		if !strings.Contains(resAtk.Message, "0 あがった！") {
			t.Errorf("expected 0 increase message, got %s", resAtk.Message)
		}

		// 守りの種
		resDef, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed4", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed4 failed: %v", err)
		}
		if chars["char-over"].Stats.Defense != oldStats.Defense {
			t.Errorf("expected Defense unchanged, got %d (was %d)", chars["char-over"].Stats.Defense, oldStats.Defense)
		}
		if !strings.Contains(resDef.Message, "0 あがった！") {
			t.Errorf("expected 0 increase message, got %s", resDef.Message)
		}

		// 素早さの種
		resAgi, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed5", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed5 failed: %v", err)
		}
		if chars["char-over"].Stats.Agility != oldStats.Agility {
			t.Errorf("expected Agility unchanged, got %d (was %d)", chars["char-over"].Stats.Agility, oldStats.Agility)
		}
		if !strings.Contains(resAgi.Message, "0 あがった！") {
			t.Errorf("expected 0 increase message, got %s", resAgi.Message)
		}

		// スキルの種 is NOT clamped by OverLevel
		oldSP := chars["char-over"].SP
		resSP, err := svc.UseHomeItem(ctx, "char-over", "inst-over-seed6", "inventory")
		if err != nil {
			t.Fatalf("UseHomeItem seed6 failed: %v", err)
		}
		if chars["char-over"].SP <= oldSP {
			t.Errorf("expected SP to increase from %d, got %d", oldSP, chars["char-over"].SP)
		}
		if strings.Contains(resSP.Message, "0 あがった！") {
			t.Errorf("expected non-zero increase message for SP seed, got %s", resSP.Message)
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
				res, err := svc.UseHomeItem(ctx, "char-1", itemCase.instID, "inventory")
				if !errors.Is(err, ErrCannotUseHere) {
					t.Fatalf("expected ErrCannotUseHere, got res=%+v, err=%v", res, err)
				}
				expectedSubstr := fmt.Sprintf("%sは戦闘中でしか使えません", itemCase.name)
				if !strings.Contains(err.Error(), expectedSubstr) {
					t.Errorf("expected error message to contain %q, got %q", expectedSubstr, err.Error())
				}
			})
		}
	})

	t.Run("cannot use non-usable item in home", func(t *testing.T) {
		res, err := svc.UseHomeItem(ctx, "char-1", "inst-other", "inventory")
		if !errors.Is(err, ErrCannotUseHere) {
			t.Fatalf("expected ErrCannotUseHere, got res=%+v, err=%v", res, err)
		}
		expectedSubstr := "爆弾岩の破片はここでは使えません"
		if !strings.Contains(err.Error(), expectedSubstr) {
			t.Errorf("expected error message to contain %q, got %q", expectedSubstr, err.Error())
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
						WithInventoryManager(invMgr),
						WithDepotManager(depotMgr),
						WithItemCatalog(cat),
					)
					if err != nil {
						t.Fatalf("NewService failed: %v", err)
					}

					res, err := svc.UseHomeItem(ctx, char.ID, "inst-"+def.ID, "inventory")
					if !errors.Is(err, ErrCannotUseHere) {
						t.Fatalf("[%s] expected ErrCannotUseHere, got res=%+v, err=%v", def.ID, res, err)
					}
					expectedSubstr := fmt.Sprintf("%sは戦闘中でしか使えません", def.Name)
					if !strings.Contains(err.Error(), expectedSubstr) {
						t.Errorf("[%s] expected error message to contain %q, got %q", def.ID, expectedSubstr, err.Error())
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

func (m *transactionalMockDepotManager) RemoveItem(ctx context.Context, characterID, itemInstanceID string) error {
	dp, ok := m.depots[characterID]
	if !ok {
		return depot.ErrNotFound
	}
	for i, it := range dp.Items {
		if it.ID == itemInstanceID {
			dp.Items = append(dp.Items[:i], dp.Items[i+1:]...)
			m.depots[characterID] = dp
			return nil
		}
	}
	return depot.ErrItemNotFound
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
		WithRNG(mrand.New(mrand.NewSource(1))),
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
	if runner.lastReq.LockInventory {
		t.Errorf("expected LockInventory=false for depot item usage")
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

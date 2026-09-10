package home

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
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
			"item-seed1":  {ID: "item-seed1", Name: "命の木の実", Price: 50, Slot: coreitem.SlotNone},
			"item-seed2":  {ID: "item-seed2", Name: "不思議な木の実", Price: 50, Slot: coreitem.SlotNone},
			"item-seed3":  {ID: "item-seed3", Name: "力の種", Price: 50, Slot: coreitem.SlotNone},
			"item-seed4":  {ID: "item-seed4", Name: "守りの種", Price: 50, Slot: coreitem.SlotNone},
			"item-seed5":  {ID: "item-seed5", Name: "素早さの種", Price: 50, Slot: coreitem.SlotNone},
			"item-seed6":  {ID: "item-seed6", Name: "スキルの種", Price: 50, Slot: coreitem.SlotNone},
			"item-happy":  {ID: "item-happy", Name: "幸せの種", Price: 3000, Slot: coreitem.SlotNone},
			"item-fight":  {ID: "item-fight", Name: "ファイト一発", Price: 3000, Slot: coreitem.SlotNone},
			"item-fight2": {ID: "item-fight2", Name: "気合の霊薬", Price: 3000, Slot: coreitem.SlotNone},
			"item-medal":  {ID: "item-medal", Name: "小さなメダル", Price: 0, Slot: coreitem.SlotNone},
			"item-herb":   {ID: "item-herb", Name: "薬草", Price: 8, Slot: coreitem.SlotNone},
			"item-potion": {ID: "item-potion", Name: "特薬草", Price: 250, Slot: coreitem.SlotNone},
			"item-drop":   {ID: "item-drop", Name: "世界樹のしずく", Price: 5000, Slot: coreitem.SlotNone},
			"item-water":  {ID: "item-water", Name: "魔法の聖水", Price: 500, Slot: coreitem.SlotNone},
			"item-other":  {ID: "item-other", Name: "爆弾岩の破片", Price: 200, Slot: coreitem.SlotNone},
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

	t.Run("cannot use battle-only item", func(t *testing.T) {
		res, err := svc.UseHomeItem(ctx, "char-1", "inst-other", "inventory")
		if !errors.Is(err, ErrCannotUseHere) {
			t.Fatalf("expected ErrCannotUseHere, got res=%+v, err=%v", res, err)
		}
	})
}

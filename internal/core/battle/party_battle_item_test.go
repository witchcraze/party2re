package battle_test

import (
	"errors"
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
)

func TestValidateItemAction(t *testing.T) {
	tests := []struct {
		name    string
		cat     item.UsageCategory
		wantErr error
	}{
		{
			name:    "Category 1 CombatOnly allowed",
			cat:     item.UsageCategoryCombatOnly,
			wantErr: nil,
		},
		{
			name:    "Category 0 None rejected",
			cat:     item.UsageCategoryNone,
			wantErr: corebattle.ErrCannotUseInCombat,
		},
		{
			name:    "Category 2 Anytime (seeds, medals) rejected",
			cat:     item.UsageCategoryAnytime,
			wantErr: corebattle.ErrCannotUseInCombat,
		},
		{
			name:    "Category 3 CombatPassive (shields, charms) rejected",
			cat:     item.UsageCategoryCombatPassive,
			wantErr: corebattle.ErrCannotUseInCombat,
		},
		{
			name:    "Category 4 DepotAfterAction rejected",
			cat:     item.UsageCategoryDepotAfterAction,
			wantErr: corebattle.ErrCannotUseInCombat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act := corebattle.ActionItem{
				ID:            "test-item",
				Name:          "テストアイテム",
				UsageCategory: tt.cat,
				Kind:          corebattle.ActionKindHeal,
				Power:         30,
			}
			err := corebattle.ValidateItemAction(act)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateItemAction() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestParticipantRejectsNonCombatItems(t *testing.T) {
	// Attempt to build a participant with a seed (Category 2)
	badItem := corebattle.ActionItem{
		ID:            "item-060",
		Name:          "力の種",
		UsageCategory: item.UsageCategoryAnytime,
		Kind:          corebattle.ActionKindBuff,
		Power:         2,
	}

	builder := corebattle.NewParticipantBuilder("hero").
		WithStats(50, 10, 5).
		WithActionItems(badItem)

	_, err := builder.Build()
	if !errors.Is(err, corebattle.ErrCannotUseInCombat) {
		t.Fatalf("expected ErrCannotUseInCombat during Build(), got %v", err)
	}

	// Create participant manually with invalid item
	pBad := corebattle.Participant{
		ID:          "hero",
		Name:        "Hero",
		HP:          50,
		MaxHP:       50,
		Attack:      10,
		Defense:     5,
		ActionItems: []corebattle.ActionItem{badItem},
	}

	engine := corebattle.Engine{}
	// ResolvePartyBattle must reject participant
	_, err = engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{pBad},
		Enemies: []corebattle.Participant{
			corebattle.MustNewParticipant("slime", 10, 5, 2),
		},
	})
	if !errors.Is(err, corebattle.ErrCannotUseInCombat) {
		t.Fatalf("ResolvePartyBattle expected ErrCannotUseInCombat, got %v", err)
	}

	// Resolve (1v1) must reject participant
	_, err = engine.Resolve(corebattle.Request{
		Participants: []corebattle.Participant{
			pBad,
			corebattle.MustNewParticipant("slime", 10, 5, 2),
		},
	})
	if !errors.Is(err, corebattle.ErrCannotUseInCombat) {
		t.Fatalf("Resolve expected ErrCannotUseInCombat, got %v", err)
	}
}

func TestPartyBattleItemExecution(t *testing.T) {
	// Hero with low HP (20/100) and a healing item (Category 1, 薬草, Power 30)
	herb := corebattle.ActionItem{
		ID:            "item-001",
		Name:          "薬草",
		UsageCategory: item.UsageCategoryCombatOnly,
		Kind:          corebattle.ActionKindHeal,
		Power:         30,
		TargetScope:   corebattle.TargetScopeSingleAlly,
	}

	hero, err := corebattle.NewParticipantBuilder("hero").
		WithStats(20, 10, 5).
		WithAgility(20). // High agility to act first
		WithActionItems(herb).
		Build()
	if err != nil {
		t.Fatalf("Build hero failed: %v", err)
	}
	hero.MaxHP = 100 // ensure maxHP is 100

	enemy, err := corebattle.NewParticipantBuilder("slime").
		WithStats(5, 1, 0).
		WithAgility(5).
		Build()
	if err != nil {
		t.Fatalf("Build slime failed: %v", err)
	}

	engine := corebattle.Engine{}
	res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies:  []corebattle.Participant{hero},
		Enemies: []corebattle.Participant{enemy},
	})
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Verify that the item was executed in turn logs
	foundItemLog := false
	for _, l := range res.Logs {
		if l.ActionName == "薬草" {
			foundItemLog = true
			if l.HealingDone != 30 {
				t.Errorf("HealingDone = %d, want 30", l.HealingDone)
			}
			if l.ActorID != "hero" {
				t.Errorf("ActorID = %s, want hero", l.ActorID)
			}
		}
	}
	if !foundItemLog {
		t.Fatalf("expected turn log for item '薬草', got logs: %+v", res.Logs)
	}
}

func TestPartyBattleItemBuffExecution(t *testing.T) {
	buffItem := corebattle.ActionItem{
		ID:            "item-buff",
		Name:          "ちからのみず",
		UsageCategory: item.UsageCategoryCombatOnly,
		Kind:          corebattle.ActionKindBuff,
		BuffStat:      "attack",
		Power:         15,
		TargetScope:   corebattle.TargetScopeAllAllies,
	}

	hero, err := corebattle.NewParticipantBuilder("hero").
		WithStats(50, 10, 5).
		WithAgility(20).
		WithActionItems(buffItem).
		Build()
	if err != nil {
		t.Fatalf("Build hero failed: %v", err)
	}

	enemy, err := corebattle.NewParticipantBuilder("slime").
		WithStats(5, 1, 0).
		WithAgility(5).
		Build()
	if err != nil {
		t.Fatalf("Build slime failed: %v", err)
	}

	engine := corebattle.Engine{}
	res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies:  []corebattle.Participant{hero},
		Enemies: []corebattle.Participant{enemy},
	})
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	foundBuffLog := false
	for _, l := range res.Logs {
		if l.ActionName == "ちからのみず" {
			foundBuffLog = true
			if l.ActorID != "hero" {
				t.Errorf("ActorID = %s, want hero", l.ActorID)
			}
		}
	}
	if !foundBuffLog {
		t.Fatalf("expected turn log for buff item, got logs: %+v", res.Logs)
	}
}

func TestAll269ItemsBattleActionMatrix(t *testing.T) {
	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatalf("InitialCatalog failed: %v", err)
	}

	combatAllowed := 0
	combatRejected := 0

	for _, ref := range item.LegacyItemCatalog {
		if ref.No == 0 {
			continue
		}
		itemID := fmt.Sprintf("item-%03d", ref.No)
		def, err := catalog.FindByID(itemID)
		if err != nil {
			t.Fatalf("[%s] item not found: %v", itemID, err)
		}

		act := corebattle.ActionItem{
			ID:            def.ID,
			Name:          def.Name,
			UsageCategory: def.UsageCategory,
			Kind:          corebattle.ActionKindHeal,
			Power:         10,
		}

		err = corebattle.ValidateItemAction(act)
		if def.UsageCategory == item.UsageCategoryCombatOnly {
			if err != nil {
				t.Errorf("[%s: %s] expected ValidateItemAction to succeed for Category 1, got %v", itemID, def.Name, err)
			}
			combatAllowed++
		} else {
			if !errors.Is(err, corebattle.ErrCannotUseInCombat) {
				t.Errorf("[%s: %s] expected ErrCannotUseInCombat for Category %v, got %v", itemID, def.Name, def.UsageCategory, err)
			}
			combatRejected++
		}
	}

	if combatAllowed != 53 {
		t.Errorf("combatAllowed = %d, want 53", combatAllowed)
	}
	if combatRejected != 215 {
		t.Errorf("combatRejected = %d, want 215", combatRejected)
	}
}

func TestPartyBattleItemConsumptionTracking(t *testing.T) {
	herb := corebattle.ActionItem{
		ID:            "item-001",
		InstanceID:    "inst-herb-1",
		Name:          "薬草",
		UsageCategory: item.UsageCategoryCombatOnly,
		Kind:          corebattle.ActionKindHeal,
		Power:         30,
		TargetScope:   corebattle.TargetScopeSingleAlly,
	}

	hero, err := corebattle.NewParticipantBuilder("hero").
		WithStats(20, 10, 5).
		WithMP(50, 50).
		WithCMP(30, 30).
		WithAgility(30).
		WithActionItems(herb).
		Build()
	if err != nil {
		t.Fatalf("Build hero failed: %v", err)
	}
	hero.MaxHP = 100

	enemy, err := corebattle.NewParticipantBuilder("slime").
		WithStats(5, 1, 0).
		WithAgility(5).
		Build()
	if err != nil {
		t.Fatalf("Build slime failed: %v", err)
	}

	engine := corebattle.Engine{}
	res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies:  []corebattle.Participant{hero},
		Enemies: []corebattle.Participant{enemy},
	})
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Verify consumed items tracking
	consumedList, ok := res.ConsumedItems["hero"]
	if !ok || len(consumedList) == 0 {
		t.Fatalf("expected consumed items for 'hero', got: %+v", res.ConsumedItems)
	}
	if consumedList[0].ID != "item-001" || consumedList[0].InstanceID != "inst-herb-1" || consumedList[0].Quantity != 1 {
		t.Errorf("consumed item mismatch: got %+v, want ID: item-001, InstanceID: inst-herb-1, Quantity: 1", consumedList[0])
	}

	// Verify remaining resources tracking
	if res.RemainingMP["hero"] != 50 {
		t.Errorf("RemainingMP = %d, want 50", res.RemainingMP["hero"])
	}
	if res.RemainingCMP["hero"] != 30 {
		t.Errorf("RemainingCMP = %d, want 30", res.RemainingCMP["hero"])
	}
}

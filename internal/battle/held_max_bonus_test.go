package battle_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

// A held 113/114 raises the battle maximum by 100 with a flat 999 cap
// (quest.cgi:1187–1189, _data.cgi:1408–1429); current values are unchanged.
func TestBuildParticipant_HeldStatReturnItemRaisesBattleMaximum(t *testing.T) {
	cases := []struct {
		name                 string
		held                 string
		maxHP, maxMP         int
		wantMaxHP, wantMaxMP int
	}{
		{"bracelet normal", "item-113", 300, 200, 400, 200},
		{"bracelet near cap", "item-113", 950, 200, 999, 200},
		// Pitfall: the flat cap lowers an over-level maximum above 999.
		{"bracelet over-level", "item-113", 1500, 200, 999, 200},
		{"ring normal", "item-114", 300, 200, 300, 300},
		{"ring near cap", "item-114", 300, 950, 300, 999},
		{"sealed key no-op", "item-259", 300, 200, 300, 200},
		{"other tool", "item-018", 300, 200, 300, 200},
		{"empty hand", "", 300, 200, 300, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			char := corecharacter.Character{ID: "char-held", Name: "Hero",
				Stats: corecharacter.Stats{HP: 250, MaxHP: tc.maxHP, MP: 150, MaxMP: tc.maxMP, Attack: 10, Defense: 10, Agility: 10}}
			inv, _ := coreinventory.New(char.ID)
			if tc.held != "" {
				if err := inv.Add(coreitem.Instance{ID: "held", DefinitionID: tc.held, Quantity: 1}); err != nil {
					t.Fatal(err)
				}
			}
			p, err := battle.BuildParticipantFromData(char, inv, coreequipment.Equipment{CharacterID: char.ID}, nil, nil, skill.BattleDefinitions{})
			if err != nil {
				t.Fatal(err)
			}
			if p.MaxHP != tc.wantMaxHP || p.MaxMP != tc.wantMaxMP || p.HP != 250 || p.MP != 150 {
				t.Fatalf("got HP %d/%d MP %d/%d, want 250/%d 150/%d", p.HP, p.MaxHP, p.MP, p.MaxMP, tc.wantMaxHP, tc.wantMaxMP)
			}
		})
	}
}

func TestHeldVitalitySurvivesResetAndStaysInBattle(t *testing.T) {
	ctx := context.Background()
	chars, inventory, depots := newMockCharRepo(), newMockInvRepo(), newMockDepotRepo()
	char := corecharacter.Character{
		ID: "hero", Name: "Hero", JobID: "job-01", Level: 1,
		Stats: corecharacter.Stats{HP: 250, MaxHP: 300, MP: 150, MaxMP: 200, Attack: 10, Defense: 10, Agility: 10},
	}
	chars.characters[char.ID] = char
	bracelet := coreitem.Instance{ID: "bracelet", DefinitionID: "item-113", Quantity: 1}
	ring := coreitem.Instance{ID: "ring", DefinitionID: "item-114", Quantity: 1}
	if err := depots.Save(ctx, depot.Depot{CharacterID: char.ID, Items: []coreitem.Instance{bracelet, ring}}); err != nil {
		t.Fatal(err)
	}
	runner, err := economy.NewService(chars, inventory, economy.WithTransactionProvider(&mockTxProvider{}))
	if err != nil {
		t.Fatal(err)
	}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(inventory),
		battle.WithDepotRepository(depots), battle.WithTransactionRunner(runner),
		battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	actor, err := svc.BuildLifetimeActor(ctx, char.ID, "")
	if err != nil || actor.MaxHP != 300 || actor.MaxMP != 200 {
		t.Fatalf("Depot-only actor = %+v, error = %v", actor, err)
	}

	weapon := coreitem.Instance{ID: "weapon", DefinitionID: "weapon-01", Quantity: 1}
	if err := depots.Save(ctx, depot.Depot{CharacterID: char.ID, Items: []coreitem.Instance{ring}}); err != nil {
		t.Fatal(err)
	}
	if err := inventory.Save(ctx, coreinventory.Inventory{CharacterID: char.ID, Items: []coreitem.Instance{weapon, bracelet}}); err != nil {
		t.Fatal(err)
	}
	actor, err = svc.BuildLifetimeActor(ctx, char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{actor}})
	if err != nil {
		t.Fatal(err)
	}
	var state corebattle.LifetimeState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	reset := corebattle.EquipmentResetContext{TargetContext: corebattle.TargetContext{ActorID: char.ID, PartyIDs: []string{char.ID}}}
	for range 2 {
		if err := state.ApplyEquipmentReset(&reset, char.ID, random.Default()); err != nil {
			t.Fatal(err)
		}
		a := state.Actors[0]
		if a.HP != 250 || a.MaxHP != 400 || a.MP != 150 || a.MaxMP != 200 {
			t.Fatalf("reset actor = %+v", a.Participant)
		}
	}
	_, err = svc.ApplyPostBattleResult(ctx, battle.ApplyPostBattleRequest{
		CharacterIDs: []string{char.ID},
		BattleResult: corebattle.PartyBattleResult{
			RemainingHP: map[string]int{char.ID: 400},
			RemainingMP: map[string]int{char.ID: 195},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := chars.FindByID(ctx, char.ID)
	if err != nil || saved.Stats.HP != 300 || saved.Stats.MaxHP != 300 || saved.Stats.MP != 195 || saved.Stats.MaxMP != 200 {
		t.Fatalf("settled stats = %+v, error = %v", saved.Stats, err)
	}
}

// Approved departure (#2369): holding item-259 (category 4) at quest entry
// enters combat normally without Depot box lookup/delivery or corrupting the row.
func TestHeldSealedKeyIsNoOpAtQuestEntry(t *testing.T) {
	ctx := context.Background()
	for _, withDepotBox := range []bool{false, true} {
		chars, inventory, depots := newMockCharRepo(), newMockInvRepo(), newMockDepotRepo()
		char := corecharacter.Character{
			ID: "hero", Name: "Hero", JobID: "job-01", Level: 1,
			Stats: corecharacter.Stats{HP: 250, MaxHP: 300, MP: 150, MaxMP: 200, Attack: 10, Defense: 10, Agility: 10},
		}
		chars.characters[char.ID] = char
		key := coreitem.Instance{ID: "key", DefinitionID: "item-259", Quantity: 1}
		if err := inventory.Save(ctx, coreinventory.Inventory{CharacterID: char.ID, Items: []coreitem.Instance{key}}); err != nil {
			t.Fatal(err)
		}
		if withDepotBox {
			box := coreitem.Instance{ID: "box", DefinitionID: "item-258", Quantity: 1}
			if err := depots.Save(ctx, depot.Depot{CharacterID: char.ID, Items: []coreitem.Instance{box}}); err != nil {
				t.Fatal(err)
			}
		}
		svc := battle.NewService(
			battle.WithCharacterRepository(chars),
			battle.WithInventoryRepository(inventory),
			battle.WithDepotRepository(depots),
			battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}),
		)

		p, err := svc.BuildParticipant(ctx, char.ID)
		if err != nil || p.Name != "Hero" || p.HP != 250 || p.MaxHP != 300 || p.MP != 150 || p.MaxMP != 200 {
			t.Fatalf("withBox=%v BuildParticipant = %+v, err = %v", withDepotBox, p, err)
		}
		actor, err := svc.BuildLifetimeActor(ctx, char.ID, "")
		if err != nil || actor.Name != "Hero" || actor.HP != 250 || actor.MaxHP != 300 || actor.MP != 150 || actor.MaxMP != 200 {
			t.Fatalf("withBox=%v BuildLifetimeActor = %+v, err = %v", withDepotBox, actor, err)
		}
		hp, mp, err := svc.BuildBattleMaxima(ctx, char)
		if err != nil || hp != 300 || mp != 200 {
			t.Fatalf("withBox=%v BuildBattleMaxima = (%d, %d), err = %v", withDepotBox, hp, mp, err)
		}

		savedInv, err := inventory.FindByCharacterID(ctx, char.ID)
		if err != nil || len(savedInv.Items) != 1 || savedInv.Items[0].ID != "key" || savedInv.Items[0].DefinitionID != "item-259" || savedInv.Items[0].Quantity != 1 {
			t.Fatalf("withBox=%v inventory mutated = %+v, err = %v", withDepotBox, savedInv, err)
		}
		savedDepot, err := depots.FindByCharacterID(ctx, char.ID)
		if withDepotBox {
			if err != nil || len(savedDepot.Items) != 1 || savedDepot.Items[0].ID != "box" || savedDepot.Items[0].DefinitionID != "item-258" || savedDepot.Items[0].Quantity != 1 {
				t.Fatalf("withBox=%v depot mutated = %+v, err = %v", withDepotBox, savedDepot, err)
			}
		} else if !errors.Is(err, depot.ErrNotFound) {
			t.Fatalf("withBox=%v depot = %+v, err = %v, want %v", withDepotBox, savedDepot, err, depot.ErrNotFound)
		}
	}
}

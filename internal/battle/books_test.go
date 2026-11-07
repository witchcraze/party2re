package battle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	coreskill "github.com/witchcraze/party2re/internal/core/skill"
)

// Discovery reads the equipped item slot through the adapter, never bag contents,
// and the body uses the adapter's base stats.
func TestBookAdapterEquippedItemAndBaseStats(t *testing.T) {
	for _, equipped := range []bool{false, true} {
		ctx := context.Background()
		chars, inventory, equipment := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
		char := corecharacter.Character{ID: "self", Level: 30, JobID: "job-01", Stats: corecharacter.Stats{HP: 100, MaxHP: 1000, MP: 48, MaxMP: 1000, Attack: 100, Defense: 100, Agility: 100}}
		chars.characters[char.ID] = char
		if err := inventory.Save(ctx, coreinventory.Inventory{CharacterID: char.ID, Items: []coreitem.Instance{{ID: "book", DefinitionID: "item-179", Quantity: 1}}}); err != nil {
			t.Fatal(err)
		}
		if equipped {
			if err := equipment.Save(ctx, coreequipment.Equipment{CharacterID: char.ID, Slots: map[coreitem.Slot]string{coreitem.SlotAccessory: "book"}}); err != nil {
				t.Fatal(err)
			}
		}
		svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(inventory), battle.WithEquipmentRepository(equipment), battle.WithRandomSource(mockRNG{}), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
		a, err := svc.BuildLifetimeActor(ctx, char.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		a.TeamID = "party"
		s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "enemy", TeamID: "enemy", HP: 100, Gear: &corebattle.BattleGearStats{}}}}}
		c := &corebattle.BookContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: a.ID, PartyIDs: []string{a.ID}, EnemyIDs: []string{"enemy"}}}}
		var d coreskill.BattleDefinitions
		d.RegisterBooks(c)
		entry, err := d.BookAction(&s, c, "book-179", &resetTableRNG{t: t})
		if !equipped {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal("bag-only book", err)
			}
			continue
		}
		if err != nil || entry.MPCost != 48 {
			t.Fatal(entry, err)
		}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{a.ID}}
		rng := &resetTableRNG{t: t}
		ops, err := d.InvocationAction(&s, &f, []string{entry.ID}, rng)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || s.Actors[0].Attack != 200 || rng.draws != 0 || chars.characters[char.ID].Stats != char.Stats {
			t.Fatal(err, s.Actors[0], rng.draws)
		}
	}
}

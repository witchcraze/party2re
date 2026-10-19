package battle_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

type resetTableRNG struct {
	t     *testing.T
	draws int
}

func (r *resetTableRNG) IntN(n int) int   { return r.Intn(n) }
func (r *resetTableRNG) Float64() float64 { r.draws++; return .98 }
func (r *resetTableRNG) Intn(int) int     { r.t.Fatal("unexpected integer draw"); return 0 }

func TestEquipmentAdapterRetainsRawWeaponAndSharedSealLoadThroughNativeReset(t *testing.T) {
	ctx := context.Background()
	chars, inventory, equipment := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
	char := corecharacter.Character{ID: "self", Level: 150, JobID: "job-01", WeaponSeal: 3, Stats: corecharacter.Stats{HP: 50, MaxHP: 100, Attack: 100, Defense: 100, Agility: 100}}
	chars.characters[char.ID] = char
	weapon := coreitem.Instance{ID: "equipped-weapon", DefinitionID: "weapon-69", Quantity: 1}
	if err := inventory.Save(ctx, coreinventory.Inventory{CharacterID: char.ID, Items: []coreitem.Instance{weapon}}); err != nil {
		t.Fatal(err)
	}
	if err := equipment.Save(ctx, coreequipment.Equipment{CharacterID: char.ID, Slots: map[coreitem.Slot]string{coreitem.SlotMainHand: weapon.ID}}); err != nil {
		t.Fatal(err)
	}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(inventory), battle.WithEquipmentRepository(equipment), battle.WithRandomSource(mockRNG{val: 45}), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(ctx, char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	a.TeamID = "party"
	if a.Gear.WeaponAttack == nil || *a.Gear.WeaponAttack != 45 {
		t.Fatal(a.Gear)
	}
	rng := &resetTableRNG{t: t}
	seals, err := battle.BuildWeaponSealTable(a.Participant, rng)
	if err != nil || seals.DragonAttack != 9 || seals.PhoenixWeight != -49 || rng.draws != 1 {
		t.Fatal(seals, rng.draws, err)
	}
	state := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a, {Participant: corebattle.Participant{ID: "npc", TeamID: "npc", HP: 100, Gear: &corebattle.BattleGearStats{}}, NPC: true}}}
	data, err := json.Marshal(struct {
		State corebattle.LifetimeState
		Seals corebattle.WeaponSealTable
	}{state, seals})
	if err != nil {
		t.Fatal(err)
	}
	var loaded struct {
		State corebattle.LifetimeState
		Seals corebattle.WeaponSealTable
	}
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	c := corebattle.EquipmentResetContext{TargetContext: corebattle.TargetContext{ActorID: "npc", PartyIDs: []string{"npc"}, EnemyIDs: []string{"self"}}, RequesterLevel: &char.Level, Seals: &loaded.Seals}
	for _, seal := range []int{3, 6} {
		loaded.State.Actors[0].WeaponSeal = seal
		for range 2 {
			if err := loaded.State.ApplyEquipmentReset(&c, "self", rng); err != nil {
				t.Fatal(err)
			}
			wantAttack, wantAgility := 302, 30
			if seal == 6 {
				wantAttack, wantAgility = 273, 109
			}
			if loaded.State.Actors[0].Attack != wantAttack || loaded.State.Actors[0].Agility != wantAgility || rng.draws != 1 {
				t.Fatal(loaded.State.Actors[0], rng.draws)
			}
		}
	}
}

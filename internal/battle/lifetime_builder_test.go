package battle_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestLifetimeGearUsesEquippedIdentities(t *testing.T) {
	chars, inventory, equipment := newMockCharRepo(), newMockInvRepo(), newMockEquipRepo()
	char := corecharacter.Character{ID: "gear", JobID: "job-01", Stats: corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 100, Defense: 100, Agility: 100}}
	chars.characters[char.ID] = char
	inv, err := coreinventory.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	equip, err := coreequipment.New(char.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"weapon-35", "item-143", "item-149"} {
		instance, err := coreitem.NewInstance(id, 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := inv.Add(instance); err != nil {
			t.Fatal(err)
		}
		if id == "item-143" {
			equip.Slots[coreitem.SlotAccessory] = instance.ID
		}
	}
	if err := inventory.Save(context.Background(), inv); err != nil {
		t.Fatal(err)
	}
	if err := equipment.Save(context.Background(), equip); err != nil {
		t.Fatal(err)
	}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(inventory), battle.WithEquipmentRepository(equipment), battle.WithJobHistoryRepository(&historyReadStub{err: corejob.ErrCharacterJobNotFound}))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil || !reflect.DeepEqual(a.ItemDefinitionIDs, []string{"item-143"}) || a.Gear == nil || a.Gear.WeaponID != "" || a.Gear.AccessoryID != "item-143" {
		t.Fatal(a, err)
	}
}

type historyReadStub struct {
	state corejob.CharacterJob
	err   error
}

func (r *historyReadStub) FindByCharacterID(context.Context, string) (corejob.CharacterJob, error) {
	return r.state, r.err
}

func TestBuildLifetimeActorHistoryIsReadOnly(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-82", SP: 111, OldJobID: "job-36", OldSP: 100, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 10, Defense: 10, Agility: 10}}
	chars.characters[char.ID] = char
	history := &historyReadStub{state: corejob.CharacterJob{CharacterID: char.ID, CurrentJobID: char.JobID, History: []corejob.Change{{FromJobID: "job-01", ToJobID: "job-02"}}, MasteredJobs: []string{"job-03"}, MasteredJobSP: map[string]int{"job-03": 80}, RecordedJobSP: map[string]int{"job-01": 17, "job-03": 20}}}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(history))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "native.svg")
	if err != nil || a.Jobs.JobID != "job-82" || a.Jobs.SP != 111 || a.Jobs.OldSP != 100 || a.OriginalJobs != a.Jobs || a.Icon != "native.svg" || a.NPC || a.Hit != 95 || a.Tension != 1 || a.JobHistory["job-02"] != nil || *a.JobHistory["job-01"] != 17 || *a.JobHistory["job-03"] != 20 {
		t.Fatal(a, err)
	}
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{a}}
	c := corebattle.TransformationContext{EffectContext: corebattle.EffectContext{TargetContext: corebattle.TargetContext{ActorID: char.ID, PartyIDs: []string{char.ID}}}, RequesterGender: "m"}
	if _, err := s.ApplyTransformation(&c, "tool-168-memory", "job-01", random.Default()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyTransformation(&c, "job-82-form", "", random.Default()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(chars.characters[char.ID], char) || history.state.RecordedJobSP["job-01"] != 17 || history.state.CurrentJobID != "job-82" || len(history.state.History) != 1 {
		t.Fatal("temporary identity escaped to permanent state", history.state, chars.characters[char.ID])
	}
	// Snapshot pointers never alias a repository-owned map value.
	*s.Actors[0].JobHistory["job-01"] = 99
	if history.state.RecordedJobSP["job-01"] != 17 {
		t.Fatal(history.state)
	}
	history.err = errors.New("history read failure")
	if _, err := svc.BuildLifetimeActor(context.Background(), char.ID, ""); !errors.Is(err, history.err) {
		t.Fatal(err)
	}
	history.err = corejob.ErrCharacterJobNotFound
	chars.characters[char.ID] = char
	char.Stats.HP = 0
	chars.characters[char.ID] = char
	missing, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil || missing.JobHistory != nil || missing.HP != 0 {
		t.Fatal(missing, err)
	}
}

func TestLifetimeHistoryIgnoresMigratedPointsWithoutRecords(t *testing.T) {
	chars := newMockCharRepo()
	char := corecharacter.Character{ID: "self", JobID: "job-82", SP: 111, OldJobID: "job-36", OldSP: 100, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 10, Defense: 10, Agility: 10}}
	chars.characters[char.ID] = char
	history := &historyReadStub{state: corejob.CharacterJob{CharacterID: char.ID, CurrentJobID: char.JobID, RecordedJobSP: map[string]int{"job-82": 111, "job-36": 100, "job-01": 0}, History: []corejob.Change{{FromJobID: "job-01", ToJobID: "job-82"}, {FromJobID: "job-02", ToJobID: "job-82"}}, MasteredJobs: []string{"job-03"}}}
	svc := battle.NewService(battle.WithCharacterRepository(chars), battle.WithJobHistoryRepository(history))
	a, err := svc.BuildLifetimeActor(context.Background(), char.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"job-82", "job-36"} {
		if _, recorded := a.JobHistory[id]; recorded {
			t.Fatal("migrated points became history", id, a)
		}
	}
	if sp, recorded := a.JobHistory["job-01"]; !recorded || sp == nil || *sp != 0 {
		t.Fatal("recorded zero lost", a)
	}
	for _, id := range []string{"job-02", "job-03"} {
		if sp, recorded := a.JobHistory[id]; !recorded || sp != nil {
			t.Fatal("unknown recorded SP lost", id, a)
		}
	}
}

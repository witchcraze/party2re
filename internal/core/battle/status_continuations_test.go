package battle

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// Five initial enemies are reachable with four players; add-enemy can grow
// a solo encounter to nine enemies (_npc_action:636–640, _skill:9834–9841).
func statusProjectionLoadFixture(enemies, absorbers int) (LifetimeState, StatusContext) {
	s := LifetimeState{Actors: []LifetimeActor{{Participant: Participant{ID: "self", TeamID: "party", HP: 100, MaxHP: 100, Gear: &BattleGearStats{}}, Tension: 1}}}
	c := StatusContext{EffectContext: EffectContext{TargetContext: TargetContext{ActorID: "self", PartyIDs: []string{"self"}, Custom: true}}}
	for i := range enemies {
		id := fmt.Sprintf("enemy-%d", i)
		a := LifetimeActor{Participant: Participant{ID: id, TeamID: "enemy", HP: 100, MaxHP: 100, MaxMP: 1000, Gear: &BattleGearStats{}}, NPC: true, Tension: 1}
		if i < absorbers {
			a.Temporary = TemporaryAbsorbMagic
		}
		s.Actors = append(s.Actors, a)
		c.EnemyIDs = append(c.EnemyIDs, id)
	}
	return s, c
}

func TestStatusContinuationIndexCannotMergeOmittedOrContextFacts(t *testing.T) {
	s, c := statusProjectionLoadFixture(1, 0)
	var set statusContinuations
	if err := set.add(s, c); err != nil {
		t.Fatal(err)
	}
	changed := s.callbackCopy()
	changed.Actors[1].RosterDetached = true // Deliberately omitted by JSON.
	if err := set.add(changed, c); err != nil {
		t.Fatal(err)
	}
	c.AdditionalEffect = true
	if err := set.add(s, c); err != nil {
		t.Fatal(err)
	}
	if err := set.add(s, c); err != nil || len(set.branches) != 3 {
		t.Fatal("index replaced exact equality", len(set.branches), err)
	}
}

func TestStatusContinuationAdmissionBudgetBoundary(t *testing.T) {
	s, c := statusProjectionLoadFixture(1, 0)
	remaining := 1
	set := statusContinuations{remaining: &remaining}
	if err := set.add(s, c); err != nil || remaining != 0 {
		t.Fatal(err, remaining)
	}
	if err := set.add(s, c); err != nil || remaining != 0 || len(set.branches) != 1 {
		t.Fatal("duplicate consumed budget", err, remaining)
	}
	s.DroppedCrystals++
	if err := set.add(s, c); !errors.Is(err, ErrUnsupportedEffect) || len(set.branches) != 1 {
		t.Fatal("budget overrun accepted", err)
	}
}

func TestAreaDeathGemLoadRejectsBeforeIntrinsicExecution(t *testing.T) {
	s, c := statusProjectionLoadFixture(3, 3)
	s.Actors[0].NPC = true // Rewards are unreachable; reset still has real gear.
	c.Defeat = &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c.TargetContext}}
	before := s.callbackCopy()
	if _, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal("area-death guard escaped bounded admission", err, c)
	}
}

package battle

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func equipmentTailFixture(npc bool, count int) (LifetimeState, EquipmentTailContext) {
	var s LifetimeState
	level := 50
	c := EquipmentTailContext{Completion: LifetimeCompletion{Accepted: true, ActorID: "self", NPC: npc, EnemiesRemain: true}}
	c.ActorID, c.ActorNPC, c.RequesterLevel = "self", npc, &level
	for i := range count + 1 {
		id, team := fmt.Sprintf("ally-%d", i), "party"
		if i == 0 {
			id = "self"
		}
		if i == count {
			id, team = "enemy", "other"
			c.EnemyIDs = []string{id}
		} else {
			c.PartyIDs = append(c.PartyIDs, id)
		}
		s.Actors = append(s.Actors, LifetimeActor{Participant: Participant{ID: id, TeamID: team,
			HP: 100, MaxHP: 1000, MP: 100, MaxMP: 1000, Attack: 100, Defense: 100, Agility: 100,
			BaseStats: BattleBaseStats{100, 100, 100}, Gear: &BattleGearStats{}}, Tension: 1, Hit: 80})
	}
	s.actor("self").NPC, s.actor("self").Tension = npc, 3
	s.actor("enemy").HP = 1000
	return s, c
}

func TestEquipmentTail164OrderAndOriginalTemporarySnapshot(t *testing.T) {
	for _, npc := range []bool{false, true} {
		s, c := equipmentTailFixture(npc, 2)
		a := s.actor("self")
		a.ItemDefinitionIDs, a.Temporary = []string{"item-164"}, TemporaryCovering
		var err error
		c.Snapshot, err = s.CaptureEquipmentTail(c.TargetContext)
		if err != nil || !c.Snapshot.HadTemporary || c.Snapshot.Item206 {
			t.Fatal(c.Snapshot, err)
		}
		rng := &targetRNG{t: t, draws: []float64{0}}
		if err := s.ApplyEquipmentTail(&c, "item-164-operation-tail", rng); err != nil || a.Temporary != "" || a.Tension != 5 || c.TensionCalls != 1 {
			t.Fatal(a, c, err)
		}
		// Equipment can change between source boundaries;246 still reads entry tmp.
		a.ItemDefinitionIDs = []string{"item-246"}
		if err := s.ApplyEquipmentTail(&c, "item-246-combat-tail", rng); err != nil || a.Temporary != "" {
			t.Fatal(a, err)
		}
		rng.done()
	}
	for _, draw := range []float64{.25, .999} {
		s, c := equipmentTailFixture(false, 1)
		s.actor("self").ItemDefinitionIDs, s.actor("self").Temporary = []string{"item-164"}, TemporaryDefense
		rng := &targetRNG{t: t, draws: []float64{draw}}
		if err := s.ApplyEquipmentTail(&c, "item-164-operation-tail", rng); err != nil || s.actor("self").Temporary != "" || s.actor("self").Tension != 3 {
			t.Fatal(s, err)
		}
		rng.done()
	}
}

func TestEquipmentTail209AndNPC202ToolBindings(t *testing.T) {
	for _, tc := range []struct {
		id, item string
		tmp      TemporaryEffect
		npc      bool
		draws    []float64
		want     int
	}{
		{"item-209-operation-tail", "item-209", TemporaryCovering, false, []float64{0}, 127},
		{"item-209-operation-tail", "item-209", TemporaryCovering, true, []float64{.999}, 135},
		{"item-209-operation-tail", "item-209", TemporaryCover, false, nil, 100},
		{"item-202-npc-tail", "item-202", "", true, []float64{0, 0}, 127},
		{"item-202-npc-tail", "item-202", "", true, []float64{.999, .999}, 159},
		{"item-202-npc-tail", "item-202", TemporaryDefense, true, nil, 100},
		{"item-202-npc-tail", "item-202", "", false, nil, 100},
	} {
		t.Run(fmt.Sprintf("%s/%t/%s/%d", tc.id, tc.npc, tc.tmp, tc.want), func(t *testing.T) {
			s, c := equipmentTailFixture(tc.npc, 1)
			a := s.actor("self")
			a.ItemDefinitionIDs, a.Temporary = []string{tc.item}, tc.tmp
			rng := &targetRNG{t: t, draws: tc.draws}
			if err := s.ApplyEquipmentTail(&c, tc.id, rng); err != nil || a.HP != tc.want || a.Tension != 3 || c.HealCalls != 0 {
				t.Fatal(a, c, err)
			}
			rng.done()
		})
	}
	//164 clears tmp before the NPC202 predicate is evaluated.
	s, c := equipmentTailFixture(true, 1)
	a := s.actor("self")
	a.ItemDefinitionIDs, a.Temporary = []string{"item-164"}, TemporaryDefense
	rng := &targetRNG{t: t, draws: []float64{.25, 0, 0}}
	if err := s.ApplyEquipmentTail(&c, "item-164-operation-tail", rng); err != nil {
		t.Fatal(err)
	}
	a.ItemDefinitionIDs = []string{"item-202"}
	if err := s.ApplyEquipmentTail(&c, "item-202-npc-tail", rng); err != nil || a.HP != 127 {
		t.Fatal(a, err)
	}
	rng.done()
}

func TestEquipmentTail206SnapshotCureHealChanceAndRestore(t *testing.T) {
	for _, npc := range []bool{false, true} {
		for _, damage := range []bool{false, true} {
			s, c := equipmentTailFixture(npc, 1)
			a := s.actor("self")
			a.ItemDefinitionIDs, a.Status, a.Tension = []string{"item-206"}, StatusSleep, 5
			var err error
			c.Snapshot, err = s.CaptureEquipmentTail(c.TargetContext)
			if err != nil || !c.Snapshot.Item206 {
				t.Fatal(c.Snapshot, err)
			}
			// Frozen206 survives loss of equipment during the action.
			a.ItemDefinitionIDs, a.Status = nil, ""
			draws, hp := []float64{0, .5}, 1000
			if damage {
				draws, hp = []float64{0, 0, 0, 0}, 910
			}
			rng := &targetRNG{t: t, draws: draws}
			if err := s.ApplyEquipmentTail(&c, "item-206-operation-tail", rng); err != nil || a.HP != 370 || a.Tension != 5 || s.actor("enemy").HP != hp || c.HealCalls != 1 || (c.DamageCalls == 1) != damage {
				t.Fatal(a, c, s.actor("enemy"), err)
			}
			rng.done()
		}
	}
	s, c := equipmentTailFixture(false, 1)
	c.Snapshot.Item206 = true
	s.actor("self").Status = StatusSleep
	if err := s.ApplyEquipmentTail(&c, "item-206-operation-tail", &targetRNG{t: t}); err != nil || s.actor("self").HP != 100 {
		t.Fatal(s, err)
	}
}

func TestEquipmentTail208RosterBoundariesAndTensionRestoration(t *testing.T) {
	for _, npc := range []bool{false, true} {
		for count := 2; count <= 6; count++ {
			t.Run(fmt.Sprintf("npc%t/size%d", npc, count), func(t *testing.T) {
				s, c := equipmentTailFixture(npc, count)
				s.actor("self").ItemDefinitionIDs = []string{"item-208"}
				s.actor("self").Status = StatusSleep
				s.actor("ally-1").Status = StatusPoison // Nonholder keeps status.
				s.actor("ally-1").Hit, s.actor("ally-1").MaxHP = 99, 3000
				if count >= 3 {
					s.actor("ally-2").HP = 0 // Still counts and gains hit95; cure cannot revive.
					s.actor("ally-2").ItemDefinitionIDs, s.actor("ally-2").Status = []string{"item-208"}, StatusSleep
				}
				var draws []float64
				if count >= 4 {
					for range count {
						draws = append(draws, 0, 0)
					}
				}
				if count >= 5 {
					for i := range count {
						if i != 2 {
							draws = append(draws, 0)
						}
					}
				}
				if count >= 6 {
					for i := range count {
						v := 1.0 / 3
						if i%2 == 0 {
							v = 0
						}
						draws = append(draws, v)
					}
				}
				rng := &targetRNG{t: t, draws: draws}
				if err := s.ApplyEquipmentTail(&c, "item-208-combat-tail", rng); err != nil {
					t.Fatal(err)
				}
				wantHP, wantEnemy, wantTension := 100, 1000, 3.0
				if count >= 4 {
					wantEnemy -= count * 9
				}
				if count >= 5 {
					wantHP = 190
				}
				if count >= 6 {
					wantTension = 5
				}
				a := s.actor("self")
				if a.HP != wantHP || s.actor("enemy").HP != wantEnemy || a.Tension != wantTension || s.actor("ally-1").Status != StatusPoison || s.actor("ally-1").Hit != 99 {
					t.Fatal(s)
				}
				if count >= 3 && (a.Status != "" || a.Hit != 95 || s.actor("ally-2").HP != 0 || s.actor("ally-2").Hit != 95 || s.actor("ally-2").Status != StatusSleep || c.CureCalls != 2) {
					t.Fatal(s, c)
				}
				if count == 2 && (a.Status != StatusSleep || a.Hit != 80) {
					t.Fatal(s)
				}
				if count >= 5 && s.actor("ally-1").HP != 280 {
					t.Fatal("200 cap", s.actor("ally-1"))
				}
				if count == 6 && (s.actor("ally-2").Tension != 1.7 || s.actor("ally-1").Tension != 1 || c.TensionCalls != 3) {
					t.Fatal(s, c)
				}
				rng.done()
			})
		}
	}
}

func TestEquipmentTailGuardianPresenceAndReflectionKinds(t *testing.T) {
	for _, npc := range []bool{false, true} {
		for _, present := range []bool{false, true} {
			for _, hp := range []int{0, 100} {
				s, c := equipmentTailFixture(npc, 2)
				s.actor("self").HP, s.actor("ally-1").HP = hp, 0
				if present {
					s.actor("ally-1").RosterIdentity = "guardian"
				}
				var draws []float64
				want := 100
				if present && (npc || hp > 0) {
					draws = append(draws, 0)
					if hp > 0 {
						draws = append(draws, 0)
						want = 104
					}
				}
				rng := &targetRNG{t: t, draws: draws}
				if err := s.ApplyEquipmentTail(&c, "guardian-mp-tail", rng); err != nil || s.actor("self").MP != want || c.MPHealCalls != 0 {
					t.Fatal(s, c, err)
				}
				rng.done()
			}
		}
		for i, kind := range []TemporaryEffect{TemporaryReflectAttack, TemporaryReflectMagic, TemporaryReflectDance, TemporaryReflectBreath} {
			s, c := equipmentTailFixture(npc, 1)
			s.actor("self").ItemDefinitionIDs, s.actor("self").Temporary = []string{"item-246"}, TemporaryDefense
			rng := &targetRNG{t: t, draws: []float64{float64(i) / 4}}
			if err := s.ApplyEquipmentTail(&c, "item-246-combat-tail", rng); err != nil || s.actor("self").Temporary != kind {
				t.Fatal(s, err)
			}
			rng.done()
		}
	}
}

func TestEquipmentTailUnsupportedLaterDamageIsAtomic(t *testing.T) {
	for _, row := range []struct {
		id     string
		count  int
		shield bool
		hp     int
	}{
		{"item-206-operation-tail", 1, false, 50},
		{"item-208-combat-tail", 4, true, 1000},
		{"item-208-combat-tail", 4, false, 30}, // Individually safe hits become lethal.
	} {
		s, c := equipmentTailFixture(false, row.count)
		c.Snapshot.Item206 = true
		s.actor("self").ItemDefinitionIDs = []string{"item-208"}
		s.actor("self").Status = ""
		s.actor("enemy").HP = row.hp
		if row.shield {
			s.actor("enemy").Temporary, s.actor("enemy").Gear = TemporaryOneHitDefense, nil
		}
		before, context := s.callbackCopy(), c
		if err := s.ApplyEquipmentTail(&c, row.id, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || !reflect.DeepEqual(context, c) {
			t.Fatal(row, s, c, err)
		}
	}
}

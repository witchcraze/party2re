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

func TestStatusProjectionLoadCompletenessAndRejection(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		enemies, absorbers, want int
	}{
		{"normal5", 5, 0, 32}, {"normal9", 9, 0, 512}, {"absorb2", 2, 2, 1600},
		{"normal5-absorb2-reject", 5, 2, 0}, {"absorb3-reject", 3, 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := statusProjectionLoadFixture(tc.enemies, tc.absorbers)
			before := s.callbackCopy()
			branches, err := s.ProjectStatusGemBranches(&c, "gem_inflict_confusion", "")
			if tc.want == 0 {
				if !errors.Is(err, ErrUnsupportedEffect) || len(branches) != 0 {
					t.Fatal("partial projection escaped", len(branches), err)
				}
			} else if err != nil || len(branches) != tc.want {
				t.Fatal(len(branches), err)
			}
			if !reflect.DeepEqual(before, s) || c.admissionBudget != nil {
				t.Fatal("load admission changed caller")
			}
			if tc.name == "absorb2" {
				seen := map[[4]int]bool{}
				for _, branch := range branches {
					a, z := branch.State.Actors[1], branch.State.Actors[2]
					flag := func(a LifetimeActor) int {
						if a.Status == StatusConfusion {
							return 1
						}
						return 0
					}
					seen[[4]int{a.MP, z.MP, flag(a), flag(z)}] = true
				}
				for a := 30; a < 50; a++ {
					for z := 30; z < 50; z++ {
						for mask := range 4 {
							if !seen[[4]int{a, z, mask & 1, mask >> 1}] {
								t.Fatal("lost independent absorption/chance", a, z, mask)
							}
						}
					}
				}
			}
		})
	}
}

func TestStatusProjectionSaturatedAbsorptionCoalescesExactly(t *testing.T) {
	s, c := statusProjectionLoadFixture(5, 5)
	for i := 1; i < len(s.Actors); i++ {
		s.Actors[i].MP, s.Actors[i].MaxMP = 34, 35
	}
	branches, err := s.ProjectStatusGemBranches(&c, "gem_inflict_confusion", "")
	if err != nil || len(branches) != 32 {
		t.Fatal(len(branches), err)
	}
	for _, branch := range branches {
		for _, a := range branch.State.Actors[1:] {
			if a.MP != 35 {
				t.Fatal(a)
			}
		}
	}
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

func BenchmarkStatusGemProjectionLoad(b *testing.B) {
	for _, tc := range []struct {
		name               string
		enemies, absorbers int
	}{{"normal5", 5, 0}, {"absorb1", 1, 1}, {"absorb2", 2, 2}, {"normal9", 9, 0}} {
		b.Run(tc.name, func(b *testing.B) {
			s, c := statusProjectionLoadFixture(tc.enemies, tc.absorbers)
			b.ReportAllocs()
			for b.Loop() {
				branches, err := s.ProjectStatusGemBranches(&c, "gem_inflict_confusion", "")
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(len(branches)), "branches/op")
			}
		})
	}
}

func BenchmarkStatusGemProjectionRejectedLoad(b *testing.B) {
	for _, death := range []bool{false, true} {
		name := "absorb3"
		if death {
			name = "death-absorb3"
		}
		b.Run(name, func(b *testing.B) {
			s, c := statusProjectionLoadFixture(3, 3)
			s.Actors[0].NPC = true
			c.Defeat = &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: c.TargetContext}}
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if death {
					err = s.ValidateStatusEntry(&c, nil, "gem_instant_death_sky", "", resetBoundary(0))
				} else {
					_, err = s.ProjectStatusGemBranches(&c, "gem_inflict_confusion", "")
				}
				if !errors.Is(err, ErrUnsupportedEffect) {
					b.Fatal(err)
				}
			}
		})
	}
}

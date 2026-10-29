package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestTreatmentMischiefInertAdmissionAndWholeActionRejection(t *testing.T) {
	_, _, catalog := treatmentFixture()
	for _, job := range []int{50, 51, 55, 56, 57, 78} {
		for _, entry := range catalog.Invocations.JobTables[job].Variants[0] {
			if catalog.Invocations.Entries[entry.ID].Execute == nil {
				continue
			}
			t.Run(entry.ID, func(t *testing.T) {
				for _, reject := range []bool{false, true} {
					s, c, d := treatmentFixture()
					args := []string{"self", "tail"}
					// The other owners' sacrifice/asset bodies are checked separately.
					if entry.ID == "job-50-entry-6" {
						continue
					}
					c.Reset = func(string) error { t.Fatal("reset during admission"); return nil }
					c.Defeat = func(string) error { t.Fatal("defeat during admission"); return nil }
					c.Status.Defeat.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
						t.Fatal("reward during admission")
						return nil
					}
					gear := s.Actors[0].Gear
					weapon := 7
					gear.WeaponAttack = &weapon
					before, err := json.Marshal(s)
					if err != nil {
						t.Fatal(err)
					}
					f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
					original := f
					ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: entry.ID}}
					if reject {
						ops = append(ops, corebattle.CallbackInvocation{Kind: "entry", EntryID: "missing"})
					}
					err = s.ValidateInvocations(&f, d.Invocations, ops, noArithmeticDraws{t})
					if (err != nil) != reject {
						t.Fatal(reject, err)
					}
					after, err := json.Marshal(s)
					if err != nil {
						t.Fatal(err)
					}
					if string(before) != string(after) || !reflect.DeepEqual(f, original) || c.Arguments != nil || c.DamageCalls != 0 || c.HealCalls != 0 || c.Status.DeathCalls != 0 || weapon != 7 || gear.WeaponAttack != &weapon || s.Actors[0].Gear != gear {
						t.Fatal(s, c, f)
					}
				}
			})
		}
	}
}

func TestTreatmentMischiefMissingDescendantsRejectBeforeEarlierHeal(t *testing.T) {
	for _, bad := range []string{"damage", "death", "later-status", "tension", "empty-armor", "later-MP"} {
		t.Run(bad, func(t *testing.T) {
			s, c, d := treatmentFixture()
			id, args := "job-51-entry-4", []string{"self"}
			switch bad {
			case "damage":
				c.Defeat = nil
			case "death":
				id, c.Status.Defeat = "job-51-entry-6", nil
			case "later-status":
				id = "job-51-entry-3"
				other := s.Actors[1]
				other.ID, other.Gear = "other", nil
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, other.ID)
			case "tension":
				id = "job-55-entry-3"
				s.Actors[1].Tension = 5
			case "empty-armor":
				id = "job-50-entry-3"
				s.Actors[0].Status = corebattle.StatusMagicSeal
			case "later-MP":
				id = "job-57-entry-5"
			}
			ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}
			if bad == "later-MP" || bad == "later-status" {
				ops = append([]corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-50-entry-0"}}, ops...)
			}
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || c.HealCalls != 0 || c.DamageCalls != 0 || c.Status.StatusCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestTreatmentMischiefEveryPositionGuardBoundary(t *testing.T) {
	_, _, catalog := treatmentFixture()
	for _, job := range []int{50, 51, 55, 56, 57, 78} {
		for _, entry := range catalog.Invocations.JobTables[job].Variants[0] {
			id := entry.ID
			if catalog.Invocations.Entries[id].Execute == nil || id == "job-50-entry-5" || id == "job-50-entry-6" || id == "job-51-entry-5" || job == 78 && id != "job-78-entry-3" {
				continue
			}
			t.Run(id, func(t *testing.T) {
				s, c, d := treatmentFixture()
				args, draws := []string{"enemy"}, []float64(nil)
				switch id {
				case "job-50-entry-0", "job-50-entry-1", "job-50-entry-4", "job-57-entry-0", "job-57-entry-2", "job-56-entry-1":
					args[0], s.Actors[0].Status, draws = "self", corebattle.StatusMagicSeal, []float64{.75}
				case "job-50-entry-2":
					s.Actors[1].HP = 1000
					c.Status.Defeat = nil
				case "job-50-entry-3":
					args[0], s.Actors[0].Defense = "self", 200
				case "job-51-entry-0":
					s.Actors[1].Hit = 50
				case "job-51-entry-1":
					s.Actors[2].Hit = 95
				case "job-51-entry-2":
					s.Actors[0].Status, draws = corebattle.StatusMagicSeal, []float64{0, .75}
				case "job-51-entry-3":
					s.Actors[1].HP, s.Actors[1].Gear, draws = 1000, nil, []float64{0}
				case "job-51-entry-4", "job-55-entry-4", "job-56-entry-0", "job-56-entry-2", "job-56-entry-4":
					s.Actors[0].HP = 0
				case "job-51-entry-6":
					s.Actors[1].HP, s.Actors[1].Temporary, c.Status.Defeat = 1000, corebattle.TemporaryOneHitDefense, nil
				case "job-51-entry-7":
					s.Actors[0].MaxMP = 0
				case "job-55-entry-0":
					s.Actors[2].Hit, draws = 0, []float64{.7}
				case "job-55-entry-1", "job-57-entry-5":
					s.Actors[1].MP = 20
				case "job-55-entry-2":
					s.Actors[2].Status, draws = corebattle.StatusMagicSeal, []float64{.7}
				case "job-55-entry-3":
					draws = []float64{.499}
				case "job-55-entry-5":
					s.Actors[1].Status = corebattle.StatusSabaku
				case "job-57-entry-1", "job-57-entry-3":
					s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
				case "job-57-entry-4":
					args[0] = "self"
				case "job-57-entry-6":
					s.Actors[0].Attack = 250
				case "job-78-entry-3":
					s.Actors[0].HP = 0
				default:
					t.Fatal("missing boundary", id)
				}
				before := append([]corebattle.LifetimeActor(nil), s.Actors...)
				executeTreatment(t, &s, c, d, id, args, draws...)
				for i, a := range s.Actors {
					if a.HP != before[i].HP || a.MP != before[i].MP || a.Hit != before[i].Hit || a.Status != before[i].Status || a.Tension != before[i].Tension {
						t.Fatal(before, s.Actors)
					}
				}
				if id == "job-50-entry-3" && s.Actors[0].Temporary != corebattle.TemporaryReduceBreath {
					t.Fatal(s.Actors[0])
				}
				if id == "job-78-entry-3" && s.Actors[0].Defense != 64 {
					t.Fatal(s.Actors[0])
				}
			})
		}
	}
}

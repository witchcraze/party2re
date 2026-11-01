package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func runNonJobInvocation(t *testing.T, s *corebattle.LifetimeState, f *corebattle.CallbackContext, d BattleDefinitions, ids []string, rng random.Generator) {
	t.Helper()
	ops, err := d.InvocationAction(s, f, ids, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(f, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
}

// ff97fc5:_data.cgi:1255–1267,2171,2174–2180: outer CMP is not child MP.
func TestNonJobInvocationRegistration(t *testing.T) {
	var d BattleDefinitions
	d.RegisterNonJobInvocations()
	want := map[string]struct {
		kind string
		cost int
	}{
		"tool-84": {"tool-84", 0}, "tool-85": {"random", 0}, "gem-83": {"random", 5},
		"gem-86": {"gem-86", 20}, "gem-87": {"gem-87", 15}, "gem-88": {"gem-88", 1},
		"gem-89": {"gem-89", 15}, "gem-90": {"gem-90", 20}, "gem-91": {"gem-91", 15}, "gem-92": {"gem-92", 40},
	}
	if len(d.Invocations.Entries) != len(want) || len(NonJobInvocationEntries()) != len(want) {
		t.Fatal("incomplete wrapper subset")
	}
	for _, entry := range NonJobInvocationEntries() {
		row, ok := want[entry.ID]
		if !ok || entry.CMPCost != row.cost || entry.Invocation.Kind != row.kind || d.Invocations.Entries[entry.ID].Invocation != entry.Invocation {
			t.Fatal(entry)
		}
	}
	for _, id := range []string{"tool-110", "gem-101", "gem-102"} {
		if _, ok := d.Invocations.Entries[id]; ok {
			t.Fatal("source failure entry enabled", id)
		}
	}
}

// Concrete calling bodies, not stand-ins: old59 wins over current60, then
// non-calling old01 permits current59. SP0/MP0 waive listed child costs.
func TestNonJobInvocationCallingBodies(t *testing.T) {
	for _, previous := range []bool{false, true} {
		for gem := 87; gem <= 92; gem++ {
			t.Run(fmt.Sprintf("gem-%d/previous-%t", gem, previous), func(t *testing.T) {
				s, c, d := callingManifestFixture()
				s.Actors[0].MP = 0
				if !previous {
					s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.OldJobID = "job-59", "job-01"
				}
				d.RegisterCallingJobs(&c)
				d.RegisterNonJobInvocations()
				if gem != 87 {
					addCallingManifestHelper(&s, &c, 59)
				}
				f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				runNonJobInvocation(t, &s, &f, d, []string{fmt.Sprintf("gem-%d", gem)}, callingZeroDraw{})
				name := map[int]string{87: "summon", 88: "dismiss", 89: "attack", 90: "special", 91: "defend", 92: "merge"}[gem]
				checkCallingIntrinsic(t, &s, &c, name, 59, [5]int{63, 108, 45, 27, 108})
				wantMP := 0
				if gem == 92 {
					wantMP = 10 // The intrinsic merge's MP increase survives.
				}
				if s.Actors[0].MP != wantMP || f.ActorID != "self" || s.Actors[3].RosterIdentity != "calling-59" || !reflect.DeepEqual(f.Arguments, []string{"enemy", "tail"}) {
					t.Fatal(s, f)
				}
			})
		}
	}
}

func TestNonJobInvocationTool84ConcreteTables(t *testing.T) {
	t.Run("extended and MP increase", func(t *testing.T) {
		s, c, d := callingManifestFixture()
		s.Actors[0].Jobs.JobID, s.Actors[0].MP = "job-59", 0
		addCallingManifestHelper(&s, &c, 59)
		d.RegisterCallingJobs(&c)
		d.RegisterNonJobInvocations()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		// Position5 merges despite listed100MP/SP150; MP gained is not refunded away.
		runNonJobInvocation(t, &s, &f, d, []string{"tool-84"}, &damageGemDraws{t: t, draws: []float64{5.5 / 7}})
		if s.Actors[0].MP != 10 || s.Actors[0].Attack != 200 || s.Actors[3].HP != 0 {
			t.Fatal(s)
		}
		s, c, d = callingManifestFixture()
		s.Actors[0].Jobs.JobID, s.Actors[0].MP = "job-59", 0
		addCallingManifestHelper(&s, &c, 59)
		d.RegisterCallingJobs(&c)
		d.RegisterNonJobInvocations()
		f.TargetContext = c.TargetContext
		// Last position is extended ride, still selectable by tool84.
		runNonJobInvocation(t, &s, &f, d, []string{"tool-84"}, &damageGemDraws{t: t, draws: []float64{.99, 0}})
		if s.Actors[0].MP != 0 || s.Actors[1].HP != 9914 || s.Actors[3].HP != 100 {
			t.Fatal(s)
		}
	})
	t.Run("current300", func(t *testing.T) {
		s, c, d := transformationRegistryFixture()
		s.Actors[0].Jobs.JobID, s.Actors[0].MP = "job-300", 0
		c.Damage = func(corebattle.TargetContext, corebattle.TargetEffect, random.Generator) error {
			t.Fatal("unselected attack")
			return nil
		}
		d.RegisterTransformations(&c)
		d.RegisterNonJobInvocations()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
		runNonJobInvocation(t, &s, &f, d, []string{"tool-84"}, &damageGemDraws{t: t, draws: []float64{0, 0, .99}})
		if s.Actors[0].Attack != 150 || s.Actors[0].Agility != 150 || s.Actors[0].MP != 0 {
			t.Fatal(s)
		}
	})
	for _, npc := range []bool{false, true} {
		t.Run(fmt.Sprintf("current402/npc-%t", npc), func(t *testing.T) {
			s, c, d := npc402Fixture()
			s.Actors[0].NPC, c.ActorNPC = npc, npc
			d.RegisterNonJobInvocations()
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "enemy", "enemy", "tail"}}
			runNonJobInvocation(t, &s, &f, d, []string{"tool-84"}, &damageGemDraws{t: t, draws: []float64{0, 0, 0, 0}})
			if s.Actors[1].HP != 703 || s.Actors[0].MP != 80 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) || c.DamageCalls != 3 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestNonJobInvocationExtendedGuards(t *testing.T) {
	for _, row := range []struct {
		job            string
		sp, mp, wantMP int
		applied        bool
	}{
		{"job-59", 149, 100, 100, false}, // Handled SP failure prevents previous63.
		{"job-59", 150, 14, 14, false},   // Handled MP failure prevents previous63.
		{"job-59", 150, 15, 0, true},
		{"job-60", 0, 15, 0, true}, // No current extended entry falls back to old59.
	} {
		t.Run(fmt.Sprint(row), func(t *testing.T) {
			s, c, d := callingManifestFixture()
			s.Actors[0].Jobs = corebattle.BattleJobs{JobID: row.job, SP: row.sp, OldJobID: "job-63", OldSP: 150}
			if row.job == "job-60" {
				s.Actors[0].Jobs.OldJobID = "job-59"
			}
			s.Actors[0].MP = row.mp
			addCallingManifestHelper(&s, &c, 59)
			d.RegisterCallingJobs(&c)
			d.RegisterNonJobInvocations()
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			runNonJobInvocation(t, &s, &f, d, []string{"gem-86"}, callingZeroDraw{})
			if (s.Actors[1].HP == 9914) != row.applied || s.Actors[0].MP != row.wantMP || len(s.Actors) != 4 {
				t.Fatal(s)
			}
		})
	}
	for _, attack := range []int{998, 999} {
		for _, mp := range []int{0, 1} {
			s, c := recoveryGemFixture(false)
			s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-73", OldJobID: "job-59", OldSP: 150}
			s.Actors[0].Attack, s.Actors[0].MP = attack, mp
			var d BattleDefinitions
			d.RegisterNonJobInvocations() // No item158, SP, tables or child ports.
			f := corebattle.CallbackContext{TargetContext: c.TargetContext}
			runNonJobInvocation(t, &s, &f, d, []string{"gem-86"}, noArithmeticDraws{t})
			want := attack
			if attack < 999 && mp > 0 {
				want += 3
			}
			if s.Actors[0].Attack != want || s.Actors[0].MP != 0 {
				t.Fatal(s)
			}
		}
	}
}

func TestNonJobInvocationMissingDescendantsRejectWithoutMutation(t *testing.T) {
	s, c, d := callingManifestFixture()
	d.RegisterCallingJobs(&c)
	d.RegisterNonJobInvocations()
	// Full player tables retain absent asset owners; general invocation must
	// reject rather than drawing only the currently executable subset.
	d.RegisterPlayerCallbacks(PlayerCallbackContexts{Roster: &c}, len(c.PartyIDs))
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	args, party, gear := append([]string(nil), f.Arguments...), append([]string(nil), f.PartyIDs...), *s.Actors[0].Gear
	for _, ids := range [][]string{{"tool-85"}, {"gem-83"}, {"gem-87", "gem-83"}, {"gem-87", "tool-110"}, {"gem-101"}, {"gem-102"}} {
		if _, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		ops := make([]corebattle.CallbackInvocation, len(ids))
		for i, id := range ids {
			ops[i] = corebattle.CallbackInvocation{Kind: "entry", EntryID: id}
		}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
	}
	after, err := json.Marshal(s)
	if err != nil || string(before) != string(after) || !reflect.DeepEqual(args, f.Arguments) || !reflect.DeepEqual(party, f.PartyIDs) || !reflect.DeepEqual(gear, *s.Actors[0].Gear) {
		t.Fatal("rejection mutated caller", err)
	}
}

package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestDefenseBranchesComposeRegisteredSealOutcomes(t *testing.T) {
	for _, npc := range []bool{false, true} {
		for _, cured := range []bool{false, true} {
			for _, reload := range []bool{false, true} {
				s, c := recoveryGemFixture(npc)
				s.Actors[0].Status = corebattle.StatusMagicSeal
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "frozen-tail"}}
				input := struct {
					State   corebattle.LifetimeState
					Context corebattle.TargetContext
					Args    []string
				}{s, frame.TargetContext, frame.Arguments}
				data, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				if reload {
					if err := json.Unmarshal(data, &input); err != nil {
						t.Fatal(err)
					}
					s = input.State
					frame = corebattle.CallbackContext{TargetContext: input.Context, Arguments: input.Args}
				}
				var d BattleDefinitions
				d.RegisterDefenseGems()
				ids := []string{"gem_guard_2", "gem_counter_magic_2", "gem_awakening_sky"}
				ops, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t})
				if err != nil {
					t.Fatal(npc, cured, reload, err)
				}
				after, _ := json.Marshal(struct {
					State   corebattle.LifetimeState
					Context corebattle.TargetContext
					Args    []string
				}{s, frame.TargetContext, frame.Arguments})
				if string(data) != string(after) {
					t.Fatal("admission changed frozen input")
				}
				draws := []float64{.25, .25}
				if cured {
					draws = []float64{.2499}
				}
				rng := &queuedRecoveryGemRNG{t: t, draws: draws}
				if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || len(rng.draws) != 0 {
					t.Fatal(npc, cured, reload, err, rng.draws)
				}
				wantStatus, wantTemporary := corebattle.StatusMagicSeal, corebattle.TemporaryEffect("")
				if cured {
					wantStatus, wantTemporary = "", corebattle.TemporaryReflectMagic
				}
				if s.Actors[0].Status != wantStatus || s.Actors[2].Temporary != wantTemporary || s.Actors[2].ExtendedTurns != 8 || s.Actors[0].CMP != 200 || !reflect.DeepEqual(frame.Arguments, []string{"ally", "frozen-tail"}) {
					t.Fatal(npc, cured, reload, s, frame)
				}
				facts, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				var restored corebattle.LifetimeState
				if err := json.Unmarshal(facts, &restored); err != nil || !reflect.DeepEqual(s, restored) {
					t.Fatal("resulting facts changed across reload", err)
				}
			}
		}
	}
}

func TestDefenseBranchesCheckFailedCureBeforeUncheckedLaterBody(t *testing.T) {
	s, c := recoveryGemFixture(false)
	s.Actors[0].Status = corebattle.StatusMagicSeal
	var d BattleDefinitions
	d.RegisterDefenseGems()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
	before, _ := json.Marshal(s)
	for _, later := range []string{"gem_nullify_phys", "gem_nullify_magic_1", "gem_counter_phys_sky", "missing"} {
		ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem_guard_2"}, {Kind: "entry", EntryID: later}}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(later, err)
		}
		after, _ := json.Marshal(s)
		if string(before) != string(after) || !reflect.DeepEqual(f.Arguments, []string{"ally"}) {
			t.Fatal("rejection changed state or arguments")
		}
	}
}

func TestDefenseBranchesKeepAbsorptionUnknown(t *testing.T) {
	s, c := recoveryGemFixture(false)
	s.Actors[2].Temporary = corebattle.TemporaryAbsorbMagic
	s.Actors[2].MP = 1
	var d BattleDefinitions
	d.RegisterDefenseGems()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
	before := s.Actors[2]
	if _, err := d.InvocationAction(&s, &f, []string{"gem_guard_2", "gem_guard_1"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(s.Actors[2], before) {
		t.Fatal("resource-incomplete projection was certified", err, s)
	}
}

func TestDefenseBranchesPreserveConfusionCoverAndFallenPaths(t *testing.T) {
	for _, confusion := range []bool{false, true} {
		s, c := recoveryGemFixture(false)
		c.RequesterLevel = nil
		s.Actors[0].Temporary = corebattle.TemporaryCover
		s.Actors[2].Temporary = corebattle.TemporaryCovering
		draws := []float64{.4, .4, .4} // Fallen ally -> self; cover redirects only the first slot.
		if confusion {
			s.Actors[0].Status = corebattle.StatusMagicSeal
			c.Item195Activated = true
			draws = []float64{.2499, 0, .99, .5, .5} // Cure, activate confusion, enemy, ally, ally.
		}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		var d BattleDefinitions
		d.RegisterDefenseGems()
		ops, err := d.InvocationAction(&s, &f, []string{"gem_guard_2", "gem_counter_magic_2", "gem_awakening_sky"}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(confusion, err)
		}
		rng := &queuedRecoveryGemRNG{t: t, draws: draws}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || len(rng.draws) != 0 {
			t.Fatal(confusion, err, rng.draws)
		}
		if confusion {
			if s.Actors[0].Status != corebattle.StatusConfusion || s.Actors[3].Temporary != corebattle.TemporaryDefense || s.Actors[2].Temporary != corebattle.TemporaryReflectMagic || s.Actors[2].ExtendedTurns != 8 {
				t.Fatal(s)
			}
		} else if s.Actors[0].Temporary != corebattle.TemporaryReflectMagic || s.Actors[0].ExtendedTurns != 8 || s.Actors[2].Temporary != corebattle.TemporaryDefense {
			t.Fatal(s)
		}
		if s.Actors[1].HP != 0 || s.Actors[1].Temporary != "" || s.Actors[0].MP != 100 || s.Actors[0].CMP != 200 {
			t.Fatal("changed unrelated resources/fallen actor", s)
		}
	}
}

func TestDefenseBranchesAdmitSkippedExtendedWithoutChildContext(t *testing.T) {
	for _, cured := range []bool{false, true} {
		s, c := recoveryGemFixture(false)
		s.Actors[0].Status, s.Actors[0].MP = corebattle.StatusMagicSeal, 0
		s.Actors[0].Jobs.JobID = "job-73"
		var d BattleDefinitions
		d.RegisterDefenseGems()
		d.RegisterNonJobInvocations()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
		ops, err := d.InvocationAction(&s, &f, []string{"gem_guard_2", "gem-86"}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(cured, err)
		}
		draw := .25
		if cured {
			draw = .2499
		}
		rng := &queuedRecoveryGemRNG{t: t, draws: []float64{draw}}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || len(rng.draws) != 0 || s.Actors[0].MP != 0 || s.Actors[0].Attack != 150 {
			t.Fatal(cured, err, s)
		}
		if (s.Actors[2].Temporary == corebattle.TemporaryDefense) != cured {
			t.Fatal(cured, s)
		}
	}
}

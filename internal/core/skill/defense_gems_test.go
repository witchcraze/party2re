package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestDefenseGemRegistrationAndBodies(t *testing.T) {
	costs := map[int]int{34: 20, 35: 33, 36: 40, 37: 37, 38: 24, 39: 12, 40: 7, 41: 24, 42: 10, 43: 19, 44: 30, 45: 35, 46: 8, 47: 17, 48: 28, 49: 14, 50: 16, 51: 36, 52: 2, 53: 40, 84: 6, 85: 13}
	temps := map[int]corebattle.TemporaryEffect{34: corebattle.TemporaryDefense, 35: corebattle.TemporaryDefense, 36: corebattle.TemporaryDefense, 37: corebattle.TemporaryStrongDefense, 38: corebattle.TemporaryReduceAttack, 39: corebattle.TemporaryReduceMagic, 40: corebattle.TemporaryReduceBreath, 41: corebattle.TemporaryNullAttack, 42: corebattle.TemporaryNullMagic, 43: corebattle.TemporaryNullMagic, 44: corebattle.TemporaryReflectAttack, 45: corebattle.TemporaryReflectAttack, 46: corebattle.TemporaryReflectMagic, 47: corebattle.TemporaryReflectMagic, 48: corebattle.TemporaryReflectMagic, 49: corebattle.TemporaryReflectBreath, 50: corebattle.TemporaryRegeneration, 51: corebattle.TemporaryRevival, 52: corebattle.TemporaryCover, 53: corebattle.TemporaryAbsorbMagic}
	if len(corebattle.DefenseGemEntries()) != len(costs) {
		t.Fatal("incomplete subset")
	}
	for _, npc := range []bool{false, true} {
		for _, e := range corebattle.DefenseGemEntries() {
			t.Run(e.ID+map[bool]string{true: "/npc", false: "/player"}[npc], func(t *testing.T) {
				s, c := recoveryGemFixture(npc)
				c.RequesterLevel = nil // These producers have no requester dependency.
				var d BattleDefinitions
				if _, err := d.DefenseGemAction(&s, c.TargetContext, []string{e.ID}, "ally", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(err)
				}
				d.RegisterDefenseGems()
				before, _ := json.Marshal(s)
				category := corebattle.CategoryMagic
				if e.SourceID == 34 || e.SourceID == 37 || e.SourceID == 84 || e.SourceID == 85 {
					category = corebattle.CategoryUntyped
				} else if e.SourceID == 52 {
					category = corebattle.CategoryPhysical
				}
				if e.Category != category {
					t.Fatal(e, category)
				}
				entries, err := d.DefenseGemAction(&s, c.TargetContext, []string{e.ID}, "ally", noArithmeticDraws{t})
				after, _ := json.Marshal(s)
				if err != nil || len(entries) != 1 || entries[0] != e || costs[e.SourceID] != e.CMPCost || string(before) != string(after) {
					t.Fatal(entries, err)
				}
				f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "tail"}}
				ops, err := d.InvocationAction(&s, &f, []string{e.ID}, noArithmeticDraws{t})
				if err != nil {
					t.Fatal(err)
				}
				if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
					t.Fatal(err)
				}
				for i, a := range s.Actors {
					want := corebattle.TemporaryEffect("")
					selected := (e.TargetScope == corebattle.TargetScopeSelf && i == 0) || (e.TargetScope == corebattle.TargetScopeSingleAlly && i == 2) || (e.TargetScope == corebattle.TargetScopeAllAllies && (i == 0 || i == 2))
					if selected {
						want = temps[e.SourceID]
					}
					if e.SourceID == 52 && i == 0 {
						want = corebattle.TemporaryCovering
					}
					if a.Temporary != want || a.MP != 100 || a.CMP != 200 {
						t.Fatal(e, a, want)
					}
					if e.SourceID >= 84 && selected && (a.Extended != corebattle.ExtendedAwakening || a.ExtendedTurns != 8) {
						t.Fatal(e, a)
					}
				}
			})
		}
	}
}

func TestDefenseGemRegisteredGuards(t *testing.T) {
	for _, e := range corebattle.DefenseGemEntries() {
		for _, draw := range []float64{.2499, .25} {
			s, c := recoveryGemFixture(false)
			s.Actors[0].Status = corebattle.StatusMagicSeal
			var d BattleDefinitions
			d.RegisterDefenseGems()
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
			before, _ := json.Marshal(s)
			unchecked := e.SourceID == 41 || e.SourceID == 42 || e.SourceID == 45
			_, err := d.InvocationAction(&s, &f, []string{e.ID}, noArithmeticDraws{t})
			if unchecked {
				if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(e, err)
				}
				if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: e.ID}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(e, err)
				}
				after, _ := json.Marshal(s)
				if string(before) != string(after) {
					t.Fatal("rejection mutated state", e)
				}
				continue
			}
			if err != nil {
				t.Fatal(e, err)
			}
			rng := &queuedRecoveryGemRNG{t: t}
			if e.Category == corebattle.CategoryMagic {
				rng.draws = []float64{draw}
			}
			if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: e.ID}}, rng); err != nil || len(rng.draws) != 0 {
				t.Fatal(e, err, rng.draws)
			}
			if e.Category == corebattle.CategoryMagic && draw == .25 {
				after, _ := json.Marshal(s)
				if string(before) != string(after) {
					t.Fatal("failed cure should be accepted no-effect", e, s)
				}
			} else if e.Category == corebattle.CategoryMagic && s.Actors[0].Status != "" {
				t.Fatal(e, s)
			}
		}
	}
}

func TestDefenseGemCompositeFrozenInputAndReload(t *testing.T) {
	s, c := recoveryGemFixture(false)
	var d BattleDefinitions
	d.RegisterDefenseGems()
	ids := []string{"gem_cover", "gem_awakening_sky", "gem_guard_2"}
	d.Invocations.Entries["custom"] = d.DefenseGemInvocation(ids)
	ids[0] = "mutated input"
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "tail"}}
	frozen, err := json.Marshal(struct {
		State     corebattle.LifetimeState
		Target    corebattle.TargetContext
		Arguments []string
	}{s, f.TargetContext, f.Arguments})
	if err != nil {
		t.Fatal(err)
	}
	ops, err := d.InvocationAction(&s, &f, []string{"custom"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if s.Actors[0].Temporary != corebattle.TemporaryDefense || s.Actors[2].Temporary != corebattle.TemporaryCover || s.Actors[0].ExtendedTurns != 8 {
		t.Fatal(s)
	}
	var loaded struct {
		State     corebattle.LifetimeState
		Target    corebattle.TargetContext
		Arguments []string
	}
	if err := json.Unmarshal(frozen, &loaded); err != nil {
		t.Fatal(err)
	}
	var restored BattleDefinitions
	restored.RegisterDefenseGems()
	restored.Invocations.Entries["custom"] = restored.DefenseGemInvocation([]string{"gem_cover", "gem_awakening_sky", "gem_guard_2"})
	loadedFrame := corebattle.CallbackContext{TargetContext: loaded.Target, Arguments: loaded.Arguments}
	if err := loaded.State.ApplyInvocations(&loadedFrame, restored.Invocations, ops, noArithmeticDraws{t}); err != nil || !reflect.DeepEqual(s, loaded.State) || !reflect.DeepEqual(f, loadedFrame) {
		t.Fatal(err, s, loaded)
	}
	committed, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(committed, &loaded.State); err != nil || !reflect.DeepEqual(s, loaded.State) {
		t.Fatal(err)
	}
}

func TestDefenseGemUnknownLaterSlotAndArguments(t *testing.T) {
	for _, ids := range [][]string{{"gem_guard_1", "gem-101"}, {"gem_awakening_1", "gem-102"}, {"gem_cover", "item-110"}, {"gem_guard_1", "", "gem_guard_1", "gem_guard_1"}} {
		s, c := recoveryGemFixture(false)
		var d BattleDefinitions
		d.RegisterDefenseGems()
		d.Invocations.Entries["custom"] = d.DefenseGemInvocation(ids)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
		before, _ := json.Marshal(s)
		if _, err := d.DefenseGemAction(&s, c.TargetContext, ids, "ally", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "custom"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		after, _ := json.Marshal(s)
		if string(before) != string(after) {
			t.Fatal("partial execution", ids)
		}
	}
	for _, custom := range []bool{false, true} {
		for _, id := range []string{"gem_guard_1", "gem_guard_2", "gem_awakening_1", "gem_awakening_sky"} {
			s, c := recoveryGemFixture(false)
			c.Custom = custom
			var d BattleDefinitions
			d.RegisterDefenseGems()
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "tail"}}
			if err := d.Invocations.Entries[id].Execute(&s, &f, noArithmeticDraws{t}); err != nil {
				t.Fatal(err)
			}
			want := []string{"ally", "tail"}
			if custom || id == "gem_guard_2" || id == "gem_awakening_sky" {
				want = []string{"tail"}
			}
			if !reflect.DeepEqual(f.Arguments, want) {
				t.Fatal(id, custom, f)
			}
		}
	}
}

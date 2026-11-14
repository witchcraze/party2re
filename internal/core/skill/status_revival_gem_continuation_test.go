package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestStatusRevivalGemsAdmitOrderedContinuation(t *testing.T) {
	for _, ids := range [][]string{
		{"gem_cure_magic_seal", "gem-64", "gem_cure_sleep"},
		{"gem-63", "gem_cure_all_abnormal", "gem-64"},
		{"gem-65", "gem_cure_sleep", "gem-64"},
	} {
		t.Run(ids[0], func(t *testing.T) {
			s, revival, d := revivalRegistryFixture()
			s.Actors[0].Status = corebattle.StatusMagicSeal
			s.Actors[2].Status = corebattle.StatusSleep
			status := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: revival.TargetContext}}
			d.RegisterRevivals(&revival)
			d.RegisterStatuses(&status, nil)
			frame := corebattle.CallbackContext{TargetContext: revival.TargetContext, Arguments: []string{"fallen", "tail"}}
			ops, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(err)
			}
			rng := &queuedRecoveryGemRNG{t: t, draws: []float64{0, 0, 0, 0, 0, 0}}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil {
				t.Fatal(err)
			}
			if s.Actors[2].HP <= 0 || s.Actors[2].Status != "" {
				t.Fatal(s.Actors[2])
			}
		})
	}
}

// Numeric oracles: ff97fc5:_data.cgi:2158–2169; _skill.cgi:9193–9233,9360–9419.
func TestStatusGemConcreteProjectionAndChanceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		id, status  string
		chance      float64
		death, area bool
	}{
		{"gem_instant_death_1", "", 20, true, false},
		{"gem_instant_death_sky", "", 20, true, true},
		{"gem_inflict_poison", corebattle.StatusDeadlyPoison, 60, false, true},
		{"gem_inflict_paralysis", corebattle.StatusParalyze, 35, false, true},
		{"gem_inflict_sleep", corebattle.StatusSleep, 35, false, true},
		{"gem_inflict_confusion", corebattle.StatusConfusion, 60, false, true},
		{"gem_inflict_seal_act", corebattle.StatusDofuu, 50, false, true},
		{"gem_inflict_seal_atk", corebattle.StatusPhysicalSeal, 60, false, true},
		{"gem_inflict_seal_magic", corebattle.StatusMagicSeal, 60, false, true},
	} {
		for _, equal := range []bool{false, true} {
			t.Run(tc.id+map[bool]string{false: "/below", true: "/equal"}[equal], func(t *testing.T) {
				s, revival, d := revivalRegistryFixture()
				s.Actors[1].HP, s.Actors[1].MaxHP, s.Actors[1].Tension = 100, 101, 5
				applied := 0
				c := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: revival.TargetContext}}
				c.Defeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &corebattle.DefeatRewards{
					Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
					Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { applied++; return nil },
				}}
				d.RegisterStatuses(&c, nil)
				f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				def := d.Invocations.Entries[tc.id]
				if def.ProjectBranches == nil {
					t.Fatal("missing concrete producer")
				}
				branches, err := def.ProjectBranches(&s, &f)
				if err != nil || len(branches) != 2 || applied != 0 {
					t.Fatal(len(branches), err, c, applied)
				}
				draw := tc.chance / 100
				if !equal {
					draw -= .00001
				}
				ops, err := d.InvocationAction(&s, &f, []string{tc.id, "gem_cure_sleep"}, noArithmeticDraws{t})
				if err != nil {
					t.Fatal(err)
				}
				rng := &damageGemDraws{t: t, draws: []float64{draw}}
				if err = s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
					t.Fatal(err)
				}
				success := !equal || tc.death
				wantStatus, wantHP := "", 100
				if success {
					wantStatus = tc.status
					if tc.death {
						wantHP = 0
					}
				}
				if s.Actors[1].Status != wantStatus || s.Actors[1].HP != wantHP {
					t.Fatal(s.Actors[1], success)
				}
				if tc.death {
					if applied != map[bool]int{false: 0, true: 1}[success] {
						t.Fatal(f.Status, applied)
					}
				}
				matched := false
				for _, branch := range branches {
					if reflect.DeepEqual(branch.State, s) {
						matched = true
					}
				}
				if !matched {
					t.Fatal("runtime state absent from pure alternatives", s)
				}
			})
		}
	}
}

func TestCureGemConcreteProjectionAndStatusCMP(t *testing.T) {
	for _, tc := range []struct{ id, status string }{
		{"gem_cure_poison", corebattle.StatusPoison}, {"gem_cure_poison", corebattle.StatusVirulentPoison},
		{"gem_cure_paralysis", corebattle.StatusParalyze}, {"gem_cure_sleep", corebattle.StatusSleep},
		{"gem_cure_confusion", corebattle.StatusConfusion}, {"gem_cure_paralyze_action", corebattle.StatusDofuu},
		{"gem_cure_atk_seal", corebattle.StatusPhysicalSeal}, {"gem_cure_magic_seal", corebattle.StatusMagicSeal},
		{"gem_cure_all_abnormal", corebattle.StatusMagicSeal},
	} {
		t.Run(tc.id+tc.status, func(t *testing.T) {
			s, revival, d := revivalRegistryFixture()
			s.Actors[0].Status, s.Actors[0].CMP = tc.status, 10
			c := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: revival.TargetContext}}
			d.RegisterStatuses(&c, nil)
			d.RegisterResources(&corebattle.DamageContext{})
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self", "tail"}}
			branches, err := d.Invocations.Entries[tc.id].ProjectBranches(&s, &f)
			if err != nil || len(branches) == 0 {
				t.Fatal(len(branches), err)
			}
			if branches[0].State.Actors[0].Status != "" || s.Actors[0].Status != tc.status {
				t.Fatal("cure not isolated", branches[0].State, s)
			}
			ops, err := d.InvocationAction(&s, &f, []string{tc.id, "gem_status_cmp_3"}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(err)
			}
			draws := []float64{}
			if tc.status == corebattle.StatusConfusion {
				draws = append(draws, 0)
			}
			if err = s.ApplyInvocations(&f, d.Invocations, ops, &damageGemDraws{t: t, draws: draws}); err != nil || s.Actors[0].CMP != 10 || s.Actors[0].Status != "" {
				t.Fatal(err, s)
			}
		})
	}
}

func TestStatusRevivalGemFrozenReloadAndRebuiltPorts(t *testing.T) {
	var reference corebattle.LifetimeState
	for _, reload := range []bool{false, true} {
		s, rc, _ := revivalRegistryFixture()
		s.Actors[0].Abilities = []string{"pharaoh"}
		s.Actors[0].MP, s.Actors[0].MaxMP, s.Actors[0].MaxHP = 81, 100, 101
		s.Actors[0].Gear.AccessoryID = "item-159"
		s.Actors[0].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 100, Agility: 100}
		s.Actors[1].HP, s.Actors[1].MaxHP = 100, 101
		input := struct {
			State     corebattle.LifetimeState
			Target    corebattle.TargetContext
			Arguments []string
			Ops       []corebattle.CallbackInvocation
			Draws     []float64
		}{s, rc.TargetContext, []string{"fallen", "tail"}, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem-66"}, {Kind: "entry", EntryID: "gem_inflict_poison"}, {Kind: "entry", EntryID: "gem_status_cmp_3"}}, []float64{0, .8, 0, .59999}}
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		if reload {
			if err = json.Unmarshal(data, &input); err != nil {
				t.Fatal(err)
			}
		}
		var d BattleDefinitions
		defeat := corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: input.Target}}
		revival := corebattle.RevivalContext{TargetContext: input.Target, NativeDefeat: &defeat}
		status := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: input.Target}}
		d.RegisterRevivals(&revival)
		d.RegisterStatuses(&status, nil)
		d.RegisterResources(&corebattle.DamageContext{})
		f := corebattle.CallbackContext{TargetContext: input.Target, Arguments: input.Arguments}
		rng := &damageGemDraws{t: t, draws: input.Draws}
		if err = input.State.ValidateInvocations(&f, d.Invocations, input.Ops, noArithmeticDraws{t}); err != nil {
			t.Fatal(err)
		}
		if err = input.State.ApplyInvocations(&f, d.Invocations, input.Ops, rng); err != nil {
			t.Fatal(err)
		}
		if rng.used != 4 || input.State.Actors[0].MP != 0 || input.State.Actors[1].Status != corebattle.StatusDeadlyPoison || f.Defeat.Cursor != (corebattle.TargetCursor{Source: "item159", ActorID: "self"}) {
			t.Fatal(input.State, f, rng.used)
		}
		saved := struct {
			State  corebattle.LifetimeState
			Cursor corebattle.TargetCursor
		}{input.State, f.Defeat.Cursor}
		facts, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if reload {
			if err = json.Unmarshal(facts, &saved); err != nil {
				t.Fatal(err)
			}
		}
		status = corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: input.Target}}
		defeat = corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: input.Target, Cursor: saved.Cursor}}
		f = corebattle.CallbackContext{TargetContext: input.Target, Arguments: input.Arguments, Status: &status, Defeat: &defeat}
		d.RegisterStatuses(&status, nil)
		if err = saved.State.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem_cure_poison"}}, noArithmeticDraws{t}); err != nil || saved.State.Actors[0].Status != "" || saved.State.Actors[1].Status != corebattle.StatusDeadlyPoison || f.Defeat.Cursor != saved.Cursor {
			t.Fatal("lost saved operation facts", f, err)
		}
		input.State = saved.State
		if reload && !reflect.DeepEqual(reference, input.State) {
			t.Fatal("reload changed resulting facts")
		}
		reference = input.State
	}
}

func TestStatusGemRegisteredNoEffectSkipsMissingDefeatContext(t *testing.T) {
	for _, mode := range []string{"immune death", "null death", "zero status"} {
		s, rc, d := revivalRegistryFixture()
		c := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: rc.TargetContext}}
		id := "gem_instant_death_1"
		draws := []float64{}
		switch mode {
		case "immune death":
			s.Actors[1].Gear = nil
		case "null death":
			s.Actors[1].HP = 100
			s.Actors[1].Temporary = corebattle.TemporaryNullMagic
		case "zero status":
			s.Actors[1].HP, s.Actors[1].MaxHP = 100, 101
			s.Actors[0].ItemDefinitionIDs = []string{"item-239"}
			c.DamageCalls = 1
			id = "gem_inflict_poison"
			draws = []float64{0}
		}
		d.RegisterStatuses(&c, nil)
		d.RegisterDefenseGems()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		ops, err := d.InvocationAction(&s, &f, []string{id, "gem_guard_1"}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(mode, err)
		}
		rng := &damageGemDraws{t: t, draws: draws}
		if err = s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) || s.Actors[0].Temporary != corebattle.TemporaryDefense {
			t.Fatal(mode, err, s, rng.used)
		}
	}
}

func TestStatusRevivalGemUnsupportedTailIsAtomicAndUnknownStaysGated(t *testing.T) {
	for _, first := range []string{"gem_cure_sleep", "gem_cure_all_abnormal", "gem-63", "gem-64", "gem-65", "gem_inflict_poison"} {
		s, rc, d := revivalRegistryFixture()
		s.Actors[0].Status = corebattle.StatusSleep
		s.Actors[1].HP, s.Actors[1].MaxHP = 100, 101
		status := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: rc.TargetContext}}
		d.RegisterRevivals(&rc)
		d.RegisterStatuses(&status, nil)
		f := corebattle.CallbackContext{TargetContext: rc.TargetContext, Arguments: []string{"fallen", "tail"}}
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: first}, {Kind: "entry", EntryID: "gem_instant_death_1"}}
		if err = s.ValidateInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(first, err)
		}
		if err = s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(first, err)
		}
		after, err := json.Marshal(s)
		if err != nil || string(before) != string(after) || !reflect.DeepEqual(f.Arguments, []string{"fallen", "tail"}) {
			t.Fatal(first, "caller mutated", err)
		}
	}
	// An arbitrary effect without a projection never acquires certification.
	s, rc, d := revivalRegistryFixture()
	d.RegisterRevivals(&rc)
	d.Invocations.Entries["unknown"] = corebattle.CallbackDefinition{
		Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
		Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
			t.Fatal("unknown prefix executed")
			return nil
		},
	}
	f := corebattle.CallbackContext{TargetContext: rc.TargetContext, Arguments: []string{"fallen"}}
	if _, err := d.InvocationAction(&s, &f, []string{"unknown", "gem-63"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestRegisteredInstantDeathRejectsUnsupportedAwakenedResetTailBeforeMutation(t *testing.T) {
	s, revival, d := revivalRegistryFixture()
	self, enemy := &s.Actors[0], &s.Actors[1]
	self.Extended = corebattle.ExtendedAwakening
	enemy.BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 100, Agility: 100}
	enemy.HP, enemy.MaxHP, enemy.MP, enemy.MaxMP = 100, 101, 80, 100
	enemy.Abilities = []string{"pharaoh"}
	power, level := 100, 99
	enemy.Gear = &corebattle.BattleGearStats{WeaponID: "weapon-71", WeaponAttack: &power, AccessoryID: "item-115"}
	enemy.ItemDefinitionIDs = []string{"weapon-71", "item-115"}
	defeat := &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: revival.TargetContext, RequesterLevel: &level}}
	status := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: revival.TargetContext}, Defeat: defeat}
	d.RegisterStatuses(&status, nil)
	d.Invocations.Entries["audit-tail"] = corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext) error {
			if s.Actors[1].Attack > 270 {
				return corebattle.ErrUnsupportedEffect
			}
			return nil
		},
		Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
			t.Fatal("unsupported tail executed after registered instant-death prefix")
			return nil
		},
	}
	frame := corebattle.CallbackContext{TargetContext: revival.TargetContext, Arguments: []string{"enemy", "tail"}}
	stateBefore, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	frameBefore, statusBefore, defeatBefore := frame, status, *defeat
	ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem_instant_death_1"}, {Kind: "entry", EntryID: "audit-tail"}}
	if _, err = d.InvocationAction(&s, &frame, []string{"gem_instant_death_1", "audit-tail"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("registered discovery admitted unsupported continuation", err)
	}
	if err = s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("registered execution admission accepted unsupported continuation", err)
	}
	stateAfter, err := json.Marshal(s)
	if err != nil || string(stateBefore) != string(stateAfter) || !reflect.DeepEqual(frameBefore, frame) ||
		!reflect.DeepEqual(statusBefore, status) || !reflect.DeepEqual(defeatBefore, *defeat) || level != 99 {
		t.Fatal("rejected registered invocation mutated caller state, context, or counters", err, s, frame, status, defeat)
	}

	// A single registered status invocation remains supported: its real maximum
	// outcome includes both the item-115 draw and the independent weapon-71 draw.
	s, revival, d = revivalRegistryFixture()
	self, enemy = &s.Actors[0], &s.Actors[1]
	self.Extended = corebattle.ExtendedAwakening
	enemy.BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 100, Agility: 100}
	enemy.HP, enemy.MaxHP, enemy.MP, enemy.MaxMP = 100, 101, 80, 100
	enemy.Abilities = []string{"pharaoh"}
	power, level = 100, 99
	enemy.Gear = &corebattle.BattleGearStats{WeaponID: "weapon-71", WeaponAttack: &power, AccessoryID: "item-115"}
	enemy.ItemDefinitionIDs = []string{"weapon-71", "item-115"}
	defeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: revival.TargetContext, RequesterLevel: &level}}
	status = corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: revival.TargetContext}, Defeat: defeat}
	d.RegisterStatuses(&status, nil)
	frame = corebattle.CallbackContext{TargetContext: revival.TargetContext, Arguments: []string{"enemy"}}
	ops, err = d.InvocationAction(&s, &frame, []string{"gem_instant_death_1"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal("supported registered continuation rejected", err)
	}
	rng := &damageGemDraws{t: t, draws: []float64{0, .9, .9}}
	if err = s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || rng.used != 3 ||
		s.Actors[1].HP != 101 || s.Actors[1].MP != 40 || s.Actors[1].Attack != 295 {
		t.Fatal("registered instant-death continuation missed the projected maximum", err, s, status, rng.used)
	}
}

func TestRevivalGemSealAlternativesAndItem195RejectUncheckedLaterDefense(t *testing.T) {
	for _, activate := range []bool{false, true} {
		s, c, d := revivalRegistryFixture()
		s.Actors[0].Status = corebattle.StatusMagicSeal
		c.Item195Activated = activate
		d.RegisterRevivals(&c)
		d.RegisterDefenseGems()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"fallen", "tail"}}
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		// A failed cure leaves magic sealed: unchecked gem41 must reject.
		if _, err = d.InvocationAction(&s, &f, []string{"gem-63", "gem_nullify_phys"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(activate, err)
		}
		after, err := json.Marshal(s)
		if err != nil || string(before) != string(after) {
			t.Fatal("rejected seal branch mutated caller", err)
		}
		for _, draw := range []float64{.24999, .25} {
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			var p corebattle.LifetimeState
			if err = json.Unmarshal(data, &p); err != nil {
				t.Fatal(err)
			}
			draws := []float64{draw}
			if activate && draw < .25 {
				draws = append(draws, .9)
			} // Cure succeeds, activation fails.
			ops, err := d.InvocationAction(&p, &f, []string{"gem-63", "gem_guard_1"}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(err)
			}
			rng := &damageGemDraws{t: t, draws: draws}
			if err = p.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) || p.Actors[2].HP != map[bool]int{false: 0, true: 50}[draw < .25] {
				t.Fatal(err, p, rng.used)
			}
		}
	}
}

func TestSacrificeGemNativeContinuationAndEarlyRejection(t *testing.T) {
	for _, protect := range []bool{false, true} {
		t.Run(map[bool]string{false: "death", true: "automatic revival"}[protect], func(t *testing.T) {
			s, c, d := revivalRegistryFixture()
			s.Actors[0].MP, s.Actors[0].MaxMP, s.Actors[0].MaxHP = 81, 100, 101
			if protect {
				s.Actors[0].Abilities = []string{"pharaoh"}
			}
			c.NativeDefeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext}}
			d.RegisterRevivals(&c)
			status := corebattle.StatusContext{EffectContext: corebattle.EffectContext{TargetContext: c.TargetContext}}
			d.RegisterStatuses(&status, nil)
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"fallen", "tail"}}
			before := s
			before.Actors = append([]corebattle.LifetimeActor(nil), s.Actors...)
			ids := []string{"gem-66", "missing"}
			_, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
			if protect {
				if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(err)
				}
				if err = s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem-66"}, {Kind: "entry", EntryID: "missing"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, s) || c.NativeDefeat.Cursor != (corebattle.TargetCursor{}) {
					t.Fatal("rejection mutated caller")
				}
			} else if err != nil {
				t.Fatal("dead branch must skip unsupported later slot", err)
			}
			ids = []string{"gem-66", "gem_cure_sleep"}
			ops, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(err)
			}
			if err = s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
				t.Fatal(err)
			}
			wantHP := 0
			if protect {
				wantHP = 101
			}
			if s.Actors[0].HP != wantHP || s.Actors[0].MP != 0 || s.Actors[2].HP != 101 {
				t.Fatal(s)
			}
			if (f.Status != nil) != protect {
				t.Fatal("later status body ran after death", f.Status)
			}
		})
	}
}

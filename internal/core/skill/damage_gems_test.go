package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

type damageGemDraws struct {
	t     *testing.T
	draws []float64
	used  int
}

func (r *damageGemDraws) Float64() float64 {
	r.t.Helper()
	if r.used >= len(r.draws) {
		r.t.Fatal("unexpected damage gem draw")
	}
	v := r.draws[r.used]
	r.used++
	return v
}
func (r *damageGemDraws) Intn(int) int   { r.t.Fatal("expected continuous damage draws"); return 0 }
func (r *damageGemDraws) IntN(n int) int { return r.Intn(n) }

func damageGemRegistryFixture() (corebattle.LifetimeState, corebattle.DamageContext, BattleDefinitions) {
	s, rc, d := rosterRegistryFixture()
	s.Actors[0].HP, s.Actors[0].MaxHP, s.Actors[0].MP, s.Actors[0].CMP = 100, 100, 80, 90
	s.Actors[0].Attack = 100
	c := rc.DamageContext
	c.Defeat = func(id string) error {
		for i := range s.Actors {
			if s.Actors[i].ID == id {
				s.Actors[i].HP = 0
			}
		}
		return nil
	}
	return s, c, d
}

func TestDamageGemRegistrationExecutesEveryConcreteProducer(t *testing.T) {
	cases := []struct {
		id                         string
		source, cost, damage, hits int
	}{
		{"gem_atk_1", 1, 4, 63, 1}, {"gem_atk_2", 2, 10, 81, 1},
		{"gem_sky_atk_1", 3, 29, 112, 1}, {"gem_sky_atk_2", 4, 40, 112, 1}, {"gem_burst_sky", 5, 46, 126, 1},
		{"gem_combo_1", 6, 22, 27, 3}, {"gem_combo_sky", 7, 31, 27, 7},
		{"gem_combo_burst_sky", 8, 46, 54, 2}, {"gem_dance_combo", 9, 27, 27, 2},
		{"gem_all_atk_1", 10, 7, 36, 1}, {"gem_all_atk_2", 11, 16, 45, 1}, {"gem_all_atk_sky", 12, 21, 72, 1},
		{"gem_magic_atk_1", 13, 5, 45, 1}, {"gem_magic_atk_2", 14, 19, 180, 1}, {"gem_magic_atk_sky", 15, 26, 450, 1},
		{"gem_all_magic_1", 16, 14, 45, 1}, {"gem_all_magic_sky", 17, 25, 180, 1}, {"gem_all_breath_1", 18, 28, 135, 1},
		{"gem_drain_hp_sky", 26, 26, 180, 1}, {"gem_kamikaze_sky", 72, 9, 90, 1}, {"gem_crisis_strike", 73, 22, 3, 1},
	}
	if len(corebattle.DamageGemEntries()) != len(cases) {
		t.Fatal("incomplete entry registry")
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			for _, npc := range []bool{false, true} {
				s, c, d := damageGemRegistryFixture()
				s.Actors[0].NPC, c.ActorNPC = npc, npc
				if _, err := d.DamageGemAction(&s, &c, tc.id, "enemy", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
					t.Fatal(err)
				}
				d.RegisterDamageGems(&c)
				entry, err := d.DamageGemAction(&s, &c, tc.id, "enemy", noArithmeticDraws{t})
				if err != nil || entry.SourceID != tc.source || entry.CMPCost != tc.cost || entry.Name == "" {
					t.Fatal(entry, err)
				}
				frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				draws := []float64{0}
				if tc.source >= 6 && tc.source <= 9 {
					draws = make([]float64, 1+3*tc.hits)
				}
				if tc.source == 26 {
					draws = []float64{0, 0}
				}
				rng := &damageGemDraws{t: t, draws: draws}
				if err = d.Invocations.Entries[tc.id].Execute(&s, &frame, rng); err != nil {
					t.Fatal(err)
				}
				if s.Actors[1].HP != 1000-tc.damage*tc.hits || c.DamageCalls != tc.hits || rng.used != len(draws) {
					t.Fatal(tc, s, c, rng.used)
				}
				if s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 {
					t.Fatal("CMP contribution charged as MP or early CMP", s)
				}
				wantArgs := []string{"tail"}
				if tc.source >= 6 && tc.source <= 12 || tc.source >= 16 && tc.source <= 18 {
					wantArgs = []string{"enemy", "tail"}
				}
				if !reflect.DeepEqual(frame.Arguments, wantArgs) {
					t.Fatal(tc, frame.Arguments)
				}
				if tc.source == 72 && s.Actors[0].HP != 0 {
					t.Fatal(s)
				}
			}
		})
	}
}

func TestDamageGemUnsupportedLaterSlotAndDelegationAreInert(t *testing.T) {
	s, c, d := damageGemRegistryFixture()
	d.RegisterDamageGems(&c)
	d.Invocations.Entries["random"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	d.Invocations.Entries["delegated"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	s.Actors[0].Jobs.JobID = "job-1"
	d.Invocations.JobTables = map[int]corebattle.CallbackTable{1: {Variants: [][]corebattle.CallbackEntry{{{ID: "gem_kamikaze_sky", Name: "sacrifice"}}}}}
	defeat := c.Defeat
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	for _, ids := range [][]string{{"gem_magic_atk_1"}, {"gem_magic_atk_1", "missing"}, {"random"}, {"delegated"}, {"gem_magic_atk_1", "gem_magic_atk_1"}} {
		c.Defeat = defeat
		if len(ids) == 1 {
			c.Defeat = nil
		}
		if _, err := d.InvocationAction(&s, &frame, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		var ops []corebattle.CallbackInvocation
		for _, id := range ids {
			ops = append(ops, corebattle.CallbackInvocation{Kind: "entry", EntryID: id})
		}
		if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || c.DamageCalls != 0 || c.AreaDamageCalls != 0 {
		t.Fatal("rejected closure mutated state", s, c)
	}
}

func TestDamageGemRepeatAndReloadPreserveFactsWithoutReplaying(t *testing.T) {
	run := func(reload bool) corebattle.LifetimeState {
		s, c, d := damageGemRegistryFixture()
		for i := range 2 {
			if i > 0 && reload {
				data, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				var saved corebattle.LifetimeState
				if err = json.Unmarshal(data, &saved); err != nil {
					t.Fatal(err)
				}
				s = saved
			}
			d = BattleDefinitions{}
			d.RegisterDamageGems(&c)
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
			ops, err := d.InvocationAction(&s, &frame, []string{"gem_magic_atk_1"}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(err)
			}
			rng := &damageGemDraws{t: t, draws: []float64{0}}
			if err = s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || rng.used != 1 {
				t.Fatal(err)
			}
		}
		if s.Actors[1].HP != 910 || c.DamageCalls != 2 || s.Actors[0].CMP != 90 {
			t.Fatal(s, c)
		}
		return s
	}
	if whole, resumed := run(false), run(true); !reflect.DeepEqual(whole, resumed) {
		t.Fatal(whole, resumed)
	}
}

func TestDamageGemSacrificeUsesSharedAutomaticRevival(t *testing.T) {
	s, c, d := damageGemRegistryFixture()
	s.Actors[0].Temporary = corebattle.TemporaryRevival
	c.Defeat = func(id string) error {
		result, err := s.ApplyAutomaticRevival(&corebattle.AutomaticRevivalContext{TargetContext: c.TargetContext, Reset: func(string) error { s.Actors[0].ResetEffects(corebattle.LifetimeReset{}); return nil }}, id, random.NewDeterministic(1))
		if err != nil || !result.Prevented {
			t.Fatal(result, err)
		}
		return err
	}
	d.RegisterDamageGems(&c)
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if err := d.Invocations.Entries["gem_kamikaze_sky"].Execute(&s, &frame, &damageGemDraws{t: t, draws: []float64{0}}); err != nil || s.Actors[0].HP <= 0 || s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 {
		t.Fatal(s, err)
	}
}

func TestDamageGemCustomMissingTargetDefaultsToActorBeforeConfusion(t *testing.T) {
	s, c, d := damageGemRegistryFixture()
	c.Custom = true
	s.Actors[0].Status = corebattle.StatusConfusion
	d.RegisterDamageGems(&c)
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	rng := &damageGemDraws{t: t, draws: []float64{0}}
	if err := d.Invocations.Entries["gem_magic_atk_1"].Execute(&s, &frame, rng); err != nil || s.Actors[0].HP != 55 || s.Actors[1].HP != 1000 || rng.used != 1 {
		t.Fatal(s, err, rng.used)
	}
}

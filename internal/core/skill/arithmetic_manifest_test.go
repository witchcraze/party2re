package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func arithmeticRegistryFixture() (corebattle.LifetimeState, *corebattle.ArithmeticContext, BattleDefinitions) {
	s := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "self", TeamID: "party", HP: 1000, MaxHP: 1000}},
		{Participant: corebattle.Participant{ID: "ally", TeamID: "party", HP: 0, MaxHP: 500, MP: 20}},
		{Participant: corebattle.Participant{ID: "enemy", TeamID: "enemy", HP: 120, MaxHP: 1000, MP: 20}},
	}}
	c := &corebattle.ArithmeticContext{TargetContext: corebattle.TargetContext{ActorID: "self", PartyIDs: []string{"self", "ally"}, EnemyIDs: []string{"enemy"}}, Discovered: corebattle.ArithmeticEntries("job-72")}
	d := BattleDefinitions{}
	d.RegisterArithmetic(c)
	return s, c, d
}

// Independent intrinsic vectors: ff97fc5:_skill.cgi, complete callback spans.
func TestArithmeticManifestIntrinsicResults(t *testing.T) {
	for _, tc := range []struct {
		id, span         string
		job, pos, sp, mp int
		wantHP           int
	}{
		{"job-31-gravity", "2656-2668", 31, 4, 94, 24, 30},
		{"job-31-death", "2673-2684", 31, 6, 155, 25, 0},
		{"job-31-reverse", "2688-2713", 31, 7, 155, 35, 21},
		{"job-72-divisor-5", "5454-5463", 72, 0, 0, 15, 120},
		{"job-72-divisor-4", "5467-5476", 72, 1, 100, 14, 120},
		{"job-72-divisor-3", "5480-5489", 72, 2, 150, 13, 120},
		{"job-72-damage", "5493-5512", 72, 3, 20, 0, 0},
		{"job-72-reflect", "5516-5527", 72, 4, 40, 0, 120},
		{"job-72-heal", "5531-5545", 72, 5, 70, 0, 570},
		{"job-72-revive", "5549-5560", 72, 6, 120, 0, 120},
		{"job-72-death", "5564-5577", 72, 7, 180, 0, 0},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, c, d := arithmeticRegistryFixture()
			var defeated []string
			c.Defeat = func(id string) error { defeated = append(defeated, id); return nil }
			c.SubmittedText = "＠倍数指定＠×反射 "
			if tc.pos == 2 && tc.job == 72 {
				s.Actors[2].MP = 18
			}
			entry := d.Invocations.JobTables[tc.job].Variants[0][tc.pos]
			if entry.ID != tc.id || entry.RequiredSP != tc.sp || entry.MPCost != tc.mp || entry.Extended != (tc.id == "job-31-reverse") {
				t.Fatal(tc.span, entry)
			}
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}, ArithmeticDivisor: 4}
			ops, err := d.InvocationAction(&s, &frame, []string{tc.id}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(tc.span, err)
			}
			rng := &damageGemDraws{t: t}
			if tc.id == "job-72-damage" || tc.id == "job-72-heal" {
				rng.draws = []float64{0}
			}
			if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || s.Actors[2].HP != tc.wantHP || rng.used != len(rng.draws) {
				t.Fatal(tc.span, s, err, rng)
			}
			reflecting := tc.id == "job-72-reflect" || tc.job == 72 && tc.pos < 3
			if (s.Actors[2].Temporary == corebattle.TemporaryReflectMagic) != reflecting || (s.Actors[1].HP == 500) != (tc.id == "job-72-revive") {
				t.Fatal(tc.span, s)
			}
			if (len(defeated) == 1) != (tc.id == "job-31-death" || tc.id == "job-72-death" || tc.id == "job-72-damage") || s.Actors[0].MP != 0 {
				t.Fatal("defeat/cost tail", tc.span, defeated, s)
			}
		})
	}
}

func TestArithmeticRegistryDefaultsAndMissingDescendant(t *testing.T) {
	s, c, d := arithmeticRegistryFixture()
	c.SubmittedText = "＠倍数指定＠×反射 "
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	for _, registry := range []BattleDefinitions{{}, {ArithmeticSkills: d.ArithmeticSkills}} {
		if _, err := registry.InvocationAction(&s, &frame, []string{"job-72-divisor-4"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	delete(d.Invocations.Entries, "job-72-reflect")
	before := append([]corebattle.LifetimeActor(nil), s.Actors...)
	if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-31-gravity"}, {Kind: "entry", EntryID: "job-72-divisor-4"}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(before, s.Actors) {
		t.Fatal(err, s)
	}
}

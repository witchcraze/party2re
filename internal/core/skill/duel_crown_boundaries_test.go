package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func executeDuel(t *testing.T, s *corebattle.LifetimeState, c *corebattle.DuelCrownContext, d BattleDefinitions, id string, args []string, draws ...float64) corebattle.CallbackContext {
	t.Helper()
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &frame, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
		t.Fatal(err, rng.used, len(draws))
	}
	return frame
}

// Independent HP vectors: two alternating rounds, .9 damage roll, no critical.
func TestDuelCrownDuelOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		actorHP, enemyHP, enemyAT   int
		both, crown                 bool
		wantActor, wantEnemy, calls int
		draws                       []float64
	}{
		{"draw", 1000, 1000, 110, false, false, 1000, 1000, 4, []float64{.5, 0, .5, 0, .5, 0, .5, 0}},
		{"damage-win", 1000, 1000, 100, false, true, 1000, 902, 4, []float64{.5, 0, .5, 0, .5, 0, .5, 0, 0}},
		{"damage-win-no-crown", 1000, 1000, 100, false, false, 1000, 902, 4, []float64{.5, 0, .5, 0, .5, 0, .5, 0, .25}},
		{"damage-loss", 1000, 1000, 120, false, false, 892, 1000, 4, []float64{.5, 0, .5, 0, .5, 0, .5, 0}},
		{"actor-win", 1000, 1, 100, false, true, 1000, 0, 1, []float64{.5, 0}},
		{"target-win", 1, 1000, 100, false, false, 0, 1000, 2, []float64{.5, 0, .5, 0}},
		{"both-death", 1000, 1, 100, true, false, 0, 1, 1, []float64{.5, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, d := duelFixture()
			s.Actors[0].Abilities = []string{"seal_shinsoku"} // The callback calls the helper, not the repeated outer command.
			s.Actors[0].HP, s.Actors[1].HP, s.Actors[1].Attack = tc.actorHP, tc.enemyHP, tc.enemyAT
			var defeated []string
			c.Defeat = func(id string) error {
				defeated = append(defeated, id)
				if tc.both {
					s.Actors[0].HP, s.Actors[1].HP = 0, 0
				}
				return nil
			}
			original := c.TargetContext
			frame := executeDuel(t, &s, c, d, "job-74-entry-2", []string{"enemy", "tail"}, tc.draws...)
			if s.Actors[0].HP != tc.wantActor || s.Actors[1].HP != tc.wantEnemy || c.DamageCalls != tc.calls || !reflect.DeepEqual(c.TargetContext, original) || !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
				t.Fatal(s, c, frame)
			}
			attack := 100
			if tc.crown {
				attack = 120
			}
			if s.Actors[0].Attack != attack || (s.Actors[0].Extended == corebattle.ExtendedCrown) != tc.crown || tc.crown && s.Actors[0].ExtendedTurns != 5 {
				t.Fatal(s.Actors[0])
			}
			if tc.both && !reflect.DeepEqual(defeated, []string{"enemy"}) {
				t.Fatal(defeated)
			}
		})
	}
}

func TestDuelCrownFallbackUsesMembersAndEmptyEnemyDraw(t *testing.T) {
	s, c, d := duelFixture()
	// A fallen requested enemy reaches members[0], the actor, rather than enemies[0].
	s.Actors[1].HP = 0
	frame := executeDuel(t, &s, c, d, "job-74-entry-2", []string{"enemy", "tail"}, 0, 0, .5, 0, .5, 0, 0, 0, .5, 0, .5, 0, 0)
	if c.DamageCalls != 2 || s.Actors[0].HP != 1000 || s.Actors[0].Attack != 100 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
		t.Fatal(s, c, frame)
	}
	// With no enemies the member fallback still consumes rand(1); the swapped
	// reply can hit self, whose HP is restored by the tie branch.
	s, c, d = duelFixture()
	s.Actors = append(s.Actors[:1], s.Actors[2:]...)
	c.EnemyIDs = nil
	executeDuel(t, &s, c, d, "job-74-entry-2", nil, 0, 0, .5, 0, 0, 0, .5, 0, 0)
	if c.DamageCalls != 2 || s.Actors[0].HP != 1000 || s.Actors[0].Attack != 100 {
		t.Fatal(s, c)
	}
}

func TestDuelCrownVerdictGuardsAndProvoke(t *testing.T) {
	// Both direct stances remain valid for a fallen, sealed actor without
	// target descendants; they do not acquire ordinary revival restrictions.
	s, c, d := duelFixture()
	s.Actors[0].HP, s.Actors[0].Status = 0, corebattle.StatusPhysicalSeal
	c.Defeat = nil
	executeDuel(t, &s, c, d, "job-74-entry-1", nil)
	if s.Actors[0].Temporary != corebattle.TemporaryReduceAttack || s.Actors[0].Status != corebattle.StatusPhysicalSeal {
		t.Fatal(s)
	}
	for _, tc := range []struct {
		draws   []float64
		status  string
		revival bool
	}{
		{[]float64{0}, "", true}, {[]float64{1.0 / 3, 0}, corebattle.StatusDeadlyPoison, false}, {[]float64{.99, .5}, corebattle.StatusDofuu, false},
	} {
		s, c, d := duelFixture()
		s.Actors[0].HP, s.Actors[0].Tension = 0, 5
		s.Actors[0].Temporary = corebattle.TemporaryDefense
		executeDuel(t, &s, c, d, "job-74-entry-4", []string{"untouched"}, tc.draws...)
		if s.Actors[0].Status != tc.status || (s.Actors[0].Temporary == corebattle.TemporaryRevival) != tc.revival || s.Actors[0].HP != 0 {
			t.Fatal(s)
		}
		wantTen := 1.0
		if tc.revival {
			wantTen = 5
		}
		if s.Actors[0].Tension != wantTen {
			t.Fatal(s)
		}
	}
	for _, draw := range []float64{.49999, .5} {
		s, c, d := duelFixture()
		s.Actors[1].Status = corebattle.StatusSabaku
		executeDuel(t, &s, c, d, "job-74-entry-0", nil, draw)
		want := 0
		if draw == .5 {
			want = 1
		}
		if c.TensionCalls != want || s.Actors[1].Tension != 1 {
			t.Fatal(s, c)
		}
	}
	// Requester level is needed only for an eligible high-tension member.
	s, c, d = duelFixture()
	s.Actors[1].Tension = 5
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, err := d.InvocationAction(&s, &frame, []string{"job-74-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	level := 69
	c.RequesterLevel = &level
	executeDuel(t, &s, c, d, "job-74-entry-0", nil, .5, .99)
	if s.Actors[1].Tension != 8 {
		t.Fatal(s)
	}
}

func TestDuelCrownFlurryCountsGrowthAndRecoil(t *testing.T) {
	for _, crown := range []bool{false, true} {
		s, c, d := duelFixture()
		if crown {
			s.Actors[0].Extended = corebattle.ExtendedCrown
		}
		var draws []float64
		for hit := 0; hit < 6; hit++ {
			if !crown {
				draws = append(draws, 0)
			} // Fresh random target, ignoring arguments.
			draws = append(draws, 0, .5, 0, 0) // damage, growth ×1.05, extend, recoil.
		}
		frame := executeDuel(t, &s, c, d, "job-74-entry-5", []string{"enemy", "tail"}, draws...)
		// floor(.45 × 100 × .9 × 1.05^i): 40,42,44,46,49,51.
		if s.Actors[1].HP != 728 || c.DamageCalls != 6 || c.End.ImmediateRiskCalls != 6 || s.Actors[0].HP != 948 {
			t.Fatal(s, c)
		}
		wantArgs := []string{"enemy", "tail"}
		if crown {
			wantArgs = []string{"tail"}
		}
		if !reflect.DeepEqual(c.Arguments, wantArgs) {
			t.Fatal(frame)
		}
	}
}

func TestDuelCrownWholeActionAdmissionAndRestoration(t *testing.T) {
	for _, id := range []string{"job-74-entry-2", "job-74-entry-3", "job-74-entry-5"} {
		s, c, d := duelFixture()
		c.Defeat = nil
		s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
		if id == "job-74-entry-3" {
			s.Actors[1].Temporary = ""
		}
		before, context := s, *c
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		if _, err := d.InvocationAction(&s, &frame, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c.Arguments, context.Arguments) || c.DamageCalls != 0 || c.End.ImmediateRiskCalls != 0 {
			t.Fatal(id, s, c)
		}
	}
	// The later, swapped actor needs its own gear before the first hit can run.
	s, c, d := duelFixture()
	s.Actors[1].Gear = nil
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &frame, []string{"job-74-entry-2"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	// An external failure during the reply restores actor, sides and saved AT.
	s, c, d = duelFixture()
	s.Actors[0].HP = 1
	failure := errors.New("defeat failed")
	c.Defeat = func(string) error { return failure }
	original := c.TargetContext
	c.Arguments = []string{"enemy"}
	err := s.ApplyDuelCrown(c, "job-74-entry-2", &damageGemDraws{t: t, draws: []float64{.5, 0, .5, 0}})
	if !errors.Is(err, failure) || s.Actors[0].Attack != 100 || !reflect.DeepEqual(c.TargetContext, original) {
		t.Fatal(err, s, c)
	}
}

package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func executeDual(t *testing.T, s *corebattle.LifetimeState, c *corebattle.DualAgilityContext, d BattleDefinitions, id string, args []string, draws ...float64) corebattle.CallbackContext {
	t.Helper()
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &frame, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&frame, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
		t.Fatal(id, err, rng.used)
	}
	return frame
}

func TestDualAgilityCapturedVariantAndArguments(t *testing.T) {
	for _, temporary := range []bool{false, true} {
		s, c, d := dualAgilityFixture()
		if temporary {
			s.Actors[0].Temporary = corebattle.TemporaryDefense
		}
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext}
		table := d.Invocations.JobTables[77]
		entry := table.Variants[table.Select(&s, &frame)][4]
		// Change the live state after selecting the table's concrete identity.
		s.Actors[0].Temporary = corebattle.TemporaryRage
		if temporary {
			s.Actors[0].Temporary = ""
		}
		count, damage := 3, 135
		if temporary {
			count, damage = 6, 294
		}
		frame = executeDual(t, &s, c, d, entry.ID, []string{"untouched"}, make([]float64, count*2)...)
		if s.Actors[1].HP != 1000-damage || c.DamageCalls != count || !reflect.DeepEqual(frame.Arguments, []string{"untouched"}) {
			t.Fatal(entry, s, frame)
		}
		if (s.Actors[0].Temporary == "") != temporary {
			t.Fatal(s.Actors[0])
		}
	}
	for _, args := range [][]string{nil, {"enemy"}, {"enemy", "other", "enemy"}} {
		s, c, d := dualAgilityFixture()
		other := s.Actors[1]
		other.ID = "other"
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, "other")
		// Fresh shifts: missing arguments draw afresh, while all explicit IDs are preserved.
		draws := []float64{0, 0, 0}
		if len(args) == 0 {
			draws = make([]float64, 6)
		}
		if len(args) == 1 {
			draws = make([]float64, 5)
		}
		frame := executeDual(t, &s, c, d, "job-83-entry-3", args, draws...)
		otherDamage := 0
		if len(args) == 3 {
			otherDamage = 38
		}
		if s.Actors[1].HP != 886+otherDamage || s.Actors[3].HP != 1000-otherDamage || len(c.Arguments) != 0 {
			t.Fatal(args, s, frame)
		}
	}
}

func TestDualAgilitySavedStatsAndNoExtraSealInvocations(t *testing.T) {
	for _, id := range []string{"job-77-entry-0", "job-83-entry-0"} {
		for _, hit := range []int{95, 99, 100} {
			s, c, d := dualAgilityFixture()
			s.Actors[0].Hit = hit
			s.Actors[0].Attack = 101 // floor(101 × .75) = 75.
			s.Actors[0].Abilities = []string{"seal_shinsoku"}
			s.Actors[0].Extended = corebattle.ExtendedPurification
			var observed []int
			c.Reset = func(string) error {
				observed = append(observed, s.Actors[0].Attack, s.Actors[0].Agility, s.Actors[0].Hit)
				// A real reset during the first attack must affect the second,
				// while the enclosing callback still restores its saved stats.
				s.Actors[0].Attack, s.Actors[0].Agility, s.Actors[0].Hit = 20, 7, 60
				return nil
			}
			draws := []float64{0, .5, 0}
			want := []int{101, 200, hit + 5}
			if id == "job-77-entry-0" {
				draws = append(draws, 0, .5, 0)
				want = []int{75, 40, hit, 20, 7, 60}
			}
			executeDual(t, &s, c, d, id, []string{"enemy"}, draws...)
			if !reflect.DeepEqual(observed, want) {
				t.Fatal(id, hit, observed, want)
			}
			if id == "job-77-entry-0" {
				if s.Actors[0].Attack != 101 || s.Actors[0].Agility != 7 || s.Actors[0].Hit != 60 || c.ToolCategory.SavedAttack != 101 {
					t.Fatal(s.Actors[0], c)
				}
			} else if s.Actors[0].Agility != 40 || s.Actors[0].Hit != hit || s.Actors[0].Attack != 20 {
				t.Fatal(s.Actors[0])
			}
		}
	}
}

func TestDualAgilityCasterDeathBetweenHits(t *testing.T) {
	for _, id := range []string{"job-77-entry-0", "job-77-entry-2", "job-77-entry-3", "job-77-entry-4", "job-77-entry-4-temporary", "job-83-entry-0", "job-83-entry-3"} {
		s, c, d := dualAgilityFixture()
		// One-use defense is bypassed by the captured release variant, which clears it.
		s.Actors[0].HP = 1
		s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
		if id == "job-77-entry-3" {
			s.Actors[0].Status = corebattle.StatusConfusion
		}
		c.Defeat = func(string) error {
			s.Actors[0].HP = 0
			s.Actors[0].Attack, s.Actors[0].Agility, s.Actors[0].Hit = 20, 7, 50
			return nil
		}
		draws := []float64{0}
		wantCalls := 1
		switch id {
		case "job-77-entry-0":
			draws, wantCalls = []float64{.5, 0, .5, 0}, 2
		case "job-83-entry-0":
			draws = []float64{.5, 0}
		case "job-77-entry-2":
			draws = []float64{0, 0}
		case "job-77-entry-3":
			draws = []float64{0, 0}
		case "job-77-entry-4", "job-77-entry-4-temporary":
			draws = []float64{0, 0}
		}
		frame := executeDual(t, &s, c, d, id, []string{"enemy", "tail"}, draws...)
		if s.Actors[0].HP != 0 || c.DamageCalls != wantCalls {
			t.Fatal(id, s, c.DamageCalls)
		}
		if id == "job-77-entry-0" && s.Actors[0].Attack != 100 {
			t.Fatal(s.Actors[0])
		}
		if id == "job-83-entry-0" && (s.Actors[0].Agility != 40 || s.Actors[0].Hit != 95) {
			t.Fatal(s.Actors[0])
		}
		if id == "job-83-entry-3" && !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
			t.Fatal(c.Arguments, frame.Arguments)
		}
	}
}

func TestDualAgilityTensionAreaAndStrongestBoundaries(t *testing.T) {
	for _, tension := range []float64{1, 3, 5} {
		s, c, d := dualAgilityFixture()
		s.Actors[1].Tension = tension
		executeDual(t, &s, c, d, "job-77-entry-1", []string{"enemy"}, 0)
		if s.Actors[1].HP != 1000-int(45*(tension+1)/2) {
			t.Fatal(tension, s)
		}
	}
	for _, draw := range []float64{0, .999} {
		s, c, d := dualAgilityFixture()
		executeDual(t, &s, c, d, "job-77-entry-2", []string{"unused"}, append([]float64{draw}, make([]float64, 2+int(draw*2))...)...)
		if c.AreaDamageCalls != 2+int(draw*2) || s.Actors[1].HP != 1000-27*c.AreaDamageCalls {
			t.Fatal(s, c)
		}
	}
	for _, agility := range []int{16, 17, 18} { // AT30×.85 = AG17×1.5 = 25.5.
		s, c, d := dualAgilityFixture()
		s.Actors[0].Attack, s.Actors[0].Agility = 30, agility
		executeDual(t, &s, c, d, "job-83-entry-5", []string{"enemy"}, 0)
		want := 22
		if agility == 18 {
			want = 24
		}
		if s.Actors[1].HP != 1000-want {
			t.Fatal(agility, s)
		}
	}
}

func TestDualAgilityDeathChanceAndDefeatDescendant(t *testing.T) {
	for _, draw := range []float64{.19, .19001} {
		s, c, d := dualAgilityFixture()
		s.Actors[1].HP = 999
		defeat := &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &corebattle.DefeatRewards{
			Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
			Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		}}
		c.Status = &corebattle.StatusContext{Defeat: defeat}
		executeDual(t, &s, c, d, "job-83-entry-2", []string{"enemy"}, draw)
		if (s.Actors[1].HP == 0) != (draw == .19) || c.Status.DeathCalls != 1 {
			t.Fatal(s, c.Status)
		}
	}
	s, c, d := dualAgilityFixture()
	s.Actors[1].HP = 999
	c.Status = &corebattle.StatusContext{}
	frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	if _, err := d.InvocationAction(&s, &frame, []string{"job-83-entry-2"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestDualAgilityAccuracyCapAndCapturedTargetDeath(t *testing.T) {
	for _, hit := range []int{74, 75, 76, 95, 99, 100} {
		s, c, d := dualAgilityFixture()
		s.Actors[0].Hit, s.Actors[2].Hit = hit, hit
		executeDual(t, &s, c, d, "job-83-entry-1", nil)
		if s.Actors[0].Hit != min(95, hit+20) || s.Actors[2].Hit != min(95, hit+20) {
			t.Fatal(hit, s)
		}
	}
	s, c, d := dualAgilityFixture()
	s.Actors[1].HP = 1
	c.Defeat = func(string) error { s.Actors[1].HP = 0; return nil }
	executeDual(t, &s, c, d, "job-77-entry-3", []string{"enemy", "fallen", "self"}, 0)
	if c.DamageCalls != 1 || !reflect.DeepEqual(c.Arguments, []string{"fallen", "self"}) || s.Actors[0].HP != 1000 {
		t.Fatal(s, c)
	}
	// A rejected external transition still restores the callback's saved values.
	for _, id := range []string{"job-77-entry-0", "job-83-entry-0"} {
		s, c, _ := dualAgilityFixture()
		s.Actors[1].HP = 1
		failure := errors.New("owner failed")
		c.Defeat = func(string) error {
			s.Actors[0].Attack, s.Actors[0].Agility, s.Actors[0].Hit = 20, 7, 50
			return failure
		}
		c.Arguments = []string{"enemy"}
		err := s.ApplyDualAgility(c, id, &damageGemDraws{t: t, draws: []float64{.5, 0}})
		if !errors.Is(err, failure) || id == "job-77-entry-0" && s.Actors[0].Attack != 100 || id == "job-83-entry-0" && (s.Actors[0].Agility != 40 || s.Actors[0].Hit != 95) {
			t.Fatal(id, err, s)
		}
	}
}

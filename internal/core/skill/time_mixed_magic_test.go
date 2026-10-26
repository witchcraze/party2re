package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func timeMagicFixture() (corebattle.LifetimeState, *corebattle.TimeMixedMagicContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].HP, s.Actors[0].Defense = 500, 80
	s.Actors[0].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 80, Agility: 40}
	s.Actors[1].HP, s.Actors[1].Defense, s.Actors[1].Agility = 900, 80, 40
	s.Actors[1].BaseStats = s.Actors[0].BaseStats
	c := &corebattle.TimeMixedMagicContext{DamageContext: base.DamageContext}
	c.Defeat = func(id string) error {
		for i := range s.Actors {
			if s.Actors[i].ID == id {
				s.Actors[i].HP = 0
			}
		}
		return nil
	}
	var d BattleDefinitions
	d.RegisterTimeMixedMagic(c)
	return s, c, d
}

func executeTimeMagic(t *testing.T, s *corebattle.LifetimeState, c *corebattle.TimeMixedMagicContext, d BattleDefinitions, id string, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
		t.Fatal(id, err, rng.used, len(draws))
	}
}

// Independent body vectors: ff97fc5:_skill.cgi, complete spans in each row.
func TestTimeMixedMagicManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp int
		span             string
		damage           int
		draws            []float64
	}{
		{29, 0, 10, 4, "2492", 0, nil},
		{29, 1, 20, 4, "2493", 0, nil},
		{29, 2, 30, 14, "2496-2502", 90, []float64{0, 0, 0, 0, 0}},
		{29, 3, 50, 9, "2504", 0, nil},
		{29, 4, 70, 9, "2505", 0, nil},
		{29, 5, 110, 30, "2506", 450, nil},
		{29, 6, 150, 50, "2509-2515", 356, []float64{0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{30, 0, 10, 5, "2540", 0, []float64{0}},
		{30, 1, 20, 5, "2541", 31, []float64{0}},
		{30, 2, 40, 5, "2544-2549", 0, nil},
		{30, 3, 60, 11, "2551", 72, []float64{0}},
		{30, 4, 80, 7, "2554-2559", 0, nil},
		{30, 5, 100, 18, "2561", 0, []float64{0}},
		{30, 6, 120, 20, "2564-2569", 0, nil},
		{30, 7, 150, 40, "2574-2579", 324, []float64{0, 0, 0, 0}},
		{30, 8, 150, 58, "2584-2603", 315, []float64{0}},
		{31, 0, 11, 11, "2612-2616", 450, []float64{0}},
		{31, 1, 44, 4, "2621-2633", 900, []float64{.5}},
		{31, 2, 66, 18, "2639", 454, []float64{0}},
		{31, 3, 77, 34, "2644-2651", 0, nil},
		{31, 5, 121, 36, "2670", 0, []float64{0, 0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := timeMagicFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.job == 30 && row.pos == 8) {
				t.Fatal(row.span, entry)
			}
			args := []string{"enemy", "tail"}
			if row.job == 29 && row.pos == 1 || row.job == 30 && (row.pos == 0 || row.pos == 2 || row.pos == 4 || row.pos == 5 || row.pos == 6) {
				args[0] = "self"
			}
			executeTimeMagic(t, &s, c, d, id, args, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if 900-e.HP != row.damage || a.MP != 80 || a.CMP != 90 {
				t.Fatal(row.span, s)
			}
			switch id {
			case "job-29-entry-0":
				if e.Agility != 22 || c.StatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-29-entry-1":
				if a.Agility != 58 || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-29-entry-3":
				if e.Agility != 26 || c.PartyStatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-29-entry-4":
				if a.Agility != 68 || c.PartyStatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-30-entry-0", "job-30-entry-5":
				want := 554
				if row.pos == 5 {
					want = 662
				}
				if a.HP != want || c.HealCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-30-entry-2", "job-30-entry-4", "job-30-entry-6":
				want := map[int]corebattle.TemporaryEffect{2: corebattle.TemporaryReduceMagic, 4: corebattle.TemporaryReflectMagic, 6: corebattle.TemporaryRevival}[row.pos]
				if a.Temporary != want {
					t.Fatal(a)
				}
			case "job-31-entry-0":
				if a.HP != 0 {
					t.Fatal(a)
				}
			case "job-31-entry-3":
				if a.Defense != 160 || a.Temporary != corebattle.TemporaryReduceMagic || s.Actors[2].Temporary != "" {
					t.Fatal(s)
				}
			case "job-31-entry-5":
				// Power500 is captured before healing the caster; fallen redirects to self.
				if a.HP != 1000 || c.HealCalls != 2 || c.PartyHealCalls != 1 {
					t.Fatal(a, c)
				}
			}
			consumed := row.job == 29 && (row.pos == 0 || row.pos == 1 || row.pos == 5) || row.job == 30 && row.pos != 7 || row.job == 31 && (row.pos == 0 || row.pos == 2)
			wantArgs := args
			if consumed {
				wantArgs = args[1:]
			}
			if !reflect.DeepEqual(c.Arguments, wantArgs) {
				t.Fatal(c.Arguments, wantArgs)
			}
		})
	}
}

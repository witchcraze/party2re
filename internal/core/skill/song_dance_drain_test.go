package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func songDanceFixture() (corebattle.LifetimeState, *corebattle.SongDanceDrainContext, BattleDefinitions) {
	s, base, _ := controlFixture()
	s.Actors[0].MaxMP = 1000
	s.Actors[2].MaxHP, s.Actors[2].Temporary = 1000, corebattle.TemporaryDefense
	c := &corebattle.SongDanceDrainContext{DamageContext: base.DamageContext, Status: &corebattle.StatusContext{}}
	c.Status.Defeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &corebattle.DefeatRewards{
		Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
	}}
	var d BattleDefinitions
	d.RegisterSongDanceDrain(c)
	return s, c, d
}

func executeSongDance(t *testing.T, s *corebattle.LifetimeState, c *corebattle.SongDanceDrainContext, d BattleDefinitions, id string, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(id, err)
	}
	if rng.used != len(draws) {
		t.Fatal(id, rng.used, len(draws))
	}
}

// Complete intrinsic vectors: ff97fc5:party2/lib/_skill.cgi:1710–1759,2036–2069.
func TestSongDanceDrainManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage int
		draws                    []float64
	}{
		{13, 0, 5, 3, 0, nil}, {13, 1, 15, 7, 0, []float64{0, 0}}, {13, 2, 30, 5, 0, nil},
		{13, 3, 40, 6, 0, nil}, {13, 4, 60, 9, 0, []float64{0}}, {13, 5, 90, 10, 0, nil},
		{14, 0, 4, 3, 0, nil}, {14, 1, 9, 3, 0, nil}, {14, 2, 16, 5, 0, nil},
		{14, 3, 30, 9, 36, []float64{0}}, {14, 5, 70, 16, 72, []float64{0, 0, 0, 0, 0}}, {14, 6, 100, 12, 0, []float64{0, 0}},
		{20, 0, 4, 4, 0, []float64{0}}, {20, 1, 9, 6, 40, []float64{0, 0}}, {20, 2, 16, 6, 0, nil},
		{20, 3, 20, 9, 0, []float64{0}}, {20, 4, 36, 7, 0, []float64{0}}, {20, 5, 60, 24, 900, []float64{0}},
		{20, 6, 100, 18, 76, []float64{0, 0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := songDanceFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended {
				t.Fatal(entry)
			}
			if id == "job-13-entry-2" {
				s.Actors[0].Status, s.Actors[2].Status = corebattle.StatusSleep, corebattle.StatusSleep
			}
			executeSongDance(t, &s, c, d, id, []string{"enemy", "tail"}, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			wantHP, wantMP := 500, 80
			if 900-e.HP != row.damage || a.CMP != 90 || s.Actors[2].HP != 0 || s.Actors[2].Temporary != corebattle.TemporaryDefense {
				t.Fatal(a, e, s.Actors[2])
			}
			switch id {
			case "job-13-entry-0", "job-14-entry-1":
				want := 75
				if row.job == 14 {
					want = 60
				}
				if e.MP != want || c.StatDownCalls != 1 || c.PartyStatDownCalls != map[int]int{13: 1, 14: 0}[row.job] {
					t.Fatal(e, c)
				}
			case "job-13-entry-1", "job-14-entry-6":
				// The party helper redirects its fallen recipient to the caster.
				wantHP = 554
				if row.job == 14 {
					wantHP = 752
				}
				if c.HealCalls != 2 || c.PartyHealCalls != 1 {
					t.Fatal(c)
				}
			case "job-13-entry-2":
				if a.Status != "" || s.Actors[2].Status != corebattle.StatusSleep || c.Status.PartyCureCalls != 1 || c.Status.CureCalls != 2 || !c.AdditionalEffect {
					t.Fatal(s.Actors, c.Status)
				}
			case "job-13-entry-3":
				if a.Defense != 90 || c.StatUpCalls != 2 || c.PartyStatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-13-entry-4", "job-20-entry-3", "job-20-entry-4":
				want := corebattle.StatusSleep
				if id == "job-20-entry-4" {
					want = corebattle.StatusConfusion
				}
				if e.Status != want || c.Status.AreaStatusCalls != 1 || c.Status.StatusCalls != 1 {
					t.Fatal(e, c.Status)
				}
			case "job-13-entry-5":
				if a.Attack != 220 || c.StatUpCalls != 2 || c.PartyStatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-14-entry-0":
				if a.Agility != 56 || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-14-entry-2":
				if a.Temporary != corebattle.TemporaryDeflect {
					t.Fatal(a)
				}
			case "job-14-entry-3", "job-14-entry-5":
				calls, area := 1, 1
				if row.pos == 5 {
					calls, area = 2, 0
				}
				if c.DamageCalls != calls || c.AreaDamageCalls != area {
					t.Fatal(c)
				}
			case "job-20-entry-0":
				if e.Status != corebattle.StatusDofuu || c.Status.StatusCalls != 1 || c.Status.AreaStatusCalls != 0 {
					t.Fatal(e, c.Status)
				}
			case "job-20-entry-1":
				wantMP = 84
				if c.DamageCalls != 1 || c.MPHealCalls != 1 {
					t.Fatal(c)
				}
			case "job-20-entry-2":
				if a.Temporary != corebattle.TemporaryReduceMagic {
					t.Fatal(a)
				}
			case "job-20-entry-5":
				if c.Status.AreaDeathCalls != 1 || c.Status.DeathCalls != 1 {
					t.Fatal(c.Status)
				}
			case "job-20-entry-6":
				wantHP = 534
				if c.DamageCalls != 1 || c.HealCalls != 1 {
					t.Fatal(c)
				}
			}
			if a.HP != wantHP || a.MP != wantMP {
				t.Fatal(a, wantHP, wantMP)
			}
			consumed := id == "job-14-entry-1" || id == "job-20-entry-0" || id == "job-20-entry-1" || id == "job-20-entry-6"
			wantArgs := 2
			if consumed {
				wantArgs = 1
			}
			if len(c.Arguments) != wantArgs || c.Arguments[len(c.Arguments)-1] != "tail" {
				t.Fatal(c.Arguments)
			}
		})
	}
}

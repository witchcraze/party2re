package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func lateSongFixture() (corebattle.LifetimeState, *corebattle.LateBreathSongContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].HP, s.Actors[0].MaxMP = 500, 1000
	c := &corebattle.LateBreathSongContext{DamageContext: base.DamageContext, HeldBooks: map[string]string{"self": "none"}, Death: &corebattle.StatusContext{}}
	c.Death.Defeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &corebattle.DefeatRewards{
		Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
	}}
	var d BattleDefinitions
	d.RegisterLateBreathSong(c)
	return s, c, d
}

func executeLateSong(t *testing.T, s *corebattle.LifetimeState, c *corebattle.LateBreathSongContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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

// Complete intrinsic vectors: ff97fc5:_skill.cgi:6686–6709,6744–6797,6859–6866.
func TestLateSongManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage int
		draws                    []float64
	}{
		{84, 0, 5, 6, 0, nil}, {84, 1, 20, 16, 126, []float64{0}},
		{84, 2, 50, 26, 90, []float64{0}}, {84, 3, 80, 14, 0, nil},
		{85, 0, 25, 1, 0, []float64{0, 0}}, {85, 1, 50, 39, 900, []float64{0}},
		{85, 2, 75, 47, 0, nil}, {85, 3, 125, 45, 0, []float64{0}},
		{85, 4, 150, 35, 90, []float64{0}}, {86, 4, 100, 54, 180, []float64{0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := lateSongFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended {
				t.Fatal(entry)
			}
			if id == "job-85-entry-1" {
				s.Actors[1].HP = 900
			}
			before := s.Actors[1].HP
			args := []string{"enemy", "tail"}
			if id == "job-84-entry-3" {
				args[0] = "self"
			}
			if id == "job-85-entry-2" {
				s.Actors[1].Status, s.Actors[1].Temporary = corebattle.StatusSleep, corebattle.TemporaryDouble
			}
			executeLateSong(t, &s, c, d, id, args, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if before-e.HP != row.damage || a.CMP != 90 {
				t.Fatal(a, e)
			}
			wantMP := 80
			switch id {
			case "job-84-entry-0":
				if a.Hit != 95 || a.Temporary != corebattle.TemporaryReduceBreath {
					t.Fatal(a)
				}
			case "job-84-entry-3":
				if a.Tension != 1.7 || c.TensionCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-85-entry-0":
				wantMP = 182
				if a.HP != 230 || c.MPHealCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-85-entry-1":
				if c.Death.DeathCalls != 1 || c.Death.AreaDeathCalls != 1 {
					t.Fatal(c.Death)
				}
			case "job-85-entry-2":
				if e.Status != "" || e.Temporary != "" {
					t.Fatal(e)
				}
			case "job-85-entry-3":
				if e.Status != corebattle.StatusSabaku || e.Tension != 1 {
					t.Fatal(e)
				}
			}
			if a.MP != wantMP {
				t.Fatal(a.MP, wantMP)
			}
			shifted := id != "job-84-entry-0" && id != "job-85-entry-0" && id != "job-85-entry-1"
			remaining := 2
			if shifted {
				remaining = 1
			}
			if len(c.Arguments) != remaining {
				t.Fatal(c.Arguments)
			}
		})
	}
}

func TestLateSongBurnCaptureAndBookPower(t *testing.T) {
	for _, row := range []struct {
		field, iconGender, requester string
		boosted                      bool
	}{
		{"burn", "m", "m", true}, {"burn", "f", "m", false}, {"", "m", "m", false}, {"burn", "f", "f", true},
	} {
		s, c, d := lateSongFixture()
		s.Actors[0].Icon = "assets/images/placeholders/battle/forms.svg#job-84-ex-" + row.iconGender
		c.BurnStrike = s.Actors[0].LateBreathSongBurn(row.field, row.requester)
		if c.BurnStrike != row.boosted {
			t.Fatal(row)
		}
		// Execution uses captured facts after the field and appearance are gone.
		s.Actors[0].Icon, s.Fields.Temporary = "changed", ""
		executeLateSong(t, &s, c, d, "job-84-entry-2", []string{"enemy"}, 0)
		want := 910
		if row.boosted {
			want = 888
		}
		if s.Actors[1].HP != want {
			t.Fatal(row, s.Actors[1])
		}
	}
	for _, book := range []string{"none", "ordinary", "forbidden"} {
		s, c, d := lateSongFixture()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		ops, err := d.InvocationAction(&s, &f, []string{"job-86-entry-4"}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		c.HeldBooks["self"] = book
		if err := s.ApplyInvocations(&f, d.Invocations, ops, &damageGemDraws{t: t, draws: []float64{0}}); err != nil {
			t.Fatal(err)
		}
		want, hp := 820, 500
		if book == "ordinary" {
			want = 640
		}
		if book == "forbidden" {
			want, hp = 325, 349
		}
		if s.Actors[1].HP != want || s.Actors[0].HP != hp {
			t.Fatal(book, s)
		}
	}
}

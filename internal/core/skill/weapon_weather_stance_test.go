package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func weaponWeatherStanceFixture() (corebattle.LifetimeState, *corebattle.WeaponWeatherStanceContext, BattleDefinitions) {
	s, base, _ := counterResetFixture()
	s.Actors[1].Defense, s.Actors[1].Agility, s.Actors[1].Hit = 80, 100, 95
	c := &corebattle.WeaponWeatherStanceContext{DamageContext: base.DamageContext, StatusDefeat: base.StatusDefeat}
	var d BattleDefinitions
	d.RegisterWeaponWeatherStance(c)
	return s, c, d
}

func executeWeaponWeatherStance(t *testing.T, s *corebattle.LifetimeState, c *corebattle.WeaponWeatherStanceContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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

// Each concrete binding has its own intrinsic vector, from ff97fc5:_skill.cgi
// 2272–2289,2408–2445,2455–2486; shared primitive tests are not the oracle.
func TestWeaponWeatherStanceManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage int
	}{
		{24, 0, 10, 6, 32}, {24, 1, 30, 6, 36}, {24, 2, 50, 16, 0},
		{24, 3, 70, 14, 45}, {24, 4, 130, 25, 207}, {24, 5, 130, 15, 0},
		{27, 0, 5, 6, 0}, {27, 1, 15, 5, 72}, {27, 2, 25, 6, 0},
		{27, 3, 40, 11, 72}, {27, 4, 55, 4, 0}, {27, 5, 70, 5, 0},
		{27, 6, 90, 27, 144}, {27, 7, 110, 15, 0}, {27, 8, 110, 45, 0},
		{28, 0, 5, 0, 0}, {28, 1, 15, 3, 0}, {28, 2, 30, 5, 0},
		{28, 4, 70, 4, 0}, {28, 5, 100, 20, 0}, {28, 6, 140, 10, 900},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := weaponWeatherStanceFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.job == 24 && row.pos == 5 || row.job == 27 && row.pos == 8) {
				t.Fatal(entry)
			}
			args, draws := []string{"enemy", "tail"}, []float64(nil)
			if row.damage > 0 || row.job == 28 && (row.pos == 1 || row.pos == 5) {
				draws = []float64{0}
			}
			if id == "job-24-entry-2" {
				args[0] = "self"
			}
			if id == "job-27-entry-4" {
				s.Actors[0].Status = corebattle.StatusSleep
			}
			if id == "job-27-entry-8" {
				s.Actors[0].Status, draws = corebattle.StatusPoison, []float64{0, 0}
			}
			if id == "job-27-entry-7" {
				s.Actors[1].Temporary, s.Actors[1].Tension = corebattle.TemporaryNullMagic, 4
			}
			s.Actors[0].Hit = 30
			executeWeaponWeatherStance(t, &s, c, d, id, args, draws...)
			a, e := s.Actors[0], s.Actors[1]
			if 900-e.HP != row.damage || a.MP != 80 || a.CMP != 90 {
				t.Fatal(a, e)
			}
			switch id {
			case "job-24-entry-2":
				if a.Attack != 200 || c.StatUpCalls != 1 || c.PartyStatUpCalls != 0 {
					t.Fatal(a, c)
				}
			case "job-24-entry-5":
				if a.Extended != corebattle.ExtendedMagicSword || a.ExtendedTurns != 7 {
					t.Fatal(a)
				}
			case "job-27-entry-0":
				if e.Hit != 76 || c.StatDownCalls != 1 || c.PartyStatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-27-entry-2":
				if e.Agility != 70 || c.StatDownCalls != 1 || c.PartyStatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-27-entry-4":
				if a.Status != "" || c.CureCalls != 2 || c.PartyCureCalls != 1 || !c.AdditionalEffect {
					t.Fatal(a, c)
				}
			case "job-27-entry-5":
				if a.Temporary != corebattle.TemporaryReflectBreath || s.Actors[2].Temporary != "" {
					t.Fatal(s)
				}
			case "job-27-entry-7":
				if e.Attack != 40 || e.Defense != 80 || e.Temporary != "" || e.Tension != 1 || c.StatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-27-entry-8":
				if a.HP != 1000 || a.Status != "" || c.HealCalls != 2 || c.PartyHealCalls != 1 || c.CureCalls != 1 || c.PartyCureCalls != 0 || c.AdditionalEffect {
					t.Fatal(a, c)
				}
			case "job-28-entry-0":
				if a.Hit != 95 || s.Actors[2].Hit != 99 {
					t.Fatal(s)
				}
			case "job-28-entry-2", "job-28-entry-4":
				want := corebattle.TemporaryDeflect
				if row.pos == 4 {
					want = corebattle.TemporaryNullAttack
				}
				if a.Temporary != want {
					t.Fatal(a)
				}
			case "job-28-entry-1", "job-28-entry-5":
				name := "immobilization"
				if row.pos == 5 {
					name = "immobilization-wave"
				}
				if entry.Name != name {
					t.Fatal(entry)
				}
				if e.Status != corebattle.StatusDofuu || c.StatusCalls != 1 || c.AreaStatusCalls != row.pos/5 {
					t.Fatal(e, c)
				}
				decision, err := s.Actors[1].CheckIncapacitation(corebattle.IncapacitationContext{}, noArithmeticDraws{t})
				if err != nil || decision.NormalAction || s.Actors[1].Status != "" {
					t.Fatal(decision, err, s.Actors[1])
				}
			case "job-28-entry-6":
				if c.DeathCalls != 1 {
					t.Fatal(c)
				}
			}
			areaCalls := 0
			if row.job == 27 && (row.pos == 3 || row.pos == 6) {
				areaCalls = 1
			}
			if row.damage > 0 && id != "job-28-entry-6" && (c.DamageCalls != 1 || c.AreaDamageCalls != areaCalls) {
				t.Fatal(c)
			}
			consumed := row.job == 24 && row.pos != 5 || row.job == 27 && (row.pos == 1 || row.pos == 7) || row.job == 28 && (row.pos == 1 || row.pos == 6)
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

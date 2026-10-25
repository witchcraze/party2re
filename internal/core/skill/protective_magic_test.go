package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func protectiveMagicFixture() (corebattle.LifetimeState, *corebattle.ProtectiveMagicContext, BattleDefinitions) {
	s, base, _ := hybridBreathFixture()
	s.Actors[2].MP, s.Actors[2].MaxMP = 15, 100
	c := &corebattle.ProtectiveMagicContext{DamageContext: base.DamageContext}
	var d BattleDefinitions
	d.RegisterProtectiveMagic(c)
	return s, c, d
}

func executeProtectiveMagic(t *testing.T, s *corebattle.LifetimeState, c *corebattle.ProtectiveMagicContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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

// Intrinsic binding vectors: ff97fc5:_skill.cgi:2722–2862. Zero damage/healing
// rolls give 90% of fixed power; fallen party-heal targets fall back to self.
func TestProtectiveMagicManifest(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, damage, hp int
		tmp                          corebattle.TemporaryEffect
		draws                        []float64
	}{
		{32, 0, 5, 5, 90, 500, "", []float64{0, 0}},
		{32, 1, 25, 10, 0, 590, "", []float64{0, 0}},
		{32, 2, 50, 20, 0, 500, corebattle.TemporaryReduceAttack, nil},
		{32, 3, 70, 30, 0, 500, corebattle.TemporaryReflectMagic, nil},
		{32, 5, 150, 50, 198, 500, "", []float64{0}},
		{33, 0, 5, 6, 0, 500, corebattle.TemporaryReduceMagic, nil},
		{33, 1, 15, 12, 63, 500, "", []float64{0}},
		{33, 2, 30, 7, 0, 500, corebattle.TemporaryReduceBreath, nil},
		{33, 3, 70, 25, 0, 716, "", []float64{0, 0}},
		{33, 4, 100, 16, 0, 500, "", nil},
		{33, 5, 130, 34, 144, 500, "", []float64{0}},
		{33, 7, 160, 33, 0, 500, "", nil},
		{34, 0, 10, 0, 0, 500, "", nil},
		{34, 1, 30, 15, 99, 500, "", []float64{0}},
		{34, 2, 60, 25, 0, 770, "", []float64{0}},
		{34, 3, 90, 40, 162, 500, "", []float64{0}},
		{34, 4, 120, 6, 0, 500, corebattle.TemporaryNullMagic, nil},
		{34, 5, 150, 80, 0, 1000, "", []float64{0, 0}},
		{34, 6, 180, 30, 166, 500, "", []float64{0}},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := protectiveMagicFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != (row.job == 33 && row.pos == 7) {
				t.Fatal(entry)
			}
			args := []string{"enemy", "tail"}
			if id == "job-33-entry-4" || id == "job-34-entry-0" {
				args[0] = "self"
			}
			if id == "job-33-entry-7" {
				s.Actors[0].Status = corebattle.StatusPoison
			}
			executeProtectiveMagic(t, &s, c, d, id, args, row.draws...)
			a, e, fallen := s.Actors[0], s.Actors[1], s.Actors[2]
			if 900-e.HP != row.damage || a.HP != row.hp || a.Temporary != row.tmp || a.MP != 80 || a.CMP != 90 || fallen.HP != 0 || fallen.Temporary != "" {
				t.Fatal(a, e, fallen)
			}
			if id == "job-33-entry-4" && (a.Attack != 200 || c.StatUpCalls != 1) {
				t.Fatal(a, c)
			}
			if id == "job-33-entry-7" && (a.Status != "" || a.Extended != corebattle.ExtendedAdditional || a.ExtendedTurns != 5 || c.CureCalls != 1) {
				t.Fatal(a, c)
			}
			wantMP := 15
			if id == "job-34-entry-6" {
				wantMP = 0
			}
			wantCalls := 0
			if row.damage > 0 {
				wantCalls = 1
			}
			if fallen.MP != wantMP || c.DamageCalls != wantCalls {
				t.Fatal(fallen, c)
			}
		})
	}
}

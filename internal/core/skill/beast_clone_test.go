package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func beastCloneFixture() (corebattle.LifetimeState, *corebattle.BeastCloneContext, BattleDefinitions) {
	s, base, _ := hybridBreathFixture()
	s.Actors[1].Defense, s.Actors[1].BaseStats.Defense = 0, 0
	s.Actors[1].Hit = 100
	level := 40
	c := &corebattle.BeastCloneContext{DamageContext: base.DamageContext}
	c.RequesterLevel = &level
	c.StatusDefeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: c.TargetContext}, Rewards: &corebattle.DefeatRewards{
		Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
	}}
	var d BattleDefinitions
	d.RegisterBeastClone(c)
	return s, c, d
}

func executeBeastClone(t *testing.T, s *corebattle.LifetimeState, c *corebattle.BeastCloneContext, d BattleDefinitions, id string, args []string, draws ...float64) {
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

// Intrinsic oracles: ff97fc5:_skill.cgi:1546–1596,1674–1703,2341–2401.
// Physical nondirect lower rolls floor(.9*power/2); direct breath floors .9*power.
func TestBeastCloneManifest(t *testing.T) {
	for _, row := range []struct{ job, pos, sp, mp, damage, draws int }{
		{10, 0, 0, 0, 168, 1}, {10, 1, 1, 0, 0, 1}, {10, 2, 3, 3, 0, 0},
		{10, 3, 10, 4, 57, 1}, {10, 4, 20, 10, 0, 1}, {10, 5, 40, 9, 0, 1},
		{10, 6, 60, 1, 0, 0}, {10, 7, 80, 7, 0, 0}, {10, 8, 100, 20, 56, 5},
		{12, 0, 0, 0, 168, 1}, {12, 1, 3, 2, 16, 1}, {12, 2, 10, 7, 0, 1},
		{12, 3, 18, 5, 45, 1}, {12, 4, 32, 11, 49, 2}, {12, 5, 50, 5, 0, 1},
		{12, 6, 80, 14, 81, 1}, {12, 7, 110, 12, 144, 2},
		{26, 0, 5, 5, 45, 1}, {26, 1, 15, 10, 0, 1}, {26, 2, 30, 7, 0, 0},
		{26, 3, 40, 7, 0, 1}, {26, 4, 50, 6, 0, 0}, {26, 5, 65, 11, 900, 1},
		{26, 6, 80, 10, 0, 0}, {26, 7, 110, 15, 0, 0}, {26, 8, 110, 30, 0, 5},
	} {
		id := fmt.Sprintf("job-%d-entry-%d", row.job, row.pos)
		t.Run(id, func(t *testing.T) {
			s, c, d := beastCloneFixture()
			entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
			extended := row.pos == 0 && (row.job == 10 || row.job == 12) || row.job == 26 && row.pos == 8
			if entry.ID != id || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended != extended {
				t.Fatal(entry)
			}
			args := []string{"enemy", "tail"}
			if id == "job-10-entry-2" || id == "job-10-entry-4" {
				args[0] = "self"
			}
			draws := make([]float64, row.draws)
			executeBeastClone(t, &s, c, d, id, args, draws...)
			a, e := s.Actors[0], s.Actors[1]
			if 900-e.HP != row.damage || a.MP != 80 || a.CMP != 90 {
				t.Fatal(a, e)
			}
			consumed := false
			switch id {
			case "job-10-entry-0", "job-12-entry-0":
				consumed = true
				if a.HP != 483 || c.End.ImmediateRiskCalls != 1 {
					t.Fatal(a, c.End)
				}
			case "job-10-entry-1":
				if a.Status != corebattle.StatusSleep || a.HP != 950 || c.HealCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-10-entry-2":
				consumed = true
				if a.Defense != 112 || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-10-entry-3":
				consumed = true
				if a.HP != 496 || c.End.ImmediateRiskCalls != 1 {
					t.Fatal(a, c.End)
				}
			case "job-10-entry-4":
				consumed = true
				if a.HP != 581 || c.HealCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-10-entry-5":
				if e.Status != corebattle.StatusSleep || c.StatusCalls != 1 || c.AreaStatusCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-10-entry-6":
				if a.Temporary != corebattle.TemporaryAbsorbMagic {
					t.Fatal(a)
				}
			case "job-10-entry-7":
				if a.Defense != 120 || a.Temporary != corebattle.TemporaryReduceMagic || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-10-entry-8":
				if c.DamageCalls != 2 {
					t.Fatal(c)
				}
			case "job-12-entry-1", "job-12-entry-3", "job-12-entry-6", "job-26-entry-0":
				if c.AreaDamageCalls != 1 || c.DamageCalls != 1 {
					t.Fatal(c)
				}
			case "job-12-entry-2", "job-26-entry-3":
				if e.Status != corebattle.StatusDeadlyPoison || c.StatusCalls != 1 || c.AreaStatusCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-12-entry-4":
				consumed = true
				if !c.AdditionalEffect || c.DamageCalls != 1 || c.StatusCalls != 1 || e.Status != corebattle.StatusParalyze {
					t.Fatal(e, c)
				}
			case "job-12-entry-5":
				consumed = true
				if e.Status != corebattle.StatusDofuu || c.StatusCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-12-entry-7":
				consumed = true
				if c.DamageCalls != 2 {
					t.Fatal(c)
				}
			case "job-26-entry-1":
				if e.Status != corebattle.StatusParalyze || c.AreaStatusCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-26-entry-2":
				if e.Hit != 80 || c.StatDownCalls != 1 || c.PartyStatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-26-entry-4":
				if a.Agility != 60 || c.StatUpCalls != 2 || c.PartyStatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-26-entry-5":
				consumed = true
				if c.DeathCalls != 1 {
					t.Fatal(c)
				}
			case "job-26-entry-6":
				if a.Agility != 80 || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-26-entry-7":
				consumed = true
				if e.Tension != 1 || e.Temporary != "" || c.StatDownCalls != 1 {
					t.Fatal(e, c)
				}
			case "job-26-entry-8":
				if len(s.Actors) != 4 || a.Temporary != corebattle.TemporaryCover || len(c.PartyIDs) != 3 {
					t.Fatal(s, c)
				}
				h := s.Actors[3]
				if h.RosterIdentity != "cover-clone" || h.HP != 112 || h.MP != 450 || h.Attack != 360 || h.Defense != 162 || h.Agility != 72 || h.Temporary != corebattle.TemporaryCovering {
					t.Fatal(h)
				}
			}
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

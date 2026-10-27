package skill

import (
	"fmt"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func npcStatusFixture() (corebattle.LifetimeState, *corebattle.NPCStatusContext, BattleDefinitions) {
	s, base, _ := npcOrderedFixture()
	s.Actors[0].NPC, s.Actors[0].MaxHP = true, 999
	s.Actors[1].HP, s.Actors[1].MaxHP = 800, 999
	c := &corebattle.NPCStatusContext{DamageContext: base.DamageContext, Death: &corebattle.DefeatContext{}}
	c.ActorNPC = true
	var d BattleDefinitions
	d.RegisterNPCStatus(c)
	return s, c, d
}

func executeNPCStatus(t *testing.T, s *corebattle.LifetimeState, c *corebattle.NPCStatusContext, d BattleDefinitions, job, pos int, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{fmt.Sprintf("npc-job-%d-position-%d", job, pos)}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
	if rng.used != len(draws) {
		t.Fatal(rng.used, draws)
	}
}

// Independent vectors: ff97fc5:_skill.cgi:7043–7178. Costs are metadata,
// applied after the intrinsic callback by the ordinary NPC consumer.
func TestNPCStatusAllPositions(t *testing.T) {
	for _, row := range []struct {
		job, pos, sp, mp, hp int
		status               string
		draws                []float64
	}{
		{88, 0, 5, 12, 800, corebattle.StatusDofuu, []float64{0}},
		{88, 1, 20, 24, 665, "", []float64{0}},
		{88, 2, 60, 15, 800, "", []float64{0}},
		{88, 3, 110, 38, 800, "", nil},
		{90, 0, 10, 4, 751, corebattle.StatusDeadlyPoison, []float64{0, 0}},
		{90, 1, 20, 6, 778, corebattle.StatusDeadlyPoison, []float64{0, 0}},
		{90, 2, 30, 8, 800, corebattle.StatusDeadlyPoison, []float64{0}},
		{91, 0, 10, 8, 746, corebattle.StatusParalyze, []float64{0, 0}},
		{91, 1, 20, 11, 764, corebattle.StatusParalyze, []float64{0, 0}},
		{91, 2, 30, 9, 800, corebattle.StatusParalyze, []float64{0}},
		{92, 0, 10, 8, 800, corebattle.StatusSleep, []float64{0}},
		{92, 1, 20, 15, 764, corebattle.StatusSleep, []float64{0, 0}},
		{92, 2, 30, 9, 800, corebattle.StatusSleep, []float64{0}},
		{93, 0, 10, 14, 0, "", []float64{0}},
		{93, 1, 20, 32, 0, "", []float64{0}},
		{93, 2, 30, 24, 0, "", []float64{0}},
		{93, 3, 40, 42, 200, corebattle.StatusDeadlyPoison, []float64{0}},
		{94, 0, 10, 1, 0, "", []float64{0}},
		{94, 1, 20, 0, 800, "", []float64{0}},
	} {
		for _, npc := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%d/npc-%t", row.job, row.pos, npc), func(t *testing.T) {
				s, c, d := npcStatusFixture()
				s.Actors[0].NPC, c.ActorNPC = npc, npc
				neutral := func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil }
				c.Death.Rewards = &corebattle.DefeatRewards{Validate: neutral, Apply: neutral}
				entry := d.Invocations.JobTables[row.job].Variants[0][row.pos]
				if entry.ID != fmt.Sprintf("npc-job-%d-position-%d", row.job, row.pos) || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended {
					t.Fatal(entry)
				}
				c.Defeat = func(id string) error {
					for i := range s.Actors {
						if s.Actors[i].ID == id {
							s.Actors[i].HP = 0
						}
					}
					return nil
				}
				executeNPCStatus(t, &s, c, d, row.job, row.pos, []string{"enemy", "tail"}, row.draws...)
				if s.Actors[1].HP != row.hp || s.Actors[1].Status != row.status || s.Actors[0].CMP != 90 {
					t.Fatal(s.Actors, c)
				}
				if row.job == 94 && row.pos == 0 {
					if s.Actors[0].HP != 0 || s.Actors[0].MP != 1 {
						t.Fatal(s.Actors[0])
					}
				} else if s.Actors[0].MP != 80 {
					t.Fatal(s.Actors[0])
				}
				if row.job == 94 && row.pos == 1 && s.Actors[0].Status != corebattle.StatusSleep {
					t.Fatal(s.Actors[0])
				}
				consumed := row.job == 88 && (row.pos == 1 || row.pos == 2) || row.job == 90 && row.pos < 2 || row.job == 91 && row.pos < 2 || row.job == 92 && row.pos < 2 || row.job == 93 && (row.pos == 0 || row.pos == 3)
				if consumed && (len(c.Arguments) != 1 || c.Arguments[0] != "tail") || !consumed && len(c.Arguments) != 2 {
					t.Fatal(c.Arguments)
				}
			})
		}
	}
}

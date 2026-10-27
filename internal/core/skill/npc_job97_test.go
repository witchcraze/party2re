package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func npc97Fixture() (corebattle.LifetimeState, *corebattle.NPCJob97Context, BattleDefinitions) {
	s, base, _ := npcOrderedFixture()
	s.Actors[0].Jobs.JobID = "job-97"
	s.Actors[1].MaxHP = 999
	c := &corebattle.NPCJob97Context{DamageContext: base.DamageContext, EquipmentReset: base.EquipmentReset}
	var d BattleDefinitions
	d.RegisterNPCJob97(c)
	return s, c, d
}

func executeNPC97(t *testing.T, s *corebattle.LifetimeState, c *corebattle.NPCJob97Context, d BattleDefinitions, pos int, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{fmt.Sprintf("npc-job-97-position-%d", pos)}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil {
		t.Fatal(err)
	}
	if rng.used != len(draws) || !reflect.DeepEqual(f.Arguments, args) {
		t.Fatal(rng.used, draws, f.Arguments, c.Arguments)
	}
}

// Independent vectors: ff97fc5:_skill.cgi:7217–7304. Ordinary selection and
// post-callback listed costs remain outside the intrinsic callback boundary.
func TestNPC97AllPositionsAndRoles(t *testing.T) {
	for pos, row := range []struct {
		mp, enemyHP, selfHP, attack, defense, damage, area, status int
		draws                                                      []float64
	}{
		{24, 886, 1000, 100, 80, 3, 0, 0, []float64{0, 0, 0, 0, 0, 0, 0}},
		{20, 880, 1000, 100, 80, 3, 0, 0, []float64{0, 0, 0, 0, 0, 0, 0}},
		{30, 1000, 1000, 100, 80, 0, 0, 0, nil},
		{9, 1000, 1000, 200, 40, 0, 0, 0, nil},
		{30, 730, 1000, 100, 80, 1, 1, 0, []float64{0}},
		{40, 888, 977, 100, 80, 1, 0, 0, []float64{0}},
		{33, 1000, 1000, 100, 80, 0, 0, 0, nil},
		{80, 640, 1000, 100, 80, 1, 1, 0, []float64{0}},
		{70, 775, 1000, 100, 80, 1, 1, 1, []float64{0, 0}},
		{42, 1000, 1000, 100, 80, 0, 0, 0, nil},
		{66, 766, 1000, 100, 80, 1, 1, 1, []float64{0, 0}},
	} {
		for _, npc := range []bool{false, true} {
			t.Run(fmt.Sprintf("position-%d/npc-%t", pos, npc), func(t *testing.T) {
				s, c, d := npc97Fixture()
				s.Actors[0].NPC, c.ActorNPC = npc, npc
				entries := d.Invocations.JobTables[97].Variants[0]
				entry := entries[pos]
				if len(entries) != 11 || entry.ID != fmt.Sprintf("npc-job-97-position-%d", pos) || entry.Name == "" || entry.RequiredSP != 0 || entry.MPCost != row.mp || entry.Extended {
					t.Fatal(entries)
				}
				executeNPC97(t, &s, c, d, pos, []string{"enemy", "tail"}, row.draws...)
				if s.Actors[1].HP != row.enemyHP || s.Actors[0].HP != row.selfHP || s.Actors[0].Attack != row.attack || s.Actors[0].Defense != row.defense || c.DamageCalls != row.damage || c.AreaDamageCalls != row.area || c.StatusCalls != row.status || s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 {
					t.Fatal(s.Actors, c)
				}
				wantArgs := []string{"enemy", "tail"}
				if pos == 5 || pos == 9 {
					wantArgs = wantArgs[1:]
				}
				if !reflect.DeepEqual(c.Arguments, wantArgs) || (pos == 4 || pos == 6) && s.Actors[0].Temporary != corebattle.TemporaryDouble || pos == 6 && (s.Actors[0].Tension != 3 || c.TensionCalls != 2) || pos == 8 && s.Actors[1].Status != corebattle.StatusParalyze || pos == 9 && s.Actors[1].Status != corebattle.StatusDeadlyPoison || pos == 10 && s.Actors[1].Status != corebattle.StatusSleep {
					t.Fatal(s.Actors, c)
				}
			})
		}
	}
}

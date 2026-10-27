package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func npc98Fixture() (corebattle.LifetimeState, *corebattle.NPCJob98Context, BattleDefinitions) {
	s, base, _ := npcOrderedFixture()
	s.Actors[0].Jobs.JobID = "job-98"
	s.Actors[1].MaxHP = 999
	c := &corebattle.NPCJob98Context{DamageContext: base.DamageContext, EquipmentReset: base.EquipmentReset}
	var d BattleDefinitions
	d.RegisterNPCJob98(c)
	return s, c, d
}

func executeNPC98(t *testing.T, s *corebattle.LifetimeState, c *corebattle.NPCJob98Context, d BattleDefinitions, pos int, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{fmt.Sprintf("npc-job-98-position-%d", pos)}, noArithmeticDraws{t})
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

// Independent intrinsic vectors: ff97fc5:_skill.cgi:7309–7408. Ordinary
// SP/MP admission, name binding and post-callback charges belong to the caller.
func TestNPC98AllPositionsAndRoles(t *testing.T) {
	for pos, row := range []struct {
		mp, enemyHP, defense, damage, area, heals int
		draws                                     []float64
	}{
		{44, 1, 80, 0, 0, 0, nil},
		{30, 1000, 80, 0, 0, 0, nil},
		{30, 1000, 80, 0, 0, 0, nil},
		{23, 1000, 160, 0, 0, 0, nil},
		{44, 1000, 80, 0, 0, 0, nil},
		{44, 1, 80, 0, 0, 0, nil},
		{40, 820, 80, 1, 1, 2, []float64{0, 0, 0}},
		{40, 910, 80, 1, 0, 0, []float64{0}},
		{80, 685, 80, 1, 1, 0, []float64{0}},
		{99, 100, 80, 4, 0, 0, []float64{0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{99, 640, 80, 1, 1, 0, []float64{0}},
	} {
		for _, npc := range []bool{false, true} {
			t.Run(fmt.Sprintf("position-%d/npc-%t", pos, npc), func(t *testing.T) {
				s, c, d := npc98Fixture()
				s.Actors[0].NPC, c.ActorNPC = npc, npc
				entries := d.Invocations.JobTables[98].Variants[0]
				entry := entries[pos]
				if len(entries) != 11 || entry.ID != fmt.Sprintf("npc-job-98-position-%d", pos) || entry.RequiredSP != 0 || entry.MPCost != row.mp || entry.Extended || entries[0].Name != entries[5].Name {
					t.Fatal(entries)
				}
				executeNPC98(t, &s, c, d, pos, []string{"enemy", "tail"}, row.draws...)
				if s.Actors[1].HP != row.enemyHP || s.Actors[0].Defense != row.defense || c.DamageCalls != row.damage || c.AreaDamageCalls != row.area || c.HealCalls != row.heals || s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 {
					t.Fatal(s.Actors, c)
				}
				wantArgs := []string{"enemy", "tail"}
				if pos == 0 || pos == 5 {
					wantArgs = wantArgs[1:]
				}
				if !reflect.DeepEqual(c.Arguments, wantArgs) || pos == 2 && s.Actors[0].Temporary != corebattle.TemporaryReflectMagic || pos == 3 && s.Actors[0].Temporary != corebattle.TemporaryReduceMagic || pos == 4 && (s.Actors[0].Temporary != corebattle.TemporaryDouble || s.Actors[0].Tension != 3 || c.TensionCalls != 2) || pos == 7 && s.Actors[1].Temporary != corebattle.TemporaryDouble || s.Actors[2].Temporary != "" {
					t.Fatal(s.Actors, c)
				}
			})
		}
	}
}

func TestNPC98HP1GuardAndTargetBinding(t *testing.T) {
	for _, pos := range []int{0, 5} {
		for _, row := range []struct{ maxHP, baseDF, wantHP int }{{998, 998, 1}, {999, 999, 1}, {1000, 999, 800}, {999, 1000, 800}} {
			s, c, d := npc98Fixture()
			s.Actors[1].HP, s.Actors[1].MaxHP, s.Actors[1].BaseStats.Defense = 800, row.maxHP, row.baseDF
			executeNPC98(t, &s, c, d, pos, []string{"enemy"})
			if s.Actors[1].HP != row.wantHP {
				t.Fatal(pos, row, s.Actors[1])
			}
		}
		// The guard uses the resolved recipient, not the submitted enemy.
		s, c, d := npc98Fixture()
		s.Actors[0].MaxHP, s.Actors[1].Temporary = 999, corebattle.TemporaryReflectMagic
		executeNPC98(t, &s, c, d, pos, []string{"enemy"})
		if s.Actors[0].HP != 1 || s.Actors[1].HP != 1000 {
			t.Fatal(s.Actors)
		}
		// A zeroed symbolic payload is still a resolved helper: this body only
		// tests the returned target, bypassing ordinary damage/status guards.
		s, c, d = npc98Fixture()
		s.Actors[1].Temporary = corebattle.TemporaryNullMagic
		executeNPC98(t, &s, c, d, pos, []string{"enemy"})
		if s.Actors[1].HP != 1 {
			t.Fatal(s.Actors)
		}
		s, c, d = npc98Fixture()
		s.Actors[1].HP = 0
		executeNPC98(t, &s, c, d, pos, nil, 0)
		if s.Actors[1].HP != 0 || len(c.Arguments) != 0 {
			t.Fatal(s.Actors, c)
		}
	}
}

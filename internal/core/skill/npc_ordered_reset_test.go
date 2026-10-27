package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func npcOrderedFixture() (corebattle.LifetimeState, *corebattle.NPCOrderedResetContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	s.Actors[0].Defense = 80
	s.Actors[0].BaseStats = corebattle.BattleBaseStats{Attack: 100, Defense: 80, Agility: 40}
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-403", SP: 190, OldJobID: "job-83", OldSP: 35}
	s.Actors[1].BaseStats = corebattle.BattleBaseStats{Attack: 20, Defense: 30, Agility: 10}
	c := &corebattle.NPCOrderedResetContext{DamageContext: base.DamageContext,
		EquipmentReset: &corebattle.EquipmentResetContext{}}
	var d BattleDefinitions
	d.RegisterNPCOrderedReset(c)
	return s, c, d
}

func executeNPCOrdered(t *testing.T, s *corebattle.LifetimeState, c *corebattle.NPCOrderedResetContext, d BattleDefinitions, pos int, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ids := []string{fmt.Sprintf("npc-job-403-position-%d", pos)}
	ops, err := d.InvocationAction(s, &f, ids, noArithmeticDraws{t})
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

// Specification vectors: ff97fc5:_skill.cgi:8077–8121. Listed SP/MP are
// metadata; ordinary eligibility/charging occurs outside these intrinsic bodies.
func TestNPCOrderedResetManifestAndRoles(t *testing.T) {
	for pos, row := range []struct {
		sp, mp, enemyHP, defense, damage, area int
		draws                                  []float64
	}{
		{10, 34, 946, 80, 1, 1, []float64{0}},
		{40, 27, 1000, 120, 0, 0, nil},
		{70, 40, 838, 80, 3, 0, []float64{0, 0, 0, 0, 0}},
		{130, 76, 550, 80, 1, 0, []float64{0}},
	} {
		for _, npc := range []bool{false, true} {
			t.Run(fmt.Sprintf("position-%d/npc-%t", pos, npc), func(t *testing.T) {
				s, c, d := npcOrderedFixture()
				s.Actors[0].NPC, c.ActorNPC = npc, npc
				entry := d.Invocations.JobTables[403].Variants[0][pos]
				if entry.ID != fmt.Sprintf("npc-job-403-position-%d", pos) || entry.RequiredSP != row.sp || entry.MPCost != row.mp || entry.Extended {
					t.Fatal(entry)
				}
				executeNPCOrdered(t, &s, c, d, pos, []string{"enemy", "tail"}, row.draws...)
				if s.Actors[1].HP != row.enemyHP || s.Actors[0].Defense != row.defense || c.DamageCalls != row.damage || c.AreaDamageCalls != row.area || s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 {
					t.Fatal(s.Actors, c)
				}
				wantArgs := []string{"enemy", "tail"}
				if pos == 2 {
					wantArgs = wantArgs[1:]
				}
				if !reflect.DeepEqual(c.Arguments, wantArgs) || pos == 1 && s.Actors[0].Temporary != corebattle.TemporaryReduceAttack {
					t.Fatal(c.Arguments, s.Actors[0])
				}
			})
		}
	}
}

func TestNPCOrderedWaveDecayAndWallNoEffectTail(t *testing.T) {
	s, c, d := npcOrderedFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	executeNPCOrdered(t, &s, c, d, 0, nil, 0, 0)
	if s.Actors[1].HP != 946 || s.Actors[3].HP != 955 {
		t.Fatal(s.Actors)
	}
	for _, blocked := range []string{"cap", "binding", "fallen", "zero-modifier"} {
		t.Run(blocked, func(t *testing.T) {
			s, c, d := npcOrderedFixture()
			s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
			switch blocked {
			case "cap":
				s.Actors[0].Defense = 200
			case "binding":
				s.Actors[0].Status = corebattle.StatusSabaku
			case "fallen":
				s.Actors[0].HP = 0
			case "zero-modifier":
				s.Fields.Default = "darkness"
			}
			defense := s.Actors[0].Defense
			executeNPCOrdered(t, &s, c, d, 1, []string{"enemy", "tail"})
			if s.Actors[0].Defense != defense || s.Actors[0].Temporary != corebattle.TemporaryReduceAttack || c.TensionCalls != 0 {
				t.Fatal(s, c)
			}
		})
	}
}

func TestNPCOrderedProngsCaptureRandomAndTension(t *testing.T) {
	s, c, d := npcOrderedFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	s.Actors[0].Tension = 3
	// Both early hits select other; the third retains the initial enemy.
	executeNPCOrdered(t, &s, c, d, 2, []string{"enemy", "tail"}, .9, 0, .9, 0, 0)
	if s.Actors[1].HP != 784 || s.Actors[3].HP != 910 || s.Actors[0].Tension != 1 {
		t.Fatal(s.Actors)
	}
	// No third damage tuple means the saved tension remains restored.
	s, c, d = npcOrderedFixture()
	s.Actors[0].Tension, s.Actors[1].HP = 4, 1
	executeNPCOrdered(t, &s, c, d, 2, []string{"enemy"}, 0, 0, 0)
	if s.Actors[0].Tension != 4 || c.DamageCalls != 1 {
		t.Fatal(s, c)
	}
	// Reflection kills the caster on the first hit; do not restore tension.
	s, c, d = npcOrderedFixture()
	s.Actors[0].HP, s.Actors[0].Tension = 1, 5
	s.Actors[1].Temporary = corebattle.TemporaryReflectAttack
	executeNPCOrdered(t, &s, c, d, 2, []string{"enemy"}, 0, 0)
	if s.Actors[0].HP != 0 || s.Actors[0].Tension != 1 || c.DamageCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestNPCOrderedDirectNPCArgumentsAndHelperRejection(t *testing.T) {
	s, c, d := npcOrderedFixture()
	s.Actors[0].NPC, c.ActorNPC = true, true
	// Direct NPC callbacks start without a target: capture consumes a draw too.
	executeNPCOrdered(t, &s, c, d, 2, nil, 0, 0, 0, 0, 0, 0)
	if s.Actors[1].HP != 838 || len(c.Arguments) != 0 {
		t.Fatal(s, c)
	}
	s, c, d = npcOrderedFixture()
	s.Actors[0].Status, s.Actors[0].Tension = corebattle.StatusPhysicalSeal, 3
	// Each physical helper rejects independently; the third restore still runs.
	executeNPCOrdered(t, &s, c, d, 2, []string{"enemy", "tail"}, .9, .9, .9)
	if s.Actors[1].HP != 1000 || c.DamageCalls != 0 || s.Actors[0].Tension != 3 {
		t.Fatal(s, c)
	}
	// The wall's untyped stat helper ignores the physical seal and applies
	// item205's requester-dependent tail before overwriting its temporary.
	s, c, d = npcOrderedFixture()
	s.Actors[0].NPC, c.ActorNPC = true, true
	s.Actors[0].Status = corebattle.StatusPhysicalSeal
	s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
	level := 99
	c.RequesterLevel = &level
	executeNPCOrdered(t, &s, c, d, 1, nil)
	if s.Actors[0].Attack != 140 || s.Actors[0].Defense != 120 || s.Actors[0].Tension != 1.7 || s.Actors[0].Temporary != corebattle.TemporaryReduceAttack || c.TensionCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestNPCOrderedVortexResetsBeforePowerAndIncludesFallen(t *testing.T) {
	for _, row := range []struct{ hp, maxHP, wantHP int }{
		{1, 100, 6}, {980, 100, 544}, {1000, 2000, 550}, {2000, 2000, 550}, {0, 100, 0},
	} {
		s, c, d := npcOrderedFixture()
		e := &s.Actors[1]
		e.HP, e.MaxHP = row.hp, row.maxHP
		e.Gear.AccessoryID, e.ItemDefinitionIDs = "item-112", []string{"item-112"}
		e.Defense, e.Tension, e.Temporary, e.Status = 900, 5, corebattle.TemporaryOneHitDefense, corebattle.StatusKinju
		e.Extended = corebattle.ExtendedPurification
		s.Actors[2].Temporary, s.Actors[2].Tension, s.Actors[2].Hit = corebattle.TemporaryRage, 5, 1
		draws := []float64{0, 0}
		if row.hp == 0 {
			draws = nil
		}
		executeNPCOrdered(t, &s, c, d, 3, nil, draws...)
		if e = &s.Actors[1]; e.HP != row.wantHP || e.MaxHP != row.maxHP || e.Defense != 30 || e.Status != "" || e.Temporary != "" || e.Extended != "" || e.Tension != 1 || s.Actors[2].HP != 0 || s.Actors[2].Temporary != "" || s.Actors[2].Tension != 1 || s.Actors[2].Hit != 95 {
			t.Fatal(row, s.Actors)
		}
	}
}

func TestNPCOrderedVortexCapAndNoCasterDeathGuard(t *testing.T) {
	for _, hp := range []int{999, 1000, 1001, 2000} {
		s, c, d := npcOrderedFixture()
		s.Actors[1].HP = hp
		executeNPCOrdered(t, &s, c, d, 3, nil, 0)
		want := hp - 450
		if hp == 999 {
			want = 550
		}
		if s.Actors[1].HP != want {
			t.Fatal(hp, s.Actors[1])
		}
	}
	s, c, d := npcOrderedFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	s.Actors[0].HP, s.Actors[0].Status = 1, corebattle.StatusConfusion
	executeNPCOrdered(t, &s, c, d, 3, nil, 0, 0, 0, 0)
	if s.Actors[0].HP != 0 || s.Actors[0].Status != "" || s.Actors[1].HP != 1000 || s.Actors[3].HP != 1000 || c.DamageCalls != 2 {
		t.Fatal(s, c)
	}
}

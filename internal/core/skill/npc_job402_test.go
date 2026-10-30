package skill

import (
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func npc402Fixture() (corebattle.LifetimeState, *corebattle.NPCJob402Context, BattleDefinitions) {
	s, base, _ := npcOrderedFixture()
	s.Actors[0].Jobs.JobID = "job-402"
	s.Actors[1].Defense = 30
	s.Actors[1].CMP, s.Actors[1].MaxCMP = 100, 100
	s.Actors[1].CMPCharge, s.Actors[1].CMPNeedCharge = 4, 3
	c := &corebattle.NPCJob402Context{DamageContext: base.DamageContext, Death: &corebattle.DefeatContext{
		Rewards: &corebattle.DefeatRewards{
			Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
			Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		},
	}}
	var d BattleDefinitions
	d.RegisterNPCJob402(c)
	return s, c, d
}

func executeNPC402(t *testing.T, s *corebattle.LifetimeState, c *corebattle.NPCJob402Context, d BattleDefinitions, pos int, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{fmt.Sprintf("npc-job-402-position-%d", pos)}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
		t.Fatal(err, rng.used, draws)
	}
}

// Independent intrinsic vectors: ff97fc5:_skill.cgi:7802–7886. Ordinary
// SP/MP eligibility and post-callback charges remain the selector's contract.
func TestNPC402AllPositionsAndRoles(t *testing.T) {
	for pos, row := range []struct {
		sp, mp, hp, calls, shifts int
		draws                     []float64
	}{
		{0, 71, 703, 3, 3, []float64{0, 0, 0}},
		{0, 78, 1000, 0, 0, nil},
		{0, 64, 912, 2, 2, []float64{0, 0, 0, .9}},
		{100, 50, 982, 2, 1, []float64{0, 0}},
		{100, 55, 974, 1, 1, []float64{0}},
		{100, 80, 949, 1, 1, []float64{0}},
	} {
		for _, npc := range []bool{false, true} {
			t.Run(fmt.Sprintf("position-%d/npc-%t", pos, npc), func(t *testing.T) {
				s, c, d := npc402Fixture()
				s.Actors[0].NPC, c.ActorNPC = npc, npc
				entries := d.Invocations.JobTables[402].Variants[0]
				e := entries[pos]
				if len(entries) != 6 || e.ID != fmt.Sprintf("npc-job-402-position-%d", pos) || e.RequiredSP != row.sp || e.MPCost != row.mp || e.Extended {
					t.Fatal(entries)
				}
				args := []string{"enemy", "enemy", "enemy", "tail"}
				executeNPC402(t, &s, c, d, pos, args, row.draws...)
				if s.Actors[1].HP != row.hp || c.DamageCalls != row.calls || !reflect.DeepEqual(c.Arguments, args[row.shifts:]) || s.Actors[0].MP != 80 || s.Actors[0].CMP != 90 {
					t.Fatal(s.Actors, c)
				}
				if pos == 1 && (s.Actors[0].Attack != 200 || s.Actors[0].Defense != 160 || s.Actors[0].Agility != 80 || s.Actors[0].Status != corebattle.StatusDofuu || c.PartyStatUpCalls != 3) {
					t.Fatal(s.Actors, c)
				}
				if pos == 5 && (s.Actors[1].CMP != 80 || s.Actors[1].CMPCharge != 0 || s.Actors[1].CMPNeedCharge != 6) {
					t.Fatal(s.Actors[1])
				}
			})
		}
	}
}

func TestNPC402TripleShiftsAndContinuesAfterCasterDeath(t *testing.T) {
	s, c, d := npc402Fixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	executeNPC402(t, &s, c, d, 0, []string{"enemy", "other", "enemy", "tail"}, 0, 0, 0)
	if s.Actors[1].HP != 802 || s.Actors[3].HP != 901 || !reflect.DeepEqual(c.Arguments, []string{"tail"}) {
		t.Fatal(s, c)
	}
	s, c, d = npc402Fixture()
	s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectMagic
	executeNPC402(t, &s, c, d, 0, []string{"enemy", "enemy", "enemy"}, 0, 0, 0)
	if s.Actors[0].HP != 0 || c.DamageCalls != 3 || s.Actors[1].HP != 1000 {
		t.Fatal(s, c)
	}
}

func TestNPC402CrossUsesReturnedTargetsAndIndependentStatusDraws(t *testing.T) {
	for _, row := range []struct {
		first, second         float64
		wantFirst, wantSecond string
	}{
		{0, .9, corebattle.StatusMagicSeal, ""}, {.9, 0, "", corebattle.StatusMagicSeal},
		{.5, .5, "", ""}, {0, 0, corebattle.StatusMagicSeal, corebattle.StatusMagicSeal},
	} {
		s, c, d := npc402Fixture()
		other := s.Actors[1]
		other.ID = "other"
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, "other")
		s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
		executeNPC402(t, &s, c, d, 2, []string{"enemy", "other", "tail"}, 0, 0, row.first, row.second)
		if s.Actors[0].HP != 957 || s.Actors[1].HP != 1000 || s.Actors[3].HP != 955 || s.Actors[0].Status != row.wantFirst || s.Actors[3].Status != row.wantSecond {
			t.Fatal(row, s.Actors)
		}
	}
	for _, blocked := range []string{"existing", "fallen", "null", "one-hit"} {
		s, c, d := npc402Fixture()
		draws := []float64{0, 0, 0, 0}
		switch blocked {
		case "existing":
			s.Actors[1].Status = corebattle.StatusPoison
		case "fallen":
			s.Actors[1].HP = 1
			draws = []float64{0, 0, 0}
		case "null":
			s.Actors[1].Temporary = corebattle.TemporaryNullMagic
			draws = []float64{0, 0, 0}
		case "one-hit":
			s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
			draws = []float64{0, 0, 0}
		}
		executeNPC402(t, &s, c, d, 2, []string{"enemy", "enemy"}, draws...)
		if blocked == "existing" && s.Actors[1].Status != corebattle.StatusPoison || blocked == "fallen" && s.Actors[1].Status != "" {
			t.Fatal(blocked, s.Actors)
		}
	}
}

func TestNPC402SpeedCountBoundariesAndNoDeathStop(t *testing.T) {
	for _, row := range []struct{ agility, hits, hp int }{
		{-350, 2, 982}, {0, 2, 982}, {34, 2, 982}, {35, 2, 982}, {70, 3, 973}, {314, 9, 919}, {315, 11, 820}, {999, 11, 820},
	} {
		s, c, d := npc402Fixture()
		// Negative delta uses a valid target agility, never a negative actor stat.
		s.Actors[0].Agility, s.Actors[1].Agility = max(0, row.agility), max(0, -row.agility)
		executeNPC402(t, &s, c, d, 3, []string{"enemy"}, make([]float64, row.hits)...)
		if c.DamageCalls != row.hits || s.Actors[1].HP != row.hp {
			t.Fatal(row, s.Actors, c.DamageCalls)
		}
	}
	s, c, d := npc402Fixture()
	s.Actors[0].HP, s.Actors[1].Temporary = 1, corebattle.TemporaryReflectAttack
	executeNPC402(t, &s, c, d, 3, []string{"enemy"}, 0, 0, 0, 0)
	if c.DamageCalls != 2 || s.Actors[0].HP != 0 {
		t.Fatal(s, c)
	}
}

func TestNPC402BarrierClearBeforeSelectionAndAreaDecay(t *testing.T) {
	s, c, d := npc402Fixture()
	other := s.Actors[1]
	other.ID, other.Defense, other.Temporary = "other", 200, corebattle.TemporaryOneHitDefense
	s.Actors[1].Temporary = corebattle.TemporaryDefense
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	executeNPC402(t, &s, c, d, 4, []string{"enemy"}, 0, 0)
	if s.Actors[1].HP != 974 || s.Actors[3].HP != 978 || s.Actors[1].Temporary != "" || s.Actors[3].Temporary != "" {
		t.Fatal(s, c)
	}
	// A fallen selected target yields no helper tuple: the area base is still5.
	s, c, d = npc402Fixture()
	s.Actors[1].HP, s.Actors[1].Temporary = 0, corebattle.TemporaryReflectMagic
	executeNPC402(t, &s, c, d, 4, []string{"enemy"})
	if s.Actors[1].Temporary != "" || c.AreaDamageCalls != 1 {
		t.Fatal(s, c)
	}
}

func TestNPC402SharedManaFallenThresholdAndChargeTail(t *testing.T) {
	for _, mp := range []int{4, 5} {
		s, c, d := npc402Fixture()
		s.Actors[2].MP = mp
		executeNPC402(t, &s, c, d, 5, []string{"enemy"}, 0)
		wantMP, wantHP := 4, 949
		if mp == 5 {
			wantMP, wantHP = 0, 859
		}
		if s.Actors[2].MP != wantMP || s.Actors[2].HP != 0 || s.Actors[1].HP != wantHP || s.Actors[1].CMP != 80 || s.Actors[1].CMPNeedCharge != 6 {
			t.Fatal(mp, s.Actors)
		}
	}
	// No damage tuple still performs a fresh target lookup for CMP reduction;
	// its charge writes address the absent tuple, not that second selection.
	s, c, d := npc402Fixture()
	s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
	executeNPC402(t, &s, c, d, 5, []string{"enemy"}, 0)
	if s.Actors[1].HP != 1000 || s.Actors[1].CMP != 80 || s.Actors[1].CMPCharge != 4 || s.Actors[1].CMPNeedCharge != 3 {
		t.Fatal(s, c)
	}
	// Cover changes the damage recipient. The second helper resolves again,
	// while the charging tail keeps the first recipient even after defeat.
	s, c, d = npc402Fixture()
	other := s.Actors[1]
	other.ID, other.Temporary = "coverer", corebattle.TemporaryCovering
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "coverer")
	s.Actors[1].Temporary = corebattle.TemporaryCover
	executeNPC402(t, &s, c, d, 5, []string{"enemy"}, 0)
	if s.Actors[1].HP != 1000 || s.Actors[1].CMP != 100 || s.Actors[3].HP != 949 || s.Actors[3].CMP != 80 || s.Actors[3].CMPNeedCharge != 6 {
		t.Fatal(s)
	}
	s, c, d = npc402Fixture()
	s.Actors[1].HP = 1
	executeNPC402(t, &s, c, d, 5, []string{"enemy"}, 0)
	if s.Actors[1].HP != 0 || s.Actors[1].CMPNeedCharge != 6 {
		t.Fatal(s)
	}
}

func TestNPC402NPCAbsentArgumentsAndBlessingNoEffectTail(t *testing.T) {
	for pos, count := range []int{6, 0, 6, 3, 2, 2} {
		s, c, d := npc402Fixture()
		s.Actors[0].NPC, c.ActorNPC = true, true
		executeNPC402(t, &s, c, d, pos, nil, make([]float64, count)...)
	}
	s, c, d := npc402Fixture()
	s.Actors[0].Status = corebattle.StatusMagicSeal
	executeNPC402(t, &s, c, d, 1, nil, .9, .9, .9)
	if s.Actors[0].Attack != 100 || s.Actors[0].Defense != 80 || s.Actors[0].Agility != 40 || s.Actors[0].Status != corebattle.StatusDofuu {
		t.Fatal(s)
	}
}

func TestNPC402CrossMissAndNullHelpersHaveNoReturnedStatusTarget(t *testing.T) {
	s, c, d := npc402Fixture()
	c.GuaranteedHit = false
	// The physical hit misses; its successful status draw has no recipient.
	executeNPC402(t, &s, c, d, 2, []string{"enemy", "enemy"}, 0, .99, .9, 0)
	if s.Actors[1].HP != 957 || s.Actors[1].Status != "" {
		t.Fatal(s)
	}
	s, c, d = npc402Fixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, "other")
	s.Actors[1].Temporary = corebattle.TemporaryNullMagic
	executeNPC402(t, &s, c, d, 2, []string{"enemy", "other"}, 0, 0, .9)
	if s.Actors[1].HP != 1000 || s.Actors[1].Status != "" || s.Actors[3].HP != 955 || s.Actors[3].Status != "" {
		t.Fatal(s)
	}
}

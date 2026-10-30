package skill

import (
	"math"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestRecoilDarkHP999Boundary(t *testing.T) {
	for _, row := range []struct {
		id       string
		hp, want int
	}{
		{"job-22-entry-0", 999, 899}, {"job-22-entry-0", 1000, 993},
		{"job-22-entry-4", 999, 799}, {"job-22-entry-4", 1000, 977},
	} {
		s, c, d := recoilDarkFixture()
		s.Actors[0].HP = row.hp
		executeRecoilDark(t, &s, c, d, []string{row.id}, []string{"enemy"}, 0)
		if s.Actors[0].HP != row.want || c.End.ImmediateRiskCalls != 1 {
			t.Fatal(row, s, c)
		}
	}
}

func TestRecoilDarkRawSelectionAndFloor(t *testing.T) {
	for _, row := range []struct {
		side, selection float64
		self, enemy     int
	}{
		{math.Nextafter(.25, 0), 0, 260, 900},
		{0, .75, 260, 900}, // Fallen friendly selection becomes caster.
		{.25, 0, 500, 630},
		{math.Nextafter(1, 0), 0, 500, 630},
	} {
		s, c, d := recoilDarkFixture()
		executeRecoilDark(t, &s, c, d, []string{"job-21-entry-5"}, nil, row.side, row.selection, 0)
		if s.Actors[0].HP != row.self || s.Actors[1].HP != row.enemy || c.DamageCalls != 0 {
			t.Fatal(row, s, c)
		}
	}
	for _, roll := range []float64{0, .5} {
		s, c, d := recoilDarkFixture()
		s.Actors[0].Attack, s.Actors[1].Defense = 1, 5
		executeRecoilDark(t, &s, c, d, []string{"job-21-entry-5"}, nil, .25, 0, 0, roll)
		if s.Actors[1].HP != 899-int(roll*2) {
			t.Fatal(roll, s)
		}
	}
	// Empty enemy selection has absent HP and follows the same self fallback.
	s, c, d := recoilDarkFixture()
	s.Actors = append(s.Actors[:1], s.Actors[2])
	c.EnemyIDs = nil
	executeRecoilDark(t, &s, c, d, []string{"job-21-entry-5"}, nil, .25, .8, 0)
	if s.Actors[0].HP != 260 {
		t.Fatal(s)
	}
}

func TestRecoilDarkRawBypassesDamageRules(t *testing.T) {
	s, c, d := recoilDarkFixture()
	s.Actors[0].HP, s.Actors[0].Tension = 10000, 3
	s.Actors[0].Status = corebattle.StatusPhysicalSeal
	s.Actors[0].ItemDefinitionIDs = []string{"item-149", "item-166"}
	s.Fields.Default = "darkness"
	s.Actors[1].HP, s.Actors[1].Defense = 10000, 5
	s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
	s.Actors[1].ItemDefinitionIDs = []string{"item-185", "item-195"}
	c.Defeat = nil
	executeRecoilDark(t, &s, c, d, []string{"job-21-entry-5"}, nil, .25, 0, 0)
	// (100*3-5*.4)*.9*3 =804.6, with no helper counters or activations.
	if s.Actors[1].HP != 9196 || s.Actors[1].Temporary != corebattle.TemporaryOneHitDefense || s.Actors[0].Tension != 1 || s.Actors[0].Status != corebattle.StatusPhysicalSeal || c.Item195Activated || c.Item185Activated || c.DamageCalls != 0 || c.End.DarknessDebit != nil {
		t.Fatal(s, c)
	}
}

func rawDefeatContext() *corebattle.DefeatContext {
	return &corebattle.DefeatContext{Rewards: &corebattle.DefeatRewards{
		Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
	}}
}

func TestRecoilDarkRawImmediateDefeatAndRevival(t *testing.T) {
	for _, revival := range []bool{false, true} {
		s, c, d := recoilDarkFixture()
		s.Actors[0].HP = 1
		if revival {
			s.Actors[0].Temporary = corebattle.TemporaryRevival
		}
		c.RawDefeat = rawDefeatContext()
		draws := []float64{0, .75, 0} // Fallen friendly -> caster.
		if revival {
			draws = append(draws, 0)
		}
		executeRecoilDark(t, &s, c, d, []string{"job-21-entry-5"}, nil, draws...)
		want := 0
		if revival {
			want = 100
		}
		if s.Actors[0].HP != want || s.Actors[0].Temporary != "" || s.Actors[0].Defense != 51 || s.Actors[0].Tension != 1 || c.DamageCalls != 0 {
			t.Fatal(s, c)
		}
	}
	// Raw tension is reset before defeat, so an eight-tension161 caster cannot revive.
	s, c, d := recoilDarkFixture()
	s.Actors[0].HP, s.Actors[0].Tension = 1, 8
	s.Actors[0].ItemDefinitionIDs = []string{"item-161"}
	c.RawDefeat = rawDefeatContext()
	executeRecoilDark(t, &s, c, d, []string{"job-21-entry-5"}, nil, 0, 0, 0)
	if s.Actors[0].HP != 0 || s.Actors[0].Tension != 1 {
		t.Fatal(s)
	}
}

func TestRecoilDarkDrainCapsReturnedDamageAndAliasing(t *testing.T) {
	for _, row := range []struct{ hp, damage, heal int }{
		{599, 269, 269}, {600, 270, 270}, {601, 270, 270}, {1, 0, 0},
	} {
		s, c, d := recoilDarkFixture()
		s.Actors[1].HP = row.hp
		draws := []float64(nil)
		if row.damage > 0 {
			draws = []float64{0, 0}
		}
		executeRecoilDark(t, &s, c, d, []string{"job-38-entry-5"}, []string{"enemy"}, draws...)
		if s.Actors[0].HP != 500+row.heal || s.Actors[1].HP != row.hp-row.damage {
			t.Fatal(row, s)
		}
	}
	// Damage resolves a double temporary, but healing keeps the requested300.
	s, c, d := recoilDarkFixture()
	s.Actors[1].Temporary = corebattle.TemporaryDouble
	executeRecoilDark(t, &s, c, d, []string{"job-38-entry-5"}, []string{"enemy"}, 0, 0)
	if s.Actors[1].HP != 360 || s.Actors[0].HP != 770 {
		t.Fatal(s)
	}
	for _, id := range []string{"job-38-entry-0", "job-38-entry-5"} {
		s, c, d = recoilDarkFixture()
		s.Actors[1].Temporary = corebattle.TemporaryReflectMagic
		draws := []float64{0, 0}
		want := 477 // 500-229+floor(229*.9)
		if id == "job-38-entry-5" {
			draws, want = []float64{0, 0, 0}, 500
		}
		executeRecoilDark(t, &s, c, d, []string{id}, []string{"enemy"}, draws...)
		if s.Actors[0].HP != want || s.Actors[1].HP != 900 {
			t.Fatal(id, s)
		}
	}
}

func TestRecoilDarkResourceAndStatusTails(t *testing.T) {
	for _, id := range []string{"job-22-entry-0", "job-22-entry-4"} {
		s, c, d := recoilDarkFixture()
		s.Fields.Default = "darkness"
		executeRecoilDark(t, &s, c, d, []string{id}, []string{"enemy"}, 0)
		if s.Actors[0].HP != 500 || c.End.ImmediateRiskCalls != 1 || c.End.DarknessDebit == nil {
			t.Fatal(s, c)
		}
	}
	for _, immune := range []bool{false, true} {
		s, c, d := recoilDarkFixture()
		s.Actors[1].MP = 40 // Exactly20% of MaxMP: no tuple and no recovery.
		if immune {
			s.Actors[1].MP, s.Actors[1].Abilities = 100, []string{"curse"}
		}
		executeRecoilDark(t, &s, c, d, []string{"job-38-entry-1"}, []string{"enemy"})
		if s.Actors[0].MP != 80 || c.MPHealCalls != 0 || c.StatDownCalls != 1 {
			t.Fatal(s, c)
		}
	}
	s, c, d := recoilDarkFixture()
	s.Actors[1].HP = 1
	executeRecoilDark(t, &s, c, d, []string{"job-22-entry-3"}, []string{"enemy"}, 0)
	if s.Actors[1].HP != 0 || s.Actors[1].Hit != 95 || c.StatDownCalls != 0 || !c.AdditionalEffect {
		t.Fatal(s, c)
	}
	// The second tension invocation crosses the requester-level-dependent stage.
	s, c, d = recoilDarkFixture()
	level := 25
	c.RequesterLevel, s.Actors[0].Tension = &level, 3
	executeRecoilDark(t, &s, c, d, []string{"job-21-entry-6"}, nil, 0)
	if s.Actors[0].Tension != 8 || c.TensionCalls != 2 {
		t.Fatal(s, c)
	}
}

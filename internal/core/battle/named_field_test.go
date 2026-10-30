package battle

import (
	"errors"
	"reflect"
	"testing"
)

func namedFieldFixture() (LifetimeState, NamedFieldContext) {
	s, damage := damageFixture()
	level := 70
	damage.RequesterLevel = &level
	death := &DefeatContext{EquipmentResetContext: EquipmentResetContext{TargetContext: damage.TargetContext, RequesterLevel: &level}, Rewards: &DefeatRewards{
		Validate: func(*LifetimeState, TargetContext, string) error { return nil }, Apply: func(*LifetimeState, TargetContext, string) error { return nil },
	}}
	return s, NamedFieldContext{DamageContext: damage, Death: death}
}

func TestNamedFieldProducerDurationsAndCreationBoundaries(t *testing.T) {
	for _, row := range []struct {
		id, field   string
		base, width int
	}{
		{"job-22-entry-5", "darkness", 5, 5}, {"job-29-entry-7", "cosmic", 5, 3}, {"job-53-entry-6", "antlion", 6, 5}, {"job-86-entry-1", "library", 11, 6},
	} {
		for _, draw := range []float64{0, .999} {
			s, c := namedFieldFixture()
			c.Arguments = []string{"enemy"}
			draws := []float64{draw}
			if row.field == "cosmic" {
				s.actor("self").MaxMP = 100
				draws = append(draws, 0)
			}
			if row.field == "antlion" {
				draws = append(draws, .5)
			}
			rng := &targetRNG{t: t, draws: draws}
			if err := s.ApplyNamedField(&c, row.id, rng); err != nil || s.Fields.Temporary != row.field || s.Fields.Turns != row.base+int(draw*float64(row.width)) {
				t.Fatal(row, err, s.Fields)
			}
			rng.done()
			before := reloadLifetime(t, s)
			// Duration is drawn before the same-name/anti creation predicate.
			if err := s.ApplyNamedField(&c, row.id, &targetRNG{t: t, draws: []float64{0}}); err != nil || !reflect.DeepEqual(before, s) {
				t.Fatal(row, err, s)
			}
		}
	}
	s, c := namedFieldFixture()
	s.Fields.AntiTurns = 1
	before := reloadLifetime(t, s)
	if err := s.ApplyNamedField(&c, "job-22-entry-5", &targetRNG{t: t, draws: []float64{0}}); err != nil || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
}

func TestNamedFieldCosmicHistoryAndAntlionProducer(t *testing.T) {
	s, c := namedFieldFixture()
	s.actor("self").MP, s.actor("self").MaxMP = 0, 100
	if err := s.ApplyNamedField(&c, "job-29-entry-7", &targetRNG{t: t, draws: []float64{0, 0}}); err != nil || s.actor("self").MP != 7 || !c.ResponseSuppressed || !s.SkillHistory[s.actor("self").TeamID]["job-29-entry-7"] {
		t.Fatal(err, c, s)
	}
	s.Fields.Temporary, s.Fields.Turns = "", 0
	c.ResponseSuppressed = false
	if err := s.ApplyNamedField(&c, "job-29-entry-7", &targetRNG{t: t, draws: []float64{0, 0}}); err != nil || c.ResponseSuppressed {
		t.Fatal(err, c)
	}
	s, c = namedFieldFixture()
	c.Arguments = []string{"enemy"}
	if err := s.ApplyNamedField(&c, "job-53-entry-6", &targetRNG{t: t, draws: []float64{0, 0}}); err != nil || s.actor("enemy").Status != StatusDofuu || len(c.Arguments) != 0 {
		t.Fatal(err, c, s)
	}
}

func TestNamedFieldFiniteChaosAllBranches(t *testing.T) {
	for branch, draws := range [][]float64{{0, .5}, {.5, 0, .75}, {.5, .5, 0}, {.5, .5, .5, 0}, {.5, .5, .5, .5, 0}, {.5, .5, .5, .5, .5}} {
		s, c := namedFieldFixture()
		s.actor("ally").HP, s.actor("ally").MaxHP = 999, 999
		s.actor("enemy").HP, s.actor("enemy").MaxHP = 1000, 1000
		s.actor("fallen").HP, s.actor("fallen").MaxHP = -1, 999
		s.actor("ally").Tension = 3
		rng := &targetRNG{t: t, draws: draws}
		before := reloadLifetime(t, s)
		if err := s.ApplyNamedField(&c, "job-56-entry-3", rng); err != nil {
			t.Fatal(branch, err)
		}
		rng.done()
		switch branch {
		case 0:
			if s.actor("self").Temporary != TemporaryReflectMagic || s.actor("ally").Temporary != TemporaryReflectMagic || s.actor("fallen").Temporary != "" {
				t.Fatal(s)
			}
		case 1:
			if s.actor("ally").Status != StatusDeadlyPoison || s.actor("ally").Tension != 1 || s.actor("fallen").Status != "" {
				t.Fatal(s)
			}
		case 2:
			if s.actor("fallen").HP != 999 || s.actor("enemy").HP != 1000 {
				t.Fatal(s)
			}
		case 3:
			if s.actor("fallen").Agility != 0 || s.actor("enemy").Agility != 0 {
				t.Fatal(s)
			}
		case 4:
			if s.actor("ally").HP != 1 || s.actor("enemy").HP != 1000 || s.actor("fallen").HP != -1 {
				t.Fatal(s)
			}
		case 5:
			if !reflect.DeepEqual(before, s) {
				t.Fatal(s)
			}
		}
	}
}

func TestNamedFieldRainbowSevenBranches(t *testing.T) {
	for branch := range 7 {
		s, c := namedFieldFixture()
		s.actor("self").HP, s.actor("self").MaxHP = 1, 1000
		s.actor("ally").HP, s.actor("ally").MaxHP = 0, 999
		s.actor("fallen").HP, s.actor("fallen").MaxHP = -1, 1000
		s.actor("self").MaxMP = 1000
		s.actor("self").Status = StatusPoison
		draws := []float64{float64(branch)/7 + .001}
		if branch == 3 || branch == 6 {
			draws = append(draws, 0, 0, 0)
		}
		rng := &targetRNG{t: t, draws: draws}
		if err := s.ApplyNamedField(&c, "job-55-entry-7", rng); err != nil {
			t.Fatal(branch, err)
		}
		rng.done()
		switch branch {
		case 0:
			if s.actor("self").Attack != 250 || c.PartyStatUpCalls != 1 {
				t.Fatal(s, c)
			}
		case 1:
			if s.actor("ally").Temporary != TemporaryRevival || s.actor("fallen").Temporary != "" {
				t.Fatal(s)
			}
		case 2:
			if s.actor("ally").HP != 999 || s.actor("fallen").HP != 1000 {
				t.Fatal(s)
			}
		case 3:
			if s.actor("self").HP != 1000 || s.actor("ally").HP != 0 {
				t.Fatal(s)
			}
		case 4:
			if s.actor("self").Hit != 95 || s.actor("self").Defense != 250 {
				t.Fatal(s)
			}
		case 5:
			if s.actor("self").Status != "" || s.actor("self").Agility != 250 || c.CureCalls != 1 {
				t.Fatal(s, c)
			}
		case 6:
			if s.actor("self").MP != 816 || c.PartyMPHealCalls != 1 {
				t.Fatal(s, c)
			}
		}
	}
}

func TestNamedFieldWholeRainbowRejectsUnsupportedLaterStat(t *testing.T) {
	s, c := namedFieldFixture()
	s.actor("ally").ItemDefinitionIDs = []string{"item-201"}
	before := reloadLifetime(t, s)
	if err := s.ApplyNamedField(&c, "job-55-entry-7", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
}

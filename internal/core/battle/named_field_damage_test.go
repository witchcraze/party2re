package battle

import "testing"

func TestNamedFieldCosmicSingleCounterAreaAndAntiReads(t *testing.T) {
	for _, row := range []struct {
		count, area, anti, want int
		draws                   []float64
	}{
		{0, 0, 0, 108, []float64{0}}, {1, 0, 0, 205, []float64{0, 0, 0}}, {1, 1, 0, 108, []float64{0}}, {1, 0, 1, 205, []float64{0, 0, 0}},
	} {
		s, c := namedFieldFixture()
		c.DamageContext = s.nativeDamageContext(c.DamageContext, c.Death, &targetRNG{t: t})
		s.Fields = LifetimeFields{Default: "cosmic", AntiTurns: row.anti}
		s.actor("enemy").HP = 1000
		c.DamageCalls, c.AreaDamageCalls = row.count, row.area
		rng := &targetRNG{t: t, draws: row.draws}
		if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect("enemy", 100, CategoryMagic, true), rng); err != nil || s.actor("enemy").HP != 1000-row.want || c.DamageCalls != row.count+1 {
			t.Fatal(row, err, s, c)
		}
		rng.done()
	}
}

func TestNamedFieldAntlionIndependentRollsAndZeroSkips(t *testing.T) {
	for _, row := range []struct {
		state, defense float64
		wantState      string
		wantDF         int
	}{
		{0, 0, StatusDofuu, 120}, {0, .5, StatusDofuu, 150}, {1.0 / 3, 0, "", 120}, {1.0 / 3, .5, "", 150},
	} {
		s, c := namedFieldFixture()
		c.Defeat = func(string) error { return nil }
		s.actor("enemy").HP = 1000
		s.Fields.Default = "antlion"
		rng := &targetRNG{t: t, draws: []float64{0, row.state, row.defense}}
		if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect("enemy", 100, CategoryMagic, true), rng); err != nil || s.actor("self").Status != row.wantState || s.actor("self").Defense != row.wantDF {
			t.Fatal(row, err, s)
		}
		rng.done()
		if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect("enemy", 0, CategoryMagic, true), &targetRNG{t: t}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNamedFieldBurnCompleteBodyAndMissingResetGate(t *testing.T) {
	s, c := namedFieldFixture()
	s.actor("enemy").HP, s.actor("enemy").MaxHP = 1000, 1000
	s.actor("dead_enemy").HP, s.actor("dead_enemy").MaxHP = 1000, 1000
	s.actor("ally").HP = 100
	s.actor("self").Jobs.JobID = "job-84"
	c.RequesterIcon = s.actor("self").Icon
	c.RequesterGender = "female"
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	if err := s.ApplyNamedField(&c, "job-84-entry-4", rng); err != nil || s.actor("enemy").HP != 676 || s.actor("ally").HP != 1 || s.actor("self").Tension != 1.7 || s.Fields.Turns != 5 || c.AreaDamageCalls != 1 || s.actor("self").Icon != nativeFormIcon("job-84-ex", "female") {
		t.Fatal(err, s, c)
	}
	rng.done()
}

func TestNamedFieldGraveCompletePossessionBody(t *testing.T) {
	s, roster := rosterFixture()
	addRosterHelper(t, &s, &roster, 64)
	s.actor("enemy").Attack, s.actor("enemy").Agility = 100, 100
	s.actor("enemy").BaseStats.Attack = 100
	s.actor("enemy").HP = 10000
	c := NamedFieldContext{DamageContext: roster.DamageContext, Arguments: []string{"enemy"}, Death: nativeDefeatContext(roster.TargetContext)}
	// Field, independent branch, then for each normal helper: roster target,
	// critical check, hit and evasion checks, damage roll.
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0, .5, 0, 0, .5, 0, 0, .5, 0, 0, .5, 0}}
	if err := s.ApplyNamedField(&c, "job-64-grave", rng); err != nil || s.Fields.Temporary != "grave" || s.Fields.Turns != 6 || c.ActorID != "player" || c.DamageCalls != 2 || s.actor("enemy").Status != StatusConfusion {
		t.Fatal(err, s, c)
	}
	rng.done()
}

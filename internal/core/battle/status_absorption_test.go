package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestDeathAdmissionProjectsAbsorptionBeforeRevival(t *testing.T) {
	for _, entry := range []string{"gem_instant_death_1", "gem_instant_death_sky"} {
		for _, tc := range []struct {
			name      string
			mp, maxMP int
			field     string
			phantom   bool
			accept    bool
		}{
			{name: "guaranteed", maxMP: 100, accept: true},
			{name: "strict threshold", maxMP: 120},
			{name: "above threshold", mp: 1, maxMP: 120, accept: true},
			{name: "only largest recovery revives", maxMP: 192},
			{name: "even largest recovery fails", maxMP: 196},
			{name: "clamped", mp: 1, maxMP: 20, accept: true},
			{name: "den attenuation", maxMP: 100, field: "den"},
			{name: "den threshold", mp: 24, maxMP: 100, field: "den"},
			{name: "den guaranteed", mp: 25, maxMP: 100, field: "den", accept: true},
			{name: "actor skips temporary", maxMP: 100, phantom: true},
			{name: "skipped temporary already has mana", mp: 26, maxMP: 100, phantom: true, accept: true},
		} {
			t.Run(entry+"/"+tc.name, func(t *testing.T) {
				s, dc := finalStrikeFixture()
				a := s.actor("enemy")
				a.Abilities, a.MP, a.MaxMP, a.Temporary = []string{"pharaoh"}, tc.mp, tc.maxMP, TemporaryAbsorbMagic
				s.actor("self").HP = 1
				s.actor("self").Gear.AccessoryID = "item-110"
				s.actor("self").ItemDefinitionIDs = []string{"item-110"}
				if tc.phantom {
					s.actor("self").Temporary = TemporaryPhantom
				}
				s.Fields.Default = tc.field
				level := 99
				seals := WeaponSealTable{DragonAttack: 25, PhoenixWeight: -41}
				dc.RequesterLevel, dc.Seals = &level, &seals
				sealsBefore := seals
				applies := 0
				dc.Rewards.Apply = func(*LifetimeState, TargetContext, string) error { applies++; return nil }
				c := StatusContext{EffectContext: EffectContext{TargetContext: dc.TargetContext}, Defeat: dc}
				before := reloadLifetime(t, s)
				party, enemies := append([]string(nil), c.PartyIDs...), append([]string(nil), c.EnemyIDs...)
				for range 2 {
					err := s.ValidateStatusEntry(&c, nil, entry, "enemy", &targetRNG{t: t})
					if (tc.accept && err != nil) || (!tc.accept && !errors.Is(err, ErrUnsupportedEffect)) {
						t.Fatalf("accept=%v: %v", tc.accept, err)
					}
					if !reflect.DeepEqual(before, s) || level != 99 || seals != sealsBefore ||
						!reflect.DeepEqual(c.PartyIDs, party) || !reflect.DeepEqual(c.EnemyIDs, enemies) ||
						c.DeathCalls != 0 || c.AreaDeathCalls != 0 || c.Defeat != dc || applies != 0 {
						t.Fatal("admission changed caller facts or executed reward hook")
					}
				}
				if !tc.accept {
					if _, err := s.ApplyStatusEntry(&c, nil, entry, "enemy", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || c.DeathCalls != 0 || c.AreaDeathCalls != 0 || applies != 0 {
						t.Fatal("unsupported execution changed caller facts", err)
					}
				}
			})
		}
	}
}

func TestAbsorbedDeathExecutionAndReload(t *testing.T) {
	for _, entry := range []string{"gem_instant_death_1", "gem_instant_death_sky"} {
		for _, tc := range []struct {
			name      string
			draw      float64
			maxMP, mp int
			wantMP    int
			field     string
			phantom   bool
		}{
			{name: "minimum", draw: 0, maxMP: 100, wantMP: 15},
			{name: "maximum", draw: .999, maxMP: 100, wantMP: 25},
			{name: "clamp", draw: .999, maxMP: 20, mp: 1, wantMP: 10},
			{name: "den minimum", draw: 0, maxMP: 100, mp: 25, wantMP: 13, field: "den"},
			{name: "den maximum", draw: .999, maxMP: 100, mp: 25, wantMP: 14, field: "den"},
			{name: "skipped temporary", maxMP: 100, mp: 26, wantMP: 13, phantom: true},
		} {
			t.Run(entry+"/"+tc.name, func(t *testing.T) {
				var reference LifetimeState
				for _, reload := range []bool{false, true} {
					s, dc := finalStrikeFixture()
					a := s.actor("enemy")
					a.Abilities, a.MP, a.MaxMP, a.Temporary = []string{"pharaoh"}, tc.mp, tc.maxMP, TemporaryAbsorbMagic
					s.actor("self").HP = 1
					s.actor("self").Gear.AccessoryID = "item-110"
					s.actor("self").ItemDefinitionIDs = []string{"item-110"}
					s.Fields.Default = tc.field
					if tc.phantom {
						s.actor("self").Temporary = TemporaryPhantom
					}
					if reload {
						s = reloadLifetime(t, s)
						dc = nativeDefeatContext(dc.TargetContext)
					}
					c := StatusContext{EffectContext: EffectContext{TargetContext: dc.TargetContext}, Defeat: dc}
					rng := &targetRNG{t: t, draws: []float64{tc.draw, 0}}
					if tc.phantom {
						rng.draws = []float64{0}
					}
					r, err := s.ApplyStatusEntry(&c, nil, entry, "enemy", rng)
					if err != nil || len(r) != 1 || !r[0].Applied || s.actor("enemy").HP != 101 || s.actor("enemy").MP != tc.wantMP || s.actor("self").HP != 1 || c.DeathCalls != 1 {
						t.Fatal(s, r, c, err)
					}
					rng.done()
					if reload && !reflect.DeepEqual(reference, s) {
						t.Fatal("reload changed absorption/revival")
					}
					reference = s
				}
			})
		}
	}
}

func TestDeathAbsorptionKeepsOriginalRecipientAcrossFieldRedirect(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		sourceAbsorbs, destAbsorbs bool
		mp                         int
		accept                     bool
	}{
		{"source recovery cannot revive destination", true, false, 0, false},
		{"destination temporary is not re-evaluated", false, true, 0, false},
		{"destination already has revival mana", true, false, 26, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, dc := finalStrikeFixture()
			source := *s.actor("enemy")
			source.ID, source.MP, source.MaxMP = "source", 0, 100
			source.Gear = &BattleGearStats{}
			source.ItemDefinitionIDs = nil
			if tc.sourceAbsorbs {
				source.Temporary = TemporaryAbsorbMagic
			}
			s.Actors = append(s.Actors, source)
			dc.EnemyIDs = append(dc.EnemyIDs, "source")
			dc.PhantomTargetID, dc.PhantomEnemyIDs = "source", []string{"enemy"}
			s.Fields.Default = "phantom"
			a := s.actor("enemy")
			a.Abilities, a.MP, a.MaxMP = []string{"pharaoh"}, tc.mp, 100
			if tc.destAbsorbs {
				a.Temporary = TemporaryAbsorbMagic
			}
			s.actor("self").HP = 1
			s.actor("self").Gear.AccessoryID = "item-110"
			s.actor("self").ItemDefinitionIDs = []string{"item-110"}
			c := StatusContext{EffectContext: EffectContext{TargetContext: dc.TargetContext}, Defeat: dc}
			before := reloadLifetime(t, s)
			err := s.ValidateStatusEntry(&c, nil, "gem_instant_death_1", "source", &targetRNG{t: t})
			if (tc.accept && err != nil) || (!tc.accept && !errors.Is(err, ErrUnsupportedEffect)) || !reflect.DeepEqual(before, s) {
				t.Fatal(s, err)
			}
			if tc.accept {
				rng := &targetRNG{t: t, draws: []float64{.5, 0, 0}}
				r, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_1", "source", rng)
				if err != nil || !r[0].Applied || r[0].MPRecoveryTarget != "source" || s.actor("source").MP != 40 || s.actor("enemy").MP != 13 || s.actor("enemy").HP != 101 || s.actor("self").HP != 1 {
					t.Fatal(s, r, err)
				}
				rng.done()
			}
		})
	}
}

func TestAreaDeathRetainsGuaranteedAbsorptionOnUnsuccessfulDeath(t *testing.T) {
	s, dc := finalStrikeFixture()
	a := s.actor("enemy")
	a.Abilities, a.MP, a.MaxMP, a.Temporary = []string{"pharaoh"}, 20, 80, TemporaryAbsorbMagic
	source := *a
	source.ID, source.MP, source.Temporary = "source", 0, TemporaryAbsorbMagic
	source.Gear = &BattleGearStats{}
	source.ItemDefinitionIDs, source.Abilities = nil, nil
	s.Actors = append(s.Actors, source)
	dc.EnemyIDs = append(dc.EnemyIDs, "source")
	dc.PhantomTargetID, dc.PhantomEnemyIDs = "source", []string{"enemy"}
	s.Fields.Default = "phantom"
	s.actor("self").HP = 1
	s.actor("self").Gear.AccessoryID = "item-110"
	s.actor("self").ItemDefinitionIDs = []string{"item-110"}
	c := StatusContext{EffectContext: EffectContext{TargetContext: dc.TargetContext}, Defeat: dc}
	before := reloadLifetime(t, s)
	if err := s.ValidateStatusEntry(&c, nil, "gem_instant_death_sky", "", &targetRNG{t: t}); err != nil || !reflect.DeepEqual(before, s) {
		t.Fatal(s, err)
	}
	for _, success := range []bool{false, true} {
		s = reloadLifetime(t, before)
		draw := .99
		wantMP := 25
		if success {
			draw, wantMP = 0, 13
		}
		rng := &targetRNG{t: t, draws: []float64{0, draw, .5, 0, 0}}
		r, err := s.ApplyStatusEntry(&c, nil, "gem_instant_death_sky", "", rng)
		if err != nil || len(r) != 2 || r[0].Applied != success || !r[1].Applied || s.actor("enemy").HP != 101 || s.actor("enemy").MP != wantMP || s.actor("source").MP != 40 || s.actor("self").HP != 1 {
			t.Fatal(s, r, err)
		}
		rng.done()
	}
}

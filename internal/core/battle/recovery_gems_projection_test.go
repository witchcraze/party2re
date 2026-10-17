package battle

import (
	"errors"
	"reflect"
	"testing"
)

func conditionalReductionFixture(stat BattleStat) (LifetimeState, EffectContext) {
	s, c := recoveryFixture()
	s.Actors = []LifetimeActor{*s.actor("self"), *s.actor("enemy")}
	c.PartyIDs, c.EnemyIDs = []string{"self"}, []string{"enemy"}
	s.actor("self").Status, s.actor("self").Tension = StatusConfusion, 3
	*s.actor("self").currentStat(stat) = 250
	s.actor("enemy").ItemDefinitionIDs = []string{"item-159"}
	return s, c
}

func TestRecoveryGemConditionalReductionRetainsCappedBranch(t *testing.T) {
	for _, tc := range []struct {
		down, up int
		stat     BattleStat
	}{{93, 29, StatAttack}, {94, 31, StatDefense}, {95, 33, StatAgility}} {
		s, c := conditionalReductionFixture(tc.stat)
		ids := []int{tc.down, tc.up, 67}
		before, contextBefore := reloadLifetime(t, s), c
		rng := &targetRNG{t: t}
		if err := s.ValidateRecoveryStatGems(ids, &c, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(tc, err)
		}
		if _, err := s.ApplyRecoveryStatGems(ids, &c, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(tc, err)
		}
		if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
			t.Fatal("conditional reduction executed before rejection", tc, s, c)
		}
		rng.done()

		// Both enemy-helper draws choose the immune enemy, then ally resolution
		// chooses the unchanged capped actor. Only complete requester context admits67.
		level := 99
		c.RequesterLevel = &level
		rng = &targetRNG{t: t, draws: []float64{.9, .9, 0, 0}}
		r, err := s.ApplyRecoveryStatGems(ids, &c, "self", rng)
		if err != nil || r[0][0].Applied || r[1][0].Applied || !r[1][0].Returned || s.actor("self").Tension != 8 || *s.actor("self").currentStat(tc.stat) != 250 || *s.actor("enemy").currentStat(tc.stat) != 150 {
			t.Fatal(tc, r, err, s)
		}
		rng.done()
	}
}

func TestRecoveryGemGuaranteedReductionThenRaiseNeedsNoRequester(t *testing.T) {
	for _, tc := range []struct {
		down, up int
		stat     BattleStat
	}{{93, 29, StatAttack}, {94, 31, StatDefense}, {95, 33, StatAgility}} {
		s, c := conditionalReductionFixture(tc.stat)
		s.actor("self").Status = ""
		s.actor("enemy").ItemDefinitionIDs = nil
		s.actor("enemy").Temporary = TemporaryReflectMagic
		rng := &targetRNG{t: t}
		r, err := s.ApplyRecoveryStatGems([]int{tc.down, tc.up, 67}, &c, "self", rng)
		if err != nil || len(r) != 3 || !r[0][0].Applied || !r[1][0].Applied || *s.actor("self").currentStat(tc.stat) != 163 || s.actor("self").Tension != 3 {
			t.Fatal(tc, r, err, s)
		}
		rng.done()
	}
}

func TestRecoveryGemImpossibleReductionPreservesAcceptedCapNoEffect(t *testing.T) {
	s, c := conditionalReductionFixture(StatAgility)
	s.actor("self").Tension = 8
	s.actor("self").ItemDefinitionIDs = []string{"item-201"}
	s.actor("self").Temporary = TemporaryNullMagic
	s.actor("enemy").Agility = 250
	rng := &targetRNG{t: t, draws: []float64{.9, .9, 0}}
	r, err := s.ApplyRecoveryStatGems([]int{95, 33, 67}, &c, "self", rng)
	if err != nil || len(r) != 3 || r[0][0].Applied || r[1][0].Applied || s.actor("self").Agility != 250 || s.actor("self").Tension != 8 {
		t.Fatal(r, err, s, c)
	}
	rng.done()
}

func TestRecoveryGemPossibleReductionStillRequiresStatTail(t *testing.T) {
	s, c := conditionalReductionFixture(StatAgility)
	s.actor("self").ItemDefinitionIDs = []string{"item-201"}
	before, contextBefore := reloadLifetime(t, s), c
	rng := &targetRNG{t: t}
	if _, err := s.ApplyRecoveryStatGems([]int{95, 33}, &c, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
		t.Fatal("possible agility tail ran before admission", s, c)
	}
	rng.done()
}

func TestRecoveryGemTargetAlternativesDoNotAccumulateInSameSlot(t *testing.T) {
	// All initial ally picks (self cover, fallen->self cover, ally) resolve to
	// ally once. One slot advances 1.7->3 without requiring requester level.
	s, c := recoveryFixture()
	s.Actors = []LifetimeActor{*s.actor("self"), *s.actor("fallen"), *s.actor("ally"), *s.actor("enemy")}
	c.PartyIDs, c.EnemyIDs, c.Custom = []string{"self", "fallen", "ally"}, []string{"enemy"}, true
	s.actor("self").Temporary = TemporaryCover
	s.actor("ally").Temporary, s.actor("ally").Tension = TemporaryCovering, 1.7
	for i, draw := range []float64{0, .4, .9} {
		state, op := reloadLifetime(t, s), c
		rng := &targetRNG{t: t, draws: []float64{draw}}
		if err := state.ValidateRecoveryStatGems([]int{74}, &op, "enemy", &targetRNG{t: t}); err != nil {
			t.Fatal(i, err)
		}
		r, err := state.ApplyRecoveryStatGem(74, &op, "enemy", rng)
		if err != nil || len(r) != 1 || r[0].TargetID != "ally" || !r[0].Applied || state.actor("ally").Tension != 3 || state.actor("self").Tension != 1 || op.TensionCalls != 1 {
			t.Fatal(i, r, err, state, op)
		}
		rng.done()
	}

	// Converging alternatives also update minimum tension for a later slot.
	s, c = recoveryFixture()
	s.Actors = []LifetimeActor{*s.actor("self"), *s.actor("fallen"), *s.actor("ally"), *s.actor("enemy")}
	c.PartyIDs, c.EnemyIDs = []string{"self", "fallen", "ally"}, []string{"enemy"}
	s.actor("self").Temporary, s.actor("self").Tension = TemporaryCovering, 3
	s.actor("ally").Temporary = TemporaryCover
	before, contextBefore := reloadLifetime(t, s), c
	if _, err := s.ApplyRecoveryStatGems([]int{74, 67}, &c, "enemy", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
		t.Fatal(err, s, c)
	}

	// [94,28,74]: whether slot 28 cures physical seal and activates confusion or
	// fails cure, slot 74 only reaches each candidate once.
	for _, tc := range []struct {
		name       string
		confused   bool
		allyStart  float64
		enemyStart float64
		draws      []float64
		reqDraws   []float64
		wantTarget string
		wantErr    error
	}{
		{"seal_cure_fails_low_stage", true, 1.7, 8, []float64{.9, .9}, nil, "ally", nil},
		{"seal_cure_succeeds_confused_picks_enemy_8", true, 1.7, 8, []float64{0, 0, 0, .9}, nil, "enemy", nil},
		{"seal_cure_succeeds_confused_picks_covered_self", true, 1.7, 8, []float64{0, 0, 0, 0}, nil, "ally", nil},
		{"seal_cure_succeeds_confused_enemy_high_stage_rejects", true, 1.7, 5, nil, []float64{.9, .9}, "", ErrUnsupportedEffect},
		{"no_confusion_low_stage_cure_fails", false, 1.7, 5, []float64{.9, .9}, nil, "ally", nil},
		{"no_confusion_low_stage_cure_succeeds", false, 1.7, 5, []float64{0, .9}, nil, "ally", nil},
		{"no_confusion_ally_high_stage_rejects_upfront", false, 5, 8, nil, []float64{.9, .9, 0}, "", ErrUnsupportedEffect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryFixture()
			s.Actors = []LifetimeActor{*s.actor("self"), *s.actor("fallen"), *s.actor("ally"), *s.actor("enemy")}
			c.PartyIDs, c.EnemyIDs = []string{"self", "fallen", "ally"}, []string{"enemy"}
			c.Custom, c.Item195Activated = true, tc.confused
			s.actor("self").Status, s.actor("self").Temporary, s.actor("self").Tension = StatusPhysicalSeal, TemporaryCover, 8
			s.actor("ally").Temporary, s.actor("ally").Tension = TemporaryCovering, tc.allyStart
			s.actor("enemy").Tension = tc.enemyStart
			before, contextBefore := reloadLifetime(t, s), c
			if err := s.ValidateRecoveryStatGems([]int{94, 28, 74}, &c, "enemy", &targetRNG{t: t}); !errors.Is(err, tc.wantErr) {
				t.Fatal(err)
			}
			if tc.wantErr != nil {
				if _, err := s.ApplyRecoveryStatGems([]int{94, 28, 74}, &c, "enemy", &targetRNG{t: t}); !errors.Is(err, tc.wantErr) {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
					t.Fatal("rejected sequence mutated state or counters", s, c)
				}
				level := 99
				c.RequesterLevel = &level
				rng := &targetRNG{t: t, draws: tc.reqDraws}
				r, err := s.ApplyRecoveryStatGems([]int{94, 28, 74}, &c, "enemy", rng)
				if err != nil || len(r) != 3 || s.actor("enemy").Defense != 20 || s.actor("self").Tension != 1 || s.actor("ally").Tension < 3 {
					t.Fatal(r, err, s, c)
				}
				rng.done()
				return
			}
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ApplyRecoveryStatGems([]int{94, 28, 74}, &c, "enemy", rng)
			if err != nil || len(r) != 3 || r[2][0].TargetID != tc.wantTarget || s.actor("enemy").Defense != 20 || s.actor("self").Tension != 1 || c.PartyStatDownCalls != 1 || c.TensionCalls != 1 {
				t.Fatal(r, err, s, c)
			}
			if tc.wantTarget == "ally" && s.actor("ally").Tension != 3 {
				t.Fatal(s)
			}
			rng.done()
		})
	}
}

func TestRecoveryGemFallenEnemyRetainsNoEffectBranch(t *testing.T) {
	for _, tc := range []struct {
		name        string
		target      string
		enemyIDs    []string
		allAlive    bool
		requester   bool
		draws       []float64
		wantTension float64
		wantErr     error
	}{
		{
			name:      "mixed_fallback_rejects_without_requester",
			target:    "ally",
			enemyIDs:  []string{"enemy", "dead_enemy"},
			requester: false,
			wantErr:   ErrUnsupportedEffect,
		},
		{
			name:      "mixed_fallback_non_initial_roster_rejects_without_requester",
			target:    "ally",
			enemyIDs:  []string{"dead_enemy", "enemy"},
			requester: false,
			wantErr:   ErrUnsupportedEffect,
		},
		{
			name:      "explicit_fallen_target_rejects_without_requester",
			target:    "dead_enemy",
			enemyIDs:  []string{"enemy", "dead_enemy"},
			requester: false,
			wantErr:   ErrUnsupportedEffect,
		},
		{
			name:        "all_alive_reflected_admitted_without_requester",
			target:      "ally",
			enemyIDs:    []string{"enemy"},
			allAlive:    true,
			requester:   false,
			draws:       []float64{0},
			wantTension: 3,
			wantErr:     nil,
		},
		{
			name:        "mixed_fallback_admitted_with_requester_fallen_chosen",
			target:      "ally",
			enemyIDs:    []string{"enemy", "dead_enemy"},
			requester:   true,
			draws:       []float64{.9, 0},
			wantTension: 8,
			wantErr:     nil,
		},
		{
			name:        "mixed_fallback_admitted_with_requester_alive_reflected_chosen",
			target:      "ally",
			enemyIDs:    []string{"enemy", "dead_enemy"},
			requester:   true,
			draws:       []float64{0},
			wantTension: 3,
			wantErr:     nil,
		},
		{
			name:        "explicit_fallen_target_admitted_with_requester",
			target:      "dead_enemy",
			enemyIDs:    []string{"enemy", "dead_enemy"},
			requester:   true,
			draws:       []float64{0},
			wantTension: 8,
			wantErr:     nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := recoveryFixture()
			c.Custom = true
			c.PartyIDs = []string{"self", "fallen", "ally"}
			c.EnemyIDs = tc.enemyIDs
			if tc.allAlive {
				s.Actors = []LifetimeActor{*s.actor("self"), *s.actor("fallen"), *s.actor("ally"), *s.actor("enemy")}
			}
			s.actor("self").Hit = 50
			s.actor("self").Tension = 3
			s.actor("enemy").Temporary = TemporaryReflectAttack
			if tc.requester {
				level := 99
				c.RequesterLevel = &level
			} else {
				c.RequesterLevel = nil
			}
			ids := []int{54, 68, 67}
			before, contextBefore := reloadLifetime(t, s), c
			rng := &targetRNG{t: t}
			if err := s.ValidateRecoveryStatGems(ids, &c, tc.target, rng); !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateRecoveryStatGems err = %v, want %v", err, tc.wantErr)
			}
			rng.done()

			if tc.wantErr != nil {
				execRNG := &targetRNG{t: t}
				if _, err := s.ApplyRecoveryStatGems(ids, &c, tc.target, execRNG); !errors.Is(err, tc.wantErr) {
					t.Fatalf("ApplyRecoveryStatGems err = %v, want %v", err, tc.wantErr)
				}
				execRNG.done()
				if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
					t.Fatalf("rejected sequence mutated state or context: state=%+v, context=%+v", s, c)
				}
				return
			}

			execRNG := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ApplyRecoveryStatGems(ids, &c, tc.target, execRNG)
			if err != nil {
				t.Fatalf("ApplyRecoveryStatGems unexpected error: %v", err)
			}
			execRNG.done()
			if len(r) != 3 {
				t.Fatalf("results len = %d, want 3", len(r))
			}
			if s.actor("self").Hit != 95 {
				t.Fatalf("self.Hit = %d, want 95", s.actor("self").Hit)
			}
			if s.actor("self").Tension != tc.wantTension {
				t.Fatalf("self.Tension = %v, want %v", s.actor("self").Tension, tc.wantTension)
			}
			if deadEnemy := s.actor("dead_enemy"); deadEnemy != nil && deadEnemy.HP != 0 {
				t.Fatalf("dead_enemy.HP = %d, want 0", deadEnemy.HP)
			}
		})
	}
}

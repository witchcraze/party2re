package battle

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestRecoveryGemTensionAdmissionAndRole(t *testing.T) {
	for _, npc := range []bool{false, true} {
		s, c := recoveryFixture()
		level := 99
		c.RequesterLevel, c.ActorNPC = &level, npc
		s.actor("self").NPC = npc
		s.actor("self").ItemDefinitionIDs = []string{"item-145", "item-161"}
		rng := &targetRNG{t: t, draws: []float64{0, 0, 0, .9, 0, 0, 0, 0}}
		if npc {
			//145 draws before its NPC predicate;161's existing defense still draws.
			rng.draws = []float64{0, 0, 0, 0, 0}
		}
		_, err := s.ApplyRecoveryStatGem(67, &c, "ally", rng)
		if err != nil {
			t.Fatal(err)
		}
		if npc {
			if s.actor("self").Tension != 3 || c.TensionCalls != 2 {
				t.Fatal(s, c)
			}
		} else if s.actor("self").Tension != 8 || c.TensionCalls != 4 {
			t.Fatal(s, c)
		}
		rng.done()
	}
	for _, ids := range [][]int{{67, 67}, {67}} {
		s, c := recoveryFixture()
		if len(ids) == 1 {
			s.actor("self").Tension = 3
		}
		before := reloadLifetime(t, s)
		_, err := s.ApplyRecoveryStatGems(ids, &c, "ally", &targetRNG{t: t})
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.TensionCalls != 0 {
			t.Fatal(ids, err, s, c)
		}
	}
	// Guaranteed reset makes the requester-level branch unreachable.
	for _, reset := range []int{69, 20} {
		s, c := recoveryFixture()
		s.actor("self").Tension = 5
		rng := &targetRNG{t: t}
		if reset == 20 {
			rng.draws = []float64{0}
		}
		if _, err := s.ApplyRecoveryStatGems([]int{reset, 67}, &c, "ally", rng); err != nil || s.actor("self").Tension != 3 {
			t.Fatal(reset, err, s)
		}
		rng.done()
	}
}

func TestRecoveryGemTargetGuardsAndResources(t *testing.T) {
	for _, tc := range []struct {
		id           int
		target, want string
		tmp          TemporaryEffect
		draws        []float64
	}{
		{68, "enemy", "self", TemporaryReflectAttack, nil},
		{68, "enemy", "enemy", TemporaryNullAttack, nil},
		{74, "ally", "fallen", TemporaryCover, nil},
		{74, "missing", "self", "", []float64{.4}},
	} {
		s, c := recoveryFixture()
		s.actor("enemy").Tension = 5
		s.actor("fallen").Temporary = TemporaryCovering
		if tc.id == 68 {
			s.actor("self").Tension = 5
			s.actor("enemy").Temporary = tc.tmp
		} else {
			s.actor("ally").Temporary = tc.tmp
		}
		rng := &targetRNG{t: t, draws: tc.draws}
		r, err := s.ApplyRecoveryStatGem(tc.id, &c, tc.target, rng)
		want := 1.7
		if tc.id == 68 {
			want = 1
		}
		if err != nil || r[0].TargetID != tc.want || s.actor(tc.want).Tension != want {
			t.Fatal(tc, r, err, s)
		}
		rng.done()
	}
	// A failed physical seal check returns before68's write.
	s, c := recoveryFixture()
	s.actor("self").Status = StatusPhysicalSeal
	s.actor("enemy").Tension = 5
	r, err := s.ApplyRecoveryStatGem(68, &c, "enemy", &targetRNG{t: t, draws: []float64{.25}})
	if err != nil || r[0].Returned || s.actor("enemy").Tension != 5 {
		t.Fatal(r, err, s)
	}
	// Absorption facts belong to this completed entry and must not be replayed.
	s, c = recoveryFixture()
	s.actor("ally").Temporary = TemporaryAbsorbMagic
	s.actor("ally").MP, s.actor("ally").MaxMP = 90, 100
	r, err = s.ApplyRecoveryStatGem(19, &c, "ally", &targetRNG{t: t, draws: []float64{0, 0}})
	if err != nil || r[0].MPRecovery != 20 || s.actor("ally").MP != 100 || s.actor("ally").HP != 190 {
		t.Fatal(r, err, s)
	}
}

func TestRecoveryGemEntryStatTailAndNoEffect(t *testing.T) {
	for _, id := range []int{32, 33} {
		for _, cap := range []bool{false, true} {
			s, c := recoveryFixture()
			target := "ally"
			if id == 32 {
				target = "self"
			}
			s.actor(target).ItemDefinitionIDs = []string{"item-201", "item-205"}
			if cap {
				s.actor(target).Agility = 250
			}
			level := 99
			called := 0
			if !cap {
				c.RequesterLevel = &level
				c.Damage = func(ctx TargetContext, hit TargetEffect, _ random.Generator) error {
					called++
					if hit.TargetID != "" || hit.Category != CategoryUntyped || hit.Payload.Number != 125 || !reflect.DeepEqual(hit.Arguments, []string{"1"}) || s.actor(target).Agility != 250 || s.actor(target).Attack != 150 {
						t.Fatal(ctx, hit, s)
					}
					c.DamageCalls++
					return nil
				}
			}
			r, err := s.ApplyRecoveryStatGem(id, &c, "ally", &targetRNG{t: t})
			if err != nil || !r[0].Returned || r[0].Applied == cap {
				t.Fatal(r, err)
			}
			if cap {
				if called != 0 || c.TensionCalls != 0 {
					t.Fatal(c, called)
				}
			} else if called != 1 || r[0].Amount != 100 || r[0].Delta != 100 || s.actor(target).Attack != 250 || s.actor(target).Tension != 1.7 || s.actor(target).Temporary != TemporaryDouble {
				t.Fatal(r, c, s)
			}
		}
	}
}

func TestRecoveryGemSavedFactsDoNotExecuteCallbacks(t *testing.T) {
	s, c := recoveryFixture()
	r, err := s.ApplyRecoveryStatGems([]int{67, 19, 19}, &c, "ally", &targetRNG{t: t, draws: []float64{0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	saved := struct {
		State LifetimeState
		Input []int
		Facts [][]EffectApplication
	}{s, []int{67, 19, 19}, r}
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var loaded = saved
	if err := json.Unmarshal(data, &loaded); err != nil || !reflect.DeepEqual(saved, loaded) {
		t.Fatal(err, loaded)
	}
	if loaded.State.actor("ally").HP != 460 {
		t.Fatal(loaded)
	}
}

func TestRecoveryGemMagicSealAndSelfRoles(t *testing.T) {
	for _, id := range []int{19, 20, 21, 22, 23, 28, 29, 30, 31, 32, 33} {
		s, c := recoveryFixture()
		s.actor("self").Status = StatusMagicSeal
		physical := id == 28 || id == 30 || id == 32
		untyped := id == 20
		rng := &targetRNG{t: t}
		if !physical && !untyped {
			rng.draws = []float64{.25}
		}
		if untyped {
			rng.draws = []float64{0}
		}
		r, err := s.ApplyRecoveryStatGem(id, &c, "ally", rng)
		if err != nil {
			t.Fatal(id, r, err)
		}
		if physical || untyped {
			if len(r) != 1 || !r[0].Applied || r[0].TargetID != "self" {
				t.Fatal(id, r)
			}
		} else {
			for _, effect := range r {
				if effect.Applied {
					t.Fatal(id, r)
				}
			}
			if c.HealCalls != 0 || c.StatUpCalls != 0 || c.PartyHealCalls != 0 {
				t.Fatal(id, c)
			}
		}
		rng.done()
	}
}

func TestRecoveryGemResourcesPrecedeStatTails(t *testing.T) {
	s, c := recoveryFixture()
	s.actor("self").MP = -3
	s.actor("ally").Temporary = TemporaryAbsorbMagic
	s.actor("ally").MP, s.actor("ally").MaxMP = 90, 100
	s.actor("ally").ItemDefinitionIDs = []string{"item-201"}
	c.Damage = func(_ TargetContext, _ TargetEffect, _ random.Generator) error {
		if s.actor("ally").MP != 100 {
			t.Fatal("absorption must precede damage", s)
		}
		s.actor("ally").MP -= 3
		return nil
	}
	r, err := s.ApplyRecoveryStatGem(33, &c, "ally", &targetRNG{t: t, draws: []float64{0}})
	if err != nil || r[0].MPRecovery != 30 || s.actor("ally").MP != 97 || s.actor("self").MP != -3 {
		t.Fatal(r, err, s)
	}
}

func TestRecoveryGemDefenseStillRoundsToNoEffect(t *testing.T) {
	s, c := recoveryFixture()
	a := s.actor("ally")
	a.ItemDefinitionIDs = []string{"item-161", "item-201"}
	a.BaseStats.Agility, a.Agility = 1, 1
	a.Temporary = TemporaryNullMagic
	rng := &targetRNG{t: t, draws: []float64{0, 0}}
	r, err := s.ApplyRecoveryStatGems([]int{74, 33}, &c, "ally", rng)
	if err != nil || r[1][0].Applied || c.StatUpCalls != 1 || a.Agility != 1 {
		t.Fatal(r, err, s, c)
	}
	rng.done()
}

func TestRecoveryGemReductionReopensOnlyReachableCaps(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		s, c := recoveryFixture()
		s.actor("self").ItemDefinitionIDs = []string{"item-201"}
		s.actor("self").Agility = 250
		s.actor("enemy").Temporary = TemporaryReflectMagic
		if blocked {
			s.actor("self").ItemDefinitionIDs = append(s.actor("self").ItemDefinitionIDs, "item-239")
			c.DamageCalls = 1
		}
		before := reloadLifetime(t, s)
		r, err := s.ApplyRecoveryStatGems([]int{95, 32}, &c, "enemy", &targetRNG{t: t})
		if blocked {
			if err != nil || r[1][0].Applied || !r[1][0].Returned {
				t.Fatal(r, err)
			}
		} else if !errors.Is(err, ErrUnsupportedEffect) || c.PartyStatDownCalls != 0 {
			t.Fatal(r, err, c)
		}
		if !reflect.DeepEqual(s, before) {
			t.Fatal(s, before)
		}
	}
}

func TestRecoveryGemDirectWritesAndTension(t *testing.T) {
	for _, id := range []int{54, 67, 68, 69, 74} {
		s, c := recoveryFixture()
		level := 99
		c.RequesterLevel = &level
		for i := range s.Actors {
			s.Actors[i].Hit, s.Actors[i].Tension = 50, 3
		}
		if id == 69 {
			s.actor("enemy").HP = 0
		}
		s.actor("self").Status = StatusMagicSeal
		rng := &targetRNG{t: t}
		if id == 67 {
			rng.draws = []float64{.99}
		}
		target := "ally"
		if id == 68 {
			target = "enemy"
		}
		r, err := s.ApplyRecoveryStatGem(id, &c, target, rng)
		if err != nil || len(r) == 0 {
			t.Fatal(id, r, err)
		}
		switch id {
		case 54:
			if s.actor("fallen").Hit != 95 || s.actor("enemy").Hit != 50 {
				t.Fatal(s)
			}
		case 67:
			if s.actor("self").Tension != 8 || s.actor("ally").Tension != 3 || c.TensionCalls != 2 {
				t.Fatal(s, c)
			}
		case 68:
			if r[0].TargetID != "enemy" || s.actor("enemy").Tension != 1 || s.actor("self").Tension != 3 {
				t.Fatal(s, r)
			}
		case 69:
			for _, a := range s.Actors {
				if a.Tension != 1 {
					t.Fatal(a)
				}
			}
		case 74:
			if s.actor("ally").Tension != 5 || c.TensionCalls != 1 {
				t.Fatal(s, c)
			}
		}
		rng.done()
	}
}

func TestRecoveryGemSequencePreflight(t *testing.T) {
	for _, ids := range [][]int{{54, 101}, {19, 33}, {31, 33}} {
		s, c := recoveryFixture()
		level := 99
		c.RequesterLevel = &level
		s.actor("self").Status = StatusMagicSeal
		s.actor("ally").ItemDefinitionIDs = []string{"item-201"}
		if ids[0] == 31 {
			s.actor("self").Status = ""
			s.actor("self").ItemDefinitionIDs = []string{"item-205"}
			s.actor("self").Temporary = TemporaryCovering
			s.actor("ally").Temporary = TemporaryCover
		}
		before, contextBefore := reloadLifetime(t, s), c
		_, err := s.ApplyRecoveryStatGems(ids, &c, "ally", &targetRNG{t: t})
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
			t.Fatal(ids, err, s, c)
		}
	}
}

func TestRecoveryGemUsesRequesterLevel(t *testing.T) {
	for _, level := range []int{24, 25, 69} {
		s, c := recoveryFixture()
		c.RequesterLevel = &level
		s.actor("self").Tension = 3
		rng := &targetRNG{t: t}
		if level >= 25 {
			rng.draws = []float64{.99}
		}
		_, err := s.ApplyRecoveryStatGem(67, &c, "ally", rng)
		want := 5.0
		if level == 69 {
			want = 8
		}
		if err != nil || s.actor("self").Tension != want || c.TensionCalls != 2 {
			t.Fatal(level, err, s, c)
		}
		rng.done()
	}
}

func TestRecoveryGemSelfTensionDoesNotCureOrActivateTargetHelpers(t *testing.T) {
	s, c := recoveryFixture()
	c.Item195Activated = true
	s.actor("self").Status = StatusMagicSeal
	s.actor("self").Agility = 250
	s.actor("enemy").ItemDefinitionIDs = []string{"item-201"}
	r, err := s.ApplyRecoveryStatGems([]int{67, 32}, &c, "self", &targetRNG{t: t})
	if err != nil || r[1][0].Applied || !r[1][0].Returned || s.actor("self").Status != StatusMagicSeal {
		t.Fatal(r, err, s, c)
	}
}

func TestRecoveryGemStatAndEnemyResetConsumeTensionBeforeLaterSlots(t *testing.T) {
	for _, id := range []int{28, 94, 68} {
		s, c := recoveryFixture()
		s.actor("self").Tension = 5
		if id == 68 {
			s.actor("enemy").Temporary = TemporaryReflectAttack
		}
		r, err := s.ApplyRecoveryStatGems([]int{id, 67}, &c, "enemy", &targetRNG{t: t})
		if err != nil || len(r) != 2 || s.actor("self").Tension != 3 {
			t.Fatal(id, r, err, s, c)
		}
	}
}

func TestRecoveryGemZeroHealingRetainsTensionForAdmission(t *testing.T) {
	s, c := recoveryFixture()
	s.actor("self").Tension = 5
	s.actor("ally").Temporary = TemporaryStrongDefense
	s.actor("ally").Status = StatusVirulentPoison
	c.Item185Activated = true
	before := reloadLifetime(t, s)
	_, err := s.ApplyRecoveryStatGems([]int{19, 67}, &c, "ally", &targetRNG{t: t})
	if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || c.HealCalls != 0 || c.TensionCalls != 0 {
		t.Fatal(err, s, c)
	}
}

func TestRecoveryGemRepeatedSlotsReloadAndFrozenInput(t *testing.T) {
	s, c := recoveryFixture()
	ids := []int{67, 19, 19}
	beforeIDs := append([]int(nil), ids...)
	var states []LifetimeState
	var facts [][][]EffectApplication
	for _, reload := range []bool{false, true} {
		state, op := reloadLifetime(t, s), c
		if reload {
			state = reloadLifetime(t, state)
		}
		rng := &targetRNG{t: t, draws: []float64{0, 0}}
		r, err := state.ApplyRecoveryStatGems(ids, &op, "ally", rng)
		if err != nil || state.actor("ally").HP != 460 || op.TensionCalls != 2 || op.HealCalls != 2 {
			t.Fatal(r, err, state, op)
		}
		states = append(states, reloadLifetime(t, state))
		facts = append(facts, r)
		rng.done()
	}
	if !reflect.DeepEqual(states[0], states[1]) || !reflect.DeepEqual(facts[0], facts[1]) || !reflect.DeepEqual(ids, beforeIDs) {
		t.Fatal(states, facts, ids)
	}
}

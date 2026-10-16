package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestRecoveryGemRejectsExcessSlotsBeforeEffects(t *testing.T) {
	for _, ids := range [][]int{{28, 67, 28, 67}, {19, 28, 67, 28}, {54, 54, 54, 54}} {
		s, c := recoveryFixture()
		before, contextBefore := reloadLifetime(t, s), c
		rng := &targetRNG{t: t}
		if err := s.ValidateRecoveryStatGems(ids, &c, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatalf("oversized sequence admitted: %v, %v", ids, err)
		}
		if _, err := s.ApplyRecoveryStatGems(ids, &c, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(ids, err)
		}
		if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
			t.Fatal("rejection mutated state or counters", ids, s, c)
		}
		rng.done()
	}
}

func TestRecoveryGemThreeSlotCapRetainsTension(t *testing.T) {
	for _, id := range []int{28, 30, 32} {
		s, c := recoveryFixture()
		// NPC templates use three positions without the player's synthesis capacity gate.
		s.actor("self").NPC, c.ActorNPC, c.Custom = true, true, true
		rng := &targetRNG{t: t}
		results, err := s.ApplyRecoveryStatGems([]int{id, 67, id}, &c, "self", rng)
		if err != nil || len(results) != 3 {
			t.Fatal(id, results, err)
		}
		if !results[0][0].Applied || !results[2][0].Returned || results[2][0].Applied || s.actor("self").Tension != 3 || c.StatUpCalls != 2 || c.TensionCalls != 2 {
			t.Fatal(id, results, s, c)
		}
		// A saved cap outcome preserves tension for admission of the next command.
		loaded := reloadLifetime(t, s)
		loaded.actor("self").ItemDefinitionIDs = []string{"item-161"}
		op := EffectContext{TargetContext: c.TargetContext, ActorNPC: c.ActorNPC}
		before, contextBefore := reloadLifetime(t, loaded), op
		if _, err := loaded.ApplyRecoveryStatGems([]int{74, id, 67}, &op, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		if !reflect.DeepEqual(loaded, before) || !reflect.DeepEqual(op, contextBefore) {
			t.Fatal("later unsupported tension executed earlier slots", id, loaded, op)
		}
		rng.done()
	}
}

func TestRecoveryGemRepeatedSealFailuresRejectWholeSequence(t *testing.T) {
	for _, tc := range []struct {
		ids  []int
		seal string
	}{
		{[]int{28, 28, 67}, StatusPhysicalSeal},
		{[]int{29, 29, 67}, StatusMagicSeal},
		{[]int{19, 19, 67}, StatusMagicSeal},
		{[]int{22, 22, 67}, StatusMagicSeal},
		{[]int{94, 93, 67}, StatusMagicSeal},
		{[]int{68, 68, 67}, StatusPhysicalSeal},
	} {
		s, c := recoveryFixture()
		s.actor("self").Attack = 200
		s.actor("self").Tension = 3
		s.actor("self").Status = tc.seal
		s.actor("enemy").Temporary = TemporaryReflectAttack
		before, contextBefore := reloadLifetime(t, s), c
		rng := &targetRNG{t: t}
		if err := s.ValidateRecoveryStatGems(tc.ids, &c, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(tc, err)
		}
		if _, err := s.ApplyRecoveryStatGems(tc.ids, &c, "self", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(tc, err)
		}
		if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
			t.Fatal("possible repeated seal failures executed earlier slots", tc, s, c)
		}
		rng.done()
		level := 99
		c.RequesterLevel = &level
		if _, err := s.ApplyRecoveryStatGems(tc.ids, &c, "self", random.NewDeterministic(1)); err != nil {
			t.Fatal("complete requester context should admit seal branches", tc, err)
		}
	}
}

func TestRecoveryGemPossibleResetFromMaximumTensionRejectsBeforeDraws(t *testing.T) {
	for _, id := range []int{28, 19, 94, 68} {
		s, c := recoveryFixture()
		s.actor("self").Tension = 8
		s.actor("self").Status = StatusConfusion
		before, contextBefore := reloadLifetime(t, s), c
		rng := &targetRNG{t: t}
		ids := []int{id, 67, 67}
		if err := s.ValidateRecoveryStatGems(ids, &c, "enemy", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		if _, err := s.ApplyRecoveryStatGems(ids, &c, "enemy", rng); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		if !reflect.DeepEqual(s, before) || !reflect.DeepEqual(c, contextBefore) {
			t.Fatal("possible tension reset was executed before rejection", id, s, c)
		}
		rng.done()
		level := 99
		c.RequesterLevel = &level
		if _, err := s.ApplyRecoveryStatGems(ids, &c, "enemy", random.NewDeterministic(1)); err != nil {
			t.Fatal("complete requester context should admit reset branches", id, err)
		}
	}
}

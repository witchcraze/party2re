package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestTensionGemRegisteredContinuations(t *testing.T) {
	for _, ids := range [][]string{
		{"gem_mind_eye", "gem_divine_tension_sky", "gem_guard_1"},
		{"gem_calm_sky", "gem_divine_tension_sky", "gem_guard_1"},
		{"gem_inspire_sky", "gem_inspire_sky", "gem_guard_1"},
	} {
		s, c := recoveryGemFixture(false)
		c.RequesterLevel = nil
		var d BattleDefinitions
		d.RegisterRecoveryStatGems(&c)
		d.RegisterDefenseGems()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "tail"}}
		before := append([]corebattle.LifetimeActor(nil), s.Actors...)
		ops, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
		if err != nil || !reflect.DeepEqual(before, s.Actors) || c.TensionCalls != 0 {
			t.Fatal(ids, "discovery", err, s, c)
		}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
			t.Fatal(ids, err)
		}
		if c.TensionCalls != 2 || s.Actors[0].Temporary != corebattle.TemporaryDefense || !reflect.DeepEqual(f.Arguments, []string{"ally", "tail"}) {
			t.Fatal(ids, s, c, f)
		}
		index := 0
		if ids[0] == "gem_inspire_sky" {
			index = 2 // Both slots reuse the original ally argument.
		}
		if s.Actors[index].Tension != 3 {
			t.Fatal(ids, s)
		}
	}
}

func TestTensionGemRegisteredFallenContinuation(t *testing.T) {
	for _, requester := range []bool{false, true} {
		s, c := recoveryGemFixture(false)
		s.Actors[0].Tension = 3
		s.Actors[3].Temporary = corebattle.TemporaryReflectAttack
		dead := s.Actors[3]
		dead.ID, dead.HP = "dead-enemy", 0
		s.Actors = append(s.Actors, dead)
		c.EnemyIDs = append(c.EnemyIDs, dead.ID)
		if !requester {
			c.RequesterLevel = nil
		}
		var d BattleDefinitions
		d.RegisterRecoveryStatGems(&c)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
		before := append([]corebattle.LifetimeActor(nil), s.Actors...)
		ids := []string{"gem_mind_eye", "gem_calm_orb", "gem_divine_tension_sky"}
		ops, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
		if !requester {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(before, s.Actors) || c.TensionCalls != 0 {
				t.Fatal("fallen no-effect lost", err, s, c)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		rng := &queuedRecoveryGemRNG{t: t, draws: []float64{.9, 0}}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || len(rng.draws) != 0 || s.Actors[0].Tension != 8 || s.Actors[1].Hit != 95 || c.TensionCalls != 2 {
			t.Fatal(err, s, c, rng.draws)
		}
	}
}

func TestTensionGemRejectsLaterUncheckedDefenseBeforeEffects(t *testing.T) {
	s, c := recoveryGemFixture(false)
	s.Actors[0].Status = corebattle.StatusMagicSeal
	var d BattleDefinitions
	d.RegisterRecoveryStatGems(&c)
	d.RegisterDefenseGems()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally"}}
	before := append([]corebattle.LifetimeActor(nil), s.Actors...)
	for _, later := range []string{"gem_nullify_magic_1", "missing"} {
		ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "gem_mind_eye"}, {Kind: "entry", EntryID: later}}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(before, s.Actors) || c.TensionCalls != 0 {
			t.Fatal(err, s, c)
		}
	}
}

func TestTensionGemSharesOperationCountersAcrossRegistrations(t *testing.T) {
	s, first := recoveryGemFixture(false)
	second := first
	var d BattleDefinitions
	d.RegisterRecoveryStatGems(&second)
	d.Invocations.Entries["first"] = d.RecoveryStatInvocation(&first, []string{"gem_inspire_sky"})
	f := corebattle.CallbackContext{TargetContext: first.TargetContext, Arguments: []string{"ally"}}
	ops, err := d.InvocationAction(&s, &f, []string{"first", "gem_inspire_sky"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil || first.TensionCalls != 1 || second.TensionCalls != 2 || f.Damage.TensionCalls != 2 || s.Actors[2].Tension != 3 {
		t.Fatal(err, first, second, f)
	}
}

func TestTensionGemCoverReplacementChangesLaterAdmission(t *testing.T) {
	for _, requester := range []bool{false, true} {
		s, c := recoveryGemFixture(false)
		s.Actors[0].Tension, s.Actors[0].Temporary = 5, corebattle.TemporaryCover
		s.Actors[2].Tension, s.Actors[2].Temporary = 1.7, corebattle.TemporaryCovering
		s.Actors[2].ItemDefinitionIDs = []string{"item-161"}
		if !requester {
			c.RequesterLevel = nil
		}
		var d BattleDefinitions
		d.RegisterRecoveryStatGems(&c)
		d.RegisterDefenseGems()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		before := append([]corebattle.LifetimeActor(nil), s.Actors...)
		ops, err := d.InvocationAction(&s, &f, []string{"gem_inspire_sky", "gem_awakening_1", "gem_inspire_sky"}, noArithmeticDraws{t})
		if !requester {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(before, s.Actors) || c.TensionCalls != 0 {
				t.Fatal("161 can reopen the actor's high-stage branch", err, s, c)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		rng := &queuedRecoveryGemRNG{t: t, draws: []float64{0, 0, .9, 0, 0}}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || len(rng.draws) != 0 || s.Actors[0].Tension != 8 || s.Actors[2].Tension != 3 || s.Actors[2].Temporary != corebattle.TemporaryDefense || c.TensionCalls != 2 {
			t.Fatal(err, s, c, rng.draws)
		}
	}
}

func TestTensionGemRegisteredCoverAlternatives(t *testing.T) {
	s, c := recoveryGemFixture(false)
	c.RequesterLevel = nil
	s.Actors[0].Temporary = corebattle.TemporaryCover
	s.Actors[2].Temporary, s.Actors[2].Tension = corebattle.TemporaryCovering, 1.7
	var d BattleDefinitions
	d.RegisterRecoveryStatGems(&c)
	d.RegisterDefenseGems()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	ops, err := d.InvocationAction(&s, &f, []string{"gem_inspire_sky", "gem_awakening_1", "gem_inspire_sky"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &queuedRecoveryGemRNG{t: t, draws: []float64{0, .9}}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || len(rng.draws) != 0 || s.Actors[2].Tension != 5 || s.Actors[0].Tension != 1 || c.TensionCalls != 2 {
		t.Fatal("one increase per slot regardless of equivalent target paths", err, s, c)
	}
}

func TestTensionGemRegisteredSealCureAlternatives(t *testing.T) {
	for _, cure := range []bool{false, true} {
		s, c := recoveryGemFixture(false)
		s.Actors[0].Status, s.Actors[0].Tension = corebattle.StatusPhysicalSeal, 3
		s.Actors[3].Temporary = corebattle.TemporaryReflectAttack
		var d BattleDefinitions
		d.RegisterRecoveryStatGems(&c)
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		ops, err := d.InvocationAction(&s, &f, []string{"gem_mind_eye", "gem_calm_orb", "gem_divine_tension_sky"}, noArithmeticDraws{t})
		if err != nil {
			t.Fatal(err)
		}
		draws, want, status := []float64{.9, 0}, 8.0, corebattle.StatusPhysicalSeal
		if cure {
			draws, want, status = []float64{0}, 3, ""
		}
		rng := &queuedRecoveryGemRNG{t: t, draws: draws}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || len(rng.draws) != 0 || s.Actors[0].Tension != want || s.Actors[0].Status != status || c.TensionCalls != 2 {
			t.Fatal(cure, err, s, c, rng.draws)
		}
	}
}

func TestTensionGemContinuationReloadAndRebuiltContexts(t *testing.T) {
	s, c := recoveryGemFixture(false)
	c.RequesterLevel = nil
	var d BattleDefinitions
	d.RegisterRecoveryStatGems(&c)
	d.RegisterDefenseGems()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"ally", "tail"}}
	ids := []string{"gem_inspire_sky", "gem_guard_1", "gem_inspire_sky"}
	ops, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	// Save the real first-slot outcome and frozen remaining command facts.
	if err := s.ApplyInvocations(&f, d.Invocations, ops[:1], noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	type checkpoint struct {
		State        corebattle.LifetimeState
		Target       corebattle.TargetContext
		Arguments    []string
		Remaining    []corebattle.CallbackInvocation
		TensionCalls int
	}
	saved := checkpoint{s, f.TargetContext, f.Arguments, ops[1:], c.TensionCalls}
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops[1:], noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	rebuiltContext := corebattle.EffectContext{TargetContext: loaded.Target, TensionCalls: loaded.TensionCalls}
	var rebuilt BattleDefinitions
	rebuilt.RegisterRecoveryStatGems(&rebuiltContext)
	rebuilt.RegisterDefenseGems()
	rebuiltFrame := corebattle.CallbackContext{TargetContext: loaded.Target, Arguments: loaded.Arguments}
	if err := loaded.State.ApplyInvocations(&rebuiltFrame, rebuilt.Invocations, loaded.Remaining, noArithmeticDraws{t}); err != nil || !reflect.DeepEqual(s, loaded.State) || rebuiltContext.TensionCalls != 2 || f.Damage.TensionCalls != 2 || c.TensionCalls != 2 || loaded.State.Actors[2].Tension != 3 {
		t.Fatal(err, loaded.State, rebuiltContext, f)
	}
}

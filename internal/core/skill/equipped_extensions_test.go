package skill

import (
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Registration, discovery and execution share one gate for every concrete entry.
func TestEquippedExtensionRegistrationAndExecution(t *testing.T) {
	var empty BattleDefinitions
	s, base, _ := dualAgilityFixture()
	c := &corebattle.EquippedExtensionContext{DamageContext: base.DamageContext}
	if _, err := empty.EquippedExtensionAction(&s, c, "extension-212", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("unregistered", err)
	}
	var nilContext BattleDefinitions
	nilContext.RegisterEquippedExtensions(nil)
	if _, err := nilContext.InvocationAction(&s, &corebattle.CallbackContext{TargetContext: c.TargetContext}, []string{"extension-212"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("nil context", err)
	}
	for _, entry := range corebattle.EquippedExtensionEntries() {
		var d BattleDefinitions
		d.RegisterEquippedExtensions(c)
		if _, ok := d.Invocations.Entries[entry.ID]; !ok || len(d.Invocations.JobTables) != 0 {
			t.Fatal(entry.ID, d.Invocations.JobTables)
		}
		s, base, _ := dualAgilityFixture()
		c := &corebattle.EquippedExtensionContext{DamageContext: base.DamageContext}
		d.RegisterEquippedExtensions(c)
		if _, err := d.EquippedExtensionAction(&s, c, entry.ID, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(entry.ID, "unequipped", err)
		}
		s.Actors[0].ItemDefinitionIDs = []string{entry.DefinitionID}
		s.Actors[0].MP = 100
		if _, err := d.EquippedExtensionAction(&s, c, "extension-244", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal("foreign identity", err)
		}
	}
	var d BattleDefinitions
	s, base, _ = dualAgilityFixture()
	c = &corebattle.EquippedExtensionContext{DamageContext: base.DamageContext}
	d.RegisterEquippedExtensions(c)
	s.Actors[0].ItemDefinitionIDs = []string{"item-212"}
	s.Actors[0].MP = 100
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	entry, err := d.EquippedExtensionAction(&s, c, "extension-212", noArithmeticDraws{t})
	if err != nil || entry.MPCost != 5 {
		t.Fatal(entry, err)
	}
	ops, err := d.InvocationAction(&s, &f, []string{"extension-212"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil {
		t.Fatal(err)
	}
	if a := s.Actors[0]; a.Extended != corebattle.ExtendedInterception || a.ExtendedTurns != 7 || a.MP != 100 {
		t.Fatal(a)
	}
}

// Tool248 reaches its body through the ordinary registration and admission.
func TestProjectileToolRegistration(t *testing.T) {
	s, base, _ := dualAgilityFixture()
	level := 40
	base.RequesterLevel = &level
	base.GuaranteedHit = true
	c := &corebattle.OrdinaryToolContext{DamageContext: base.DamageContext, Death: &corebattle.DefeatContext{Rewards: &corebattle.DefeatRewards{}}}
	var d BattleDefinitions
	d.RegisterOrdinaryTools(c)
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
	ops, err := d.InvocationAction(&s, &f, []string{"tool-248"}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	rng := &damageGemDraws{t: t, draws: []float64{0, 0}}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != 2 {
		t.Fatal(err, rng.used)
	}
	if len(c.Results) != 1 || !c.Results[0].Consume || c.Results[0].ID != "tool-248" {
		t.Fatal(c.Results)
	}
}

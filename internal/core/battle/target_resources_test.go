package battle

import "testing"

// One family-installed hook must reach both damage and status primitives:
// seal_kotowari converts each physical helper to magic with a 3 MP debit.
func TestOwnTargetResourcesReachesDamageAndStatusPrimitives(t *testing.T) {
	s, tc := targetFixture()
	s.Actors, tc.EnemyIDs = s.Actors[:3], nil
	s.Actors[0].MP, s.Actors[0].Abilities = 10, []string{"seal_kotowari"}
	rng := &targetRNG{t: t}
	d := DamageContext{EffectContext: EffectContext{TargetContext: tc}}
	restore := s.ownTargetResources(&d.EffectContext)
	if _, err := s.ApplyDamage(&d, TargetEffect{Category: CategoryPhysical, Payload: CallbackPayload{Number: 100}}, rng); err != nil || s.Actors[0].MP != 7 {
		t.Fatal(s.Actors[0].MP, err)
	}
	st := StatusContext{EffectContext: d.EffectContext}
	st.symbolicConversion = true // as status entries do
	if _, err := s.ApplyStatus(&st, TargetEffect{Category: CategoryPhysical, Payload: CallbackPayload{Symbol: StatusDofuu}}, 100, false, false, rng); err != nil || s.Actors[0].MP != 4 {
		t.Fatal(s.Actors[0].MP, err)
	}
	restore()
	if d.targetResources != nil {
		t.Fatal("hook not restored")
	}
	rng.done()
}

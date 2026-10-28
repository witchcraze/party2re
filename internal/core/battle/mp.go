package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ApplyMPHealing preserves _mp_h's requested recovery, exclusions and post-cap
// sanctuary penalty. HP and the enclosing callback's resource cost are separate.
func (s *LifetimeState) ApplyMPHealing(c *EffectContext, e TargetEffect, rng random.Generator) (EffectApplication, error) {
	r := EffectApplication{}
	if err := s.validateRecovery(c, e, rng); err != nil {
		return r, err
	}
	target, err := s.ResolvePartyTarget(c.TargetContext, e, rng)
	if err == nil && c.targetResources != nil {
		c.targetResources(target)
	}
	r.TargetResolution = target
	if err != nil || !target.Resolved {
		return r, err
	}
	t, a := s.actor(target.TargetID), s.actor(c.ActorID)
	if t.HP <= 0 || target.Payload.Number == 0 {
		return r, nil
	}
	v, _, err := s.ModifyEffect(c, target.TargetEffect, ModifierMPHealing, "", rng)
	if err != nil || v == 0 {
		return r, err
	}
	c.MPHealCalls++
	if t.Temporary == TemporaryRage {
		t.Temporary = ""
	}
	r.Amount = int(v*(.9+rng.Float64()*.3)*a.Tension) + 1
	if hasItem(t.ItemDefinitionIDs, "item-189") || hasItem(t.ItemDefinitionIDs, "item-202") || (hasItem(t.ItemDefinitionIDs, "item-198") && a.MP > int(float64(a.MaxMP)*.5)) {
		r.Amount = 0
	}
	a.Tension = 1
	before := t.MP
	total := before + r.Amount
	t.MP = min(t.MaxMP, total)
	if s.Fields.Current() == "sanctuary" && target.Category == CategoryMagic && total > t.MaxMP {
		t.MP = max(1, t.MP-(total-t.MaxMP))
	}
	r.Returned, r.Delta, r.Applied = true, t.MP-before, t.MP != before
	return r, nil
}

func ordinaryToolMP(id string) float64 {
	switch id {
	case "tool-11":
		return 40
	case "tool-12":
		return 100
	case "tool-13":
		return 999
	case "tool-101":
		return 50
	}
	return 0
}

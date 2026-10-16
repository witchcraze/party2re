package battle

import "github.com/witchcraze/party2re/internal/core/random"

func (s *LifetimeState) validateRecovery(c *EffectContext, e TargetEffect, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if err := e.validate(); err != nil || e.Payload.Symbol != "" || e.Payload.Number < 0 {
		return ErrUnsupportedEffect
	}
	return nil
}

func (s *LifetimeState) healingBasePower(c *EffectContext, e TargetEffect) float64 {
	a, t := s.actor(c.ActorID), s.actor(e.TargetID)
	v, original := e.Payload.Number, e.Payload.Number
	if t.Temporary == TemporaryDouble {
		original *= .5
	}
	if hasItem(a.ItemDefinitionIDs, "item-146") {
		v *= 1.8
	}
	if hasItem(a.ItemDefinitionIDs, "item-256") {
		v *= 1.2
	}
	if hasItem(a.ItemDefinitionIDs, "item-148") && e.Category == CategorySong && original < 50 {
		v *= 10
	}
	if s.SpecialBossField && c.DamageCalls > 0 && !(s.Fields.Current() == "sanctuary" && e.Category == CategoryMagic) {
		v = min(v*.5, float64(t.MaxHP)*.5)
	}
	return v
}

// ApplyHealing implements _heal:9067–9135, with the already recorded operation
// counters, actor tension, one-time splash and post-cap sanctuary overflow.
func (s *LifetimeState) ApplyHealing(c *EffectContext, e TargetEffect, rng random.Generator) (EffectApplication, error) {
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
	t := s.actor(target.TargetID)
	v := target.Payload.Number
	if t.HP <= 0 || v == 0 {
		return r, nil
	}
	c.HealCalls++
	a := s.actor(c.ActorID)
	v = s.healingBasePower(c, target.TargetEffect)
	sanctuary := s.Fields.Current() == "sanctuary" && target.Category == CategoryMagic
	modified := target.TargetEffect
	modified.Payload.Number = v
	v, _, err = s.ModifyEffect(c, modified, ModifierHPHealing, "", rng)
	if err != nil || v == 0 {
		return r, err
	}
	if t.Temporary == TemporaryRage {
		t.Temporary = ""
	}
	r.Returned = true
	r.Amount = int(v * (.9 + rng.Float64()*.3) * a.Tension)
	a.Tension = 1
	before := t.HP
	total := before + r.Amount
	t.HP = min(t.MaxHP, total)
	if hasItem(a.ItemDefinitionIDs, "item-256") && c.PartyHealCalls == 0 && c.DamageCalls == 0 {
		c.PartyHealCalls++
		amount := min(r.Amount/2, int(float64(t.MaxHP)*.3))
		for _, id := range c.PartyIDs {
			other := s.actor(id)
			if id == t.ID || other.HP <= 0 {
				continue
			}
			old := other.HP
			other.HP = min(other.MaxHP, other.HP+amount)
			r.IncidentalHealing = append(r.IncidentalHealing, HealingFact{TargetID: id, Amount: amount, Delta: other.HP - old})
		}
	}
	if sanctuary && total > t.MaxHP {
		t.HP = max(1, t.HP-(total-t.MaxHP))
	}
	r.Delta = t.HP - before
	r.Applied = r.Delta != 0
	return r, nil
}

func (s *LifetimeState) ApplyPartyHealing(c *EffectContext, power float64, category CallbackCategory, rng random.Generator) ([]EffectApplication, error) {
	e := TargetEffect{Category: category, Payload: CallbackPayload{Number: power}}
	if err := s.validateRecovery(c, e, rng); err != nil {
		return nil, err
	}
	for _, id := range c.PartyIDs {
		e.TargetID = id
		if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
			return nil, err
		}
	}
	allowed, err := s.actor(c.ActorID).CheckCategorySeal(category, rng)
	if err != nil || !allowed {
		return nil, err
	}
	c.PartyHealCalls++
	if s.actor(c.ActorID).Extended == ExtendedAdditional && category == CategoryMagic {
		power *= 1.5
	}
	var results []EffectApplication
	for _, id := range c.PartyIDs {
		e.TargetID, e.Payload.Number = id, power
		r, err := s.ApplyHealing(c, e, rng)
		if err != nil {
			return results, err
		}
		results = append(results, r)
	}
	return results, nil
}

// ApplyToolHealing preserves optional tuple assignment, including absent
// payload after cover. Unknown temporaries have no hook and retain both values.
// There is no post-hook living guard, category check, tension or overflow.
func (s *LifetimeState) ApplyToolHealing(c TargetContext, targetID string, power float64, useTemporary bool, rng random.Generator) (EffectApplication, error) {
	e := TargetEffect{TargetID: targetID, Payload: CallbackPayload{Number: power}}
	r := EffectApplication{}
	if err := s.validateRecovery(&EffectContext{TargetContext: c}, e, rng); err != nil {
		return r, err
	}
	t := s.actor(targetID)
	if t == nil {
		return r, ErrInvalidParticipant
	}
	if t.HP <= 0 {
		return r, nil
	}
	if useTemporary {
		switch t.Temporary {
		case TemporaryCover:
			found := false
			for _, a := range s.Actors {
				if a.TeamID == t.TeamID && a.Temporary == TemporaryCovering {
					found = true
					break
				}
			}
			if !found {
				return r, ErrUnsupportedEffect
			}
		case TemporaryReduceAttack, TemporaryReduceMagic, TemporaryReduceBreath,
			TemporaryReflectAttack, TemporaryReflectMagic, TemporaryReflectBreath, TemporaryReflectDance,
			TemporaryDeflect, TemporaryAbsorbMagic, TemporaryNullAttack, TemporaryNullMagic:
			// The registered untyped hook returns an absent pair, addressing an
			// unrepresented empty record. Reject instead of retaining spell values.
			return r, ErrUnsupportedEffect
		}
		tmp, err := s.ResolveTemporary(c, e, rng)
		if err != nil {
			return r, err
		}
		if tmp.TargetID != nil {
			targetID = *tmp.TargetID
		}
		if tmp.Payload != nil {
			power = tmp.Payload.Number
		} else if t.Temporary == TemporaryCover {
			power = 0
		}
		t = s.actor(targetID)
	}
	r.TargetResolution = TargetResolution{TargetEffect: TargetEffect{TargetID: targetID, Payload: CallbackPayload{Number: power}}, Resolved: true}
	r.Returned = true
	r.Amount = int(power * (.9 + rng.Float64()*.3))
	before := t.HP
	t.HP = min(t.MaxHP, t.HP+r.Amount)
	r.Delta, r.Applied = t.HP-before, t.HP != before
	return r, nil
}

package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ApplyToolDamage has no spell target/category/tension/modifier/counter hooks.
// Interception uses the current actor's items and a nonlethal direct tool hit.
func (s *LifetimeState) ApplyToolDamage(c *DamageContext, targetID string, power float64, direct, nonlethal bool, rng random.Generator) (EffectApplication, error) {
	r := EffectApplication{}
	if c == nil {
		return r, ErrInvalidRequest
	}
	if err := s.validateRecovery(&c.EffectContext, TargetEffect{Payload: CallbackPayload{Number: power}}, rng); err != nil {
		return r, err
	}
	t := s.actor(targetID)
	if t == nil {
		return r, ErrInvalidParticipant
	}
	if t.ConsumeOneHitDefense() {
		r.Returned = true
		return r, nil
	}
	if power == 0 && t.HP > 0 {
		return r, nil
	}
	if err := c.ToolCategory.validate(); err != nil {
		return r, err
	}
	if t.Gear == nil || (!nonlethal && c.Defeat == nil) {
		return r, ErrUnsupportedEffect
	}
	// Neither absent nor job77's numeric package category equals physical.
	r.Amount = rollDamage(power, toolDefense(t), direct, t.BaseStats.Defense, rng)
	before := t.HP
	t.HP -= r.Amount
	if t.HP <= 0 {
		if nonlethal {
			t.HP = 1
		} else {
			t.HP = 0
			if err := c.Defeat(t.ID); err != nil {
				return r, err
			}
			t = s.actor(targetID)
		}
	}
	r.TargetResolution = TargetResolution{TargetEffect: TargetEffect{TargetID: targetID}, Resolved: true}
	r.Returned, r.Applied, r.Delta = true, true, t.HP-before
	return r, nil
}

func toolDefense(t *LifetimeActor) float64 {
	if hasItem(t.ItemDefinitionIDs, "item-162") {
		return (float64(t.Defense)*1.5 + float64(t.Gear.ArmorDefense)) * .3
	}
	return float64(t.Defense+t.Gear.ArmorDefense) * .3
}

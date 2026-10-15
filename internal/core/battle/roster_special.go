package battle

import "github.com/witchcraze/party2re/internal/core/random"

func (s *LifetimeState) validateCallingSpecial(c *RosterContext, job int, helperID, targetID string, rng random.Generator) error {
	h := s.actor(helperID)
	if job == 68 || job == 69 {
		form := 2
		if h.RosterIdentity == callingIdentity(job) {
			form = h.SummonForm
		}
		switch form {
		case 0:
			for _, power := range []float64{float64(s.actor(c.ActorID).Attack) * 3, 20} {
				if err := s.ValidateDamage(&c.DamageContext, rosterEffect(targetID, power, CategoryPhysical, false), rng); err != nil {
					return err
				}
			}
			return nil
		case 1:
			return s.ValidateDamage(&c.DamageContext, rosterEffect(targetID, 220, CategoryMagic, true), rng)
		default:
			return s.validatePartyTarget(c.TargetContext, rosterEffect(targetID, 999, CategoryMagic, false), rng)
		}
	}
	if job != 59 && job != 60 && job != 65 {
		// Death/status callbacks (#1365), MP drain (#1337), enemy possession
		// and full random closure (#1357) are not generic damage substitutes.
		return ErrUnsupportedEffect
	}
	coefficients := [2]float64{1.5, 1.2}
	if job == 60 {
		coefficients = [2]float64{1.7, 1.5}
	}
	for i, coefficient := range coefficients {
		if err := s.ValidateDamage(&c.DamageContext, rosterEffect(targetID, float64(h.Attack)*coefficient, CategoryPhysical, i == 1), rng); err != nil {
			return err
		}
	}
	if job == 65 {
		return s.ValidateDamage(&c.DamageContext, rosterEffect(targetID, float64(h.Attack)*2, CategoryPhysical, false), rng)
	}
	power := 220.0
	if job == 60 {
		power = 230
	}
	return s.validateAreaDamage(&c.DamageContext, rosterEffect("", power, CategoryBreath, true), rng)
}

func (s *LifetimeState) applyCallingSpecial(c *RosterContext, job int, helperID, targetID string, rng random.Generator) (RosterResult, error) {
	r := RosterResult{HelperID: helperID, TargetConsumed: true}
	h := s.actor(helperID)
	e := rosterEffect(targetID, 0, CategoryPhysical, false)
	if job == 68 || job == 69 {
		form := 2
		if h.RosterIdentity == callingIdentity(job) {
			form = h.SummonForm
		}
		switch form {
		case 0:
			e.Payload.Number = 20
			if rng.Float64()*2 < 1 {
				e.Payload.Number = float64(s.actor(c.ActorID).Attack) * 3
			}
		case 1:
			e = rosterEffect(targetID, 220, CategoryMagic, true)
		default:
			heal, err := s.ApplyHealing(&c.EffectContext, rosterEffect(targetID, 999, CategoryMagic, false), rng)
			return RosterResult{Applied: heal.Applied, HelperID: helperID, TargetConsumed: true, Effects: []EffectApplication{heal}}, err
		}
	} else {
		first, second := 1.5, 1.2
		if job == 60 {
			first, second = 1.7, 1.5
		}
		if rng.Float64()*2 < 1 {
			e.Payload.Number = float64(h.Attack) * first
		} else if rng.Float64()*2 < 1 {
			e = rosterEffect(targetID, float64(h.Attack)*second, CategoryPhysical, true)
		} else if job == 65 {
			e.Payload.Number = float64(h.Attack) * 2
		} else {
			power := 220.0
			if job == 60 {
				power = 230
			}
			effects, err := s.ApplyAreaDamage(&c.DamageContext, power, CategoryBreath, true, rng)
			return RosterResult{Applied: len(effects) > 0, HelperID: helperID, Effects: effects}, err
		}
	}
	hit, err := s.ApplyDamage(&c.DamageContext, e, rng)
	r.Effects, r.Applied = []EffectApplication{hit}, hit.Applied
	return r, err
}

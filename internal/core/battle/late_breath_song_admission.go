package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateLateBreathSong admits descendants on private state without hooks or
// RNG. Raw chain/cure bodies intentionally bypass shared status immunities.
func (s *LifetimeState) ValidateLateBreathSong(c *LateBreathSongContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = append([]string(nil), c.Arguments...)
	a := p.actor(c.ActorID)
	switch id {
	case "job-84-entry-0", "job-85-entry-2":
		return nil
	case "job-84-entry-1", "job-84-entry-2", "job-86-entry-4":
		if id == "job-86-entry-4" {
			book, ok := c.HeldBooks[c.ActorID]
			if !ok || book != "none" && book != "ordinary" && book != "forbidden" || a.MaxMP < 0 || a.MaxMP > 1e12 {
				return ErrUnsupportedEffect
			}
		}
		return p.ValidateDamage(&child.DamageContext, p.lateSongDamage(&child, id, child.shift()), rng)
	case "job-84-entry-3":
		// A successful dance cure occurs before the untyped party helper and
		// can enable item195 confusion, including nonparty tension recipients.
		if a.Status == StatusDanceSeal {
			a.Status = ""
		}
		e := controlSymbol(child.shift(), "passion", CategoryUntyped)
		if err := p.validatePartyTarget(child.TargetContext, e, rng); err != nil {
			return err
		}
		if _, err := p.gemTensionCeiling(&child.EffectContext, a, 1); err != nil {
			return err
		}
		for _, candidate := range p.partyTargetCandidates(child.TargetContext, e, nil) {
			if candidate.TargetID != a.ID {
				if _, err := p.gemTensionCeiling(&child.EffectContext, p.actor(candidate.TargetID), 1); err != nil {
					return err
				}
			}
		}
	case "job-85-entry-0":
		if a.MaxHP < 0 || a.MaxHP > 1e12 || a.HP < 0 || a.HP > 1e12 {
			return ErrUnsupportedEffect
		}
		// Sacrifice can revive a fallen caster to1 before the MP target helper.
		a.HP = max(1, a.HP-int(float64(a.MaxHP)*.36))
		return p.validatePartyTarget(child.TargetContext, controlEffect(a.ID, 1, CategoryUntyped), rng)
	case "job-85-entry-1":
		if c.Death == nil {
			return ErrUnsupportedEffect
		}
		death := *c.Death
		death.EffectContext = child.EffectContext
		return p.ValidateAreaStatus(&death, controlSymbol("", "instant_death", CategorySong), 18, false, true, rng)
	case "job-85-entry-3":
		_, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), StatusSabaku, CategorySong), false)
		return err
	case "job-85-entry-4":
		targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), StatusSabaku, CategorySong), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if err := p.ValidateDamage(&child.DamageContext, p.lateSongDamage(&child, id, target), rng); err != nil {
				return err
			}
		}
	default:
		return ErrUnsupportedEffect
	}
	return nil
}

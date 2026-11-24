package battle

import (
	"strconv"

	"github.com/witchcraze/party2re/internal/core/random"
)

// NPCJob406Definition binds the stage23 EX boss body to the response's damage
// and defeat ports (_skill.cgi:8247–8283).
func NPCJob406Definition(id string) CallbackDefinition {
	return CallbackDefinition{
		Validate: func(s *LifetimeState, f *CallbackContext) error {
			return s.validateNPC406(f, id, random.Default())
		},
		Execute: func(s *LifetimeState, f *CallbackContext, rng random.Generator) error {
			return s.applyNPC406(f, id, rng)
		},
	}
}

const npc406Snowman = "assets/images/placeholders/battle/snowman.svg#"

func npc406Pick(target string) TargetEffect {
	return controlSymbol(target, "snowfall", CategoryUntyped)
}

// npc406Target is the body's own target check. Later helpers receive its pick,
// or no target when it fell or was blocked, and resolve it again.
func (s *LifetimeState) npc406Target(c *DamageContext, target string, rng random.Generator) (string, error) {
	r, err := s.ResolveEnemyTarget(c.TargetContext, npc406Pick(target), rng)
	if err != nil {
		return "", err
	}
	if c.targetResources != nil {
		c.targetResources(r)
	}
	if !r.Resolved {
		return "", nil
	}
	return r.TargetID, nil
}

// npc406Spread is the barrage's rand(MaxHP×0.2) range for a pick. An absent
// pick reads rand(0), which Perl treats as rand(1).
func npc406Spread(s *LifetimeState, id string) float64 {
	if t := s.actor(id); t != nil && t.MaxHP != 0 {
		return float64(t.MaxHP) * .2
	}
	return 1
}

func (s *LifetimeState) validateNPC406(f *CallbackContext, id string, rng random.Generator) error {
	if f == nil || f.Damage == nil || !f.Damage.ActorNPC {
		return ErrUnsupportedEffect
	}
	if err := s.validateTargetContext(f.TargetContext, rng); err != nil {
		return err
	}
	a := s.actor(f.ActorID)
	if !a.NPC {
		return ErrUnsupportedEffect
	}
	damage := *f.Damage
	damage.TargetContext = f.TargetContext
	seal := func(p *LifetimeState, c DamageContext, target string, chance float64) error {
		status := StatusContext{EffectContext: c.EffectContext, Defeat: f.Defeat}
		return p.ValidateStatus(&status, controlSymbol(target, StatusDofuu, CategoryPhysical), chance, false, false, rng)
	}
	switch id {
	case "npc-job-406-position-0":
		targets, err := s.controlTargets(&ControlClimaxContext{DamageContext: damage}, npc406Pick(npc408Target(f)), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if err := seal(s, damage, target, 100); err != nil {
				return err
			}
			if err := s.ValidateStatDown(&damage.EffectContext, controlEffect(target, .2, CategoryPhysical), StatAgility, rng); err != nil {
				return err
			}
		}
	case "npc-job-406-position-1":
		p := s.callbackCopy()
		damage = p.nativeDamageContext(damage, f.Defeat, rng)
		return p.admitNPC405Area(&damage, f.Defeat, dualAgilityEffect("", float64(a.Defense), CategoryPhysical, false), rng)
	case "npc-job-406-position-2":
		damage.AdditionalEffect = true
		p := s.callbackCopy()
		damage = p.nativeDamageContext(damage, f.Defeat, rng)
		for range 6 {
			if p.actor(f.ActorID).HP <= 0 {
				return nil
			}
			targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: damage}, npc406Pick(""), false)
			if err != nil {
				return err
			}
			spread := 0.0
			for _, target := range targets {
				spread = max(spread, npc406Spread(&p, target))
			}
			// An unspecified hit covers every pick's re-resolution.
			if err := p.admitNPC405Hit(&damage, f.Defeat, dualAgilityEffect("", 5, CategoryPhysical, true), 5+spread, rng); err != nil {
				return err
			}
			for _, target := range targets {
				if err := seal(&p, damage, target, 15); err != nil {
					return err
				}
			}
		}
	default:
		return ErrUnsupportedEffect
	}
	return nil
}

func (s *LifetimeState) applyNPC406(f *CallbackContext, id string, rng random.Generator) error {
	damage := f.Damage
	damage.TargetContext = f.TargetContext
	previous := *damage
	native := s.nativeDamageContext(*damage, f.Defeat, rng)
	damage.Reset, damage.Defeat, damage.responseDefeat = native.Reset, native.Defeat, f.Defeat
	resources := func(r TargetResolution) {
		a := s.actor(f.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	damage.targetResources, damage.EffectContext.targetResources = resources, resources
	defer func() {
		damage.Reset, damage.Defeat, damage.responseDefeat = previous.Reset, previous.Defeat, previous.responseDefeat
		damage.targetResources, damage.EffectContext.targetResources = previous.targetResources, previous.EffectContext.targetResources
		// The add-effect flag lasts one NPC action (_npc_action.cgi:101).
		damage.AdditionalEffect = previous.AdditionalEffect
	}()
	seal := func(target string, chance float64) error {
		status := StatusContext{EffectContext: damage.EffectContext, Defeat: f.Defeat}
		_, err := s.ApplyStatus(&status, controlSymbol(target, StatusDofuu, CategoryPhysical), chance, false, false, rng)
		damage.EffectContext = status.EffectContext
		return err
	}
	switch id {
	case "npc-job-406-position-0":
		target, err := s.npc406Target(damage, npc408Target(f), rng)
		if err != nil || rng.Float64()*10 >= 7 {
			return err
		}
		if err := seal(target, 100); err != nil {
			return err
		}
		if _, err := s.ApplyStatDown(&damage.EffectContext, controlEffect(target, .2, CategoryPhysical), StatAgility, rng); err != nil {
			return err
		}
		form := 3
		if rng.Float64()*2 < 1 {
			form = 1
		} else if rng.Float64()*2 < 1 {
			form = 2
		}
		// The form is written even when the seal or slow had no effect.
		if t := s.actor(target); t != nil {
			t.Icon = npc406Snowman + strconv.Itoa(form)
		}
	case "npc-job-406-position-1":
		_, err := s.ApplyAreaDamage(damage, float64(s.actor(f.ActorID).Defense), CategoryPhysical, false, rng)
		return err
	case "npc-job-406-position-2":
		damage.AdditionalEffect = true
		hits := int(rng.Float64()*4 + 3)
		for range hits {
			if s.actor(f.ActorID).HP <= 0 {
				break
			}
			target, err := s.npc406Target(damage, "", rng)
			if err != nil {
				return err
			}
			if _, err := s.ApplyDamage(damage, dualAgilityEffect(target, 5+rng.Float64()*npc406Spread(s, target), CategoryPhysical, true), rng); err != nil {
				return err
			}
			if err := seal(target, 15); err != nil {
				return err
			}
		}
	}
	return nil
}

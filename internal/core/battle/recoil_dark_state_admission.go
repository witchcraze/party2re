package battle

import (
	"math"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateRecoilDarkState admits descendants without committed draws, external
// Apply hooks, argument consumption, or changes to caller-owned nested state.
func (s *LifetimeState) ValidateRecoilDarkState(c *RecoilDarkStateContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	a := p.actor(c.ActorID)
	switch id {
	case "job-21-entry-1", "job-38-entry-2":
		return nil
	case "job-21-entry-2", "job-22-entry-2", "job-38-entry-4":
		symbol, category, chance := recoilDarkStatus(id)
		return p.ValidateAreaStatus(&StatusContext{EffectContext: child.EffectContext}, controlSymbol("", symbol, category), chance, false, false, rng)
	case "job-21-entry-5":
		return p.validateRawRecoilStrike(&child, rng)
	case "job-21-entry-6":
		_, err := p.gemTensionCeiling(&child.EffectContext, a, 2)
		return err
	case "job-22-entry-1", "job-38-entry-3":
		a.Defense -= int(float64(a.Defense) * .5)
		return p.ValidateStatUp(&child.EffectContext, controlEffect(c.ActorID, 1, CategoryUntyped), StatAttack, rng)
	case "job-38-entry-1":
		if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(child.shift(), .2, CategoryMagic), rng); err != nil {
			return err
		}
		return p.validatePartyTarget(child.TargetContext, controlEffect(c.ActorID, 1, CategoryMagic), rng)
	case "job-38-entry-5":
		return p.validateCappedLifeDrain(&child, rng)
	case "job-21-entry-0", "job-21-entry-3", "job-21-entry-4", "job-22-entry-0", "job-22-entry-3", "job-22-entry-4", "job-38-entry-0":
		e := recoilDarkStrike(a, id, child.shift())
		if err := p.ValidateDamage(&child.DamageContext, e, rng); err != nil {
			return err
		}
		// Magic self-healing has no external descendants. Its explicit target
		// remains represented even after defeat/revival, confusion and redirects.
		if id == "job-38-entry-0" {
			return p.validatePartyTarget(child.TargetContext, controlEffect(c.ActorID, 1, CategoryMagic), rng)
		}
		if id == "job-22-entry-3" {
			return p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect("", .1, CategoryPhysical), rng)
		}
		return nil
	}
	return ErrUnsupportedEffect
}

func (s *LifetimeState) validateCappedLifeDrain(c *RecoilDarkStateContext, rng random.Generator) error {
	e, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, controlSymbol(c.shift(), "drain", CategoryMagic))
	if err != nil || len(c.EnemyIDs) == 0 {
		return err
	}
	// Only a successful first helper reaches damage. Its seal was cured; retain
	// each recipient/resource alternative separately for the second resolution.
	if s.actor(c.ActorID).categorySealed(e.Category) {
		s.actor(c.ActorID).Status = ""
	}
	candidates, err := s.statusTargetCandidates(&c.DamageContext, e)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		p, child := s.callbackCopy(), *c
		applyStatusResources(&p, candidate)
		power := float64(min(300, p.actor(candidate.TargetID).HP/2))
		if err := p.ValidateDamage(&child.DamageContext, dualAgilityEffect(candidate.TargetID, power, CategoryMagic, true), rng); err != nil {
			return err
		}
	}
	return s.validatePartyTarget(c.TargetContext, controlEffect(c.ActorID, 1, CategoryMagic), rng)
}

func (s *LifetimeState) validateRawRecoilStrike(c *RecoilDarkStateContext, rng random.Generator) error {
	targets := append(slices.Clone(c.PartyIDs), c.EnemyIDs...)
	if len(c.EnemyIDs) == 0 {
		targets = append(targets, c.ActorID)
	}
	for _, id := range targets {
		p := s.callbackCopy()
		a, t := p.actor(c.ActorID), p.actor(id)
		if t.HP <= 0 {
			t = a
		}
		base := float64(a.Attack)*3 - float64(t.Defense)*.4
		maximum := int(base * ((.9 + .3*math.Nextafter(1, 0)) * a.Tension))
		if int(base*(.9*a.Tension)) < 1 {
			maximum = max(2, maximum)
		}
		if t.HP > maximum {
			continue
		}
		if c.RawDefeat == nil {
			return ErrUnsupportedEffect
		}
		a.Tension, t.HP = 1, 0
		defeat := *c.RawDefeat
		defeat.TargetContext = c.TargetContext
		if err := p.ValidateDefeat(&defeat, t.ID, rng); err != nil {
			return err
		}
	}
	return nil
}

// Only exact inline transitions are projected across an enclosing invocation.
// Unknown damage/random results retain the existing AfterEffects gate.
func (s *LifetimeState) ProjectRecoilDarkState(c *RecoilDarkStateContext, id string) (bool, error) {
	if err := s.ValidateRecoilDarkState(c, id, random.Default()); err != nil {
		return false, err
	}
	switch id {
	case "job-21-entry-1":
		s.actor(c.ActorID).Temporary = TemporaryDeflect
		return true, nil
	case "job-38-entry-2":
		if !s.actor(c.ActorID).categorySealed(CategoryMagic) {
			s.actor(c.ActorID).Temporary = TemporaryNullMagic
			return true, nil
		}
	}
	return false, nil
}

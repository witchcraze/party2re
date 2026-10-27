package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateNPCJob97 shares whole-body admission with discovery, retaining ordered
// defense consumption and the distinct target resolutions of later helpers.
func (s *LifetimeState) ValidateNPCJob97(c *NPCJob97Context, id string, rng random.Generator) error {
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
	case "npc-job-97-position-0", "npc-job-97-position-1":
		if a.HP <= 0 {
			return nil
		}
		coefficient, hits := .85, 4
		if id == "npc-job-97-position-1" {
			coefficient, hits = .9, 5
		}
		return p.validateRepeatedDamage(&child.DamageContext, dualAgilityEffect("", float64(a.Attack)*coefficient, CategoryPhysical, false), hits, rng)
	case "npc-job-97-position-2":
		return p.ValidateStatusEntry(&StatusContext{EffectContext: child.EffectContext}, child.EquipmentReset, "job-35-entry-1", "", rng)
	case "npc-job-97-position-3":
		a.Defense -= int(float64(a.Defense) * .5)
		return p.ValidateStatUp(&child.EffectContext, controlEffect(c.ActorID, 1, CategoryUntyped), StatAttack, rng)
	case "npc-job-97-position-4":
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 300, CategoryPhysical, true), rng)
	case "npc-job-97-position-5":
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), float64(a.Attack)*2.5, CategoryPhysical, false), rng)
	case "npc-job-97-position-6":
		a.Temporary = TemporaryDouble
		_, err := p.gemTensionCeiling(&child.EffectContext, a, 2)
		return err
	case "npc-job-97-position-7":
		return p.validateAreaDamage(&child.DamageContext, dualAgilityEffect("", 400, CategoryBreath, true), rng)
	case "npc-job-97-position-8", "npc-job-97-position-10":
		power, chance, status := 250.0, 20.0, StatusParalyze
		if id == "npc-job-97-position-10" {
			power, chance, status = 260, 30, StatusSleep
		}
		child.AdditionalEffect = true
		return p.validateAreaDamageStatus(&child.DamageContext, power, status, chance, rng)
	case "npc-job-97-position-9":
		targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), "life-reduction", CategoryMagic), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if target == "" {
				continue
			}
			t := p.actor(target)
			if t.MaxHP <= 999 && t.BaseStats.Defense <= 999 {
				if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .75, CategoryMagic), rng); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return ErrUnsupportedEffect
}

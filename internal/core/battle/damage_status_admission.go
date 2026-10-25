package battle

import "github.com/witchcraze/party2re/internal/core/random"

// Admit the status tail after ordered area damage, including consumed defenses,
// seal cures, item195 activation and crossing current-HP immunity.
func (s *LifetimeState) validateAreaDamageStatus(c *DamageContext, power float64, status string, chance float64, rng random.Generator) error {
	p, child := s.callbackCopy(), *c
	a := p.actor(c.ActorID)
	e := dualAgilityEffect("", power, CategoryMagic, true)
	if err := p.validateAreaDamage(&child, e, rng); err != nil {
		return err
	}
	// A guaranteed first helper count suppresses item239's status tail even
	// when one-hit defense blocks damage. Keep the skipped-area alternative.
	counted := a.HP > 0 && !a.categorySealed(CategoryMagic) && len(child.EnemyIDs) > 0
	if counted {
		first := e
		first.TargetID = child.EnemyIDs[0]
		converted, _, err := a.ConvertEnemyCategory(child.TargetContext, first)
		if err != nil {
			return err
		}
		candidates := p.damageCandidates(&child, converted)
		counted = len(candidates) > 0
		for _, candidate := range candidates {
			counted = counted && candidate.Payload.Number != 0
		}
	}
	if !counted {
		if err := p.validateControlStatuses(&ControlClimaxContext{DamageContext: child}, []string{status}, CategoryMagic, chance, rng); err != nil {
			return err
		}
	}
	if a.HP <= 0 {
		return nil
	}
	if a.categorySealed(CategoryMagic) {
		a.Status = ""
	}
	child.AreaDamageCalls++
	if counted {
		child.DamageCalls++
	}
	for _, target := range child.EnemyIDs {
		e.TargetID = target
		converted, _, err := a.ConvertEnemyCategory(child.TargetContext, e)
		if err != nil {
			return err
		}
		var lowered []string
		for _, candidate := range p.damageCandidates(&child, converted) {
			t := p.actor(candidate.TargetID)
			if candidate.Payload.Number != 0 && t.Temporary != TemporaryOneHitDefense && damageHasPower(a, t, candidate) {
				lowered = append(lowered, t.ID)
			}
		}
		if err := p.admitDamageSequence(&child, []TargetEffect{e}, rng); err != nil {
			return err
		}
		for _, id := range lowered {
			if p.actor(id).HP > 999 {
				p.actor(id).HP = 999
			}
		}
	}
	// A positive hit can cross current-HP status immunity. Its gear and
	// defeat/reset descendants were already admitted by the damage owner.
	return p.validateControlStatuses(&ControlClimaxContext{DamageContext: child}, []string{status}, CategoryMagic, chance, rng)
}

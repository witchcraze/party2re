package battle

import (
	"math"

	"github.com/witchcraze/party2re/internal/core/random"
)

func (s *LifetimeState) applyNamedDamageTail(c *DamageContext, category CallbackCategory, power float64, rng random.Generator) error {
	if s.Fields.Current() == "cosmic" && category == CategoryMagic && c.DamageCalls > 1 && c.AreaDamageCalls == 0 {
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol("", "cosmic", CategoryMagic), rng)
		if err != nil {
			return err
		}
		if c.targetResources != nil {
			c.targetResources(r)
		}
		if r.Resolved && s.actor(r.TargetID).HP > 0 {
			if _, err := s.ApplyToolDamage(c, r.TargetID, power*.9, true, false, rng); err != nil {
				return err
			}
		}
	}
	if s.Fields.Current() == "antlion" && s.actor(c.ActorID).HP > 0 {
		a := s.actor(c.ActorID)
		if rng.Float64()*3 < 1 {
			a.Status = StatusDofuu
		}
		if rng.Float64()*2 < 1 {
			a.Defense = max(int(float64(a.BaseStats.Defense)*.2), a.Defense-int(float64(a.BaseStats.Defense)*.3))
		}
	}
	return nil
}

func (s *LifetimeState) projectNamedDamageTail(c DamageContext, defeat *DefeatContext, category CallbackCategory, low, high float64, followed bool, rng random.Generator) ([]LifetimeState, error) {
	states := []LifetimeState{s.callbackCopy()}
	if s.Fields.Current() == "cosmic" && category == CategoryMagic && c.DamageCalls > 1 && c.AreaDamageCalls == 0 {
		e := controlSymbol("", "cosmic", CategoryMagic)
		candidates, err := s.statusTargetCandidates(&c, e)
		if err != nil {
			return nil, err
		}
		states = nil
		for _, candidate := range candidates {
			p := s.callbackCopy()
			applyStatusResources(&p, candidate)
			t := p.actor(candidate.TargetID)
			if t.HP <= 0 || t.ConsumeOneHitDefense() {
				states = appendDefeatState(states, p)
				continue
			}
			if err := c.ToolCategory.validate(); err != nil {
				return nil, err
			}
			if t.Gear == nil {
				return nil, ErrUnsupportedEffect
			}
			minimum := rollDamage(low*.9, toolDefense(t), true, t.BaseStats.Defense, resetBoundary(0))
			maximum := rollDamage(high*.9, toolDefense(t), true, t.BaseStats.Defense, resetBoundary(math.Nextafter(1, 0)))
			if low*.9*.9 < 1 || t.BaseStats.Defense > 999 && (low*.9*.5-toolDefense(t))*.9 < 1 {
				minimum, maximum = 1, max(2, maximum)
			}
			for _, hp := range damageSurvivorHPs(t.HP, minimum, maximum) {
				q := p.callbackCopy()
				q.actor(t.ID).HP = hp
				states = appendDefeatState(states, q)
			}
			if t.HP <= maximum {
				if defeat == nil {
					return nil, ErrUnsupportedEffect
				}
				t.HP = 0
				child := *defeat
				child.TargetContext = c.TargetContext
				if err := p.ValidateDefeat(&child, t.ID, rng); err != nil {
					return nil, err
				}
				variants, err := p.projectDefeatContinuation(&child, t.ID, rng, map[string]bool{}, followed)
				if err != nil {
					return nil, err
				}
				for _, v := range variants {
					states = appendDefeatState(states, v)
				}
			}
		}
		if len(candidates) == 0 {
			states = append(states, s.callbackCopy())
		}
	}
	if s.Fields.Current() == "antlion" {
		var variants []LifetimeState
		for _, state := range states {
			if state.actor(c.ActorID).HP <= 0 {
				variants = appendDefeatState(variants, state)
				continue
			}
			for mask := range 4 {
				p := state.callbackCopy()
				a := p.actor(c.ActorID)
				if mask&1 != 0 {
					a.Status = StatusDofuu
				}
				if mask&2 != 0 {
					a.Defense = max(int(float64(a.BaseStats.Defense)*.2), a.Defense-int(float64(a.BaseStats.Defense)*.3))
				}
				variants = appendDefeatState(variants, p)
			}
		}
		states = variants
	}
	return states, nil
}

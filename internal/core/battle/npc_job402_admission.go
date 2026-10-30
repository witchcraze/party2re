package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateNPCJob402 uses private native projections for ordered damage and
// defeat/reset descendants. Admission never calls external Apply hooks or RNG.
func (s *LifetimeState) ValidateNPCJob402(c *NPCJob402Context, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	switch id {
	case "npc-job-402-position-0":
		return p.validateNPC402Hits(&child, id, "", 3, rng)
	case "npc-job-402-position-1":
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
			for _, target := range child.PartyIDs {
				e := controlEffect(target, .5, CategoryMagic)
				if err := p.validateStatUp(&child.EffectContext, e, stat, rng, admission); err != nil {
					return err
				}
				p.recordStatUp205(&child.EffectContext, e, stat, &admission)
			}
			// Earlier helpers can cure the caster's magic seal.
			if p.actor(c.ActorID).categorySealed(CategoryMagic) {
				p.actor(c.ActorID).Status = ""
			}
		}
		return nil
	case "npc-job-402-position-2":
		return p.validateNPC402Hits(&child, id, "", 2, rng)
	case "npc-job-402-position-3", "npc-job-402-position-4":
		if id == "npc-job-402-position-4" {
			for _, target := range child.EnemyIDs {
				p.actor(target).Temporary = ""
			}
		}
		targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), "ordered-strike", CategoryUntyped), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if id == "npc-job-402-position-3" {
				v := npc402SpeedCount(p.actor(c.ActorID), p.actor(target))
				hits := v + 1
				if v == 9 {
					hits++
				}
				if err := p.validateNPC402Hits(&child, id, target, hits, rng); err != nil {
					return err
				}
			} else {
				power := 5.0
				if t := p.actor(target); t != nil {
					power += float64(t.Defense) * .8
				}
				if err := p.validateNPC402Area(&child, power, rng); err != nil {
					return err
				}
			}
		}
		return nil
	case "npc-job-402-position-5":
		power := p.npc402SharedPower(&child)
		e := dualAgilityEffect(child.shift(), power, CategoryUntyped, false)
		states, err := p.projectCallbackDamage(&child.DamageContext, child.Death, e, power, true, rng)
		if err != nil {
			return err
		}
		for _, next := range states {
			if err := next.State.validateControlDown(&ControlClimaxContext{DamageContext: next.Context}, controlEffect(next.TargetID, .2, CategoryUntyped), rng); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrUnsupportedEffect
}

func (s *LifetimeState) validateNPC402Hits(c *NPCJob402Context, id, target string, hits int, rng random.Generator) error {
	states := []damageContinuation{{State: s.callbackCopy(), Context: c.DamageContext}}
	for hit := range hits {
		argument := target
		if id != "npc-job-402-position-3" {
			argument = c.shift()
		}
		var next []damageContinuation
		for _, state := range states {
			a := state.State.actor(c.ActorID)
			e := dualAgilityEffect(argument, 110, CategoryMagic, true)
			if id == "npc-job-402-position-2" {
				e.Payload.Number = float64(a.Defense) * .6
				if hit == 1 {
					e = dualAgilityEffect(argument, float64(a.Attack)*1.2, CategoryPhysical, false)
				}
			} else if id == "npc-job-402-position-3" {
				e = dualAgilityEffect(argument, float64(a.Attack)*.4, CategoryPhysical, false)
				if hit == 10 {
					e = dualAgilityEffect(argument, float64(a.Attack), CategoryPhysical, true)
				}
			}
			continuations, err := state.State.projectCallbackDamage(&state.Context, c.Death, e, e.Payload.Number, hit+1 < hits, rng)
			if err != nil {
				return err
			}
			if hit+1 < hits {
				for _, continuation := range continuations {
					next = appendDamageContinuation(next, continuation.State, continuation.Context, "")
				}
			}
		}
		states = next
	}
	return nil
}

func (s *LifetimeState) validateNPC402Area(c *NPCJob402Context, power float64, rng random.Generator) error {
	e := dualAgilityEffect("", power, CategoryUntyped, true)
	decays := []bool{true}
	if s.canPreserveAreaPower(&c.DamageContext, e) {
		decays = []bool{false}
		// Different optional weapon rolls can straddle the item144 threshold.
		for mask := range 4 {
			bits := mask
			draw := func() float64 { v := float64(bits&1) * .5; bits >>= 1; return v }
			if s.modifyDamage(&c.DamageContext, nil, power, CategoryUntyped, true, false, draw) > 300 {
				decays = append(decays, true)
				break
			}
		}
	}
	for _, decay := range decays {
		context := c.DamageContext
		context.AreaDamageCalls++
		states := []damageContinuation{{State: s.callbackCopy(), Context: context}}
		value := power
		for index, target := range c.EnemyIDs {
			var next []damageContinuation
			for _, state := range states {
				if state.State.actor(c.ActorID).HP <= 0 {
					continue
				}
				e := dualAgilityEffect(target, value, CategoryUntyped, true)
				continuations, err := state.State.projectCallbackDamage(&state.Context, c.Death, e, value, index+1 < len(c.EnemyIDs), rng)
				if err != nil {
					return err
				}
				for _, continuation := range continuations {
					next = appendDamageContinuation(next, continuation.State, continuation.Context, "")
				}
			}
			states = next
			if decay {
				value *= .85
			}
		}
	}
	return nil
}

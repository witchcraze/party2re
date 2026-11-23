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
		return p.admitNPCPartyStats(&child.EffectContext, .5, rng)
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
	targets := make([]string, hits)
	for hit := range targets {
		targets[hit] = target
		if id != "npc-job-402-position-3" {
			targets[hit] = c.shift()
		}
	}
	return s.validateNPCHits(&c.DamageContext, c.Death, hits, func(a *LifetimeActor, hit int) TargetEffect {
		switch {
		case id == "npc-job-402-position-2" && hit == 1:
			return dualAgilityEffect(targets[hit], float64(a.Attack)*1.2, CategoryPhysical, false)
		case id == "npc-job-402-position-2":
			return dualAgilityEffect(targets[hit], float64(a.Defense)*.6, CategoryMagic, true)
		case id == "npc-job-402-position-3" && hit == 10:
			return dualAgilityEffect(targets[hit], float64(a.Attack), CategoryPhysical, true)
		case id == "npc-job-402-position-3":
			return dualAgilityEffect(targets[hit], float64(a.Attack)*.4, CategoryPhysical, false)
		}
		return dualAgilityEffect(targets[hit], 110, CategoryMagic, true)
	}, rng)
}

// validateNPCHits projects ordered single-target hits, including defeat
// continuations; effect reads the actor as each hit's continuation left it.
func (s *LifetimeState) validateNPCHits(c *DamageContext, death *DefeatContext, hits int, effect func(*LifetimeActor, int) TargetEffect, rng random.Generator) error {
	states := []damageContinuation{{State: s.callbackCopy(), Context: *c}}
	for hit := range hits {
		var next []damageContinuation
		for _, state := range states {
			e := effect(state.State.actor(c.ActorID), hit)
			continuations, err := state.State.projectCallbackDamage(&state.Context, death, e, e.Payload.Number, hit+1 < hits, rng)
			if err != nil {
				return err
			}
			for _, continuation := range continuations {
				next = appendDamageContinuation(next, continuation.State, continuation.Context, "")
			}
		}
		states = next
	}
	return nil
}

// admitNPCPartyStats admits the three ordered party stat helpers on p.
func (p *LifetimeState) admitNPCPartyStats(c *EffectContext, coefficient float64, rng random.Generator) error {
	admission := statUpAdmission{mayReplace: make(map[string]float64)}
	for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
		for _, target := range c.PartyIDs {
			e := controlEffect(target, coefficient, CategoryMagic)
			if err := p.validateStatUp(c, e, stat, rng, admission); err != nil {
				return err
			}
			p.recordStatUp205(c, e, stat, &admission)
		}
		// Earlier helpers can cure the caster's magic seal.
		if p.actor(c.ActorID).categorySealed(CategoryMagic) {
			p.actor(c.ActorID).Status = ""
		}
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

package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateControlClimax follows each body's ordered writes on private state.
// It never draws RNG, invokes Apply hooks, or changes the submitted context.
func (s *LifetimeState) ValidateControlClimax(c *ControlClimaxContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	known := false
	for _, job := range []int{53, 58, 70} {
		for _, entry := range ControlClimaxEntries(job) {
			known = known || entry.ID == id
		}
	}
	if !known || id == "job-53-entry-6" || id == "job-70-entry-3" {
		return ErrUnsupportedEffect
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	switch id {
	case "job-53-entry-0", "job-53-entry-1", "job-53-entry-4":
		status := StatusDofuu
		if id == "job-53-entry-1" {
			status = StatusDeadlyPoison
		}
		if id == "job-53-entry-4" {
			status = StatusPhysicalSeal
		}
		return p.ValidateStatus(&StatusContext{EffectContext: child.EffectContext}, controlSymbol(child.shift(), status, CategoryBreath), 80, false, false, rng)
	case "job-53-entry-2", "job-58-entry-2":
		statuses, category, chance := []string{StatusDeadlyPoison, StatusParalyze, StatusSleep}, CategoryBreath, 60.0
		if id == "job-58-entry-2" {
			statuses, category, chance = []string{StatusMagicSeal, StatusPhysicalSeal, StatusDofuu}, CategoryMagic, 70
		}
		return p.validateControlStatuses(&child, statuses, category, chance, rng)
	case "job-53-entry-3":
		return p.validatePartyTarget(child.TargetContext, controlSymbol(child.shift(), "counter", CategoryBreath), rng)
	case "job-53-entry-5":
		targets, err := p.controlTargets(&child, controlSymbol(child.shift(), "possession", CategoryBreath), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if target == "" {
				continue
			}
			q, nested := p.callbackCopy(), child
			nested.TargetContext = possessionContext(child.TargetContext, target)
			nested.ActorNPC = q.actor(target).NPC
			if err := q.validateNormalStrikes(&nested.DamageContext, "", 1, rng); err != nil {
				return err
			}
		}
	case "job-58-entry-0":
		targets, err := p.controlTargets(&child, controlSymbol(child.shift(), "life-reduction", CategoryMagic), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if target != "" {
				if err := p.validateControlDown(&child, controlEffect(target, .7, CategoryMagic), rng); err != nil {
					return err
				}
			}
		}
	case "job-58-entry-1":
		p.actor(child.ActorID).Status = ""
		return p.validateControlBuffs(&child, 1, StatAgility, rng)
	case "job-58-entry-3":
		if child.MPDrain == nil || child.MPDrain.Validate == nil || child.MPDrain.Apply == nil {
			return ErrUnsupportedEffect
		}
		return child.MPDrain.Validate(&p, &child.EffectContext, controlEffect(child.shift(), .4, CategoryMagic))
	case "job-58-entry-4":
		a := p.actor(child.ActorID)
		a.Temporary = TemporaryDouble
		_, err := p.gemTensionCeiling(&child.EffectContext, a, 2)
		return err
	case "job-58-entry-5":
		for _, target := range child.EnemyIDs {
			if err := p.validateControlDown(&child, controlEffect(target, .75, CategoryMagic), rng); err != nil {
				return err
			}
		}
	case "job-58-entry-6":
		status, tmp := p.consumeArrow(&child)
		targets, err := p.controlTargets(&child, controlSymbol(child.shift(), "arrow", CategoryUntyped), status || tmp != "")
		if err != nil {
			return err
		}
		for _, target := range targets {
			statuses := []string{""}
			if status {
				statuses = []string{StatusMagicSeal, StatusPhysicalSeal, StatusDofuu}
			}
			for _, seal := range statuses {
				q, nested := p.callbackCopy(), child
				if status {
					q.actor(target).Status = seal
				}
				if tmp != "" {
					q.actor(target).Temporary = tmp
				}
				e := controlEffect(target, float64(q.actor(child.ActorID).Attack)*.7, CategoryMagic)
				e.Arguments = []string{"1"}
				if err := q.ValidateDamage(&nested.DamageContext, e, rng); err != nil {
					return err
				}
			}
		}
	case "job-70-entry-0":
		return p.validatePartyTarget(child.TargetContext, controlEffect(child.ActorID, 300, CategoryUntyped), rng)
	case "job-70-entry-1":
		return p.validateControlBuffs(&child, .4, StatDefense, rng)
	case "job-70-entry-2":
		e := controlEffect("", 180, CategoryMagic)
		e.Arguments = []string{"1"}
		return p.validateAreaDamage(&child.DamageContext, e, rng)
	case "job-70-entry-4":
		return p.validateClimax(&child, rng)
	}
	return nil
}

func (s *LifetimeState) validateControlStatuses(c *ControlClimaxContext, statuses []string, category CallbackCategory, chance float64, rng random.Generator) error {
	actorStates := []string{s.actor(c.ActorID).Status}
	for _, target := range c.EnemyIDs {
		next := slices.Clone(actorStates)
		for _, state := range actorStates {
			p, child := s.callbackCopy(), *c
			p.actor(c.ActorID).Status = state
			for _, status := range statuses {
				e := controlSymbol(target, status, category)
				if err := p.ValidateStatus(&StatusContext{EffectContext: child.EffectContext}, e, chance, false, false, rng); err != nil {
					return err
				}
			}
			// A reflected seal can replace the actor's earlier status. Its later
			// cure can then enable item195 confusion on a subsequent helper.
			if p.actor(c.ActorID).categorySealed(category) {
				p.actor(c.ActorID).Status = ""
				if !slices.Contains(next, "") {
					next = append(next, "")
				}
			}
			e := controlSymbol(target, statuses[0], category)
			converted, _, err := p.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
			if err != nil {
				return err
			}
			candidates, err := p.statusTargetCandidates(&child.DamageContext, converted)
			if err != nil {
				return err
			}
			for _, candidate := range candidates {
				t := p.actor(candidate.TargetID)
				probability := controlEffect(candidate.TargetID, chance, candidate.Category)
				power := p.finalEffectPower(&child.EffectContext, p.initialEffectPower(&child.EffectContext, probability, ModifierStatus))
				if t.ID != c.ActorID || t.HP <= 0 || statusImmune(t, false, false, p.Fields.Current()) || power <= 0 || candidate.Payload.Symbol == "" || hasItem(t.ItemDefinitionIDs, "item-161") && t.Tension >= 5 {
					continue
				}
				for _, status := range statuses {
					if !slices.Contains(next, status) {
						next = append(next, status)
					}
				}
			}
		}
		actorStates = next
	}
	return nil
}

func (s *LifetimeState) validateControlDown(c *ControlClimaxContext, e TargetEffect, rng random.Generator) error {
	if err := s.validateRecovery(&c.EffectContext, e, rng); err != nil {
		return err
	}
	_, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	return err
}

// The source's unguarded arrow/halo writes cannot address an empty record.
// Preserve ordinary guarded no-effect paths separately from those missing facts.
func (s *LifetimeState) controlTargets(c *ControlClimaxContext, e TargetEffect, required bool) ([]string, error) {
	a := s.actor(c.ActorID)
	converted, _, err := a.ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return nil, err
	}
	var ids []string
	if len(c.EnemyIDs) > 0 {
		for _, candidate := range s.damageCandidates(&c.DamageContext, converted) {
			if !slices.Contains(ids, candidate.TargetID) {
				ids = append(ids, candidate.TargetID)
			}
		}
	}
	unresolved := len(c.EnemyIDs) == 0 || a.categorySealed(converted.Category)
	if a.Status != StatusConfusion {
		if t := s.actor(e.TargetID); t != nil && t.TeamID != a.TeamID {
			unresolved = unresolved || t.HP <= 0
		} else {
			for _, target := range c.EnemyIDs {
				unresolved = unresolved || s.actor(target).HP <= 0
			}
		}
	}
	if unresolved {
		if required {
			return nil, ErrUnsupportedEffect
		}
		ids = append(ids, "")
	}
	return ids, nil
}

func (s *LifetimeState) validateControlBuffs(c *ControlClimaxContext, coefficient float64, second BattleStat, rng random.Generator) error {
	e := controlEffect(c.ActorID, coefficient, CategoryPhysical)
	if err := s.ValidateStatUp(&c.EffectContext, e, StatAttack, rng); err != nil {
		return err
	}
	// First AT may consume tension or clear a physical seal. Retain failure and
	// success paths, including item186 healing before the later stat helper.
	for _, consumed := range []bool{false, true} {
		p, child := s.callbackCopy(), *c
		if consumed {
			p.actor(c.ActorID).Tension = 1
			if p.actor(c.ActorID).categorySealed(CategoryPhysical) {
				p.actor(c.ActorID).Status = ""
			}
		}
		if err := p.ValidateStatUp(&child.EffectContext, e, second, rng); err != nil {
			return err
		}
	}
	return nil
}

func (s *LifetimeState) validateClimax(c *ControlClimaxContext, rng random.Generator) error {
	a := s.actor(c.ActorID)
	power := max(a.Attack, a.Defense, a.Agility)
	targets, err := s.controlTargets(c, controlSymbol(c.shift(), "halo", CategoryUntyped), true)
	if err != nil {
		return err
	}
	for _, target := range targets {
		p, child := s.callbackCopy(), *c
		tmp := p.actor(target).Temporary
		p.actor(target).Temporary = ""
		e := controlEffect(target, float64(power), CategoryMagic)
		e.Arguments = []string{"1"}
		converted, _, err := p.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
		if err != nil {
			return err
		}
		returned := false
		for _, candidate := range p.damageCandidates(&child.DamageContext, converted) {
			t := p.actor(candidate.TargetID)
			if t.HP <= 0 {
				t = p.actor(c.ActorID)
			}
			returned = returned || candidate.Payload.Number > 0 && t.Temporary != TemporaryOneHitDefense && damageHasPower(p.actor(c.ActorID), t, candidate)
		}
		if err := p.admitDamageSequence(&child.DamageContext, []TargetEffect{e}, rng); err != nil {
			return err
		}
		if !returned || a.HP <= 0 {
			continue
		}
		child.DamageCalls++
		// The original target regains its temporary only on the living branch.
		for _, alive := range []bool{true, false} {
			q, tail := p.callbackCopy(), child
			if alive {
				q.actor(target).Temporary = tmp
			} else {
				q.actor(target).HP = 0
			}
			if err := q.validateClimaxTail(&tail, power, rng); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *LifetimeState) validateClimaxTail(c *ControlClimaxContext, power int, rng random.Generator) error {
	a := s.actor(c.ActorID)
	if a.HP <= 0 {
		return nil
	}
	switch power {
	case a.Attack:
		// Magnitude does not select descendants; retain a positive power after
		// the first hit's defense consumption and item195 activation.
		e := controlEffect("", float64(power), CategoryUntyped)
		e.Arguments = []string{"1"}
		return s.validateAreaDamage(&c.DamageContext, e, rng)
	case a.Defense:
		for _, id := range c.PartyIDs {
			if err := s.validatePartyTarget(c.TargetContext, controlEffect(id, float64(a.MaxHP)*.3, CategoryUntyped), rng); err != nil {
				return err
			}
		}
	default:
		return s.ValidateStatUp(&c.EffectContext, controlEffect(c.ActorID, .3, CategoryUntyped), StatAgility, rng)
	}
	return nil
}

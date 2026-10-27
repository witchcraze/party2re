package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ValidateNPCStatus is shared by discovery and execution. All projections are
// private and retain accepted no-effect paths without a committed RNG draw.
func (s *LifetimeState) ValidateNPCStatus(c *NPCStatusContext, id string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p, child := s.callbackCopy(), *c
	child.Arguments = slices.Clone(c.Arguments)
	a := p.actor(c.ActorID)
	status := StatusContext{EffectContext: child.EffectContext, Defeat: child.Death}
	switch id {
	case "npc-job-88-position-0":
		return p.ValidateAreaStatus(&status, controlSymbol("", StatusDofuu, CategoryPhysical), 60, false, false, rng)
	case "npc-job-88-position-1":
		if a.Gear == nil {
			return ErrUnsupportedEffect
		}
		coefficient := 1.5
		if a.Gear.WeaponID != "" {
			coefficient = .8
		}
		return p.ValidateDamage(&child.DamageContext, dualAgilityEffect(child.shift(), float64(a.Attack)*coefficient, CategoryPhysical, true), rng)
	case "npc-job-88-position-2":
		return p.validatePartyTarget(c.TargetContext, controlSymbol(child.shift(), "grooming", CategoryUntyped), rng)
	case "npc-job-88-position-3":
		return nil
	case "npc-job-90-position-0", "npc-job-90-position-1", "npc-job-91-position-0", "npc-job-91-position-1", "npc-job-92-position-1":
		power, category, direct, symbol, chance := npcStatusStrike(id, a.Attack)
		child.AdditionalEffect = true
		return p.validateNPCStatusStrike(&child, dualAgilityEffect(child.shift(), power, category, direct), symbol, chance, rng)
	case "npc-job-90-position-2":
		return p.ValidateAreaStatus(&status, controlSymbol("", StatusDeadlyPoison, CategoryBreath), 60, false, false, rng)
	case "npc-job-91-position-2":
		return p.ValidateAreaStatus(&status, controlSymbol("", StatusParalyze, CategoryBreath), 35, false, false, rng)
	case "npc-job-92-position-0":
		return p.ValidateStatus(&status, controlSymbol(child.shift(), StatusSleep, CategoryMagic), 65, false, false, rng)
	case "npc-job-92-position-2":
		return p.ValidateAreaStatus(&status, controlSymbol("", StatusSleep, CategoryBreath), 35, false, false, rng)
	case "npc-job-93-position-0":
		return p.ValidateStatus(&status, controlSymbol(child.shift(), "death", CategoryMagic), 20, false, true, rng)
	case "npc-job-93-position-1":
		return p.ValidateAreaStatus(&status, controlSymbol("", "death", CategoryMagic), 20, false, true, rng)
	case "npc-job-93-position-2":
		return p.ValidateAreaStatus(&status, controlSymbol("", "death", CategoryDance), 17, false, true, rng)
	case "npc-job-93-position-3":
		targets, err := p.controlTargets(&ControlClimaxContext{DamageContext: child.DamageContext}, controlSymbol(child.shift(), "life-reduction", CategoryMagic), false)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if target != "" && p.actor(target).MaxHP <= 999 && p.actor(target).BaseStats.Defense <= 999 {
				if err := p.validateControlDown(&ControlClimaxContext{DamageContext: child.DamageContext}, controlEffect(target, .75, CategoryMagic), rng); err != nil {
					return err
				}
			}
		}
		return nil
	case "npc-job-94-position-0":
		if child.Death == nil {
			return ErrUnsupportedEffect
		}
		e := controlSymbol("", "death", CategoryUntyped)
		states, err := p.projectAreaDeath(&status, e, 60, false, true, rng)
		if err != nil {
			return err
		}
		defeat := *child.Death
		defeat.TargetContext = child.TargetContext
		for _, state := range states {
			if err := state.ValidateDefeat(&defeat, c.ActorID, rng); err != nil {
				return err
			}
		}
		return nil
	case "npc-job-94-position-1":
		a.Status = StatusSleep
		e := controlEffect(c.ActorID, float64(a.MaxHP)*.5, CategoryUntyped)
		if err := p.validateRecovery(&child.EffectContext, e, rng); err != nil {
			return err
		}
		return p.validatePartyTarget(child.TargetContext, e, rng)
	}
	return ErrUnsupportedEffect
}

func (s *LifetimeState) validateNPCStatusStrike(c *NPCStatusContext, e TargetEffect, symbol string, chance float64, rng random.Generator) error {
	if err := s.ValidateDamage(&c.DamageContext, e, rng); err != nil {
		return err
	}
	converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return err
	}
	if len(c.EnemyIDs) == 0 {
		return nil
	}
	for _, candidate := range s.damageCandidates(&c.DamageContext, converted) {
		p, child := s.callbackCopy(), *c
		a, t := p.actor(c.ActorID), p.actor(candidate.TargetID)
		if t.HP <= 0 {
			t = a
		}
		if candidate.Payload.Number == 0 || t.Temporary == TemporaryOneHitDefense || !damageHasPower(a, t, candidate) {
			continue
		}
		// A returned living hit can cross current-HP immunity. Keep the original
		// MaxHP/baseDF for NPC immunity and the returned recipient for the next helper.
		t.HP = 1
		if a.categorySealed(converted.Category) {
			a.Status = ""
		}
		child.DamageCalls++
		child.Item185Activated = child.Item185Activated || hasItem(t.ItemDefinitionIDs, "item-185")
		child.Item195Activated = child.Item195Activated || hasItem(t.ItemDefinitionIDs, "item-195")
		status := StatusContext{EffectContext: child.EffectContext}
		if err := p.ValidateStatus(&status, controlSymbol(t.ID, symbol, e.Category), chance, false, false, rng); err != nil {
			return err
		}
	}
	return nil
}

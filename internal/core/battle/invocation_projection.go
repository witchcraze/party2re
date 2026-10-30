package battle

import (
	"maps"
	"slices"
)

func cloneCallbackContext(c CallbackContext) CallbackContext {
	c.PartyIDs = slices.Clone(c.PartyIDs)
	c.EnemyIDs = slices.Clone(c.EnemyIDs)
	c.PhantomEnemyIDs = slices.Clone(c.PhantomEnemyIDs)
	c.Arguments = slices.Clone(c.Arguments)
	c.Invoke = nil
	if c.Damage != nil {
		damage := *c.Damage
		damage.PartyIDs = slices.Clone(damage.PartyIDs)
		damage.EnemyIDs = slices.Clone(damage.EnemyIDs)
		damage.PhantomEnemyIDs = slices.Clone(damage.PhantomEnemyIDs)
		if damage.RequesterLevel != nil {
			level := *damage.RequesterLevel
			damage.RequesterLevel = &level
		}
		if damage.End.DarknessDebit != nil {
			debit := *damage.End.DarknessDebit
			damage.End.DarknessDebit = &debit
		}
		c.Damage = &damage
	}
	return c
}

func (s LifetimeState) callbackCopy() LifetimeState {
	s.Actors = slices.Clone(s.Actors)
	s.SkillBans = maps.Clone(s.SkillBans)
	s.SkillHistory = maps.Clone(s.SkillHistory)
	for id, history := range s.SkillHistory {
		s.SkillHistory[id] = maps.Clone(history)
	}
	for i := range s.Actors {
		a := &s.Actors[i]
		a.Abilities = slices.Clone(a.Abilities)
		a.ItemDefinitionIDs = slices.Clone(a.ItemDefinitionIDs)
		a.ConsumedItems = slices.Clone(a.ConsumedItems)
		a.Skills = slices.Clone(a.Skills)
		a.ActionItems = slices.Clone(a.ActionItems)
		a.CustomSkills = slices.Clone(a.CustomSkills)
		for j := range a.CustomSkills {
			a.CustomSkills[j].Gems = slices.Clone(a.CustomSkills[j].Gems)
		}
		a.JobHistory = maps.Clone(a.JobHistory)
		for id, sp := range a.JobHistory {
			if sp != nil {
				value := *sp
				a.JobHistory[id] = &value
			}
		}
		if a.Gear != nil {
			gear := *a.Gear
			a.Gear = &gear
			if gear.WeaponAttack != nil {
				value := *gear.WeaponAttack
				a.Gear.WeaponAttack = &value
			}
		}
	}
	return s
}

// Project only a declared exact transition. Unknown random/delegated outcomes
// retain the existing conservative AfterEffects admission, with no pool changes.
func (p *callbackAdmission) project(op CallbackInvocation) (bool, error) {
	return p.projectAfter(op, make(map[CallbackInvocation]bool))
}

func (p *callbackAdmission) projectAfter(op CallbackInvocation, seen map[CallbackInvocation]bool) (bool, error) {
	if seen[op] {
		return false, nil
	}
	seen[op] = true
	if op.Kind == "entry" {
		def := p.d.Entries[op.EntryID]
		if def.Project != nil {
			return def.Project(p.s, p.c)
		}
		if def.Invocation.Kind != "" {
			return p.projectAfter(def.Invocation, seen)
		}
		return false, nil
	}
	name, calling := callingCommand(op.Kind)
	if !calling {
		return false, nil
	}
	job := callingJob(p.s.actor(p.c.ActorID).Jobs)
	if job == 0 {
		return true, nil
	}
	entries, err := p.s.callbackTable(p.c, p.d, job)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if callingMatch(entry.CallbackEntry, name) {
			child := explicitCallbackTarget(p.c)
			copy := *p
			copy.c = &child
			known, err := copy.projectAfter(CallbackInvocation{Kind: "entry", EntryID: entry.ID}, seen)
			p.c.TargetContext = child.TargetContext
			p.c.ArithmeticDivisor = child.ArithmeticDivisor
			return known, err
		}
	}
	return true, nil
}

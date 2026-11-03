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
	if c.Status != nil {
		status := *c.Status
		f := cloneCallbackContext(CallbackContext{Damage: &DamageContext{EffectContext: status.EffectContext}, Defeat: status.Defeat})
		status.EffectContext, status.Defeat = f.Damage.EffectContext, f.Defeat
		c.Status = &status
	}
	if c.Defeat != nil {
		defeat := *c.Defeat
		f := cloneCallbackContext(CallbackContext{TargetContext: defeat.TargetContext})
		defeat.TargetContext = f.TargetContext
		if defeat.RequesterLevel != nil {
			level := *defeat.RequesterLevel
			defeat.RequesterLevel = &level
		}
		if defeat.Seals != nil {
			seals := *defeat.Seals
			defeat.Seals = &seals
		}
		c.Defeat = &defeat
	}
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
		s.Actors[i] = s.Actors[i].Clone()
	}
	return s
}

// Clone isolates mutable encounter/template facts and admission projections.
func (actor LifetimeActor) Clone() LifetimeActor {
	a := &actor
	if a.NPCLoadout != nil {
		loadout := *a.NPCLoadout
		a.NPCLoadout = &loadout
	}
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
	return actor
}

// Unknown projections cannot leak tentative writes into conservative admission.
func (p *callbackAdmission) project(op CallbackInvocation) ([]CallbackContinuation, error) {
	state := p.s.callbackCopy()
	context := cloneCallbackContext(*p.c)
	copy := *p
	copy.s, copy.c = &state, &context
	return copy.projectAfter(op, make(map[CallbackInvocation]bool))
}

func (p *callbackAdmission) projectAfter(op CallbackInvocation, seen map[CallbackInvocation]bool) ([]CallbackContinuation, error) {
	if seen[op] {
		return nil, nil
	}
	seen[op] = true
	defer delete(seen, op)
	if op.Kind == "entry" {
		if op.GeneratedChildID != "" {
			if op.GeneratedChildMP > p.s.actor(p.c.ActorID).MP {
				return []CallbackContinuation{{State: *p.s, Context: *p.c}}, nil
			}
			return p.projectAfter(CallbackInvocation{Kind: "entry", EntryID: op.GeneratedChildID}, seen)
		}
		def := p.d.Entries[op.EntryID]
		if def.ProjectBranches != nil {
			return def.ProjectBranches(p.s, p.c)
		}
		if def.Project != nil {
			known, err := def.Project(p.s, p.c)
			if err != nil || !known {
				return nil, err
			}
			return []CallbackContinuation{{State: *p.s, Context: *p.c}}, nil
		}
		if def.Invocation.Kind != "" {
			return p.projectAfter(def.Invocation, seen)
		}
		return nil, nil
	}
	switch op.Kind {
	case "random", "tool-84":
		return p.projectRandom(op.Kind, seen)
	case "job-82-generated":
		child, err := p.s.generatedCallback(p.c, p.d)
		if err != nil {
			return nil, err
		}
		return p.projectSelection(callbackSelection{child: &child}, false, seen)
	case "gem-86":
		return p.projectExtended(seen)
	}
	name, calling := callingCommand(op.Kind)
	if !calling {
		return nil, nil
	}
	job := callingJob(p.s.actor(p.c.ActorID).Jobs)
	if job == 0 {
		return []CallbackContinuation{{State: *p.s, Context: *p.c}}, nil
	}
	entries, err := p.s.callbackTable(p.c, p.d, job)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if callingMatch(entry.CallbackEntry, name) {
			return p.projectSelection(entry, true, seen)
		}
	}
	return []CallbackContinuation{{State: *p.s, Context: *p.c}}, nil
}

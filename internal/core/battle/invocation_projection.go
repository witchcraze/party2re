package battle

import (
	"maps"
	"slices"
)

func cloneCallbackContext(c CallbackContext) CallbackContext {
	originalDefeat := c.Defeat
	c.PartyIDs = slices.Clone(c.PartyIDs)
	c.EnemyIDs = slices.Clone(c.EnemyIDs)
	c.MemberOnlyIDs = slices.Clone(c.MemberOnlyIDs)
	c.PhantomEnemyIDs = slices.Clone(c.PhantomEnemyIDs)
	c.Arguments = slices.Clone(c.Arguments)
	c.Inspection = maps.Clone(c.Inspection)
	c.Invoke = nil
	c.rollbacks = nil
	if c.Assets != nil {
		assets := *c.Assets
		assets.HelperToolIDs = slices.Clone(assets.HelperToolIDs)
		assets.StoredTools = slices.Clone(assets.StoredTools)
		assets.StoredOutcomes = slices.Clone(assets.StoredOutcomes)
		if assets.HeldToolID != nil {
			held := *assets.HeldToolID
			assets.HeldToolID = &held
		}
		c.Assets = &assets
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
		if defeat.Rewards != nil {
			rewards := *defeat.Rewards
			defeat.Rewards = &rewards
		}
		if defeat.Fortress != nil {
			fortress := *defeat.Fortress
			defeat.Fortress = &fortress
		}
		c.Defeat = &defeat
	}
	if c.Status != nil {
		status := *c.Status
		f := cloneCallbackContext(CallbackContext{Damage: &DamageContext{EffectContext: status.EffectContext}})
		status.EffectContext = f.Damage.EffectContext
		if status.Defeat != nil {
			if status.Defeat == originalDefeat {
				status.Defeat = c.Defeat
			} else {
				status.Defeat = cloneCallbackContext(CallbackContext{Defeat: status.Defeat}).Defeat
			}
		}
		c.Status = &status
	}
	if c.Damage != nil {
		damage := *c.Damage
		if damage.responseDefeat != nil {
			if damage.responseDefeat == originalDefeat {
				damage.responseDefeat = c.Defeat
			} else {
				damage.responseDefeat = cloneCallbackContext(CallbackContext{Defeat: damage.responseDefeat}).Defeat
			}
		}
		damage.PartyIDs = slices.Clone(damage.PartyIDs)
		damage.EnemyIDs = slices.Clone(damage.EnemyIDs)
		damage.MemberOnlyIDs = slices.Clone(damage.MemberOnlyIDs)
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
	s.npcResponse = nil
	s.Actors = slices.Clone(s.Actors)
	s.SkillBans = maps.Clone(s.SkillBans)
	s.SkillHistory = maps.Clone(s.SkillHistory)
	for id, history := range s.SkillHistory {
		s.SkillHistory[id] = maps.Clone(history)
	}
	s.StoredToolHistory = maps.Clone(s.StoredToolHistory)
	for color, history := range s.StoredToolHistory {
		s.StoredToolHistory[color] = maps.Clone(history)
	}
	for i := range s.Actors {
		s.Actors[i] = s.Actors[i].Clone()
	}
	s.rollbacks = nil
	return s
}

func (s LifetimeState) executionCopy() LifetimeState {
	copy := s.callbackCopy()
	if s.npcResponse != nil {
		facts := *s.npcResponse
		facts.defeats = slices.Clone(facts.defeats)
		copy.npcResponse = &facts
	}
	return copy
}

// Clone isolates mutable encounter/template facts and admission projections.
func (actor LifetimeActor) Clone() LifetimeActor {
	a := &actor
	if a.HeldToolID != nil {
		held := *a.HeldToolID
		a.HeldToolID = &held
	}
	if a.NPCLoadout != nil {
		loadout := *a.NPCLoadout
		a.NPCLoadout = &loadout
	}
	if a.Custom != nil {
		custom := *a.Custom
		a.Custom = &custom
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
	case "gem_secret_art_sky":
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

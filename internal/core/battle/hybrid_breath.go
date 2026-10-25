package battle

import "github.com/witchcraze/party2re/internal/core/random"

// HybridBreathContext carries operation facts; consumers rebuild ports after
// reload and retain the selected entry and arguments with the frozen command.
type HybridBreathContext struct {
	DamageContext
	Arguments                    []string
	StatusCalls, AreaStatusCalls int
}

// HybridBreathEntries preserves positions, including the separately owned
// half-revival body at39.5. Table presence alone never certifies that body.
func HybridBreathEntries(job int) []CallbackEntry {
	switch job {
	case 39:
		return []CallbackEntry{
			{ID: "job-39-entry-0", Name: "magic-wave-25", RequiredSP: 3, MPCost: 5},
			{ID: "job-39-entry-1", Name: "party-defense", RequiredSP: 7, MPCost: 7},
			{ID: "job-39-entry-2", Name: "heal-30", RequiredSP: 11, MPCost: 3},
			{ID: "job-39-entry-3", Name: "defense-wave", RequiredSP: 16, MPCost: 7},
			{ID: "job-39-entry-4", Name: "confusion-wave", RequiredSP: 28, MPCost: 10},
			{ID: "job-39-entry-5", Name: "半蘇生", RequiredSP: 50, MPCost: 20},
			{ID: "job-39-entry-6", Name: "breath-wave-180", RequiredSP: 99, MPCost: 40},
			{ID: "job-39-entry-7", Name: "agility-strike", RequiredSP: 99, MPCost: 10, Extended: true},
		}
	case 40:
		return []CallbackEntry{
			{ID: "job-40-entry-0", Name: "magic-70", RequiredSP: 25, MPCost: 8},
			{ID: "job-40-entry-1", Name: "magic-wave-60", RequiredSP: 50, MPCost: 11},
			{ID: "job-40-entry-2", Name: "mp-wave", RequiredSP: 99, MPCost: 1},
		}
	case 41:
		return []CallbackEntry{
			{ID: "job-41-entry-0", Name: "breath-wave-15", RequiredSP: 10, MPCost: 1},
			{ID: "job-41-entry-1", Name: "breath-wave-55", RequiredSP: 30, MPCost: 6},
			{ID: "job-41-entry-2", Name: "breath-wave-115", RequiredSP: 60, MPCost: 14},
			{ID: "job-41-entry-3", Name: "paralysis-wave", RequiredSP: 90, MPCost: 9},
			{ID: "job-41-entry-4", Name: "breath-wave-195", RequiredSP: 120, MPCost: 34},
			{ID: "job-41-entry-5", Name: "guarded-assault", RequiredSP: 120, MPCost: 45, Extended: true},
		}
	case 44:
		return []CallbackEntry{
			{ID: "job-44-entry-0", Name: "heavy-kick", RequiredSP: 20, MPCost: 4},
			{ID: "job-44-entry-1", Name: "defense-guard", RequiredSP: 40, MPCost: 5},
			{ID: "job-44-entry-2", Name: "defense-recoil", RequiredSP: 60, MPCost: 8},
			{ID: "job-44-entry-3", Name: "magic-170", RequiredSP: 80, MPCost: 14},
			{ID: "job-44-entry-4", Name: "heal-150", RequiredSP: 100, MPCost: 7},
			{ID: "job-44-entry-5", Name: "agility-magic", RequiredSP: 120, MPCost: 15},
		}
	}
	return nil
}

func hybridBreathDamage(id string, a *LifetimeActor) (TargetEffect, bool, bool) {
	e := dualAgilityEffect("", 0, CategoryMagic, true)
	area := false
	switch id {
	case "job-39-entry-0":
		e.Payload.Number, area = 25, true
	case "job-39-entry-6":
		e.Payload.Number, e.Category, area = 180, CategoryBreath, true
	case "job-39-entry-7":
		e.Payload.Number, e.Category = float64(a.Agility)*1.8, CategoryPhysical
	case "job-40-entry-0":
		e.Payload.Number = 70
	case "job-40-entry-1":
		e.Payload.Number, area = 60, true
	case "job-40-entry-2":
		e.Payload.Number, area = float64(a.MP)*2, true
	case "job-41-entry-0":
		e.Payload.Number, e.Category, area = 15, CategoryBreath, true
	case "job-41-entry-1":
		e.Payload.Number, e.Category, area = 55, CategoryBreath, true
	case "job-41-entry-2":
		e.Payload.Number, e.Category, area = 115, CategoryBreath, true
	case "job-41-entry-4":
		e.Payload.Number, e.Category, area = 195, CategoryBreath, true
	case "job-44-entry-0":
		e = dualAgilityEffect("", float64(a.Attack)*1.4, CategoryPhysical, false)
	case "job-44-entry-2":
		e = dualAgilityEffect("", float64(a.Defense)*2, CategoryPhysical, false)
	case "job-44-entry-3":
		e.Payload.Number = 170
	case "job-44-entry-5":
		e.Payload.Number = float64(a.Agility) * 1.5
	default:
		return e, false, false
	}
	return e, area, true
}

func (c *HybridBreathContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func (s *LifetimeState) hybridAssault(target string, actorID string) TargetEffect {
	coefficient := 1.8
	if t := s.actor(target); t != nil && t.Temporary != "" {
		coefficient = 2
	}
	return dualAgilityEffect(target, float64(s.actor(actorID).Attack)*coefficient, CategoryPhysical, true)
}

// ApplyHybridBreath executes intrinsic bodies; the invoking owner charges costs
// after normal return. Random/tool MP restoration stays in ApplyInvocations.
func (s *LifetimeState) ApplyHybridBreath(c *HybridBreathContext, id string, rng random.Generator) error {
	if err := s.ValidateHybridBreath(c, id, rng); err != nil {
		return err
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	if e, area, ok := hybridBreathDamage(id, s.actor(c.ActorID)); ok {
		if area {
			_, err := s.ApplyAreaDamage(&c.DamageContext, e.Payload.Number, e.Category, true, rng)
			if err == nil && id == "job-40-entry-2" {
				s.actor(c.ActorID).MP = 1
			}
			return err
		}
		e.TargetID = c.shift()
		r, err := s.ApplyDamage(&c.DamageContext, e, rng)
		if err == nil && id == "job-44-entry-2" && r.Amount > 0 {
			_, err = s.ApplyRecoil(&c.EffectContext, &c.End, float64(r.Amount)*.07, rng)
		}
		return err
	}
	switch id {
	case "job-39-entry-1", "job-39-entry-3":
		_, err := s.ApplyPartyStat(&c.EffectContext, .25, CategoryMagic, StatDefense, id == "job-39-entry-3", rng)
		return err
	case "job-39-entry-2", "job-44-entry-4":
		power := 30.0
		if id == "job-44-entry-4" {
			power = 150
		}
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.shift(), power, CategoryMagic), rng)
		return err
	case "job-39-entry-4", "job-41-entry-3":
		status, category, chance := StatusConfusion, CategoryMagic, 50.0
		if id == "job-41-entry-3" {
			status, category, chance = StatusParalyze, CategoryBreath, 35
		}
		child := StatusContext{EffectContext: c.EffectContext, StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls}
		_, err := s.ApplyAreaStatus(&child, controlSymbol("", status, category), chance, false, false, rng)
		c.EffectContext, c.StatusCalls, c.AreaStatusCalls = child.EffectContext, child.StatusCalls, child.AreaStatusCalls
		return err
	case "job-41-entry-5":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "assault", CategoryUntyped), rng)
		if err != nil {
			return err
		}
		resources(r)
		_, err = s.ApplyDamage(&c.DamageContext, s.hybridAssault(r.TargetID, c.ActorID), rng)
		return err
	case "job-44-entry-1":
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, .5, CategoryUntyped), StatDefense, rng)
		return err
	}
	return ErrUnsupportedEffect
}

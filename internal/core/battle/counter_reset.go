package battle

import "github.com/witchcraze/party2re/internal/core/random"

// CounterResetContext contains operation facts; ports are rebuilt after reload.
type CounterResetContext struct {
	DamageContext
	Arguments                                []string
	StatusDefeat                             *DefeatContext
	StatusCalls, AreaStatusCalls, DeathCalls int
}

// CounterResetEntries includes the separately owned reset35.1 and copy36.4.
// Unbound positions reject the whole random/tool closure, preserving its pool.
func CounterResetEntries(job int) []CallbackEntry {
	switch job {
	case 35:
		return []CallbackEntry{
			{ID: "job-35-entry-0", Name: "deflection", RequiredSP: 10, MPCost: 4},
			{ID: "job-35-entry-1", Name: "effect-dispel", RequiredSP: 30, MPCost: 30},
			{ID: "job-35-entry-2", Name: "magic-death", RequiredSP: 60, MPCost: 14},
			{ID: "job-35-entry-3", Name: "breath-wave-180", RequiredSP: 90, MPCost: 40},
			{ID: "job-35-entry-4", Name: "meditation-300", RequiredSP: 120, MPCost: 25},
			{ID: "job-35-entry-5", Name: "magic-immunity", RequiredSP: 150, MPCost: 6},
			{ID: "job-35-entry-6", Name: "paralyzing-magic-wave", RequiredSP: 180, MPCost: 70},
		}
	case 36:
		return []CallbackEntry{
			{ID: "job-36-entry-0", Name: "dance-counter", RequiredSP: 10, MPCost: 5},
			{ID: "job-36-entry-1", Name: "breath-counter", RequiredSP: 20, MPCost: 5},
			{ID: "job-36-entry-2", Name: "magic-counter", RequiredSP: 40, MPCost: 5},
			{ID: "job-36-entry-3", Name: "attack-counter", RequiredSP: 60, MPCost: 5},
			{ID: "job-36-copy", Name: "姿写し", RequiredSP: 100, MPCost: 50},
		}
	case 37:
		return []CallbackEntry{
			{ID: "job-37-entry-0", Name: "magic-seal-wave", RequiredSP: 5, MPCost: 10},
			{ID: "job-37-entry-1", Name: "magic-absorption", RequiredSP: 10, MPCost: 1},
			{ID: "job-37-entry-2", Name: "dance-seal-wave", RequiredSP: 15, MPCost: 6},
			{ID: "job-37-entry-3", Name: "party-magic-barrier", RequiredSP: 25, MPCost: 6},
			{ID: "job-37-entry-4", Name: "magic-reflection", RequiredSP: 40, MPCost: 5},
			{ID: "job-37-entry-5", Name: "physical-seal", RequiredSP: 60, MPCost: 7},
			{ID: "job-37-entry-6", Name: "meditation-300", RequiredSP: 80, MPCost: 25},
			{ID: "job-37-entry-7", Name: "weapon-armor-break", RequiredSP: 100, MPCost: 15},
		}
	}
	return nil
}

func (c *CounterResetContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func counterResetTemporary(id string) TemporaryEffect {
	switch id {
	case "job-35-entry-0":
		return TemporaryDeflect
	case "job-36-entry-0":
		return TemporaryReflectDance
	case "job-36-entry-1":
		return TemporaryReflectBreath
	case "job-36-entry-2":
		return TemporaryReflectMagic
	case "job-36-entry-3":
		return TemporaryReflectAttack
	}
	return ""
}

func (c *CounterResetContext) statusContext() StatusContext {
	return StatusContext{EffectContext: c.EffectContext, Defeat: c.StatusDefeat,
		StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls, DeathCalls: c.DeathCalls}
}

// ApplyCounterReset executes intrinsic callbacks. The enclosing invocation
// retains ordinary costs and indirect MP restoration, including no-effect.
func (s *LifetimeState) ApplyCounterReset(c *CounterResetContext, id string, rng random.Generator) error {
	if err := s.ValidateCounterReset(c, id, rng); err != nil {
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
	a := s.actor(c.ActorID)
	if tmp := counterResetTemporary(id); tmp != "" {
		a.Temporary = tmp
		return nil
	}
	switch id {
	case "job-35-entry-5", "job-37-entry-1":
		allowed, err := a.CheckCategorySeal(CategoryMagic, rng)
		if err != nil || !allowed {
			return err
		}
		a.Temporary = TemporaryNullMagic
		if id == "job-37-entry-1" {
			a.Temporary = TemporaryAbsorbMagic
		}
	case "job-35-entry-3":
		_, err := s.ApplyAreaDamage(&c.DamageContext, 180, CategoryBreath, true, rng)
		return err
	case "job-35-entry-4", "job-37-entry-6":
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, 300, CategoryUntyped), rng)
		return err
	case "job-37-entry-3":
		for _, target := range c.PartyIDs {
			if t := s.actor(target); t.HP > 0 {
				t.Temporary = TemporaryReduceMagic
			}
		}
	case "job-37-entry-4":
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), string(TemporaryReflectMagic), CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			s.actor(r.TargetID).Temporary = TemporaryReflectMagic
		}
	case "job-37-entry-7":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "break", CategoryPhysical), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			t := s.actor(r.TargetID)
			t.Temporary, t.Tension, t.Attack, t.Defense = "", 1, t.BaseStats.Attack, t.BaseStats.Defense
		}
	default:
		if id == "job-35-entry-6" {
			c.AdditionalEffect = true
			if _, err := s.ApplyAreaDamage(&c.DamageContext, 150, CategoryMagic, true, rng); err != nil {
				return err
			}
		}
		child := c.statusContext()
		var err error
		switch id {
		case "job-35-entry-2":
			_, err = s.ApplyStatus(&child, controlSymbol(c.shift(), "death", CategoryMagic), 20, false, true, rng)
		case "job-35-entry-6":
			_, err = s.ApplyAreaStatus(&child, controlSymbol("", StatusParalyze, CategoryMagic), 20, false, false, rng)
		case "job-37-entry-0":
			_, err = s.ApplyAreaStatus(&child, controlSymbol("", StatusMagicSeal, CategoryMagic), 70, false, false, rng)
		case "job-37-entry-2":
			_, err = s.ApplyAreaStatus(&child, controlSymbol("", StatusDanceSeal, CategoryDance), 70, false, false, rng)
		case "job-37-entry-5":
			_, err = s.ApplyStatus(&child, controlSymbol(c.shift(), StatusPhysicalSeal, CategoryUntyped), 80, false, false, rng)
		}
		c.EffectContext, c.StatusCalls, c.AreaStatusCalls, c.DeathCalls = child.EffectContext, child.StatusCalls, child.AreaStatusCalls, child.DeathCalls
		return err
	}
	return nil
}

package battle

import "github.com/witchcraze/party2re/internal/core/random"

// WeaponWeatherStanceContext carries operation facts. Consumers save counters,
// arguments and target/cursor facts, then rebuild operation ports after reload.
type WeaponWeatherStanceContext struct {
	DamageContext
	Arguments                                                           []string
	StatusDefeat                                                        *DefeatContext
	StatusCalls, AreaStatusCalls, DeathCalls, CureCalls, PartyCureCalls int
}

func (c *WeaponWeatherStanceContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func (c *WeaponWeatherStanceContext) statusContext() StatusContext {
	return StatusContext{EffectContext: c.EffectContext, Defeat: c.StatusDefeat,
		StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls, DeathCalls: c.DeathCalls,
		CureCalls: c.CureCalls, PartyCureCalls: c.PartyCureCalls}
}

func weaponWeatherDamage(id string, a *LifetimeActor) (TargetEffect, bool, bool) {
	e := dualAgilityEffect("", 0, CategoryPhysical, false)
	area := false
	switch id {
	case "job-24-entry-0":
		e.Payload.Number = float64(a.Attack) * 1.2
	case "job-24-entry-1":
		e = dualAgilityEffect("", float64(a.Attack)*.4, CategoryUntyped, true)
	case "job-24-entry-3":
		e.Payload.Number = float64(a.Attack) * 1.5
	case "job-24-entry-4":
		e = dualAgilityEffect("", 230, CategoryPhysical, true)
	case "job-27-entry-1":
		e = dualAgilityEffect("", 80, CategoryUntyped, true)
	case "job-27-entry-3":
		e, area = dualAgilityEffect("", 80, CategoryMagic, true), true
	case "job-27-entry-6":
		e, area = dualAgilityEffect("", 160, CategoryMagic, true), true
	default:
		return e, false, false
	}
	return e, area, true
}

// These assignments have no helper category/living gate or RNG. Keep their
// exact writes available to ordered admission as well as execution.
func (s *LifetimeState) applyWeaponStanceRaw(c *WeaponWeatherStanceContext, id string) bool {
	a := s.actor(c.ActorID)
	switch id {
	case "job-24-entry-5":
		a.Extended, a.ExtendedTurns = ExtendedMagicSword, 7
	case "job-27-entry-5":
		for _, target := range c.PartyIDs {
			if t := s.actor(target); t.HP > 0 {
				t.Temporary = TemporaryReflectBreath
			}
		}
	case "job-28-entry-0":
		a.Hit = 95
	case "job-28-entry-2":
		a.Temporary = TemporaryDeflect
	case "job-28-entry-4":
		a.Temporary = TemporaryNullAttack
	default:
		return false
	}
	return true
}

// ApplyWeaponWeatherStance runs complete intrinsic bodies. Listed MP belongs
// to the invoking owner; indirect charges/restoration stay in ApplyInvocations.
func (s *LifetimeState) ApplyWeaponWeatherStance(c *WeaponWeatherStanceContext, id string, rng random.Generator) error {
	if err := s.ValidateWeaponWeatherStance(c, id, rng); err != nil {
		return err
	}
	if s.applyWeaponStanceRaw(c, id) {
		return nil
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	if e, area, ok := weaponWeatherDamage(id, s.actor(c.ActorID)); ok {
		if area {
			_, err := s.ApplyAreaDamage(&c.DamageContext, e.Payload.Number, e.Category, true, rng)
			return err
		}
		e.TargetID = c.shift()
		_, err := s.ApplyDamage(&c.DamageContext, e, rng)
		return err
	}
	switch id {
	case "job-24-entry-2":
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.shift(), 1, CategoryMagic), StatAttack, rng)
		return err
	case "job-27-entry-0", "job-27-entry-2":
		coefficient, category, stat := .2, CategoryBreath, StatHit
		if id == "job-27-entry-2" {
			coefficient, category, stat = .3, CategoryMagic, StatAgility
		}
		_, err := s.ApplyPartyStat(&c.EffectContext, coefficient, category, stat, true, rng)
		return err
	case "job-27-entry-7":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "break", CategoryPhysical), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		t := s.actor(r.TargetID)
		t.Temporary, t.Tension, t.Attack = "", 1, t.BaseStats.Attack
		_, err = s.ApplyStatDown(&c.EffectContext, controlEffect(t.ID, .2, CategoryPhysical), StatAttack, rng)
		return err
	}
	child := c.statusContext()
	var err error
	switch id {
	case "job-27-entry-4":
		_, err = s.ApplyPartyCure(&child, controlSymbol("", StatusSleep, CategoryMagic), rng)
	case "job-27-entry-8":
		c.PartyHealCalls++
		child.EffectContext = c.EffectContext
		for _, target := range c.PartyIDs {
			if _, err = s.ApplyHealing(&c.EffectContext, controlEffect(target, 300, CategoryUntyped), rng); err != nil {
				return err
			}
			child.EffectContext = c.EffectContext
			if _, err = s.ApplyCure(&child, controlSymbol(target, s.actor(target).Status, CategoryUntyped), rng); err != nil {
				return err
			}
			c.EffectContext = child.EffectContext
		}
	case "job-28-entry-1":
		_, err = s.ApplyStatus(&child, controlSymbol(c.shift(), StatusPhysicalSeal, CategoryPhysical), 80, false, false, rng)
	case "job-28-entry-5":
		_, err = s.ApplyAreaStatus(&child, controlSymbol("", StatusPhysicalSeal, CategoryPhysical), 65, false, false, rng)
	case "job-28-entry-6":
		_, err = s.ApplyStatus(&child, controlSymbol(c.shift(), "death", CategoryPhysical), 25, false, true, rng)
	}
	c.EffectContext, c.StatusCalls, c.AreaStatusCalls, c.DeathCalls = child.EffectContext, child.StatusCalls, child.AreaStatusCalls, child.DeathCalls
	c.CureCalls, c.PartyCureCalls = child.CureCalls, child.PartyCureCalls
	return err
}

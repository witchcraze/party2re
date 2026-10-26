package battle

import "github.com/witchcraze/party2re/internal/core/random"

// BeastCloneContext retains operation facts; ports are rebuilt after reload.
type BeastCloneContext struct {
	DamageContext
	Arguments                                []string
	RequesterItemID                          string
	StatusDefeat                             *DefeatContext
	StatusCalls, AreaStatusCalls, DeathCalls int
}

// BeastCloneEntries preserves every position at _skill.cgi:1546–1596,
// 1674–1703,2341–2401, including both distinct opening-strike identities.
func BeastCloneEntries(job int) []CallbackEntry {
	switch job {
	case 10:
		return []CallbackEntry{
			{ID: "job-10-entry-0", Name: "vitality-recoil", Extended: true},
			{ID: "job-10-entry-1", Name: "sleep-recovery", RequiredSP: 1},
			{ID: "job-10-entry-2", Name: "magic-defense", RequiredSP: 3, MPCost: 3},
			{ID: "job-10-entry-3", Name: "defense-recoil", RequiredSP: 10, MPCost: 4},
			{ID: "job-10-entry-4", Name: "magic-heal-90", RequiredSP: 20, MPCost: 10},
			{ID: "job-10-entry-5", Name: "sleep-song", RequiredSP: 40, MPCost: 9},
			{ID: "job-10-entry-6", Name: "magic-absorption", RequiredSP: 60, MPCost: 1},
			{ID: "job-10-entry-7", Name: "defense-magic-guard", RequiredSP: 80, MPCost: 7},
			{ID: "job-10-entry-8", Name: "agility-barrage", RequiredSP: 100, MPCost: 20},
		}
	case 12:
		return []CallbackEntry{
			{ID: "job-12-entry-0", Name: "vitality-recoil", Extended: true},
			{ID: "job-12-entry-1", Name: "breath-wave-18", RequiredSP: 3, MPCost: 2},
			{ID: "job-12-entry-2", Name: "poison-breath", RequiredSP: 10, MPCost: 7},
			{ID: "job-12-entry-3", Name: "breath-wave-50", RequiredSP: 18, MPCost: 5},
			{ID: "job-12-entry-4", Name: "paralyzing-strike", RequiredSP: 32, MPCost: 11},
			{ID: "job-12-entry-5", Name: "physical-immobilization", RequiredSP: 50, MPCost: 5},
			{ID: "job-12-entry-6", Name: "breath-wave-90", RequiredSP: 80, MPCost: 14},
			{ID: "job-12-entry-7", Name: "captured-pair", RequiredSP: 110, MPCost: 12},
		}
	case 26:
		return []CallbackEntry{
			{ID: "job-26-entry-0", Name: "breath-wave-50", RequiredSP: 5, MPCost: 5},
			{ID: "job-26-entry-1", Name: "paralysis-breath", RequiredSP: 15, MPCost: 10},
			{ID: "job-26-entry-2", Name: "accuracy-wave", RequiredSP: 30, MPCost: 7},
			{ID: "job-26-entry-3", Name: "poison-breath", RequiredSP: 40, MPCost: 7},
			{ID: "job-26-entry-4", Name: "party-agility", RequiredSP: 50, MPCost: 6},
			{ID: "job-26-entry-5", Name: "vital-strike", RequiredSP: 65, MPCost: 11},
			{ID: "job-26-entry-6", Name: "self-agility", RequiredSP: 80, MPCost: 10},
			{ID: "job-26-entry-7", Name: "armor-break", RequiredSP: 110, MPCost: 15},
			{ID: "job-26-entry-8", Name: "cover-clone", RequiredSP: 110, MPCost: 30, Extended: true},
		}
	}
	return nil
}

func (c *BeastCloneContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func beastBreathPower(id string) float64 {
	switch id {
	case "job-12-entry-1":
		return 18
	case "job-12-entry-3", "job-26-entry-0":
		return 50
	case "job-12-entry-6":
		return 90
	}
	return 0
}

func beastStatus(id string) (string, CallbackCategory, float64, bool) {
	switch id {
	case "job-10-entry-5":
		return StatusSleep, CategorySong, 45, true
	case "job-12-entry-2", "job-26-entry-3":
		return StatusDeadlyPoison, CategoryBreath, 60, true
	case "job-26-entry-1":
		return StatusParalyze, CategoryBreath, 35, true
	case "job-12-entry-5":
		return StatusDofuu, CategoryPhysical, 85, false
	case "job-26-entry-5":
		return "death", CategoryPhysical, 19, false
	}
	return "", "", 0, false
}

func (c *BeastCloneContext) statusContext() StatusContext {
	return StatusContext{EffectContext: c.EffectContext, Defeat: c.StatusDefeat,
		StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls, DeathCalls: c.DeathCalls}
}

func (s *LifetimeState) applyBeastStatus(c *BeastCloneContext, e TargetEffect, chance float64, area, death bool, rng random.Generator) error {
	child := c.statusContext()
	var err error
	if area {
		_, err = s.ApplyAreaStatus(&child, e, chance, false, death, rng)
	} else {
		_, err = s.ApplyStatus(&child, e, chance, false, death, rng)
	}
	c.EffectContext = child.EffectContext
	c.StatusCalls, c.AreaStatusCalls, c.DeathCalls = child.StatusCalls, child.AreaStatusCalls, child.DeathCalls
	return err
}

// ApplyBeastClone composes intrinsic bodies. Listed charges and indirect MP
// restoration remain with the invoking owner, before/after this boundary.
func (s *LifetimeState) ApplyBeastClone(c *BeastCloneContext, id string, rng random.Generator) error {
	if err := s.ValidateBeastClone(c, id, rng); err != nil {
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
	if power := beastBreathPower(id); power != 0 {
		_, err := s.ApplyAreaDamage(&c.DamageContext, power, CategoryBreath, true, rng)
		return err
	}
	if status, category, chance, area := beastStatus(id); status != "" {
		e := controlSymbol("", status, category)
		if !area {
			e.TargetID = c.shift()
		}
		return s.applyBeastStatus(c, e, chance, area, id == "job-26-entry-5", rng)
	}
	a := s.actor(c.ActorID)
	switch id {
	case "job-10-entry-0", "job-12-entry-0", "job-10-entry-3":
		power, recoil := float64(a.HP)*.75, .1
		if id == "job-10-entry-3" {
			power, recoil = float64(a.Defense)*1.6, .07
		}
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, CategoryPhysical, false), rng)
		if err == nil && r.Amount > 0 {
			_, err = s.ApplyRecoil(&c.EffectContext, &c.End, float64(r.Amount)*recoil, rng)
		}
		return err
	case "job-10-entry-1":
		a.Status = StatusSleep
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, float64(a.MaxHP)*.5, CategoryUntyped), rng)
		return err
	case "job-10-entry-2", "job-10-entry-7", "job-26-entry-6":
		target, power, category, stat := c.ActorID, .5, CategoryUntyped, StatDefense
		if id == "job-10-entry-2" {
			target, power, category = c.shift(), .4, CategoryMagic
		}
		if id == "job-26-entry-6" {
			power, stat = 1, StatAgility
		}
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(target, power, category), stat, rng)
		if err == nil && id == "job-10-entry-7" {
			s.actor(c.ActorID).Temporary = TemporaryReduceMagic
		}
		return err
	case "job-10-entry-4":
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.shift(), 90, CategoryMagic), rng)
		return err
	case "job-10-entry-6":
		_, err := s.ApplyTemporaryGem(53, c.TargetContext, "", rng)
		return err
	case "job-10-entry-8", "job-12-entry-7":
		count, target := 2, ""
		if id == "job-10-entry-8" {
			count += int(rng.Float64() * 3)
		} else {
			target = c.shift()
		}
		for range count {
			a = s.actor(c.ActorID)
			if a.HP <= 0 {
				break
			}
			power := float64(a.Agility) * 1.6
			if id == "job-12-entry-7" {
				power = float64(a.Attack) * 1.6
			}
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, power, CategoryPhysical, false), rng); err != nil {
				return err
			}
		}
	case "job-12-entry-4":
		c.AdditionalEffect = true
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(a.Attack)*1.1, CategoryPhysical, false), rng)
		if err != nil || r.TargetID == "" || !r.Returned || s.actor(r.TargetID).HP <= 0 {
			return err
		}
		return s.applyBeastStatus(c, controlSymbol(r.TargetID, StatusParalyze, CategoryPhysical), 40, false, false, rng)
	case "job-26-entry-2", "job-26-entry-4":
		power, stat, down := .25, StatAgility, false
		if id == "job-26-entry-2" {
			power, stat, down = .2, StatHit, true
		}
		_, err := s.ApplyPartyStat(&c.EffectContext, power, CategoryMagic, stat, down, rng)
		return err
	case "job-26-entry-7":
		child := HighPowerContext{DamageContext: c.DamageContext, Arguments: c.Arguments}
		err := s.ApplyHighPower(&child, "job-52-entry-2", rng)
		c.DamageContext, c.Arguments = child.DamageContext, child.Arguments
		return err
	case "job-26-entry-8":
		return s.applyCoverClone(c, rng)
	}
	return nil
}

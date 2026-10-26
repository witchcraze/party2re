package battle

import "github.com/witchcraze/party2re/internal/core/random"

// BasicMagicContext retains operation facts; consumers rebuild ports after reload.
type BasicMagicContext struct {
	DamageContext
	Arguments              []string
	CureCalls, StatusCalls int
}

// BasicMagicEntries preserves source order, including the separately owned
// representative5.3/6.1/6.3 and revival5.7 bodies (_skill.cgi:1235–1274).
func BasicMagicEntries(job int) []CallbackEntry {
	switch job {
	case 5:
		return []CallbackEntry{
			{ID: "job-5-entry-0", Name: "mp-magic", Extended: true},
			{ID: "job-5-entry-1", Name: "defense-up", RequiredSP: 1, MPCost: 3},
			{ID: "job-5-entry-2", Name: "poison-cure", RequiredSP: 3, MPCost: 2},
			{ID: "job-5-entry-3", Name: "heal-30", RequiredSP: 6, MPCost: 3},
			{ID: "job-5-entry-4", Name: "magic-wave-10-35", RequiredSP: 12, MPCost: 4},
			{ID: "job-5-entry-5", Name: "heal-90", RequiredSP: 24, MPCost: 10},
			{ID: "job-5-entry-6", Name: "magic-wave-25-65", RequiredSP: 45, MPCost: 10},
			{ID: "job-5-entry-7", Name: "半蘇生", RequiredSP: 60, MPCost: 20},
			{ID: "job-5-entry-8", Name: "heal-999", RequiredSP: 100, MPCost: 30},
			{ID: "job-5-entry-9", Name: "magic-wave-80-180", RequiredSP: 100, MPCost: 23, Extended: true},
		}
	case 6:
		return []CallbackEntry{
			{ID: "job-6-entry-0", Name: "mp-magic", Extended: true},
			{ID: "job-6-entry-1", Name: "magic-20", RequiredSP: 1, MPCost: 2},
			{ID: "job-6-entry-2", Name: "defense-down", RequiredSP: 4, MPCost: 4},
			{ID: "job-6-entry-3", Name: "magic-wave-25", RequiredSP: 8, MPCost: 5},
			{ID: "job-6-entry-4", Name: "enemy-hit-down", RequiredSP: 14, MPCost: 7},
			{ID: "job-6-entry-5", Name: "magic-70", RequiredSP: 20, MPCost: 8},
			{ID: "job-6-entry-6", Name: "sleep", RequiredSP: 30, MPCost: 8},
			{ID: "job-6-entry-7", Name: "magic-wave-60", RequiredSP: 55, MPCost: 11},
			{ID: "job-6-entry-8", Name: "magic-220", RequiredSP: 90, MPCost: 30},
			{ID: "job-6-entry-9", Name: "magic-600", RequiredSP: 90, MPCost: 40, Extended: true},
		}
	}
	return nil
}

func (c *BasicMagicContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// basicMagicDamage returns the continuous base interval before helper rounding.
func (s *LifetimeState) basicMagicDamage(c *BasicMagicContext, id string) (base, width float64, area bool) {
	switch id {
	case "job-5-entry-0", "job-6-entry-0":
		return float64(s.actor(c.ActorID).MP) * .18, 0, false
	case "job-5-entry-4":
		return 10, 25, true
	case "job-5-entry-6":
		return 25, 40, true
	case "job-5-entry-9":
		return 80, 100, true
	case "job-6-entry-5":
		return 70, 0, false
	case "job-6-entry-7":
		return 60, 0, true
	case "job-6-entry-8":
		return 220, 0, false
	case "job-6-entry-9":
		return 600, 0, false
	}
	return 0, 0, false
}

// ApplyBasicMagic composes intrinsic bodies; ordinary costs and indirect MP
// restoration remain with their invoking consumer.
func (s *LifetimeState) ApplyBasicMagic(c *BasicMagicContext, id string, rng random.Generator) error {
	if err := s.ValidateBasicMagic(c, id, rng); err != nil {
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
	switch id {
	case "job-5-entry-1":
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.shift(), .4, CategoryMagic), StatDefense, rng)
		return err
	case "job-5-entry-2":
		child := StatusContext{EffectContext: c.EffectContext, CureCalls: c.CureCalls}
		_, err := s.ApplyCure(&child, controlSymbol(c.shift(), StatusDeadlyPoison, CategoryMagic), rng)
		c.EffectContext, c.CureCalls = child.EffectContext, child.CureCalls
		return err
	case "job-5-entry-5", "job-5-entry-8":
		power := 90.0
		if id == "job-5-entry-8" {
			power = 999
		}
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.shift(), power, CategoryMagic), rng)
		return err
	case "job-6-entry-2":
		_, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), .4, CategoryMagic), StatDefense, rng)
		return err
	case "job-6-entry-4":
		_, err := s.ApplyPartyStat(&c.EffectContext, .2, CategoryMagic, StatHit, true, rng)
		return err
	case "job-6-entry-6":
		child := StatusContext{EffectContext: c.EffectContext, StatusCalls: c.StatusCalls}
		_, err := s.ApplyStatus(&child, controlSymbol(c.shift(), StatusSleep, CategoryMagic), 65, false, false, rng)
		c.EffectContext, c.StatusCalls = child.EffectContext, child.StatusCalls
		return err
	}
	power, width, area := s.basicMagicDamage(c, id)
	if width != 0 {
		power += rng.Float64() * width
	}
	if area {
		_, err := s.ApplyAreaDamage(&c.DamageContext, power, CategoryMagic, true, rng)
		return err
	}
	_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, CategoryMagic, true), rng)
	return err
}

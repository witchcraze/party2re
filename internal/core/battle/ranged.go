package battle

import "github.com/witchcraze/party2re/internal/core/random"

// RangedContext retains operation facts; rebuild effect ports after reload.
type RangedContext struct {
	DamageContext
	Arguments []string
	Status    StatusContext
}

// RangedEntries preserves the complete positional tables at _skill.cgi:
// 1603–1664,3132–3179,3778–3806, including SP0 and both job11 extensions.
func RangedEntries(job int) []CallbackEntry {
	switch job {
	case 11:
		return []CallbackEntry{
			{ID: "job-11-entry-0", Name: "agile-arrow", Extended: true},
			{ID: "job-11-entry-1", Name: "binding-arrow", RequiredSP: 5, MPCost: 3},
			{ID: "job-11-entry-2", Name: "steady-aim", RequiredSP: 10},
			{ID: "job-11-entry-3", Name: "arrow-sweep", RequiredSP: 20, MPCost: 8},
			{ID: "job-11-entry-4", Name: "mana-arrow", RequiredSP: 40, MPCost: 4},
			{ID: "job-11-entry-5", Name: "blinding-arrow", RequiredSP: 60, MPCost: 6},
			{ID: "job-11-entry-6", Name: "sleeping-arrow", RequiredSP: 90, MPCost: 20},
			{ID: "job-11-entry-7", Name: "arrow-flurry", RequiredSP: 110, MPCost: 24},
			{ID: "job-11-entry-8", Name: "bow-charge", RequiredSP: 110, MPCost: 12, Extended: true},
		}
	case 42:
		return []CallbackEntry{
			{ID: "job-42-entry-0", Name: "confusion-magic", RequiredSP: 15, MPCost: 7},
			{ID: "job-42-entry-1", Name: "silencing-magic", RequiredSP: 30, MPCost: 6},
			{ID: "job-42-entry-2", Name: "silent-step", RequiredSP: 55, MPCost: 10},
			{ID: "job-42-entry-3", Name: "poison-strike", RequiredSP: 80, MPCost: 4},
			{ID: "job-42-entry-4", Name: "paralyzing-strike", RequiredSP: 100, MPCost: 8},
			{ID: "job-42-entry-5", Name: "poison-collapse", RequiredSP: 120, MPCost: 42},
			{ID: "job-42-entry-6", Name: "fatal-strike", RequiredSP: 150, MPCost: 24},
			{ID: "job-42-entry-7", Name: "unseen-strike", RequiredSP: 150, MPCost: 42, Extended: true},
		}
	case 54:
		return []CallbackEntry{
			{ID: "job-54-entry-0", Name: "suppressing-shot", RequiredSP: 5, MPCost: 4},
			{ID: "job-54-entry-1", Name: "aimed-shot", RequiredSP: 15, MPCost: 7},
			{ID: "job-54-entry-2", Name: "steady-sight", RequiredSP: 30},
			{ID: "job-54-entry-3", Name: "binding-shot", RequiredSP: 60, MPCost: 12},
			{ID: "job-54-entry-4", Name: "fixed-barrage", RequiredSP: 90, MPCost: 34},
			{ID: "job-54-entry-5", Name: "shot-flurry", RequiredSP: 130, MPCost: 44},
		}
	}
	return nil
}

func (c *RangedContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func rangedTail(id string) (string, float64, bool) {
	switch id {
	case "job-11-entry-5":
		return "", .07, false
	case "job-11-entry-6":
		return StatusSleep, 55, false
	case "job-42-entry-3":
		return StatusDeadlyPoison, 70, false
	case "job-42-entry-4":
		return StatusParalyze, 35, false
	case "job-42-entry-6":
		return "death", 30, true
	case "job-54-entry-3":
		return StatusDofuu, 30, false
	}
	return "", 0, false
}

func rangedDamage(a *LifetimeActor, id, target string) TargetEffect {
	power, category, direct := float64(a.Attack)*1.2, CategoryPhysical, false
	switch id {
	case "job-11-entry-0", "job-11-entry-3":
		power = float64(a.Agility) * 1.4
	case "job-11-entry-6", "job-42-entry-3":
		power = float64(a.Attack) * 1.1
	case "job-11-entry-7":
		power = float64(a.Attack) * .85
	case "job-42-entry-6":
		power = float64(a.Attack) * 1.7
	case "job-42-entry-7":
		power, category, direct = float64(a.Agility)*1.7, CategoryUntyped, true
	case "job-54-entry-1":
		power, direct = float64(a.Attack)*.6, true
	case "job-54-entry-3":
		power, direct = float64(a.Attack)*.5, true
	case "job-54-entry-4":
		power, direct = 180, true
	case "job-54-entry-5":
		power, direct = float64(a.Attack)*.7, true
	}
	return dualAgilityEffect(target, power, category, direct)
}

// ApplyRanged executes intrinsic bodies; enclosing invocations own listed costs.
func (s *LifetimeState) ApplyRanged(c *RangedContext, id string, rng random.Generator) error {
	if err := s.ValidateRanged(c, id, rng); err != nil {
		return err
	}
	defer func() { c.Status.EffectContext = c.EffectContext }()
	a := s.actor(c.ActorID)
	switch id {
	case "job-11-entry-2", "job-54-entry-2":
		a.Hit = 95
		return nil
	case "job-11-entry-8":
		a.Extended, a.ExtendedTurns = ExtendedBow, 10
		return s.statTension(&c.EffectContext, a, false, rng)
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	c.Status.EffectContext = c.EffectContext
	switch id {
	case "job-11-entry-1", "job-54-entry-0", "job-42-entry-0", "job-42-entry-1":
		symbol, chance, category := StatusDofuu, 80.0, CategoryPhysical
		if id == "job-42-entry-0" {
			symbol, category = StatusConfusion, CategoryMagic
		} else if id == "job-42-entry-1" {
			symbol, chance, category = StatusMagicSeal, 90, CategoryMagic
		}
		_, err := s.ApplyStatus(&c.Status, controlSymbol(c.shift(), symbol, category), chance, false, false, rng)
		c.EffectContext = c.Status.EffectContext
		return err
	case "job-42-entry-2":
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, 1, CategoryUntyped), StatAgility, rng)
		return err
	case "job-11-entry-4":
		r, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), .15, CategoryPhysical), StatMP, rng)
		if err != nil || !r.Returned || r.TargetID == "" {
			return err
		}
		_, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryPhysical), rng)
		return err
	case "job-42-entry-5":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "collapse", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		t := s.actor(r.TargetID)
		if t.MaxHP <= 999 && t.BaseStats.Defense <= 999 && rng.Float64()*1.5 < 1 {
			_, err = s.ApplyStatDown(&c.EffectContext, controlEffect(t.ID, .75, CategoryMagic), StatHP, rng)
			// The direct write uses the first helper's target even if stat-down redirects.
			if err == nil {
				s.actor(r.TargetID).Status = StatusDeadlyPoison
			}
		}
		return err
	case "job-11-entry-3", "job-54-entry-4":
		e := rangedDamage(a, id, "")
		direct, _ := damageFlags(e)
		_, err := s.ApplyAreaDamage(&c.DamageContext, e.Payload.Number, e.Category, direct, rng)
		return err
	case "job-11-entry-7", "job-54-entry-5":
		count := 3 + int(rng.Float64()*2)
		for range count {
			a = s.actor(c.ActorID)
			if a.HP <= 0 {
				break
			}
			if _, err := s.ApplyDamage(&c.DamageContext, rangedDamage(a, id, ""), rng); err != nil {
				return err
			}
		}
		return nil
	}
	symbol, chance, death := rangedTail(id)
	if chance != 0 {
		c.AdditionalEffect = true
		previousReset, previousDefeat := c.Reset, c.Defeat
		damage := s.nativeDamageContext(c.DamageContext, c.Status.Defeat, rng)
		c.Reset, c.Defeat = damage.Reset, damage.Defeat
		defer func() { c.Reset, c.Defeat = previousReset, previousDefeat }()
	}
	r, err := s.ApplyDamage(&c.DamageContext, rangedDamage(a, id, c.shift()), rng)
	if err != nil || chance == 0 || !r.Returned || r.TargetID == "" || s.actor(r.TargetID).HP <= 0 {
		return err
	}
	if symbol == "" {
		_, err = s.ApplyStatDown(&c.EffectContext, controlEffect(r.TargetID, chance, CategoryPhysical), StatHit, rng)
	} else {
		c.Status.EffectContext = c.EffectContext
		_, err = s.ApplyStatus(&c.Status, controlSymbol(r.TargetID, symbol, CategoryPhysical), chance, false, death, rng)
		c.EffectContext = c.Status.EffectContext
	}
	return err
}

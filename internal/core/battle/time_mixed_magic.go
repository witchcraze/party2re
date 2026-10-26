package battle

import "github.com/witchcraze/party2re/internal/core/random"

// TimeMixedMagicContext shares complete damage/reset/defeat and stat-tail ports
// with their existing owners. Rebuild ports and operation counters after reload.
type TimeMixedMagicContext struct {
	DamageContext
	Arguments []string
}

func (c *TimeMixedMagicContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func timeMagicTemporary(id string) TemporaryEffect {
	switch id {
	case "job-30-entry-2":
		return TemporaryReduceMagic
	case "job-30-entry-4":
		return TemporaryReflectMagic
	case "job-30-entry-6":
		return TemporaryRevival
	}
	return ""
}

// ApplyTimeMixedMagic runs intrinsic bodies only; listed costs belong to the
// invoking consumer and indirect cost/restoration rules to ApplyInvocations.
func (s *LifetimeState) ApplyTimeMixedMagic(c *TimeMixedMagicContext, id string, rng random.Generator) error {
	if err := s.ValidateTimeMixedMagic(c, id, rng); err != nil {
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
	if tmp := timeMagicTemporary(id); tmp != "" {
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), string(tmp), CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved && (id != "job-30-entry-2" || s.actor(r.TargetID).HP > 0) {
			s.actor(r.TargetID).Temporary = tmp
		}
		return nil
	}
	a := s.actor(c.ActorID)
	switch id {
	case "job-29-entry-0", "job-29-entry-5":
		coefficient, stat := .45, StatAgility
		if id == "job-29-entry-5" {
			coefficient, stat = .5, StatHP
		}
		_, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), coefficient, CategoryMagic), stat, rng)
		return err
	case "job-29-entry-1":
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.shift(), .45, CategoryMagic), StatAgility, rng)
		return err
	case "job-29-entry-3", "job-29-entry-4", "job-31-entry-3":
		coefficient, stat := .35, StatAgility
		if id == "job-31-entry-3" {
			coefficient, stat = .5, StatDefense
		}
		_, err := s.ApplyPartyStat(&c.EffectContext, coefficient, CategoryMagic, stat, id == "job-29-entry-3", rng)
		if err == nil && id == "job-31-entry-3" {
			// This raw tail runs even when the preceding magic buff was sealed.
			for _, target := range c.PartyIDs {
				if t := s.actor(target); t.HP > 0 {
					t.Temporary = TemporaryReduceMagic
				}
			}
		}
		return err
	case "job-30-entry-0", "job-30-entry-5":
		power := 60.0
		if id == "job-30-entry-5" {
			power = 180
		}
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.shift(), power, CategoryMagic), rng)
		return err
	case "job-31-entry-5":
		_, err := s.ApplyPartyHealing(&c.EffectContext, float64(a.HP), CategoryMagic, rng)
		return err
	case "job-31-entry-1":
		allowed, err := a.CheckCategorySeal(CategoryMagic, rng)
		if err != nil || !allowed {
			return err
		}
		t := &s.Actors[int(rng.Float64()*float64(len(s.Actors)))]
		if t.HP <= 0 {
			t = a
		}
		if t.HP <= 999 && t.Defense <= 999 {
			return c.Defeat(t.ID)
		}
		return nil
	case "job-30-entry-8":
		target := c.shift()
		if t := s.actor(target); t != nil {
			if t.TeamID == a.TeamID {
				if rng.Float64() < .5 {
					t.Temporary = TemporaryReduceAttack
				}
				_, err := s.ApplyPartyHealing(&c.EffectContext, 250, CategoryMagic, rng)
				return err
			}
			_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, 350*(a.Tension+5)/6, CategoryMagic, true), rng)
			return err
		}
		for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
			a = s.actor(c.ActorID)
			if a.HP <= 0 {
				return nil
			}
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect("", float64(*a.currentStat(stat)), CategoryMagic, true), rng); err != nil {
				return err
			}
		}
		return nil
	}
	count, power, target := 1, 0.0, ""
	switch id {
	case "job-29-entry-2":
		count, power = 2+int(rng.Float64()*2), 50
	case "job-29-entry-6":
		count, power = 4+int(rng.Float64()*3), 99
	case "job-30-entry-1":
		target, power = c.shift(), 35
	case "job-30-entry-3":
		target, power = c.shift(), 80
	case "job-30-entry-7":
		count, power = 2, 180
	case "job-31-entry-0":
		target, power = c.shift(), float64(a.HP)
	case "job-31-entry-2":
		target, power = c.shift(), float64(a.MaxHP-a.HP+5)
	}
	for range count {
		if (id == "job-29-entry-2" || id == "job-29-entry-6" || id == "job-30-entry-7") && s.actor(c.ActorID).HP <= 0 {
			break
		}
		if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, power, CategoryMagic, true), rng); err != nil {
			return err
		}
	}
	if id == "job-31-entry-0" {
		return c.Defeat(c.ActorID)
	}
	return nil
}

package battle

import "github.com/witchcraze/party2re/internal/core/random"

// MachineFamiliarContext retains operation facts; consumers freeze requester
// gender and the selected form identity and rebuild runtime ports after reload.
type MachineFamiliarContext struct {
	DamageContext
	Arguments       []string
	RequesterGender string
	CureCalls       int
}

// FamiliarEntries retains the inspection and wallet positions owned elsewhere.
func FamiliarEntries() []CallbackEntry {
	return []CallbackEntry{
		{ID: "job-81-entry-0", Name: "knee-strike", RequiredSP: 10, MPCost: 4},
		{ID: "job-81-entry-1", Name: "inspection", RequiredSP: 30},
		{ID: "job-81-entry-2", Name: "plunder", RequiredSP: 55, MPCost: 22},
		{ID: "job-81-entry-3", Name: "chance-strike", RequiredSP: 95, MPCost: 10},
		{ID: "job-81-entry-4", Name: "party-song", RequiredSP: 130, MPCost: 43},
	}
}

func (c *MachineFamiliarContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// ApplyMachineFamiliar executes intrinsic bodies, leaving listed costs and
// indirect MP restoration with the enclosing invocation owner.
func (s *LifetimeState) ApplyMachineFamiliar(c *MachineFamiliarContext, id string, rng random.Generator) error {
	if err := s.ValidateMachineFamiliar(c, id, rng); err != nil {
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
	switch id {
	case "job-79-entry-0":
		r, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), .25, CategoryMagic), StatMP, rng)
		if err != nil || !r.Returned {
			return err
		}
		_, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryMagic), rng)
		return err
	case "job-79-entry-1", "job-81-entry-0", "job-81-entry-3", "job-82-entry-5":
		power, category, direct := float64(a.Agility)*2.2, CategoryPhysical, false
		switch id {
		case "job-81-entry-0":
			power = float64(a.Attack) * 1.2
		case "job-81-entry-3":
			power = 20
			if rng.Float64() < .5 {
				power = float64(a.Attack) * 3
			}
		case "job-82-entry-5":
			power, category, direct = 170, CategoryBreath, true
		}
		_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, category, direct), rng)
		return err
	case "job-79-entry-4":
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), 220, CategoryPhysical, true), rng)
		if err != nil || !r.Returned || r.TargetID == "" {
			return err
		}
		if err := s.statTension(&c.EffectContext, s.actor(c.ActorID), false, rng); err != nil {
			return err
		}
		if s.actor(r.TargetID).HP <= 0 {
			return s.statTension(&c.EffectContext, s.actor(c.ActorID), false, rng)
		}
	case "job-81-entry-4":
		// is_bad_state has no song branch (_skill.cgi:9575–9609).
		for _, target := range c.PartyIDs {
			if s.actor(target).HP <= 0 {
				continue
			}
			for _, stat := range []BattleStat{StatAttack, StatDefense} {
				if _, err := s.ApplyStatUp(&c.EffectContext, controlEffect(target, .2, CategorySong), stat, rng); err != nil {
					return err
				}
			}
			if rng.Float64() < .25 {
				if _, err := s.ApplyHealing(&c.EffectContext, controlEffect(target, 150, CategorySong), rng); err != nil {
					return err
				}
			}
		}
	case "job-82-entry-1":
		target, attack := c.shift(), a.Attack
		a.Attack = int(float64(attack) * 1.2)
		defer func() { s.actor(c.ActorID).Attack = attack }()
		_, err := s.applyNormalStrike(&c.DamageContext, target, rng)
		return err
	case "job-82-entry-2":
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "tea", CategoryMagic), rng)
		if err != nil || !r.Resolved {
			return err
		}
		resources(r)
		if _, err := s.ApplyHealing(&c.EffectContext, controlEffect(r.TargetID, 150, CategoryMagic), rng); err != nil {
			return err
		}
		child := StatusContext{EffectContext: c.EffectContext, CureCalls: c.CureCalls}
		_, err = s.ApplyCure(&child, controlSymbol(r.TargetID, s.actor(r.TargetID).Status, CategoryMagic), rng)
		c.EffectContext, c.CureCalls = child.EffectContext, child.CureCalls
		return err
	case "job-82-entry-3":
		stat := []BattleStat{StatAttack, StatDefense, StatAgility}[int(rng.Float64()*3)]
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, .18, CategoryUntyped), stat, rng)
		return err
	case "job-82-entry-4":
		count := 3 + int(rng.Float64()*2)
		for range count {
			a = s.actor(c.ActorID)
			if a.HP <= 0 {
				break
			}
			r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect("", float64(a.Attack)*.9, CategoryPhysical, false), rng)
			if err != nil || !r.Returned || r.TargetID == "" {
				return err
			}
		}
	}
	return nil
}

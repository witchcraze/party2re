package battle

import "github.com/witchcraze/party2re/internal/core/random"

// RitualDanceContext shares operation counters and the existing effect owners.
// Runtime hooks are rebuilt by the consumer; Arguments are invocation-local.
type RitualDanceContext struct {
	DamageContext
	Arguments                               []string
	StatusCalls, AreaStatusCalls, CureCalls int
}

// RitualDanceEntries retains both complete tables, including the two revivals
// registered by their existing owner. partySize is frozen at table construction.
func RitualDanceEntries(job, partySize int) []CallbackEntry {
	switch job {
	case 75:
		return []CallbackEntry{
			{ID: "job-75-entry-0", Name: "sealing-curse", RequiredSP: 20, MPCost: 16},
			{ID: "job-75-entry-1", Name: "temporary-piercing", RequiredSP: 40, MPCost: 13},
			{ID: "job-75-entry-2", Name: "accuracy-restoration", RequiredSP: 60, MPCost: 5},
			{ID: "job-75-entry-3", Name: "status-transfer", RequiredSP: 80, MPCost: 9},
			{ID: "job-75-entry-4", Name: "life-for-mana", RequiredSP: 110, MPCost: 1},
			{ID: "job-75-entry-5", Name: "魔力転生", RequiredSP: 120},
			{ID: "job-75-entry-6", Name: "forbidden-curse", RequiredSP: 140, MPCost: 44},
		}
	case 76:
		return []CallbackEntry{
			{ID: "job-76-entry-0", Name: "accuracy-reduction", RequiredSP: 10, MPCost: 7},
			{ID: "job-76-entry-1", Name: "sleep-song", RequiredSP: 20, MPCost: 9},
			{ID: "job-76-entry-2", Name: "life-drain", RequiredSP: 50, MPCost: 24},
			{ID: "job-76-entry-3", Name: "献命の舞", RequiredSP: 90, MPCost: 1},
			{ID: "job-76-entry-4", Name: "cure-or-protection", RequiredSP: 110, MPCost: 37},
			{ID: "job-76-entry-5", Name: "group-dance", RequiredSP: 130, MPCost: 12 * partySize},
		}
	}
	return nil
}

func (c *RitualDanceContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	target := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return target
}

// ApplyRitualDance executes intrinsic bodies; outer listed costs and indirect
// cost restoration remain with the existing invocation/consumer boundaries.
func (s *LifetimeState) ApplyRitualDance(c *RitualDanceContext, id string, rng random.Generator) error {
	if err := s.ValidateRitualDance(c, id, rng); err != nil {
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
	case "job-75-entry-0", "job-76-entry-1":
		sc := StatusContext{EffectContext: c.EffectContext, StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls}
		var err error
		if id == "job-75-entry-0" {
			for _, target := range c.EnemyIDs {
				status := []string{StatusMagicSeal, StatusPhysicalSeal, StatusKinju}[int(rng.Float64()*3)]
				if _, err = s.ApplyStatus(&sc, controlSymbol(target, status, CategoryMagic), 75, false, false, rng); err != nil {
					break
				}
			}
		} else {
			_, err = s.ApplyAreaStatus(&sc, controlSymbol("", StatusSleep, CategorySong), 50, false, false, rng)
		}
		c.EffectContext, c.StatusCalls, c.AreaStatusCalls = sc.EffectContext, sc.StatusCalls, sc.AreaStatusCalls
		return err
	case "job-75-entry-1":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlEffect(c.shift(), 0, CategoryUntyped), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			t := s.actor(r.TargetID)
			if t.Temporary != "" {
				t.Temporary = ""
				loss := min(5000, t.HP-int(float64(t.HP)*.8))
				t.HP = max(1, t.HP-loss)
			}
		}
	case "job-75-entry-2", "job-75-entry-3", "job-76-entry-4":
		category, symbol := CategoryUntyped, "accuracy"
		if id != "job-75-entry-2" {
			symbol = "cure"
		}
		if id == "job-75-entry-3" {
			category = CategoryMagic
		}
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), symbol, category), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		t := s.actor(r.TargetID)
		switch id {
		case "job-75-entry-2":
			t.Hit = 95
		case "job-75-entry-3":
			if t.Status != "" {
				status, target := t.Status, s.randomMember(rng)
				t.Status = ""
				s.actor(target).Status = status
			}
		case "job-76-entry-4":
			if t.Status != "" {
				sc := StatusContext{EffectContext: c.EffectContext, CureCalls: c.CureCalls}
				_, err := s.ApplyCure(&sc, controlSymbol(t.ID, t.Status, CategoryUntyped), rng)
				c.EffectContext, c.CureCalls = sc.EffectContext, sc.CureCalls
				return err
			}
			t.Temporary = []TemporaryEffect{TemporaryNullAttack, TemporaryReduceAttack}[int(rng.Float64()*2)]
		}
	case "job-75-entry-4":
		spent := int(float64(a.MaxHP) * .3 * (.9 + .3*rng.Float64()))
		before := a.HP
		a.HP = max(1, a.HP-spent)
		_, err := s.ApplyMPHealing(&c.EffectContext, controlEffect(a.ID, float64(before-a.HP)*.4+5, CategoryUntyped), rng)
		return err
	case "job-75-entry-6":
		allowed, err := a.CheckCategorySeal(CategoryMagic, rng)
		if err != nil || !allowed {
			return err
		}
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "forbidden", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if !r.Resolved {
			return nil
		}
		t := s.actor(r.TargetID)
		if rng.Float64() < .5 {
			t.Status, t.Tension = []string{StatusKinju, StatusDofuu}[int(rng.Float64()*2)], 1
		}
		if rng.Float64() < .5 {
			t.Temporary = TemporaryDouble
		}
	case "job-76-entry-0":
		_, err := s.ApplyPartyStat(&c.EffectContext, .2, CategoryMagic, StatHit, true, rng)
		return err
	case "job-76-entry-2":
		hit, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), 150+200*rng.Float64(), CategoryMagic, true), rng)
		if err != nil || hit.TargetID == "" || !hit.Returned {
			return err
		}
		_, err = s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, float64(hit.Amount)*1.2, CategoryMagic), rng)
		return err
	case "job-76-entry-5":
		supplied := s.actor(c.shift()) != nil
		for _, target := range c.PartyIDs {
			if s.actor(target).HP > 0 {
				if err := s.applyGroupDance(c, target, supplied, rng); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func groupDanceEffect(a *LifetimeActor, supplied, direct bool) TargetEffect {
	target, power := "", float64(a.Attack)
	if supplied {
		target = a.ID
		if direct {
			power *= .8
		}
	} else if !direct {
		power = float64(int(power * 1.2))
	}
	return dualAgilityEffect(target, power, CategoryDance, direct)
}

func (s *LifetimeState) applyGroupDance(c *RitualDanceContext, target string, supplied bool, rng random.Generator) error {
	origin, npc := c.ActorID, c.ActorNPC
	a := s.actor(target)
	tension := a.Tension
	c.ActorID, c.ActorNPC = target, a.NPC
	if !supplied {
		a.Tension = 1
	}
	defer func() {
		if !supplied {
			s.actor(target).Tension = tension
		}
		c.ActorID, c.ActorNPC = origin, npc
	}()
	_, err := s.ApplyDamage(&c.DamageContext, groupDanceEffect(a, supplied, rng.Float64() < .25), rng)
	return err
}

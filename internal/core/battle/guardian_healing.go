package battle

import "github.com/witchcraze/party2re/internal/core/random"

// GuardianHealingContext holds operation facts; rebuild ports after reload.
type GuardianHealingContext struct {
	DamageContext
	Arguments                                               []string
	StatusCalls, AreaStatusCalls, CureCalls, PartyCureCalls int
}

// GuardianHealingEntries preserves full job17/18 order, including separately
// registered revival bodies. Names are generic native identities.
func GuardianHealingEntries(job int) []CallbackEntry {
	switch job {
	case 17:
		return []CallbackEntry{
			{ID: "job-17-entry-0", Name: "cover", RequiredSP: 10},
			{ID: "job-17-entry-1", Name: "heal-30", RequiredSP: 20, MPCost: 3},
			{ID: "job-17-entry-2", Name: "party-magic-barrier", RequiredSP: 40, MPCost: 6},
			{ID: "job-17-entry-3", Name: "party-paralysis-cure", RequiredSP: 60, MPCost: 5},
			{ID: "job-17-entry-4", Name: "献命回復", RequiredSP: 100, MPCost: 1},
			{ID: "job-17-entry-5", Name: "defense-wave", RequiredSP: 130, MPCost: 20},
			{ID: "job-17-entry-6", Name: "完全蘇生", RequiredSP: 160, MPCost: 80},
		}
	case 18:
		return []CallbackEntry{
			{ID: "job-18-entry-0", Name: "poison-cure", RequiredSP: 5, MPCost: 2},
			{ID: "job-18-entry-1", Name: "party-dance-seal", RequiredSP: 15, MPCost: 6},
			{ID: "job-18-entry-2", Name: "party-waking-song", RequiredSP: 30, MPCost: 5},
			{ID: "job-18-entry-3", Name: "magic-reflection", RequiredSP: 50, MPCost: 5},
			{ID: "job-18-entry-4", Name: "蘇生の声", RequiredSP: 70, MPCost: 27},
			{ID: "job-18-entry-5", Name: "party-heal-120", RequiredSP: 90, MPCost: 25},
			{ID: "job-18-entry-6", Name: "完全蘇生", RequiredSP: 120, MPCost: 80},
			{ID: "job-18-entry-7", Name: "restoration-strike", RequiredSP: 120, MPCost: 100, Extended: true},
		}
	}
	return nil
}

func (c *GuardianHealingContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// ApplyGuardianHealing composes existing effect owners, without outer costs.
func (s *LifetimeState) ApplyGuardianHealing(c *GuardianHealingContext, id string, rng random.Generator) error {
	if err := s.ValidateGuardianHealing(c, id, rng); err != nil {
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
	case "job-17-entry-0", "job-18-entry-3":
		symbol, category := "cover", CategoryPhysical
		if id == "job-18-entry-3" {
			symbol, category = "reflection", CategoryMagic
		}
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), symbol, category), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			if id == "job-18-entry-3" {
				s.actor(r.TargetID).Temporary = TemporaryReflectMagic
			} else if r.TargetID != c.ActorID {
				a.Temporary, s.actor(r.TargetID).Temporary = TemporaryCovering, TemporaryCover
			}
		}
	case "job-17-entry-1":
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.shift(), 30, CategoryMagic), rng)
		return err
	case "job-17-entry-2":
		allowed, err := a.CheckCategorySeal(CategoryMagic, rng)
		if err != nil || !allowed {
			return err
		}
		for _, target := range c.PartyIDs {
			if t := s.actor(target); t.HP > 0 {
				t.Temporary = TemporaryReduceMagic
			}
		}
	case "job-17-entry-5":
		_, err := s.ApplyAreaDamage(&c.DamageContext, float64(a.Defense)*1.5, CategoryPhysical, false, rng)
		return err
	case "job-18-entry-5":
		_, err := s.ApplyPartyHealing(&c.EffectContext, 120, CategoryMagic, rng)
		return err
	case "job-18-entry-7":
		saved := a.Tension
		a.Tension = 1
		total := 0
		for _, target := range c.PartyIDs {
			t := s.actor(target)
			if t.HP <= 0 {
				t.HP = 1
			}
			power := min(300, int(float64(t.MaxHP)*.3))
			r, err := s.ApplyHealing(&c.EffectContext, controlEffect(target, float64(power), CategoryMagic), rng)
			if err != nil {
				return err
			}
			total += r.Amount
		}
		if !s.SkillHistory[a.TeamID][id] {
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(total), CategoryMagic, true), rng); err != nil {
				return err
			}
			s.RecordFirstUse(s.actor(c.ActorID).TeamID, id)
		}
		s.actor(c.ActorID).Tension = saved
	default:
		sc := StatusContext{EffectContext: c.EffectContext, StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls, CureCalls: c.CureCalls, PartyCureCalls: c.PartyCureCalls}
		var err error
		switch id {
		case "job-17-entry-3":
			_, err = s.ApplyPartyCure(&sc, controlSymbol("", StatusParalyze, CategoryMagic), rng)
		case "job-18-entry-0":
			_, err = s.ApplyCure(&sc, controlSymbol(c.shift(), StatusDeadlyPoison, CategoryMagic), rng)
		case "job-18-entry-1":
			_, err = s.ApplyAreaStatus(&sc, controlSymbol("", StatusDanceSeal, CategoryDance), 70, false, false, rng)
		case "job-18-entry-2":
			_, err = s.ApplyPartyCure(&sc, controlSymbol("", StatusSleep, CategorySong), rng)
		}
		c.EffectContext = sc.EffectContext
		c.StatusCalls, c.AreaStatusCalls, c.CureCalls, c.PartyCureCalls = sc.StatusCalls, sc.AreaStatusCalls, sc.CureCalls, sc.PartyCureCalls
		return err
	}
	return nil
}

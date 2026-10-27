package battle

import "github.com/witchcraze/party2re/internal/core/random"

// NPCJob98Context supplies intrinsic ordinary job98 bodies (_skill.cgi:7309–7408).
// Other combat-mode tables, NPC selection/costs and tails remain consumer-owned.
type NPCJob98Context struct {
	DamageContext
	EquipmentReset *EquipmentResetContext
	Arguments      []string
}

func (c *NPCJob98Context) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// ApplyNPCJob98 validates the entire body before any draw, resource, argument or
// effect write. Accepted no-effect helpers retain their unconditional tails.
func (s *LifetimeState) ApplyNPCJob98(c *NPCJob98Context, id string, rng random.Generator) error {
	if err := s.ValidateNPCJob98(c, id, rng); err != nil {
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
	case "npc-job-98-position-0", "npc-job-98-position-5":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "life-to-one", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			t := s.actor(r.TargetID)
			if t.MaxHP <= 999 && t.BaseStats.Defense <= 999 {
				t.HP = 1
			}
		}
	case "npc-job-98-position-1":
		// This is the same all-member, tension-preserving dispel as job35.1.
		reset := *c.EquipmentReset
		reset.Cursor = c.Cursor
		_, err := s.ApplyStatusEntry(&StatusContext{EffectContext: c.EffectContext}, &reset, "job-35-entry-1", "", rng)
		c.Cursor, *c.EquipmentReset = reset.Cursor, reset
		return err
	case "npc-job-98-position-2":
		allowed, err := s.actor(c.ActorID).CheckCategorySeal(CategoryMagic, rng)
		if err != nil || !allowed {
			return err
		}
		for _, target := range c.PartyIDs {
			if t := s.actor(target); t.HP > 0 {
				t.Temporary = TemporaryReflectMagic
			}
		}
	case "npc-job-98-position-3":
		if _, err := s.ApplyPartyStat(&c.EffectContext, .5, CategoryMagic, StatDefense, false, rng); err != nil {
			return err
		}
		for _, target := range c.PartyIDs {
			if t := s.actor(target); t.HP > 0 {
				t.Temporary = TemporaryReduceMagic
			}
		}
	case "npc-job-98-position-4":
		a := s.actor(c.ActorID)
		a.Temporary = TemporaryDouble
		for range 2 {
			if err := s.statTension(&c.EffectContext, a, false, rng); err != nil {
				return err
			}
		}
	case "npc-job-98-position-6":
		if _, err := s.ApplyAreaDamage(&c.DamageContext, 200, CategoryMagic, true, rng); err != nil {
			return err
		}
		_, err := s.ApplyPartyHealing(&c.EffectContext, 200, CategoryMagic, rng)
		return err
	case "npc-job-98-position-7":
		for _, target := range c.EnemyIDs {
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, 100, CategoryBreath, true), rng); err != nil {
				return err
			}
			s.actor(target).Temporary = TemporaryDouble
		}
	case "npc-job-98-position-8":
		_, err := s.ApplyAreaDamage(&c.DamageContext, 350, CategoryBreath, true, rng)
		return err
	case "npc-job-98-position-9":
		hits := 4 + int(rng.Float64()*3)
		for range hits {
			if s.actor(c.ActorID).HP <= 0 {
				break
			}
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect("", 250, CategoryMagic, true), rng); err != nil {
				return err
			}
		}
	case "npc-job-98-position-10":
		_, err := s.ApplyAreaDamage(&c.DamageContext, 400, CategoryMagic, true, rng)
		return err
	}
	return nil
}

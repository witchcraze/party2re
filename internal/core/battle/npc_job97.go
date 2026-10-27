package battle

import "github.com/witchcraze/party2re/internal/core/random"

// NPCJob97Context supplies intrinsic ordinary job97 bodies (_skill.cgi:7215–7307).
// Selection, listed costs and durable operation deduplication remain consumer-owned.
type NPCJob97Context struct {
	DamageContext
	EquipmentReset               *EquipmentResetContext
	Arguments                    []string
	StatusCalls, AreaStatusCalls int
}

func (c *NPCJob97Context) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// ApplyNPCJob97 admits the complete body before arguments, RNG or effect writes.
func (s *LifetimeState) ApplyNPCJob97(c *NPCJob97Context, id string, rng random.Generator) error {
	if err := s.ValidateNPCJob97(c, id, rng); err != nil {
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
	case "npc-job-97-position-0", "npc-job-97-position-1":
		width, coefficient := 2, .85
		if id == "npc-job-97-position-1" {
			width, coefficient = 3, .9
		}
		hits := 3 + int(rng.Float64()*float64(width))
		for range hits {
			a := s.actor(c.ActorID)
			if a.HP <= 0 {
				break
			}
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect("", float64(a.Attack)*coefficient, CategoryPhysical, false), rng); err != nil {
				return err
			}
		}
	case "npc-job-97-position-2":
		reset := *c.EquipmentReset
		reset.Cursor = c.Cursor
		_, err := s.ApplyStatusEntry(&StatusContext{EffectContext: c.EffectContext}, &reset, "job-35-entry-1", "", rng)
		c.Cursor, *c.EquipmentReset = reset.Cursor, reset
		return err
	case "npc-job-97-position-3":
		a := s.actor(c.ActorID)
		a.Defense -= int(float64(a.Defense) * .5)
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, 1, CategoryUntyped), StatAttack, rng)
		return err
	case "npc-job-97-position-4":
		if _, err := s.ApplyAreaDamage(&c.DamageContext, 300, CategoryPhysical, true, rng); err != nil {
			return err
		}
		s.actor(c.ActorID).Temporary = TemporaryDouble
	case "npc-job-97-position-5":
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(s.actor(c.ActorID).Attack)*2.5, CategoryPhysical, false), rng)
		if err != nil || r.Amount <= 0 {
			return err
		}
		_, err = s.ApplyRecoil(&c.EffectContext, &c.End, float64(r.Amount)*.2, rng)
		return err
	case "npc-job-97-position-6":
		s.actor(c.ActorID).Temporary = TemporaryDouble
		for range 2 {
			if err := s.statTension(&c.EffectContext, s.actor(c.ActorID), false, rng); err != nil {
				return err
			}
		}
	case "npc-job-97-position-7":
		_, err := s.ApplyAreaDamage(&c.DamageContext, 400, CategoryBreath, true, rng)
		return err
	case "npc-job-97-position-8", "npc-job-97-position-10":
		power, chance, status := 250.0, 20.0, StatusParalyze
		if id == "npc-job-97-position-10" {
			power, chance, status = 260, 30, StatusSleep
		}
		c.AdditionalEffect = true
		if _, err := s.ApplyAreaDamage(&c.DamageContext, power, CategoryMagic, true, rng); err != nil {
			return err
		}
		child := StatusContext{EffectContext: c.EffectContext, StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls}
		_, err := s.ApplyAreaStatus(&child, controlSymbol("", status, CategoryMagic), chance, false, false, rng)
		c.EffectContext, c.StatusCalls, c.AreaStatusCalls = child.EffectContext, child.StatusCalls, child.AreaStatusCalls
		return err
	case "npc-job-97-position-9":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "life-reduction", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			t := s.actor(r.TargetID)
			if t.MaxHP <= 999 && t.BaseStats.Defense <= 999 {
				if _, err := s.ApplyStatDown(&c.EffectContext, controlEffect(r.TargetID, .75, CategoryMagic), StatHP, rng); err != nil {
					return err
				}
				s.actor(r.TargetID).Status = StatusDeadlyPoison
			}
		}
	}
	return nil
}

package battle

import "github.com/witchcraze/party2re/internal/core/random"

// NPCStatusContext supplies intrinsic job88/90–94 bodies. Selection, listed
// charges and durable callback deduplication remain with the ordinary consumer.
// Damage's Reset/Defeat ports retain their complete-admission contract.
type NPCStatusContext struct {
	DamageContext
	Death                                                    *DefeatContext
	Arguments                                                []string
	StatusCalls, AreaStatusCalls, DeathCalls, AreaDeathCalls int
}

func (c *NPCStatusContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// ApplyNPCStatus admits the full body before arguments, random draws or writes.
// Accepted no-effect helpers still retain each source body's explicit tails.
func (s *LifetimeState) ApplyNPCStatus(c *NPCStatusContext, id string, rng random.Generator) error {
	if err := s.ValidateNPCStatus(c, id, rng); err != nil {
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
	case "npc-job-88-position-0":
		return s.npcStatus(c, controlSymbol("", StatusDofuu, CategoryPhysical), 60, true, false, rng)
	case "npc-job-88-position-1":
		coefficient := 1.5
		if a.Gear.WeaponID != "" {
			coefficient = .8
		}
		_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(a.Attack)*coefficient, CategoryPhysical, true), rng)
		return err
	case "npc-job-88-position-2":
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "grooming", CategoryUntyped), rng)
		if err == nil {
			resources(r)
		}
		return err // The source reads the resolved weapon but writes nothing.
	case "npc-job-88-position-3":
		return nil // Only unused locals are computed by the original body.
	case "npc-job-90-position-0", "npc-job-90-position-1", "npc-job-91-position-0", "npc-job-91-position-1", "npc-job-92-position-1":
		power, category, direct, status, chance := npcStatusStrike(id, a.Attack)
		c.AdditionalEffect = true
		hit, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, category, direct), rng)
		if err != nil || !hit.Returned || hit.TargetID == "" || s.actor(hit.TargetID).HP <= 0 {
			return err
		}
		return s.npcStatus(c, controlSymbol(hit.TargetID, status, category), chance, false, false, rng)
	case "npc-job-90-position-2":
		return s.npcStatus(c, controlSymbol("", StatusDeadlyPoison, CategoryBreath), 60, true, false, rng)
	case "npc-job-91-position-2":
		return s.npcStatus(c, controlSymbol("", StatusParalyze, CategoryBreath), 35, true, false, rng)
	case "npc-job-92-position-0":
		return s.npcStatus(c, controlSymbol(c.shift(), StatusSleep, CategoryMagic), 65, false, false, rng)
	case "npc-job-92-position-2":
		return s.npcStatus(c, controlSymbol("", StatusSleep, CategoryBreath), 35, true, false, rng)
	case "npc-job-93-position-0":
		return s.npcStatus(c, controlSymbol(c.shift(), "death", CategoryMagic), 20, false, true, rng)
	case "npc-job-93-position-1":
		return s.npcStatus(c, controlSymbol("", "death", CategoryMagic), 20, true, true, rng)
	case "npc-job-93-position-2":
		return s.npcStatus(c, controlSymbol("", "death", CategoryDance), 17, true, true, rng)
	case "npc-job-93-position-3":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "life-reduction", CategoryMagic), rng)
		if err != nil {
			return err
		}
		resources(r)
		if r.Resolved {
			t := s.actor(r.TargetID)
			if t.MaxHP <= 999 && t.BaseStats.Defense <= 999 && rng.Float64() < .5 {
				if _, err := s.ApplyStatDown(&c.EffectContext, controlEffect(r.TargetID, .75, CategoryMagic), StatHP, rng); err != nil {
					return err
				}
				s.actor(r.TargetID).Status = StatusDeadlyPoison
			}
		}
	case "npc-job-94-position-0":
		if err := s.npcStatus(c, controlSymbol("", "death", CategoryUntyped), 60, true, true, rng); err != nil {
			return err
		}
		// Defeat sees the living actor so automatic revival may prevent sacrifice.
		c.Death.TargetContext = c.TargetContext
		if _, err := s.ApplyDefeat(c.Death, c.ActorID, rng); err != nil {
			return err
		}
		s.actor(c.ActorID).MP = 1
	case "npc-job-94-position-1":
		a.Status = StatusSleep
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, float64(a.MaxHP)*.5, CategoryUntyped), rng)
		return err
	}
	return nil
}

func npcStatusStrike(id string, attack int) (float64, CallbackCategory, bool, string, float64) {
	switch id {
	case "npc-job-90-position-0":
		return float64(attack) * 1.1, CategoryPhysical, false, StatusDeadlyPoison, 70
	case "npc-job-90-position-1":
		return 25, CategoryMagic, true, StatusDeadlyPoison, 80
	case "npc-job-91-position-0":
		return float64(attack) * 1.2, CategoryPhysical, false, StatusParalyze, 30
	case "npc-job-91-position-1":
		return float64(attack) * .8, CategoryPhysical, false, StatusParalyze, 40
	default:
		return float64(attack) * .8, CategoryPhysical, false, StatusSleep, 55
	}
}

func (s *LifetimeState) npcStatus(c *NPCStatusContext, e TargetEffect, chance float64, area, death bool, rng random.Generator) error {
	status := StatusContext{EffectContext: c.EffectContext, Defeat: c.Death, StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls, DeathCalls: c.DeathCalls, AreaDeathCalls: c.AreaDeathCalls}
	var err error
	if area {
		_, err = s.ApplyAreaStatus(&status, e, chance, false, death, rng)
	} else {
		_, err = s.ApplyStatus(&status, e, chance, false, death, rng)
	}
	c.EffectContext = status.EffectContext
	c.StatusCalls, c.AreaStatusCalls, c.DeathCalls, c.AreaDeathCalls = status.StatusCalls, status.AreaStatusCalls, status.DeathCalls, status.AreaDeathCalls
	return err
}

package battle

import "github.com/witchcraze/party2re/internal/core/random"

// NPCJob402Context retains intrinsic callback facts. Rebuild native defeat/reset
// ports after reload; NPC selection, listed costs and receipts stay with callers.
type NPCJob402Context struct {
	DamageContext
	Death     *DefeatContext
	Arguments []string
}

func (c *NPCJob402Context) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func npc402SpeedCount(actor, target *LifetimeActor) int {
	agility := 0
	if target != nil {
		agility = target.Agility
	}
	return min(9, max(1, (actor.Agility-agility)/35))
}

func (s *LifetimeState) npc402SharedPower(c *NPCJob402Context) float64 {
	add := 0
	for _, id := range c.PartyIDs {
		if t := s.actor(id); id != c.ActorID && t.MP >= 5 {
			t.MP -= 5
			add += 200
		}
	}
	a := s.actor(c.ActorID)
	return float64(a.Attack+a.Defense+a.Agility)*.6 + float64(add)
}

// ApplyNPCJob402 admits the entire ordered body before arguments, resources,
// counters, external hooks or committed random draws change.
func (s *LifetimeState) ApplyNPCJob402(c *NPCJob402Context, id string, rng random.Generator) error {
	if err := s.ValidateNPCJob402(c, id, rng); err != nil {
		return err
	}
	previous := c.DamageContext
	native := s.nativeDamageContext(c.DamageContext, c.Death, rng)
	c.Reset, c.Defeat = native.Reset, native.Defeat
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() {
		c.Reset, c.Defeat = previous.Reset, previous.Defeat
		c.targetResources, c.EffectContext.targetResources = previous.targetResources, previous.EffectContext.targetResources
	}()
	switch id {
	case "npc-job-402-position-0":
		for range 3 {
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), 110, CategoryMagic, true), rng); err != nil {
				return err
			}
		}
	case "npc-job-402-position-1":
		for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
			if _, err := s.ApplyPartyStat(&c.EffectContext, .5, CategoryMagic, stat, false, rng); err != nil {
				return err
			}
		}
		s.actor(c.ActorID).Status = StatusDofuu
	case "npc-job-402-position-2":
		first, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(s.actor(c.ActorID).Defense)*.6, CategoryMagic, true), rng)
		if err != nil {
			return err
		}
		second, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(s.actor(c.ActorID).Attack)*1.2, CategoryPhysical, false), rng)
		if err != nil {
			return err
		}
		for _, r := range []EffectApplication{first, second} {
			draw := rng.Float64()
			if t := s.actor(r.TargetID); draw < .5 && r.Returned && t != nil && t.HP > 0 && t.Status == "" {
				t.Status = StatusMagicSeal
			}
		}
	case "npc-job-402-position-3", "npc-job-402-position-4":
		if id == "npc-job-402-position-4" {
			for _, target := range c.EnemyIDs {
				s.actor(target).Temporary = ""
			}
		}
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), "ordered-strike", CategoryUntyped), rng)
		if err != nil {
			return err
		}
		resources(r)
		target := ""
		if r.Resolved {
			target = r.TargetID
		}
		if id == "npc-job-402-position-4" {
			power := 5.0
			if t := s.actor(target); t != nil {
				power += float64(t.Defense) * .8
			}
			_, err := s.ApplyAreaDamage(&c.DamageContext, power, CategoryUntyped, true, rng)
			return err
		}
		v := npc402SpeedCount(s.actor(c.ActorID), s.actor(target))
		for range v + 1 {
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, float64(s.actor(c.ActorID).Attack)*.4, CategoryPhysical, false), rng); err != nil {
				return err
			}
		}
		if v == 9 {
			_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, float64(s.actor(c.ActorID).Attack), CategoryPhysical, true), rng)
			return err
		}
	case "npc-job-402-position-5":
		power := s.npc402SharedPower(c)
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, CategoryUntyped, false), rng)
		if err != nil {
			return err
		}
		target := ""
		if r.Returned {
			target = r.TargetID
		}
		if _, err := s.ApplyStatDown(&c.EffectContext, controlEffect(target, .2, CategoryUntyped), StatCMP, rng); err != nil {
			return err
		}
		// A missing damage tuple still invokes stat-down with an absent target;
		// the source's empty-record charge writes do not address a participant.
		if t := s.actor(target); t != nil {
			t.CMPCharge, t.CMPNeedCharge = 0, 6
		}
	}
	return nil
}

package battle

import "github.com/witchcraze/party2re/internal/core/random"

// NPCJob405Context retains intrinsic callback facts. Rebuild native defeat/reset
// ports after reload; NPC selection, listed costs and receipts stay with callers.
type NPCJob405Context struct {
	DamageContext
	Death     *DefeatContext
	Arguments []string
}

func (c *NPCJob405Context) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

// NPCJob405Wounded selects the table variant. The source builds the table
// before NPC start-of-action cures and poison, so callers read HP at selection.
func NPCJob405Wounded(a *LifetimeActor) bool {
	return float64(a.HP) <= float64(a.MaxHP)*.5
}

var npc405Statuses = [...]StatusEffect{StatusMagicSeal, StatusPhysicalSeal, StatusDanceSeal, StatusDofuu}

// npc405Members counts every other battle member, on both sides and fallen.
func (s *LifetimeState) npc405Members(actorID string) int {
	n := 0
	for _, a := range s.Actors {
		if a.ID != actorID {
			n++
		}
	}
	return n
}

// ApplyNPCJob405 admits the entire ordered body before arguments, resources,
// counters, external hooks or committed random draws change.
func (s *LifetimeState) ApplyNPCJob405(c *NPCJob405Context, id string, rng random.Generator) error {
	if err := s.ValidateNPCJob405(c, id, rng); err != nil {
		return err
	}
	previous := c.DamageContext
	native := s.nativeDamageContext(c.DamageContext, c.Death, rng)
	c.Reset, c.Defeat = native.Reset, native.Defeat
	defer s.ownTargetResources(&c.EffectContext)()
	defer func() {
		c.Reset, c.Defeat = previous.Reset, previous.Defeat
	}()
	switch id {
	case "npc-job-405-position-0", "npc-job-405-position-0-wounded":
		hits := 4
		if id == "npc-job-405-position-0-wounded" {
			hits = 5
		}
		for range hits {
			if _, err := s.ApplyAreaDamage(&c.DamageContext, 80+rng.Float64()*30, CategoryMagic, true, rng); err != nil {
				return err
			}
			// Unguarded: fallen enemies and existing statuses are overwritten.
			for _, target := range c.EnemyIDs {
				if rng.Float64() < .25 {
					s.actor(target).Status = StatusMagicSeal
				}
			}
		}
	case "npc-job-405-position-1":
		for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
			if _, err := s.ApplyPartyStat(&c.EffectContext, .6, CategoryMagic, stat, false, rng); err != nil {
				return err
			}
		}
		a := s.actor(c.ActorID)
		a.Status = StatusDofuu
		a.Temporary = [...]TemporaryEffect{TemporaryReflectAttack, TemporaryReflectMagic, TemporaryReflectBreath, TemporaryReflectDance}[int(rng.Float64()*4)]
	case "npc-job-405-position-2":
		first, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(s.actor(c.ActorID).Defense)*.6, CategoryMagic, true), rng)
		if err != nil {
			return err
		}
		second, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(s.actor(c.ActorID).Attack)*1.3, CategoryPhysical, false), rng)
		if err != nil {
			return err
		}
		seal := func(r EffectApplication) {
			if t := s.actor(r.TargetID); r.Returned && t != nil && t.HP > 0 && t.Status == "" {
				t.Status = npc405Statuses[int(rng.Float64()*4)]
			}
		}
		seal(first)
		if rng.Float64()*1.5 < 1 {
			seal(second)
		}
	case "npc-job-405-position-3":
		return s.applyNPC405Renewal(c, rng)
	case "npc-job-405-position-4":
		for _, target := range c.EnemyIDs {
			npc405Cleave(s.actor(target))
		}
		_, err := s.ApplyAreaDamage(&c.DamageContext, 300, CategoryUntyped, true, rng)
		return err
	case "npc-job-405-position-4-wounded":
		for _, target := range c.EnemyIDs {
			npc405Cleave(s.actor(target))
			if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, float64(int(500+rng.Float64()*300)), CategoryUntyped, true), rng); err != nil {
				return err
			}
		}
	case "npc-job-405-position-5":
		a := s.actor(c.ActorID)
		power := float64(a.Attack+a.Defense+a.Agility)*.7 + float64(200*s.npc405Members(c.ActorID))
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, CategoryUntyped, false), rng)
		if err != nil {
			return err
		}
		return s.npc405DrainCharge(c, r, .3, rng)
	case "npc-job-405-position-5-wounded":
		return s.applyNPC405Chain(c, rng)
	}
	return nil
}

func npc405Cleave(t *LifetimeActor) {
	t.Temporary = ""
	t.Defense = min(t.Defense, t.BaseStats.Defense)
}

// A missing damage tuple still invokes stat-down with an absent target; the
// charge writes then address no participant.
func (s *LifetimeState) npc405DrainCharge(c *NPCJob405Context, r EffectApplication, coefficient float64, rng random.Generator) error {
	target := ""
	if r.Returned {
		target = r.TargetID
	}
	if _, err := s.ApplyStatDown(&c.EffectContext, controlEffect(target, coefficient, CategoryUntyped), StatCMP, rng); err != nil {
		return err
	}
	if t := s.actor(target); t != nil {
		t.CMPCharge, t.CMPNeedCharge = 0, 10
	}
	return nil
}

// applyNPC405Chain strikes a random living enemy up to once per original enemy.
// The carried power is scaled by the tension read before each hit.
func (s *LifetimeState) applyNPC405Chain(c *NPCJob405Context, rng random.Generator) error {
	a := s.actor(c.ActorID)
	damage := float64(a.Attack+a.Defense+a.Agility) + float64(300*s.npc405Members(c.ActorID))
	for count := 1; ; count++ {
		var living []string
		for _, id := range c.EnemyIDs {
			if s.actor(id).HP > 0 {
				living = append(living, id)
			}
		}
		carried := damage * s.actor(c.ActorID).Tension
		target := ""
		// An empty list still consumes the draw and leaves the target unspecified.
		if i := int(rng.Float64() * float64(len(living))); i < len(living) {
			target = living[i]
		}
		r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, damage, CategoryUntyped, false), rng)
		if err != nil {
			return err
		}
		if err := s.npc405DrainCharge(c, r, .5, rng); err != nil {
			return err
		}
		alive := false
		for _, id := range c.EnemyIDs {
			alive = alive || s.actor(id).HP > 0
		}
		damage = float64(int(carried * .8))
		if s.actor(c.ActorID).HP <= 0 || !alive || count >= len(c.EnemyIDs) {
			return nil
		}
	}
}

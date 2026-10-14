package battle

import "github.com/witchcraze/party2re/internal/core/random"

// modifyDamage is the ordered damage branch of _ex_effect:9948–10129.
// nil target is the area pre-evaluation's absent target, not the actor.
func (s *LifetimeState) modifyDamage(c *DamageContext, t *LifetimeActor, v float64, category CallbackCategory, direct, normal bool, rng random.Generator) float64 {
	a := s.actor(c.ActorID)
	item := func(id string) bool { return hasItem(a.ItemDefinitionIDs, id) }
	weapon := item
	v = initialDamagePower(a, t, v, category, direct)
	if t != nil {
		if item("item-192") && t.Temporary != "" {
			v = float64(int(v * (1.5 + rng.Float64()*.3)))
		}
		if t.Status == StatusKinju {
			v *= 1.25
		}
		if hasItem(t.ItemDefinitionIDs, "item-190") && rng.Float64() < 1.0/6 {
			v *= .5
		}
	}
	field := s.Fields.Current()
	if field == "cosmic" && category == CategoryMagic {
		v *= 1.2
	}
	if t != nil {
		if (t.Temporary == TemporaryDefense || t.Temporary == TemporaryStrongDefense || t.Temporary == TemporaryOneHitDefense) &&
			(hasItem(t.ItemDefinitionIDs, "item-161") || hasItem(t.ItemDefinitionIDs, "item-162") || hasItem(t.ItemDefinitionIDs, "item-163")) {
			v *= .6
		}
		if hasItem(t.ItemDefinitionIDs, "item-204") && t.Status != "" {
			v *= 1.2
		}
		if hasItem(t.ItemDefinitionIDs, "item-209") && t.Temporary == TemporaryCovering {
			v *= .3
		}
	}
	if weapon("weapon-34") && category == CategoryMagic && rng.Float64() < .25 {
		v *= 1.2
	}
	if weapon("weapon-35") && category == CategoryPhysical && rng.Float64() < .25 {
		v *= 1.2
	}
	if weapon("weapon-36") && category == CategoryBreath {
		v *= 1.2
	}
	if (weapon("weapon-37") || weapon("weapon-63")) && rng.Float64() < .125 {
		v *= 1.5
	}
	if weapon("weapon-68") && float64(a.HP) <= float64(a.MaxHP)*.3 {
		v *= (float64(a.MaxHP)*1.8 - float64(a.HP)) / float64(a.MaxHP)
	}
	if t != nil && hasItem(t.ItemDefinitionIDs, "item-200") && category == CategoryPhysical && !direct && !normal {
		v *= 1.2
	}
	if a.Extended == ExtendedWarGod && category == CategoryPhysical && !direct && !normal {
		v *= 1.3
	}
	if item("item-239") && c.AreaDamageCalls == 0 {
		switch {
		case c.DamageCalls == 2:
			v *= 1.2
		case c.DamageCalls == 3:
			v *= 1.4
		case c.DamageCalls >= 4:
			v *= 1.6
		}
	}
	if field == "burn" {
		v *= 1.2
	}
	if field == "darkness" && category == CategoryPhysical && !normal {
		v *= 1.3
	}
	if c.Ordered {
		v *= 1.1
	}
	if t != nil && t.Extended == ExtendedPurification {
		v *= .8
	}
	if a.Extended == ExtendedFlame && category == CategoryPhysical && !direct {
		v *= .9
	}
	if field == "sanctuary" && category == CategoryMagic {
		v *= 1.2
	}
	if c.Item185Activated {
		v *= .75
	}
	if c.BowEffect {
		v *= 1.5
	}
	if item("item-190") && category == CategoryDance {
		v *= 1.5
	}
	if item("item-144") && c.AreaDamageCalls > 0 && v < 300 {
		v *= 1.3
	}
	return v
}

// The last truncations which can make a positive modifier exactly zero.
func initialDamagePower(a, t *LifetimeActor, v float64, category CallbackCategory, direct bool) float64 {
	if hasItem(a.ItemDefinitionIDs, "item-149") {
		if direct {
			v *= 1.1
		} else {
			v *= 1.5
		}
	}
	if hasItem(a.ItemDefinitionIDs, "item-143") && category == CategoryMagic {
		threshold := v
		if t != nil && t.Temporary == TemporaryDouble {
			threshold *= .5
		}
		if threshold < 300 {
			v = float64(int(v * 2))
		}
	}
	if hasItem(a.ItemDefinitionIDs, "item-189") && category == CategoryBreath {
		v = float64(int(v * 1.5))
	}
	return v
}

func damageHasPower(a, t *LifetimeActor, e TargetEffect) bool {
	direct, _ := damageFlags(e)
	power := initialDamagePower(a, t, e.Payload.Number, e.Category, direct)
	if hasItem(a.ItemDefinitionIDs, "item-192") && t.Temporary != "" {
		power = float64(int(power * 1.8))
	}
	return power != 0
}

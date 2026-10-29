package battle

import "github.com/witchcraze/party2re/internal/core/random"

// TreatmentChanceContext shares operation facts with the existing effect and
// defeat owners. Consumers restore facts and rebuild ports after reload.
type TreatmentChanceContext struct {
	DamageContext
	Arguments []string
	Status    StatusContext
}

func (c *TreatmentChanceContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func treatmentChanceCure(id string) (string, CallbackCategory) {
	switch id {
	case "job-43-entry-0":
		return StatusDeadlyPoison, CategoryUntyped
	case "job-43-entry-1":
		return StatusParalyze, CategoryUntyped
	case "job-43-entry-2":
		return StatusSleep, CategorySong
	}
	return "", CategoryUntyped
}

// ApplyTreatmentChance executes intrinsic bodies only. Ordinary listed costs
// and indirect cost restoration stay with the existing invocation boundary.
func (s *LifetimeState) ApplyTreatmentChance(c *TreatmentChanceContext, id string, rng random.Generator) error {
	if err := s.ValidateTreatmentChance(c, id, rng); err != nil {
		return err
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	c.Status.EffectContext = c.EffectContext
	defer func() { c.Status.EffectContext = c.EffectContext }()
	if symbol, category := treatmentChanceCure(id); symbol != "" {
		_, err := s.ApplyPartyCure(&c.Status, controlSymbol("", symbol, category), rng)
		c.EffectContext = c.Status.EffectContext
		return err
	}
	switch id {
	case "job-43-entry-3", "job-43-entry-4", "job-45-entry-0", "job-45-entry-5":
		category, symbol := CategoryMagic, "regeneration"
		if id == "job-43-entry-4" || id == "job-45-entry-0" {
			category, symbol = CategoryUntyped, "cure"
		}
		if id == "job-45-entry-0" {
			symbol = "accuracy"
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
		case "job-43-entry-4":
			c.Status.EffectContext = c.EffectContext
			_, err = s.ApplyCure(&c.Status, controlSymbol(t.ID, t.Status, CategoryMagic), rng)
			c.EffectContext = c.Status.EffectContext
			return err
		case "job-45-entry-0":
			t.Hit = 95
		default:
			t.Temporary = TemporaryRegeneration
		}
	case "job-43-entry-5":
		_, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.shift(), 400, CategoryMagic), rng)
		return err
	case "job-45-entry-1", "job-46-entry-1":
		category, symbol, chance, death := CategoryMagic, StatusDofuu, 85.0, false
		if id == "job-46-entry-1" {
			category, symbol, chance, death = CategoryPhysical, "death", 20, true
		}
		_, err := s.ApplyStatus(&c.Status, controlSymbol(c.shift(), symbol, category), chance, false, death, rng)
		c.EffectContext = c.Status.EffectContext
		return err
	case "job-45-entry-2":
		_, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, .5, CategoryUntyped), StatDefense, rng)
		if err == nil {
			s.actor(c.ActorID).Temporary = TemporaryReduceMagic
		}
		return err
	case "job-45-entry-3":
		r, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), .18, CategoryDance), StatMP, rng)
		if err != nil || !r.Returned {
			return err
		}
		_, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount), CategoryDance), rng)
		return err
	case "job-45-entry-4":
		r, err := s.ApplyStatDown(&c.EffectContext, controlEffect(c.shift(), .4, CategorySong), StatAttack, rng)
		if err != nil || !r.Returned {
			return err
		}
		if _, err := s.ApplyStatDown(&c.EffectContext, controlEffect(r.TargetID, .4, CategorySong), StatDefense, rng); err != nil {
			return err
		}
		s.actor(r.TargetID).Icon = "assets/images/placeholders/battle/frog.svg"
	case "job-45-entry-6":
		_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), 300, CategoryPhysical, true), rng)
		return err
	case "job-45-entry-7":
		allowed, err := s.actor(c.ActorID).CheckCategorySeal(CategoryDance, rng)
		if err != nil || !allowed {
			return err
		}
		for _, target := range c.PartyIDs {
			if err := s.statTension(&c.EffectContext, s.actor(target), false, rng); err != nil {
				return err
			}
			t := s.actor(target)
			if t.HP <= t.MP {
				_, err = s.ApplyHealing(&c.EffectContext, controlEffect(target, 100, CategoryDance), rng)
			} else {
				_, err = s.ApplyMPHealing(&c.EffectContext, controlEffect(target, 10+rng.Float64()*30, CategoryDance), rng)
			}
			if err != nil {
				return err
			}
		}
	case "job-46-entry-0":
		x, y, z := int(rng.Float64()*5), int(rng.Float64()*5), int(rng.Float64()*5)
		if x != y || y != z {
			_, err := s.ApplyAreaDamage(&c.DamageContext, float64(7*(x+y+z)), CategoryUntyped, true, rng)
			return err
		}
		power := float64((x + 2) * 100)
		if x == 4 {
			_, err := s.ApplyAreaStatus(&c.Status, controlSymbol("", "death", CategoryUntyped), 90, false, true, rng)
			c.EffectContext = c.Status.EffectContext
			return err
		}
		if x == 0 || x == 2 {
			_, err := s.ApplyPartyHealing(&c.EffectContext, power, CategoryUntyped, rng)
			return err
		}
		_, err := s.ApplyAreaDamage(&c.DamageContext, power, CategoryUntyped, false, rng)
		return err
	case "job-46-entry-2", "job-46-entry-4":
		sides := 6.0
		if id == "job-46-entry-4" {
			sides = 3
		}
		x, y := int(rng.Float64()*sides)+1, int(rng.Float64()*6)+1
		_ = rng.Float64() // The third source die is drawn but never used in either formula.
		power := x*100 + y*10 + x
		if id == "job-46-entry-2" {
			power /= 2
		}
		if _, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), float64(power), CategoryMagic, true), rng); err != nil {
			return err
		}
		if id == "job-46-entry-2" {
			_, err := s.ApplyRecoil(&c.EffectContext, &c.End, float64((6-x)*10+(6-y))+float64(6-x)*.1, rng)
			return err
		}
	case "job-46-entry-3":
		// Source31.1 has the same raw roulette body; supply its admitted defeat.
		child := TimeMixedMagicContext{DamageContext: c.DamageContext}
		child.Defeat = func(target string) error {
			c.Status.Defeat.TargetContext = c.TargetContext
			_, err := s.ApplyDefeat(c.Status.Defeat, target, rng)
			return err
		}
		return s.ApplyTimeMixedMagic(&child, "job-31-entry-1", rng)
	case "job-46-entry-5":
		if rng.Float64() >= .5 {
			a := s.actor(c.ActorID)
			a.HP = max(1, a.HP-a.HP/2)
			return nil
		}
		for _, target := range c.EnemyIDs {
			t := s.actor(target)
			switch {
			case t.HP <= 999 && t.BaseStats.Defense <= 999:
				c.Status.EffectContext = c.EffectContext
				_, err := s.ApplyStatus(&c.Status, controlSymbol(target, "death", CategoryUntyped), 60, false, true, rng)
				c.EffectContext = c.Status.EffectContext
				if err != nil {
					return err
				}
			case rng.Float64()*10 > 7:
				continue
			case t.HP > 999:
				power := min(1800, t.MaxHP/2)
				damage := s.coinDamageContext(c, rng)
				reset, defeat := c.Reset, c.Defeat
				c.Reset, c.Defeat = damage.Reset, damage.Defeat
				_, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(target, float64(power), CategoryUntyped, true), rng)
				c.Reset, c.Defeat = reset, defeat
				if err != nil {
					return err
				}
			default:
				t.HP = max(1, t.HP-t.HP/2)
			}
		}
	}
	return nil
}

// Coin damage and its admission share the same native reset/defeat owner.
func (s *LifetimeState) coinDamageContext(c *TreatmentChanceContext, rng random.Generator) DamageContext {
	damage := c.DamageContext
	damage.Reset = func(target string) error {
		if c.Status.Defeat == nil {
			return ErrUnsupportedEffect
		}
		c.Status.Defeat.TargetContext = c.TargetContext
		return s.ApplyEquipmentReset(&c.Status.Defeat.EquipmentResetContext, target, rng)
	}
	damage.Defeat = func(target string) error {
		if c.Status.Defeat == nil {
			return ErrUnsupportedEffect
		}
		c.Status.Defeat.TargetContext = c.TargetContext
		_, err := s.ApplyDefeat(c.Status.Defeat, target, rng)
		return err
	}
	return damage
}

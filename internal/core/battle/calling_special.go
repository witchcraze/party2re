package battle

import (
	"strconv"
	"strings"

	"github.com/witchcraze/party2re/internal/core/random"
)

func callingAppearance(h *LifetimeActor, job int) int {
	for form := range 4 {
		if h.Icon == "assets/images/placeholders/battle/helpers.svg#"+callingIdentity(job)+"-"+strconv.Itoa(form) {
			return form
		}
	}
	return -1
}

func callingArcher(h *LifetimeActor) bool {
	form := callingAppearance(h, 80)
	if form == 0 || form == 2 {
		return true
	}
	return strings.HasPrefix(h.Icon, nativeFormIcon("job-55", "m"))
}

// CallingRandomReachable is the source's living-helper/appearance guard, before
// any intrinsic effects. The invocation owner admits the complete random pool.
func (s *LifetimeState) CallingRandomReachable(c TargetContext) bool {
	h := s.actor(s.FindCallingHelper(c, 80))
	return h != nil && h.HP > 0 && !callingArcher(h)
}

func callingStatusContext(c *RosterContext) StatusContext {
	return StatusContext{EffectContext: c.EffectContext, Defeat: c.Death,
		StatusCalls: c.StatusCalls, AreaStatusCalls: c.AreaStatusCalls,
		DeathCalls: c.DeathCalls, AreaDeathCalls: c.AreaDeathCalls}
}

func (s *LifetimeState) applyCallingStatus(c *RosterContext, e TargetEffect, chance float64, area, death bool, rng random.Generator) (bool, error) {
	child := callingStatusContext(c)
	var hits []StatusApplication
	var err error
	if area {
		hits, err = s.ApplyAreaStatus(&child, e, chance, false, death, rng)
	} else {
		var hit StatusApplication
		hit, err = s.ApplyStatus(&child, e, chance, false, death, rng)
		hits = []StatusApplication{hit}
	}
	c.EffectContext = child.EffectContext
	c.StatusCalls, c.AreaStatusCalls = child.StatusCalls, child.AreaStatusCalls
	c.DeathCalls, c.AreaDeathCalls = child.DeathCalls, child.AreaDeathCalls
	applied := false
	for _, hit := range hits {
		applied = applied || hit.Applied
	}
	return applied, err
}

func (s *LifetimeState) applyCallingBody(c *RosterContext, job int, helperID, targetID string, rng random.Generator) (RosterResult, error) {
	r := RosterResult{HelperID: helperID}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(v TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-v.MPDebit)
		applyStatusResources(s, v)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	status := func(value string, category CallbackCategory, chance float64, area, death bool) error {
		r.TargetConsumed = !area
		applied, err := s.applyCallingStatus(c, controlSymbol(targetID, value, category), chance, area, death, rng)
		r.Applied = r.Applied || applied
		return err
	}
	hit := func(power float64, category CallbackCategory, direct bool) (EffectApplication, error) {
		r.TargetConsumed = true
		v, err := s.ApplyDamage(&c.DamageContext, rosterEffect(targetID, power, category, direct), rng)
		r.Effects = append(r.Effects, v)
		return v, err
	}
	area := func(power float64, category CallbackCategory) error {
		v, err := s.ApplyAreaDamage(&c.DamageContext, power, category, true, rng)
		r.Effects = append(r.Effects, v...)
		return err
	}
	h := s.actor(helperID)
	var err error
	switch job {
	case 61:
		switch {
		case rng.Float64()*2 < 1:
			err = status("instant-death", CategoryMagic, 22, true, true)
		case rng.Float64()*2 < 1:
			c.AdditionalEffect = true
			if err = area(120, CategoryMagic); err == nil {
				err = status(StatusDeadlyPoison, CategoryMagic, 40, true, false)
			}
		default:
			err = status(StatusDeadlyPoison, CategoryBreath, 70, true, false)
		}
	case 62:
		switch {
		case rng.Float64()*2 < 1:
			var v EffectApplication
			v, err = hit(100, CategoryMagic, true)
			if err == nil && v.Returned && v.TargetID != "" {
				v, err = s.ApplyHealing(&c.EffectContext, rosterEffect(helperID, float64(v.Amount), CategoryMagic, false), rng)
				r.Effects = append(r.Effects, v)
			}
		case rng.Float64()*2 < 1:
			r.TargetConsumed = true
			var v EffectApplication
			v, err = s.ApplyStatDown(&c.EffectContext, rosterEffect(targetID, .3, CategoryMagic, false), StatMP, rng)
			r.Effects = append(r.Effects, v)
			if err == nil && v.Returned && v.TargetID != "" {
				v, err = s.ApplyMPHealing(&c.EffectContext, rosterEffect(c.ActorID, float64(v.Amount), CategoryMagic, false), rng)
				r.Effects = append(r.Effects, v)
			}
		default:
			err = status(StatusConfusion, CategorySong, 70, true, false)
		}
	case 63:
		value, chance := StatusSleep, 50.0
		if rng.Float64()*2 < 1 {
			value, chance = StatusDeadlyPoison, 90
		} else if rng.Float64()*2 < 1 {
			value = StatusParalyze
		}
		err = status(value, CategoryBreath, chance, true, false)
	case 64:
		switch {
		case rng.Float64()*2 < 1:
			r.TargetConsumed = true
			child := ControlClimaxContext{DamageContext: c.DamageContext, Arguments: []string{targetID}}
			err = s.ApplyControlClimax(&child, "job-53-entry-5", rng)
			c.DamageContext = child.DamageContext
			r.Applied = err == nil
		case rng.Float64()*2 < 1:
			err = status(StatusPhysicalSeal, CategoryUntyped, 80, false, false)
		default:
			err = status(StatusDofuu, CategoryPhysical, 60, true, false)
		}
	case 66:
		switch {
		case rng.Float64()*2 < 1:
			err = status("instant-death", CategoryDance, 20, true, true)
		case rng.Float64()*2 < 1:
			err = status("instant-death", CategoryPhysical, 40, false, true)
		default:
			for _, id := range c.EnemyIDs {
				value := []string{StatusPhysicalSeal, StatusDofuu}[int(rng.Float64()*2)]
				applied, callErr := s.applyCallingStatus(c, controlSymbol(id, value, CategoryMagic), 75, false, false, rng)
				r.Applied = r.Applied || applied
				if callErr != nil {
					err = callErr
					break
				}
			}
		}
	case 67:
		switch callingAppearance(h, 67) {
		case 0:
			err = status(StatusDeadlyPoison, CategoryBreath, 80, true, false)
		case 1:
			err = area(220, CategoryBreath)
		default:
			c.AdditionalEffect = true
			if err = area(150, CategoryMagic); err == nil {
				err = status(StatusParalyze, CategoryMagic, 20, true, false)
			}
		}
	case 71:
		switch {
		case rng.Float64()*3 < 1:
			_, err = hit(float64(h.Attack)*1.4, CategoryPhysical, false)
		case rng.Float64()*2 < 1:
			r.TargetConsumed = true
			var v EffectApplication
			v, err = s.ApplyHealing(&c.EffectContext, rosterEffect(targetID, 150, CategoryMagic, false), rng)
			r.Effects = append(r.Effects, v)
		default:
			err = status(StatusDofuu, CategoryDance, 80, true, false)
		}
	case 80:
		return s.applyCallingFairy(c, helperID, rng)
	case 87:
		switch {
		case rng.Float64()*3 < 1:
			_, err = hit(float64(h.Agility)*2, CategoryPhysical, false)
		case rng.Float64()*2 < 1:
			_, err = hit(float64(h.Attack)*1.2, CategoryPhysical, false)
		default:
			var v EffectApplication
			v, err = s.ApplyStatUp(&c.EffectContext, rosterEffect(helperID, 1, CategoryPhysical, false), StatAttack, rng)
			r.Effects = append(r.Effects, v)
			if err == nil {
				err = status(StatusDofuu, CategoryUntyped, 75, false, false)
			}
		}
		if err == nil {
			if form := callingAppearance(s.actor(helperID), 87); form >= 0 && form < 2 {
				s.actor(helperID).SummonForm = form
				s.growHelper(c, helperID, 3, rng)
			}
		}
	default:
		err = ErrUnsupportedEffect
	}
	for _, v := range r.Effects {
		r.Applied = r.Applied || v.Applied
	}
	return r, err
}

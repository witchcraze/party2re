package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// Admission uses the same fixed coefficients as execution. Multi-hit entries
// use an upper bound here; execution draws their individual powers below.
func damageGemBase(a *LifetimeActor, id int) (TargetEffect, bool) {
	e := TargetEffect{Category: CategoryPhysical}
	area := false
	switch id {
	case 1, 5:
		e.Payload.Number = float64(a.Attack) * 1.4
	case 2:
		e.Payload.Number = float64(a.Attack) * 1.8
	case 3, 4:
		e.Payload.Number = float64(a.Attack) * 2.5
	case 6, 7, 9:
		e.Payload.Number = float64(a.Attack) * .8
	case 8:
		e.Payload.Number = float64(a.Attack) * .9
	case 10, 11, 12:
		e.Payload.Number = float64(a.Attack) * map[int]float64{10: .8, 11: 1, 12: 1.6}[id]
		area = true
	case 13, 14, 15, 16, 17, 18:
		e.Payload.Number = map[int]float64{13: 50, 14: 200, 15: 500, 16: 50, 17: 200, 18: 150}[id]
		e.Category = CategoryMagic
		area = id >= 16
	case 26:
		e.Category = CategoryMagic
	case 72:
		e.Category = CategoryMagic
		e.Payload.Number = float64(a.HP)
	case 73:
		e.Payload.Number = float64(a.MaxHP-a.HP+5) * .75
	}
	if id == 9 {
		e.Category = CategoryDance
	}
	if id == 18 {
		e.Category = CategoryBreath
	}
	if id == 5 || id == 8 || id >= 13 {
		e.Arguments = []string{"1"}
	}
	return e, area
}

func damageGemHitRange(sourceID int) (base, width int) {
	base, width = 3, 2
	switch sourceID {
	case 7:
		base = 7
	case 8:
		base = 2
	case 9:
		base, width = 2, 3
	}
	return
}

// ValidateDamageGem is shared by explicit discovery and execution. Whole-entry
// admission remains separate from these finite formula certifications.
func (s *LifetimeState) ValidateDamageGem(sourceID int, c *DamageContext, targetID string, rng random.Generator) error {
	if sourceID < 1 || (sourceID > 18 && sourceID != 26 && sourceID != 72 && sourceID != 73) {
		return ErrUnsupportedEffect
	}
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	if sourceID == 72 && c.Defeat == nil {
		return ErrUnsupportedEffect
	}
	e, area := damageGemBase(s.actor(c.ActorID), sourceID)
	e.TargetID = targetID
	if sourceID == 26 {
		first := TargetEffect{TargetID: targetID, Category: CategoryMagic, Payload: CallbackPayload{Symbol: "drain"}}
		converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, first)
		if err != nil {
			return err
		}
		projected := *s
		projected.Actors = slices.Clone(s.Actors)
		if projected.actor(c.ActorID).categorySealed(converted.Category) {
			projected.actor(c.ActorID).Status = ""
		}
		for _, candidate := range projected.damageCandidates(c, converted) {
			e.TargetID = candidate.TargetID
			e.Payload.Number = float64(min(300, int(float64(s.actor(candidate.TargetID).HP)*.2)))
			if err := projected.ValidateDamage(c, e, rng); err != nil {
				return err
			}
		}
		return nil
	}
	if sourceID >= 6 && sourceID <= 9 {
		e.TargetID = ""
		base, width := damageGemHitRange(sourceID)
		hits := base + width - 1
		if s.actor(c.ActorID).HP <= 0 {
			hits = 0
		}
		return s.validateRepeatedDamage(c, e, hits, rng)
	}
	if area {
		e.TargetID = ""
		return s.validateAreaDamage(c, e, rng)
	}
	return s.ValidateDamage(c, e, rng)
}

// ApplyDamageGem certifies only these concrete formulas and their tails at
// _data.cgi:2089–2106,2114,2160–2161. It does not register scalar batch DTOs or
// admit an enclosing custom/random/delegated callback. 101/102 always reject.
func (s *LifetimeState) ApplyDamageGem(sourceID int, c *DamageContext, targetID string, rng random.Generator) ([]EffectApplication, error) {
	if err := s.ValidateDamageGem(sourceID, c, targetID, rng); err != nil {
		return nil, err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		if r.MPDebit != 0 {
			a := s.actor(c.ActorID)
			a.MP = max(0, a.MP-r.MPDebit)
		}
		if r.MPRecoveryTarget != "" {
			t := s.actor(r.MPRecoveryTarget)
			t.MP = min(t.MaxMP, t.MP+r.MPRecovery)
		}
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	a := s.actor(c.ActorID)
	e, area := damageGemBase(a, sourceID)
	e.TargetID = targetID
	var prefix []EffectApplication
	switch sourceID {
	case 6, 7, 8, 9:
		base, width := damageGemHitRange(sourceID)
		hits := int(rng.Float64()*float64(width)) + base
		var results []EffectApplication
		for range hits {
			a = s.actor(c.ActorID)
			if a.HP <= 0 {
				break
			}
			variation := .2
			if sourceID == 8 {
				variation = .3
				e.Arguments = []string{"1"}
			}
			e.TargetID, e.Payload.Number = "", float64(a.Attack)*(.6+rng.Float64()*variation)
			r, err := s.ApplyDamage(c, e, rng)
			if err != nil {
				return results, err
			}
			results = append(results, r)
		}
		return results, nil
	case 26:
		target, err := s.ResolveEnemyTarget(c.TargetContext, TargetEffect{TargetID: targetID, Category: CategoryMagic, Payload: CallbackPayload{Symbol: "drain"}}, rng)
		if err == nil {
			resources(target)
		}
		prefix = append(prefix, EffectApplication{TargetResolution: target})
		if err != nil || !target.Resolved {
			return prefix, err
		}
		e.TargetID, e.Category = target.TargetID, CategoryMagic
		e.Payload.Number = float64(min(300, int(float64(s.actor(target.TargetID).HP)*.2)))
		e.Arguments = []string{"1"}
	}
	if area {
		direct, _ := damageFlags(e)
		return s.ApplyAreaDamage(c, e.Payload.Number, e.Category, direct, rng)
	}
	r, err := s.ApplyDamage(c, e, rng)
	if err != nil {
		return nil, err
	}
	results := append(prefix, r)
	a = s.actor(c.ActorID)
	if sourceID == 3 && r.Amount > 0 {
		power := float64(a.HP) * .2
		if a.HP > 999 {
			power = float64(r.Amount) * .2
		}
		tail, err := s.ApplyRecoil(&c.EffectContext, &c.End, power, rng)
		return append(results, tail), err
	}
	if sourceID == 26 && r.Returned && r.TargetID != "" {
		tail, err := s.ApplyHealing(&c.EffectContext, TargetEffect{TargetID: a.ID, Category: CategoryMagic, Payload: e.Payload}, rng)
		return append(results, tail), err
	}
	if sourceID == 72 {
		if err := c.Defeat(a.ID); err != nil {
			return results, err
		}
	}
	return results, nil
}

type NormalHit struct {
	EffectApplication
	Critical bool
}

// ValidateNormalAttack checks every reachable strike without drawing or changing state.
func (s *LifetimeState) ValidateNormalAttack(c *DamageContext, targetID string, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	strikes := 1
	if hasAbility(s.actor(c.ActorID).Abilities, "seal_shinsoku") {
		strikes = 2
	}
	a := s.actor(c.ActorID)
	initialTargetID := targetID
	if t := s.actor(initialTargetID); t == nil || t.TeamID == a.TeamID {
		initialTargetID = ""
	}
	hits := strikes
	if hasItem(s.actor(c.ActorID).ItemDefinitionIDs, "item-147") && !c.ActorNPC {
		ids := []string{initialTargetID}
		if initialTargetID == "" {
			ids = c.EnemyIDs
		}
		for _, id := range ids {
			if s.actor(id).HP > 0 {
				hits *= 2
				break
			}
		}
	}
	return s.validateRepeatedDamage(c, TargetEffect{TargetID: initialTargetID, Category: CategoryPhysical, Payload: CallbackPayload{Number: float64(s.actor(c.ActorID).Attack)}, Arguments: []string{"0", "1"}}, hits, rng)
}

// ApplyNormalAttack preserves both seal10 invocations and item147's additional
// strike. Direct critical power is 75% here; the damage helper never adds it.
func (s *LifetimeState) ApplyNormalAttack(c *DamageContext, targetID string, rng random.Generator) ([]NormalHit, error) {
	if err := s.ValidateNormalAttack(c, targetID, rng); err != nil {
		return nil, err
	}
	strikes := 1
	if hasAbility(s.actor(c.ActorID).Abilities, "seal_shinsoku") {
		strikes = 2
	}
	if len(c.EnemyIDs) == 0 {
		return nil, nil
	}
	var results []NormalHit
	for range strikes {
		a := s.actor(c.ActorID)
		t := s.actor(targetID)
		if t == nil || t.TeamID == a.TeamID {
			t = s.actor(randomRoster(c.EnemyIDs, rng))
		}
		initialTarget := t.ID
		critical := s.exceedAg(c, a, t, rng)
		power := float64(a.Attack)
		arg := "0"
		if critical {
			power *= .75
			arg = "1"
		}
		r, err := s.ApplyDamage(c, TargetEffect{TargetID: t.ID, Category: CategoryPhysical, Payload: CallbackPayload{Number: power}, Arguments: []string{arg, "1"}}, rng)
		if err != nil {
			return results, err
		}
		results = append(results, NormalHit{EffectApplication: r, Critical: critical})
		a, t = s.actor(c.ActorID), s.actor(initialTarget)
		if hasItem(a.ItemDefinitionIDs, "item-147") && a.TeamID != t.TeamID && rng.Float64() < 1.0/3 && !c.ActorNPC && t.HP > 0 {
			r, err := s.ApplyDamage(c, TargetEffect{TargetID: t.ID, Category: CategoryPhysical, Payload: CallbackPayload{Number: float64(a.Attack)}}, rng)
			if err != nil {
				return results, err
			}
			results = append(results, NormalHit{EffectApplication: r})
		}
	}
	return results, nil
}

// ApplyExtension201 uses the submitted damage target, then the post-damage
// package cursor. A fully admitted reset hook may change that cursor (item159).
func (s *LifetimeState) ValidateExtension201(c *DamageContext, targetID string, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	a := s.actor(c.ActorID)
	if a.Agility <= a.BaseStats.Agility {
		return nil
	}
	if (c.Cursor.Source != "item150" && c.Cursor.Source != "item159") || s.actor(c.Cursor.ActorID) == nil || s.actor(c.Cursor.ActorID).Gear == nil {
		return ErrUnsupportedEffect
	}
	return s.ValidateDamage(c, TargetEffect{TargetID: targetID, Payload: CallbackPayload{Number: float64(a.Agility) * 1.6}, Arguments: []string{"1"}}, rng)
}

func (s *LifetimeState) ApplyExtension201(c *DamageContext, targetID string, rng random.Generator) (EffectApplication, error) {
	r := EffectApplication{}
	if err := s.ValidateExtension201(c, targetID, rng); err != nil {
		return r, err
	}
	a := s.actor(c.ActorID)
	if a.Agility <= a.BaseStats.Agility {
		return r, nil
	}
	r, err := s.ApplyDamage(c, TargetEffect{TargetID: targetID, Payload: CallbackPayload{Number: float64(a.Agility) * 1.6}, Arguments: []string{"1"}}, rng)
	if err != nil {
		return r, err
	}
	t := s.actor(c.Cursor.ActorID)
	t.Agility = t.BaseStats.Agility - t.Gear.WeaponWeight - t.Gear.ArmorWeight
	a = s.actor(c.ActorID)
	if a.Agility > a.BaseStats.Agility {
		a.Agility = a.BaseStats.Agility
	}
	return r, nil
}

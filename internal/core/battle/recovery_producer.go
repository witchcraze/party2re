package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ApplyRecoveryStatGem executes a complete native body, including target MP
// facts. Enclosing admission/costs stay with its invoking consumer.
func (s *LifetimeState) ApplyRecoveryStatGem(sourceID int, c *EffectContext, targetID string, rng random.Generator) ([]EffectApplication, error) {
	if err := s.ValidateRecoveryStatGems([]int{sourceID}, c, targetID, rng); err != nil {
		return nil, err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	previous := c.targetResources
	c.targetResources = func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		if r.MPRecoveryTarget != "" {
			t := s.actor(r.MPRecoveryTarget)
			t.MP = min(t.MaxMP, t.MP+r.MPRecovery)
		}
	}
	defer func() { c.targetResources = previous }()
	return s.applyRecoveryStatGem(sourceID, c, targetID, rng)
}

func (s *LifetimeState) applyRecoveryStatGem(sourceID int, c *EffectContext, targetID string, rng random.Generator) ([]EffectApplication, error) {
	e := TargetEffect{TargetID: targetID, Category: CategoryMagic}
	stat := StatAttack
	var r EffectApplication
	var err error
	switch sourceID {
	case 19, 20, 21:
		e.Payload.Number = 300
		if sourceID == 19 {
			e.Payload.Number = 100
		}
		if sourceID == 20 {
			e.TargetID, e.Category = c.ActorID, CategoryUntyped
		}
		r, err = s.ApplyHealing(c, e, rng)
	case 22, 23:
		power := 100.0
		if sourceID == 23 {
			power = 300
		}
		return s.ApplyPartyHealing(c, power, CategoryMagic, rng)
	case 28, 29, 30, 31, 32, 33:
		if sourceID >= 32 {
			stat = StatAgility
		} else if sourceID >= 30 {
			stat = StatDefense
		}
		if sourceID%2 == 0 {
			e.TargetID, e.Category = c.ActorID, CategoryPhysical
		}
		e.Payload.Number = 1
		r, err = s.ApplyStatUp(c, e, stat, rng)
	case 93, 94, 95, 96:
		coefficient := .25
		switch sourceID {
		case 94:
			stat = StatDefense
		case 95:
			stat = StatAgility
		case 96:
			stat, coefficient = StatHit, .2
		}
		return s.ApplyPartyStat(c, coefficient, CategoryMagic, stat, true, rng)
	case 54, 69:
		ids := c.PartyIDs
		if sourceID == 69 {
			ids = make([]string, len(s.Actors))
			for i, a := range s.Actors {
				ids[i] = a.ID
			}
		}
		var results []EffectApplication
		for _, id := range ids {
			a := s.actor(id)
			changed := a.Hit != 95
			if sourceID == 54 {
				a.Hit = 95
			} else {
				changed, a.Tension = a.Tension != 1, 1
			}
			results = append(results, EffectApplication{TargetResolution: TargetResolution{TargetEffect: TargetEffect{TargetID: id}, Resolved: true}, Returned: true, Applied: changed})
		}
		return results, nil
	case 67, 74:
		target := TargetResolution{TargetEffect: TargetEffect{TargetID: c.ActorID}, Resolved: true}
		if sourceID == 74 {
			target, err = s.ResolvePartyTarget(c.TargetContext, TargetEffect{TargetID: targetID, Category: CategoryUntyped, Payload: CallbackPayload{Symbol: "encourage"}}, rng)
			if err != nil {
				return nil, err
			}
			c.targetResources(target)
		}
		// tenshon's absent-target fallback is part of74's body, not a return guard.
		if !target.Resolved {
			target.TargetID = c.ActorID
		}
		t := s.actor(target.TargetID)
		before, temporary := t.Tension, t.Temporary
		if err := s.statTension(c, t, false, rng); err != nil {
			return nil, err
		}
		if sourceID == 67 {
			if err := s.statTension(c, t, false, rng); err != nil {
				return nil, err
			}
		}
		return []EffectApplication{{TargetResolution: target, Returned: true, Applied: t.Tension != before || t.Temporary != temporary}}, nil
	case 68:
		target, err := s.ResolveEnemyTarget(c.TargetContext, TargetEffect{TargetID: targetID, Category: CategoryPhysical, Payload: CallbackPayload{Symbol: "destroy"}}, rng)
		r.TargetResolution = target
		if err == nil {
			c.targetResources(target)
		}
		if err != nil || !target.Resolved {
			return []EffectApplication{r}, err
		}
		t := s.actor(target.TargetID)
		r.Returned, r.Applied = true, t.Tension != 1
		t.Tension = 1
	default:
		return nil, ErrUnsupportedEffect
	}
	return []EffectApplication{r}, err
}

// TargetCursor represents only the two verified package-cursor setters or an
// absent cursor in the current request. Submitted/helper/loop targets are not
// provenance. The roster owner must supply the resolved named/fallback helper.
type TargetCursor struct {
	Source  string
	ActorID string
}

type AttackDistribution struct {
	HolderID  string
	Cap       int
	RaisedIDs []string
	Tails     []EffectApplication
}

func (s *LifetimeState) validateDistributedAttack(c *EffectContext, helperID string, cursor TargetCursor, rng random.Generator) (AttackDistribution, error) {
	r := AttackDistribution{Cap: 999}
	if c == nil {
		return r, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return r, err
	}
	helper := s.actor(helperID)
	if helper == nil || helper.HP <= 0 {
		return r, nil
	}
	switch cursor.Source {
	case "":
		if cursor.ActorID != "" {
			return r, ErrUnsupportedEffect
		}
	case "item150", "item159":
		a := s.actor(cursor.ActorID)
		if a == nil {
			return r, ErrUnsupportedEffect
		}
		if a.OverLevel {
			r.Cap = 1998
		}
	default:
		return r, ErrUnsupportedEffect
	}
	if helper != nil && !slices.Contains(c.PartyIDs, helperID) {
		return r, ErrUnsupportedEffect
	}
	if helper != nil && helper.HP > 0 {
		// Earlier205 tails can change both later power and target resolution.
		admission := statUpAdmission{mayReplace: make(map[string]float64)}
		for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
			for _, id := range c.PartyIDs {
				e := TargetEffect{TargetID: id, Category: CategoryDance, Payload: CallbackPayload{Number: .5}}
				if err := s.validateStatUp(c, e, stat, rng, admission); err != nil {
					return r, err
				}
				s.recordStatUp205(c, e, stat, &admission)
			}
		}
	}
	return r, nil
}

func (s *LifetimeState) ValidateDistributedAttack(c *EffectContext, helperID string, cursor TargetCursor, rng random.Generator) error {
	_, err := s.validateDistributedAttack(c, helperID, cursor, rng)
	return err
}

// ApplyDistributedAttack implements calling80's distribution and four ordered
// tails (_skill.cgi:6436–6465); the roster owner resolves the helper first.
func (s *LifetimeState) ApplyDistributedAttack(c *EffectContext, helperID string, cursor TargetCursor, rng random.Generator) (AttackDistribution, error) {
	r, err := s.validateDistributedAttack(c, helperID, cursor, rng)
	if err != nil {
		return r, err
	}
	helper := s.actor(helperID)
	allowed, err := s.actor(c.ActorID).CheckCategorySeal(CategoryDance, rng)
	if err != nil || !allowed || helper == nil || helper.HP <= 0 {
		return r, err
	}
	maximum := 0
	for _, id := range c.PartyIDs {
		if s.actor(id).Attack > maximum {
			maximum, r.HolderID = s.actor(id).Attack, id
		}
	}
	maximum = min(maximum, r.Cap)
	for _, id := range c.PartyIDs {
		a := s.actor(id)
		if a.HP > 0 && id != r.HolderID && a.Attack < maximum {
			a.Attack = maximum
			r.RaisedIDs = append(r.RaisedIDs, id)
		}
	}
	for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
		if rng.Float64() < .25 {
			tail, err := s.ApplyPartyStat(c, .5, CategoryDance, stat, false, rng)
			if err != nil {
				return r, err
			}
			r.Tails = append(r.Tails, tail...)
		}
	}
	if rng.Float64() < .25 {
		tail, err := s.ApplyPartyHealing(c, 200, CategoryDance, rng)
		if err != nil {
			return r, err
		}
		r.Tails = append(r.Tails, tail...)
	}
	return r, nil
}

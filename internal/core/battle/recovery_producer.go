package battle

import (
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// ApplyRecoveryStatGem certifies the concrete helper calls at
// _data.cgi:2107–2111,2116–2121,2181–2184. It does not admit an enclosing custom,
// random or delegated command or certify the batch resolver's fixed Power DTOs.
func (s *LifetimeState) ApplyRecoveryStatGem(sourceID int, c *EffectContext, targetID string, rng random.Generator) ([]EffectApplication, error) {
	if c == nil {
		return nil, ErrInvalidRequest
	}
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

// ApplyDistributedAttack implements calling80's distribution and four ordered
// tails (_skill.cgi:6436–6465); summoning/appearance selection remains #1339.
func (s *LifetimeState) ApplyDistributedAttack(c *EffectContext, helperID string, cursor TargetCursor, rng random.Generator) (AttackDistribution, error) {
	r := AttackDistribution{Cap: 999}
	if c == nil {
		return r, ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return r, err
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
	helper := s.actor(helperID)
	if helper != nil && !slices.Contains(c.PartyIDs, helperID) {
		return r, ErrUnsupportedEffect
	}
	if helper != nil && helper.HP > 0 {
		// Earlier205 tails can increase tension/replace a temporary, so a later
		// rounded-zero projection alone cannot admit a missing damage executor.
		powerMayChange := false
		for _, stat := range []BattleStat{StatAttack, StatDefense, StatAgility} {
			for _, id := range c.PartyIDs {
				e := TargetEffect{TargetID: id, Category: CategoryDance, Payload: CallbackPayload{Number: .5}}
				if err := s.validateStatUp(c, e, stat, rng, powerMayChange); err != nil {
					return r, err
				}
				powerMayChange = powerMayChange || s.statUpMayChangePower(c, e, stat)
			}
		}
	}
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

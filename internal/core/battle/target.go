package battle

import (
	"math"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// CallbackCategory is independent of element, direct damage and tool identity.
type CallbackCategory string

const (
	CategoryUntyped  CallbackCategory = ""
	CategoryPhysical CallbackCategory = "physical"
	CategoryMagic    CallbackCategory = "magic"
	CategoryBreath   CallbackCategory = "breath"
	CategoryDance    CallbackCategory = "dance"
	CategorySong     CallbackCategory = "song"
)

const (
	StatusPhysicalSeal = "physical_seal"
	StatusMagicSeal    = "magic_seal"
	StatusDanceSeal    = "dance_seal"
)

// CallbackPayload retains fractions until the consuming helper rounds them.
// Symbol is nonempty for status, revival and temporary-effect payloads.
type CallbackPayload struct {
	Number float64
	Symbol string
}

// TargetContext contains operation-local caller facts. Rosters are ordered and
// include fallen members. PhantomTargetID is supplied only after the roster
// owner's matching-copy existence predicate; PhantomEnemyIDs is its complete
// ordered phantom pool. Command identifies the outer submitted callback.
type TargetContext struct {
	ActorID          string
	PartyIDs         []string
	EnemyIDs         []string
	Command          string
	Custom           bool
	Item195Activated bool
	PhantomTargetID  string
	PhantomEnemyIDs  []string
}

// TargetEffect is the input and continuation of a spell target helper.
// Arguments retain callback-specific trailing parameters; magic sword replaces
// them with direct=1. MPDebit/MPRecovery are ordered resource operations for the
// resource owner, not HP healing or cancellation of the incoming effect.
type TargetEffect struct {
	TargetID  string
	Payload   CallbackPayload
	Category  CallbackCategory
	Arguments []string
}

type TargetResolution struct {
	TargetEffect
	Resolved         bool
	MPDebit          int
	MPRecovery       int
	MPRecoveryTarget string
}

func (e TargetEffect) validate() error {
	switch e.Category {
	case CategoryUntyped, CategoryPhysical, CategoryMagic, CategoryBreath, CategoryDance, CategorySong:
	default:
		return ErrUnsupportedEffect
	}
	if math.IsNaN(e.Payload.Number) || math.IsInf(e.Payload.Number, 0) || (e.Payload.Symbol != "" && e.Payload.Number != 0) {
		return ErrUnsupportedEffect
	}
	return nil
}

func (s *LifetimeState) validateTargetContext(c TargetContext, rng random.Generator) error {
	if rng == nil || s.actor(c.ActorID) == nil {
		return ErrInvalidRequest
	}
	seen := make(map[string]bool, len(s.Actors))
	for _, a := range s.Actors {
		if a.ID == "" || seen[a.ID] {
			return ErrInvalidParticipant
		}
		seen[a.ID] = true
	}
	seen = make(map[string]bool, len(s.Actors))
	actor := s.actor(c.ActorID)
	for side, ids := range [][]string{c.PartyIDs, c.EnemyIDs} {
		for _, id := range ids {
			a := s.actor(id)
			if a == nil || seen[id] || (a.TeamID == actor.TeamID) != (side == 0) {
				return ErrInvalidParticipant
			}
			seen[id] = true
		}
	}
	if len(seen) != len(s.Actors) || !slices.Contains(c.PartyIDs, c.ActorID) {
		return ErrInvalidParticipant
	}
	if c.PhantomTargetID != "" && !slices.Contains(c.EnemyIDs, c.PhantomTargetID) {
		return ErrUnsupportedEffect
	}
	for _, id := range c.PhantomEnemyIDs {
		if !slices.Contains(c.EnemyIDs, id) || id == c.PhantomTargetID {
			return ErrUnsupportedEffect
		}
	}
	return nil
}

// CheckCategorySeal preserves each helper's fresh cure attempt. Song has no
// matching seal predicate. This does not run action-incapacitation checks.
func (a *LifetimeActor) CheckCategorySeal(category CallbackCategory, rng random.Generator) (bool, error) {
	if err := (TargetEffect{Category: category}).validate(); err != nil {
		return false, err
	}
	if !a.categorySealed(category) {
		return true, nil
	}
	if rng == nil {
		return false, ErrInvalidRequest
	}
	if rng.Float64() < 0.25 {
		a.Status = ""
		return true, nil
	}
	return false, nil
}

func (a LifetimeActor) categorySealed(category CallbackCategory) bool {
	return (category == CategoryPhysical && a.Status == StatusPhysicalSeal) ||
		(category == CategoryMagic && a.Status == StatusMagicSeal) ||
		(category == CategoryDance && a.Status == StatusDanceSeal)
}

// ConvertEnemyCategory precedes empty-enemy and seal checks. It returns the MP
// debit even when a later gate rejects the helper. _skill.cgi:10153–10194.
func (a LifetimeActor) ConvertEnemyCategory(c TargetContext, e TargetEffect) (TargetEffect, int, error) {
	if err := e.validate(); err != nil {
		return TargetEffect{}, 0, err
	}
	if hasItem(a.ItemDefinitionIDs, "item-166") && e.Payload.Number > 0 {
		switch e.Category {
		case CategoryPhysical:
			e.Category = CategoryUntyped
			if a.Tension < 3 {
				e.Payload.Number = math.Trunc(e.Payload.Number * 0.8)
			} else if a.Tension > 3 {
				e.Payload.Number = math.Trunc(e.Payload.Number * 1.2)
			}
		case CategoryMagic:
			e.Payload.Number = 0
		}
	}
	if a.Extended == ExtendedMagicSword && !c.Custom && magicSwordCommand(c.Command) {
		if e.Payload.Symbol != "" {
			return TargetEffect{}, 0, ErrUnsupportedEffect
		}
		if e.Category == CategoryPhysical {
			e.Category = CategoryMagic
		}
		// The source's multiplier/direct rewrite is outside its category test.
		e.Payload.Number = math.Trunc(e.Payload.Number * 1.2)
		e.Arguments = []string{"1"}
	}
	if e.Category == CategoryPhysical && hasAbility(a.Abilities, "seal_kotowari") {
		if e.Payload.Symbol != "" {
			return TargetEffect{}, 0, ErrUnsupportedEffect
		}
		e.Category = CategoryMagic
		e.Payload.Number = math.Trunc(e.Payload.Number * 0.8)
		return e, 3, nil
	}
	return e, 0, nil
}

// Generic identifiers for the five original outer-command whitelist entries.
func magicSwordCommand(command string) bool {
	switch command {
	case "attack", "flame_slash", "lightning_slash", "metal_slash", "wide_slash":
		return true
	}
	return false
}

func (s *LifetimeState) activateConfusion(c TargetContext, chance float64, rng random.Generator) {
	a := s.actor(c.ActorID)
	if c.Item195Activated && a.Status == "" && rng.Float64() < chance {
		a.Status = StatusConfusion
	}
}

func (s *LifetimeState) randomMember(rng random.Generator) string {
	return s.Actors[int(rng.Float64()*float64(len(s.Actors)))].ID
}

func randomRoster(ids []string, rng random.Generator) string {
	return ids[int(rng.Float64()*float64(len(ids)))]
}

func (s *LifetimeState) validatePartyTarget(c TargetContext, e TargetEffect, rng random.Generator) error {
	if err := s.validateTargetContext(c, rng); err != nil {
		return err
	}
	if err := e.validate(); err != nil {
		return err
	}
	a := s.actor(c.ActorID)
	// Reject an unrepresented possible branch before cure/activation/selection
	// draws. Whole-action admission must include these context predicates.
	if s.SpecialBossField && s.actor(e.TargetID) == nil &&
		(a.Status == StatusConfusion || (c.Item195Activated && (a.Status == "" || a.categorySealed(e.Category)))) {
		return ErrUnsupportedEffect
	}
	if e.Category == CategoryPhysical && len(c.EnemyIDs) == 0 {
		for _, id := range s.partyInitialTargetIDs(c, e) {
			if target := s.actor(id); target != nil && target.Temporary == TemporaryDeflect {
				return ErrUnsupportedEffect
			}
		}
	}
	return nil
}

func (s *LifetimeState) partyInitialTargetIDs(c TargetContext, e TargetEffect) []string {
	a := s.actor(c.ActorID)
	ids := []string{e.TargetID}
	if a.Status == StatusConfusion || (c.Item195Activated && (a.Status == "" || a.categorySealed(e.Category))) {
		ids = nil
		for _, member := range s.Actors {
			ids = append(ids, member.ID)
		}
	} else {
		if !slices.Contains(c.PartyIDs, e.TargetID) {
			ids = slices.Clone(c.PartyIDs)
		}
		for i, id := range ids {
			if actor := s.actor(id); actor != nil && actor.HP <= 0 && e.Payload.Symbol != "revival" {
				ids[i] = c.ActorID
			}
		}
	}
	return ids
}

// partyTargetCandidates follows _check_party's possible target paths without
// draws or mutations. Call only after validatePartyTarget. Temporary redirects
// run once; the replacement target's temporary is not evaluated recursively.
// mayDouble contains only earlier reachable205 tails in the same operation.
func (s *LifetimeState) partyTargetCandidates(c TargetContext, e TargetEffect, mayDouble map[string]bool) []TargetEffect {
	return s.temporaryTargetCandidates(c, e, s.partyInitialTargetIDs(c, e), mayDouble)
}

func (s *LifetimeState) temporaryTargetCandidates(c TargetContext, e TargetEffect, ids []string, mayDouble map[string]bool) []TargetEffect {
	var targets []TargetEffect
	for _, id := range ids {
		t := s.actor(id)
		if mayDouble[id] && t.Temporary != TemporaryDouble {
			doubled := e
			doubled.TargetID = id
			if doubled.Payload.Symbol == "" {
				doubled.Payload.Number *= 2
			}
			targets = append(targets, doubled)
		}
		candidate := e
		appendTarget := func(targetID string) {
			candidate.TargetID = targetID
			targets = append(targets, candidate)
		}
		scale := func(multiplier float64) {
			if candidate.Payload.Symbol == "" {
				candidate.Payload.Number *= multiplier
			}
		}
		switch t.Temporary {
		case TemporaryCover:
			covered := false
			for _, member := range s.Actors {
				if member.TeamID == t.TeamID && member.Temporary == TemporaryCovering {
					appendTarget(member.ID)
					if !mayDouble[member.ID] {
						covered = true
						break
					}
				}
			}
			if covered {
				continue
			}
		case TemporaryStrongDefense:
			scale(.1)
		case TemporaryDefense:
			scale(.5)
		case TemporaryReduceAttack, TemporaryReduceMagic, TemporaryReduceBreath:
			if (t.Temporary == TemporaryReduceAttack && e.Category == CategoryPhysical) ||
				(t.Temporary == TemporaryReduceMagic && e.Category == CategoryMagic) ||
				(t.Temporary == TemporaryReduceBreath && e.Category == CategoryBreath) {
				scale(.25)
			}
		case TemporaryDouble:
			scale(2)
		case TemporaryReflectAttack, TemporaryReflectMagic, TemporaryReflectBreath, TemporaryReflectDance:
			if (t.Temporary == TemporaryReflectAttack && e.Category == CategoryPhysical) ||
				(t.Temporary == TemporaryReflectMagic && e.Category == CategoryMagic) ||
				(t.Temporary == TemporaryReflectBreath && e.Category == CategoryBreath) ||
				(t.Temporary == TemporaryReflectDance && e.Category == CategoryDance) {
				if hasItem(t.ItemDefinitionIDs, "item-163") {
					scale(2)
				}
				id = c.ActorID
			}
		case TemporaryDeflect:
			if e.Category == CategoryPhysical {
				for _, roster := range [][]string{c.PartyIDs, c.EnemyIDs} {
					for _, targetID := range roster {
						appendTarget(targetID)
					}
				}
				continue
			}
		case TemporaryNullAttack, TemporaryNullMagic:
			if (t.Temporary == TemporaryNullAttack && e.Category == CategoryPhysical) ||
				(t.Temporary == TemporaryNullMagic && e.Category == CategoryMagic) {
				candidate.Payload = CallbackPayload{}
			}
		case TemporaryPhantom:
			for _, targetID := range c.PartyIDs {
				appendTarget(targetID)
			}
		}
		appendTarget(id)
	}
	return targets
}

// ResolvePartyTarget implements the spell ally helper, including revival's
// fallen-target exception. Tool helpers must use their own guards and the
// optional temporary hook rather than acquiring spell fallback/seal behavior.
func (s *LifetimeState) ResolvePartyTarget(c TargetContext, e TargetEffect, rng random.Generator) (TargetResolution, error) {
	r := TargetResolution{TargetEffect: e}
	if err := s.validatePartyTarget(c, e, rng); err != nil {
		return r, err
	}
	a := s.actor(c.ActorID)
	allowed, err := a.CheckCategorySeal(e.Category, rng)
	if err != nil || !allowed {
		return r, err
	}
	s.activateConfusion(c, 1.0/3, rng)
	if a.Status == StatusConfusion {
		if !s.SpecialBossField || rng.Float64() < 1.0/3 {
			r.TargetID = s.randomMember(rng)
		}
	} else {
		if !slices.Contains(c.PartyIDs, r.TargetID) {
			r.TargetID = randomRoster(c.PartyIDs, rng)
		}
		if s.actor(r.TargetID).HP <= 0 && e.Payload.Symbol != "revival" {
			r.TargetID = a.ID
		}
	}
	if s.actor(r.TargetID) == nil {
		// Boss-field confusion can retain an absent submitted target. Its
		// empty-record side effects are not a certified participant context.
		return r, ErrUnsupportedEffect
	}
	return s.applyTargetTemporary(c, r, rng)
}

// ResolveEnemyTarget preserves both successive confusion replacements and the
// original short circuits. A resolved target is not yet an applied hit: the
// consuming helper owns zero/fallen/one-hit-defense and callback-specific tails.
func (s *LifetimeState) ResolveEnemyTarget(c TargetContext, e TargetEffect, rng random.Generator) (TargetResolution, error) {
	r := TargetResolution{TargetEffect: e}
	if err := s.validateTargetContext(c, rng); err != nil {
		return r, err
	}
	converted, debit, err := s.actor(c.ActorID).ConvertEnemyCategory(c, e)
	if err != nil {
		return r, err
	}
	s.activateConfusion(c, 0.2, rng)
	r.TargetEffect, r.MPDebit = converted, debit
	if len(c.EnemyIDs) == 0 {
		return r, nil
	}
	a := s.actor(c.ActorID)
	allowed, err := a.CheckCategorySeal(r.Category, rng)
	if err != nil || !allowed {
		return r, err
	}
	if a.Status == StatusConfusion {
		if r.TargetID != a.ID {
			r.TargetID = s.randomMember(rng)
		}
		if r.TargetID != a.ID || (s.SpecialBossField && rng.Float64()*3 <= 1) {
			r.TargetID = s.randomMember(rng)
		}
	} else {
		if !slices.Contains(c.EnemyIDs, r.TargetID) {
			r.TargetID = randomRoster(c.EnemyIDs, rng)
		}
		if s.actor(r.TargetID).HP <= 0 {
			return r, nil
		}
	}
	if a.Temporary != TemporaryPhantom {
		r, err = s.applyTargetTemporary(c, r, rng)
		if err != nil {
			return r, err
		}
	} else {
		r.Resolved = true
	}
	if s.Fields.Current() == "phantom" && r.TargetID == c.PhantomTargetID && len(c.PhantomEnemyIDs) > 0 {
		r.TargetID = randomRoster(c.PhantomEnemyIDs, rng)
	}
	return r, nil
}

func (s *LifetimeState) applyTargetTemporary(c TargetContext, r TargetResolution, rng random.Generator) (TargetResolution, error) {
	tmp, err := s.ResolveTemporary(c, r.TargetEffect, rng)
	if err != nil {
		return r, err
	}
	if tmp.TargetID != nil {
		r.TargetID = *tmp.TargetID
	}
	if tmp.Payload != nil {
		r.Payload = *tmp.Payload
	}
	r.MPRecovery, r.MPRecoveryTarget = tmp.MPRecovery, tmp.MPRecoveryTarget
	r.Resolved = true
	return r, nil
}

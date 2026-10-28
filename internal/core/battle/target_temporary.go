package battle

import (
	"math"

	"github.com/witchcraze/party2re/internal/core/random"
)

// TemporaryReplacement preserves optional helper returns. Spells retain absent
// values; tool hooks use their own tuple-assignment semantics. Callers that omit
// a tool's optional hook must not invoke this method. No recursive dispatch.
type TemporaryReplacement struct {
	TargetID         *string
	Payload          *CallbackPayload
	MPRecovery       int
	MPRecoveryTarget string
}

// ResolveTemporary evaluates exactly the selected target's one temporary
// effect. Resource writes belong to the resource owner; return requested recovery before
// its MaxMP clamp, without replacing or canceling the incoming payload.
// _skill.cgi:9464–9573; optional tool hooks:10559–10584.
func (s *LifetimeState) ResolveTemporary(c TargetContext, e TargetEffect, rng random.Generator) (TemporaryReplacement, error) {
	r := TemporaryReplacement{}
	if err := s.validateTargetContext(c, rng); err != nil {
		return r, err
	}
	if err := e.validate(); err != nil {
		return r, err
	}
	target := s.actor(e.TargetID)
	if target == nil {
		return r, ErrInvalidParticipant
	}
	if target.Temporary == TemporaryDeflect && e.Category == CategoryPhysical && len(c.EnemyIDs) == 0 {
		return r, ErrUnsupportedEffect
	}
	redirect := func(id string) { r.TargetID = &id }
	scale := func(multiplier float64) {
		if e.Payload.Symbol == "" {
			payload := CallbackPayload{Number: e.Payload.Number * multiplier}
			r.Payload = &payload
			redirect(target.ID)
		}
	}
	switch target.Temporary {
	case TemporaryCover:
		for _, a := range s.Actors {
			if a.TeamID == target.TeamID && a.Temporary == TemporaryCovering {
				redirect(a.ID)
				break
			}
		}
	case TemporaryStrongDefense:
		scale(0.1)
	case TemporaryDefense:
		scale(0.5)
	case TemporaryReduceAttack, TemporaryReduceMagic, TemporaryReduceBreath:
		if (target.Temporary == TemporaryReduceAttack && e.Category == CategoryPhysical) ||
			(target.Temporary == TemporaryReduceMagic && e.Category == CategoryMagic) ||
			(target.Temporary == TemporaryReduceBreath && e.Category == CategoryBreath) {
			scale(0.25)
		}
	case TemporaryDouble:
		scale(2)
	case TemporaryReflectAttack, TemporaryReflectMagic, TemporaryReflectBreath, TemporaryReflectDance:
		if (target.Temporary == TemporaryReflectAttack && e.Category == CategoryPhysical) ||
			(target.Temporary == TemporaryReflectMagic && e.Category == CategoryMagic) ||
			(target.Temporary == TemporaryReflectBreath && e.Category == CategoryBreath) ||
			(target.Temporary == TemporaryReflectDance && e.Category == CategoryDance) {
			// Perl's numeric != "" means symbolic/zero payloads are not doubled.
			if hasItem(target.ItemDefinitionIDs, "item-163") && e.Payload.Symbol == "" && e.Payload.Number != 0 {
				scale(2)
			}
			redirect(c.ActorID)
		}
	case TemporaryDeflect:
		if e.Category == CategoryPhysical {
			ids := c.EnemyIDs
			if rng.Float64() < 0.5 {
				ids = c.PartyIDs
			}
			redirect(randomRoster(ids, rng))
		}
	case TemporaryAbsorbMagic:
		if e.Category == CategoryMagic {
			draw := rng.Float64()
			value := 30 + int(draw*20)
			if e.Payload.Number >= 50 {
				value = int(math.Trunc(e.Payload.Number * (0.2 + draw*0.3)))
			}
			if s.Fields.Current() == "den" {
				value = int(math.Trunc(float64(value)*0.025)) + 1
			}
			r.MPRecovery, r.MPRecoveryTarget = value, target.ID
		}
	case TemporaryNullAttack, TemporaryNullMagic:
		if (target.Temporary == TemporaryNullAttack && e.Category == CategoryPhysical) ||
			(target.Temporary == TemporaryNullMagic && e.Category == CategoryMagic) {
			payload := CallbackPayload{}
			r.Payload = &payload
			redirect(target.ID)
		}
	case TemporaryPhantom:
		redirect(randomRoster(c.PartyIDs, rng))
		if rng.Float64() < 0.5 {
			redirect(target.ID)
		}
		payload := e.Payload
		r.Payload = &payload
	}
	return r, nil
}

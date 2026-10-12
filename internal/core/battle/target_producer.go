package battle

import "github.com/witchcraze/party2re/internal/core/random"

type TemporaryApplication struct {
	Applied          bool
	MPRecovery       int
	MPRecoveryTarget string
}

// ApplyTemporaryGem reconstructs only the finite temporary producers at
// _data.cgi:2122–2141. sourceID is provenance, not a transport/catalog action ID.
// Enclosing custom admission/costs and the later regeneration/revival/MP effects
// retain their respective owners; this operation alone cannot certify a slot.
// Applied=false with nil error is a normally returning no-effect callback.
func (s *LifetimeState) ApplyTemporaryGem(sourceID int, c TargetContext, targetID string, rng random.Generator) (TemporaryApplication, error) {
	result := TemporaryApplication{}
	if err := s.validateTargetContext(c, rng); err != nil {
		return result, err
	}
	var effect TemporaryEffect
	scope := TargetScopeSingleAlly
	category := CategoryMagic
	checkSeal := true
	uncheckedTarget := false
	switch sourceID {
	case 34, 35, 36:
		effect = TemporaryDefense
		if sourceID == 34 {
			scope, checkSeal = TargetScopeSelf, false
		}
		if sourceID == 36 {
			scope = TargetScopeAllAllies
		}
	case 37:
		effect, scope, checkSeal = TemporaryStrongDefense, TargetScopeSelf, false
	case 38:
		effect, scope = TemporaryReduceAttack, TargetScopeAllAllies
	case 39:
		effect, scope = TemporaryReduceMagic, TargetScopeAllAllies
	case 40:
		effect, scope = TemporaryReduceBreath, TargetScopeAllAllies
	case 41:
		effect, uncheckedTarget = TemporaryNullAttack, true
	case 42:
		effect, uncheckedTarget = TemporaryNullMagic, true
	case 43:
		effect, scope = TemporaryNullMagic, TargetScopeAllAllies
	case 44:
		effect, scope = TemporaryReflectAttack, TargetScopeSelf
	case 45:
		effect, uncheckedTarget = TemporaryReflectAttack, true
	case 46:
		effect, scope = TemporaryReflectMagic, TargetScopeSelf
	case 47:
		effect = TemporaryReflectMagic
	case 48:
		effect, scope = TemporaryReflectMagic, TargetScopeAllAllies
	case 49:
		effect, scope = TemporaryReflectBreath, TargetScopeAllAllies
	case 50:
		effect = TemporaryRegeneration
	case 51:
		effect = TemporaryRevival
	case 52:
		effect, category = TemporaryCover, CategoryPhysical
	case 53:
		effect, scope = TemporaryAbsorbMagic, TargetScopeSelf
	default:
		return result, ErrUnsupportedEffect
	}
	a := s.actor(c.ActorID)
	// These three source callbacks write through an absent helper result when
	// sealed. The empty-record mutation is unrepresented; reject before a draw.
	if uncheckedTarget && a.Status == StatusMagicSeal {
		return result, ErrUnsupportedEffect
	}
	if scope == TargetScopeSingleAlly {
		r, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: targetID, Category: category, Payload: CallbackPayload{Symbol: "temporary:" + string(effect)}}, rng)
		if err != nil || !r.Resolved {
			return result, err
		}
		if sourceID == 52 {
			if r.TargetID == a.ID {
				return result, nil
			}
			a.Temporary = TemporaryCovering
		}
		s.actor(r.TargetID).Temporary = effect
		return TemporaryApplication{Applied: true, MPRecovery: r.MPRecovery, MPRecoveryTarget: r.MPRecoveryTarget}, nil
	}
	if checkSeal {
		allowed, err := a.CheckCategorySeal(category, rng)
		if err != nil || !allowed {
			return result, err
		}
	}
	if scope == TargetScopeSelf {
		a.Temporary = effect
		result.Applied = true
		return result, nil
	}
	for _, id := range c.PartyIDs {
		target := s.actor(id)
		if target.HP > 0 {
			target.Temporary = effect
			result.Applied = true
		}
	}
	return result, nil
}

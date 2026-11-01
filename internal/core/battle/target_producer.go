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
	entry, err := defenseGemEntry(sourceID)
	if err != nil || entry.temporary == "" {
		return result, ErrUnsupportedEffect
	}
	scope, category := entry.TargetScope, entry.Category
	uncheckedTarget := sourceID == 41 || sourceID == 42 || sourceID == 45
	a := s.actor(c.ActorID)
	// These three source callbacks write through an absent helper result when
	// sealed. The empty-record mutation is unrepresented; reject before a draw.
	if uncheckedTarget && a.Status == StatusMagicSeal {
		return result, ErrUnsupportedEffect
	}
	if scope == TargetScopeSingleAlly {
		r, err := s.ResolvePartyTarget(c, entry.target(targetID), rng)
		if err != nil || !r.Resolved {
			return result, err
		}
		return TemporaryApplication{Applied: entry.write(s, c, r.TargetID), MPRecovery: r.MPRecovery, MPRecoveryTarget: r.MPRecoveryTarget}, nil
	}
	if category != CategoryUntyped {
		allowed, err := a.CheckCategorySeal(category, rng)
		if err != nil || !allowed {
			return result, err
		}
	}
	if scope == TargetScopeSelf {
		result.Applied = entry.write(s, c, c.ActorID)
		return result, nil
	}
	for _, id := range c.PartyIDs {
		target := s.actor(id)
		if target.HP > 0 {
			entry.write(s, c, id)
			result.Applied = true
		}
	}
	return result, nil
}

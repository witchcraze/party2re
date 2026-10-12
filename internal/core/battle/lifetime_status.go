package battle

import "github.com/witchcraze/party2re/internal/core/random"

// StatusDecision reports the reached normal-action block and a possible defeat.
// The consumer resolves a defeat/reset with fresh equipment facts before saving.
type StatusDecision struct {
	NormalAction bool
	NeedsDefeat  bool
}

// IncapacitationContext keeps the quest marker distinct from a named field.
type IncapacitationContext struct {
	NPC              bool
	Bypass           bool
	SpecialBossField bool
	PhantomField     bool
}

// CheckIncapacitation is for explicit operations only. Equipment/ability cure
// hooks precede it. Ordinary NPC confusion cures at 1/3, player confusion at 1/5.
// Extra NPC cures use the explicit boss marker/phantom field predicates.
// _battle.cgi:686–741; _npc_action.cgi:234–310.
func (a *LifetimeActor) CheckIncapacitation(c IncapacitationContext, rng random.Generator) (StatusDecision, error) {
	decision := StatusDecision{NormalAction: true}
	if rng == nil {
		return StatusDecision{}, ErrInvalidRequest
	}
	// The curse branch draws before checking item204, unlike the other branches.
	if a.Status == StatusKinju && rng.Float64() < 0.25 && !c.Bypass {
		a.HP = max(0, a.HP-a.poisonDamage(rng))
		a.Status = ""
		return StatusDecision{NeedsDefeat: a.HP == 0}, nil
	}
	if !c.Bypass {
		switch a.Status {
		case StatusDofuu:
			a.Status = ""
			return StatusDecision{}, nil
		case StatusParalyze, StatusSleep:
			if rng.Float64() < 1.0/3 || (c.NPC && c.SpecialBossField && rng.Float64() < 0.5) || (c.NPC && c.PhantomField && rng.Float64() < 0.5) {
				a.Status = ""
			}
			return StatusDecision{}, nil // curing does not execute the action
		case StatusSabaku:
			if rng.Float64() < 0.25 {
				if rng.Float64() < 0.5 || (c.NPC && c.SpecialBossField && rng.Float64() < 0.5) {
					a.Status = ""
				}
				return StatusDecision{}, nil
			}
		}
	}
	if a.Status == StatusConfusion {
		chance := 0.2
		if c.NPC {
			chance = 1.0 / 3
		}
		if rng.Float64() < chance || (c.NPC && c.SpecialBossField && rng.Float64() < 0.5) || (c.NPC && c.PhantomField && rng.Float64() < 0.5) {
			a.Status = ""
		}
	}
	return decision, nil
}

func (a LifetimeActor) poisonDamage(rng random.Generator) int {
	damage := a.MaxHP / 10
	if damage > 999 {
		damage = 950 + rng.Intn(100)
	}
	return damage
}

// ApplyPoison runs only in the reached normal-action block, regardless of tail
// suppression. Returns defeat facts; no automatic revival or asset restoration.
// Standard poison cures only when the target survives the debit.
func (a *LifetimeActor) ApplyPoison(rng random.Generator) (bool, error) {
	if a.HP <= 0 {
		return false, nil
	}
	if a.Status != StatusPoison && a.Status != StatusDeadlyPoison && a.Status != StatusVirulentPoison {
		return false, nil
	}
	if rng == nil {
		return false, ErrInvalidRequest
	}
	a.HP = max(0, a.HP-a.poisonDamage(rng))
	if a.HP == 0 {
		return true, nil
	}
	if a.Status == StatusPoison && rng.Float64() < 0.2 {
		a.Status = ""
	}
	return false, nil
}

// ApplyPartyPoison is the player normal-action block's other-ally virulent
// poison pass. NPC normal actions omit it. Returned deaths must be resolved
// before the actor's own poison and before saving (_battle.cgi:792–807).
func (s *LifetimeState) ApplyPartyPoison(actorID string, partyIDs []string, rng random.Generator) ([]string, error) {
	if s.actor(actorID) == nil || rng == nil {
		return nil, ErrInvalidRequest
	}
	for _, id := range partyIDs {
		if s.actor(id) == nil {
			return nil, ErrInvalidParticipant
		}
	}
	var deaths []string
	for _, id := range partyIDs {
		a := s.actor(id)
		if id == actorID || a.Status != StatusVirulentPoison {
			continue
		}
		dead, err := a.ApplyPoison(rng)
		if err != nil {
			return nil, err
		}
		if dead {
			deaths = append(deaths, id)
		}
	}
	return deaths, nil
}

// AdvanceTension applies the base staged rule. Accessory recursion/defense hooks
// are separate effects; requesterLevel is the requesting player's original level.
func (a *LifetimeActor) AdvanceTension(requesterLevel int, rng random.Generator) error {
	if a.Status == StatusSabaku {
		return nil
	}
	switch {
	case a.Tension <= 1:
		a.Tension = 1.7
	case a.Tension <= 2:
		a.Tension = 3
	case a.Tension <= 3:
		a.Tension = 5
	case requesterLevel < 25 || a.Tension >= 8:
		return nil
	default:
		if rng == nil {
			return ErrInvalidRequest
		}
		if requesterLevel >= int(rng.Float64()*50+20) {
			a.Tension = 8
		}
	}
	return nil
}

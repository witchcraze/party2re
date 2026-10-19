package battle

import "github.com/witchcraze/party2re/internal/core/random"

type StatusEntry struct {
	CallbackEntry
	SourceID, CMP int
	TargetScope   string
}

// StatusEntries certifies only the seventeen gem bodies and job35.1. The
// calling catalog and ordinary tools/books/NPCs retain their separate owners.
func StatusEntries() []StatusEntry {
	return []StatusEntry{
		{CallbackEntry: CallbackEntry{ID: "gem_cure_poison", Name: "party-poison-cure"}, SourceID: 55, CMP: 6, TargetScope: TargetScopeAllAllies},
		{CallbackEntry: CallbackEntry{ID: "gem_cure_paralysis", Name: "party-paralysis-cure"}, SourceID: 56, CMP: 6, TargetScope: TargetScopeAllAllies},
		{CallbackEntry: CallbackEntry{ID: "gem_cure_sleep", Name: "party-sleep-cure"}, SourceID: 57, CMP: 6, TargetScope: TargetScopeAllAllies},
		{CallbackEntry: CallbackEntry{ID: "gem_cure_confusion", Name: "party-confusion-cure"}, SourceID: 58, CMP: 6, TargetScope: TargetScopeAllAllies},
		{CallbackEntry: CallbackEntry{ID: "gem_cure_paralyze_action", Name: "party-immobilization-cure"}, SourceID: 59, CMP: 6, TargetScope: TargetScopeAllAllies},
		{CallbackEntry: CallbackEntry{ID: "gem_cure_atk_seal", Name: "party-physical-seal-cure"}, SourceID: 60, CMP: 11, TargetScope: TargetScopeAllAllies},
		{CallbackEntry: CallbackEntry{ID: "gem_cure_magic_seal", Name: "party-magic-seal-cure"}, SourceID: 61, CMP: 11, TargetScope: TargetScopeAllAllies},
		{CallbackEntry: CallbackEntry{ID: "gem_cure_all_abnormal", Name: "current-status-cure"}, SourceID: 62, CMP: 38, TargetScope: TargetScopeSingleAlly},
		{CallbackEntry: CallbackEntry{ID: "gem_instant_death_1", Name: "magic-instant-death"}, SourceID: 70, CMP: 15, TargetScope: TargetScopeSingleEnemy},
		{CallbackEntry: CallbackEntry{ID: "gem_instant_death_sky", Name: "area-magic-instant-death"}, SourceID: 71, CMP: 26, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "gem_inflict_poison", Name: "area-poison-breath"}, SourceID: 75, CMP: 6, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "gem_inflict_paralysis", Name: "area-paralysis-breath"}, SourceID: 76, CMP: 6, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "gem_inflict_sleep", Name: "area-sleep-breath"}, SourceID: 77, CMP: 6, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "gem_inflict_confusion", Name: "area-confusion-magic"}, SourceID: 78, CMP: 6, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "gem_inflict_seal_act", Name: "area-immobilization"}, SourceID: 79, CMP: 6, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "gem_inflict_seal_atk", Name: "area-physical-seal"}, SourceID: 80, CMP: 6, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "gem_inflict_seal_magic", Name: "area-magic-seal"}, SourceID: 81, CMP: 6, TargetScope: TargetScopeAllEnemies},
		{CallbackEntry: CallbackEntry{ID: "job-35-entry-1", Name: "effect-dispel", RequiredSP: 30, MPCost: 30}, TargetScope: "all_members"},
	}
}

func statusEntryEffect(source int, targetID string) (TargetEffect, float64) {
	e := TargetEffect{TargetID: targetID}
	chance := 0.0
	switch source {
	case 55, 75:
		e.Payload.Symbol = StatusDeadlyPoison
	case 56, 76:
		e.Payload.Symbol = StatusParalyze
	case 57, 77:
		e.Payload.Symbol = StatusSleep
	case 58, 78:
		e.Payload.Symbol = StatusConfusion
	case 59, 79:
		e.Payload.Symbol = StatusDofuu
	case 60, 80:
		e.Payload.Symbol = StatusPhysicalSeal
	case 61, 81:
		e.Payload.Symbol = StatusMagicSeal
	case 62:
		e.Payload.Symbol = "current_status_cure"
	case 70, 71:
		e.Payload.Symbol = "death"
		e.Category = CategoryMagic
		chance = 20
	}
	if source >= 75 && source <= 81 {
		chance = 60
		switch source {
		case 75:
			e.Category = CategoryBreath
		case 76, 77:
			e.Category = CategoryBreath
			chance = 35
		case 78:
			e.Category = CategoryMagic
		case 79:
			e.Category = CategoryPhysical
			chance = 50
		default:
			e.Category = CategoryPhysical
		}
	}
	return e, chance
}

func (s *LifetimeState) ValidateStatusEntry(c *StatusContext, reset *EquipmentResetContext, id, targetID string, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if id == "job-35-entry-1" {
		if reset == nil {
			return ErrUnsupportedEffect
		}
		child := *reset
		child.TargetContext = c.TargetContext
		for _, a := range s.Actors {
			if err := s.ValidateEquipmentReset(&child, a.ID, rng); err != nil {
				return err
			}
		}
		return nil
	}
	for _, entry := range StatusEntries() {
		if entry.ID != id {
			continue
		}
		if c.Custom && s.actor(targetID) == nil {
			targetID = c.ActorID
		}
		e, chance := statusEntryEffect(entry.SourceID, targetID)
		if entry.SourceID >= 70 {
			if entry.SourceID == 70 {
				return s.ValidateStatus(c, e, chance, false, true, rng)
			}
			return s.ValidateAreaStatus(c, e, chance, false, entry.SourceID == 71, rng)
		}
		if entry.SourceID == 62 {
			return s.validatePartyTarget(c.TargetContext, e, rng)
		}
		for _, target := range c.PartyIDs {
			e.TargetID = target
			if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrUnsupportedEffect
}

// ApplyStatusEntry runs intrinsic bodies only; learning, table assembly and late
// outer MP/CMP debits remain the ordinary/custom invocation owner's contract.
func (s *LifetimeState) ApplyStatusEntry(c *StatusContext, reset *EquipmentResetContext, id, targetID string, rng random.Generator) ([]StatusApplication, error) {
	if err := s.ValidateStatusEntry(c, reset, id, targetID, rng); err != nil {
		return nil, err
	}
	if id == "job-35-entry-1" {
		reset.TargetContext = c.TargetContext
		for _, member := range s.Actors {
			tension := s.actor(member.ID).Tension
			if err := s.ApplyEquipmentReset(reset, member.ID, rng); err != nil {
				return nil, err
			}
			s.actor(member.ID).Tension = tension
		}
		return nil, nil
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	previous := c.targetResources
	c.targetResources = func(r TargetResolution) {
		s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	defer func() { c.targetResources = previous }()
	for _, entry := range StatusEntries() {
		if entry.ID != id {
			continue
		}
		e, chance := statusEntryEffect(entry.SourceID, targetID)
		switch {
		case entry.SourceID == 62:
			target, err := s.ResolvePartyTarget(c.TargetContext, e, rng)
			if err != nil || !target.Resolved {
				return nil, err
			}
			c.targetResources(target)
			e.TargetID = target.TargetID
			e.Payload.Symbol = s.actor(target.TargetID).Status
			r, err := s.ApplyCure(c, e, rng)
			return []StatusApplication{r}, err
		case entry.SourceID < 70:
			return s.ApplyPartyCure(c, e, rng)
		case entry.SourceID == 70:
			r, err := s.ApplyStatus(c, e, chance, false, true, rng)
			return []StatusApplication{r}, err
		default:
			return s.ApplyAreaStatus(c, e, chance, false, entry.SourceID == 71, rng)
		}
	}
	return nil, ErrUnsupportedEffect
}

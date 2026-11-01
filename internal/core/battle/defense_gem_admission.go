package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateDefenseGems follows each possible status/target/temporary transition.
// MP amounts cannot change these guards and stay with the executing helper.
func (s *LifetimeState) ValidateDefenseGems(ids []int, c TargetContext, targetID string, rng random.Generator) error {
	if s == nil || len(ids) == 0 {
		return ErrInvalidRequest
	}
	if len(ids) > 3 {
		return ErrUnsupportedEffect
	}
	if err := s.validateTargetContext(c, rng); err != nil {
		return err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	states := []LifetimeState{s.callbackCopy()}
	for i, id := range ids {
		e, err := defenseGemEntry(id)
		if err != nil {
			return err
		}
		var next []LifetimeState
		for _, state := range states {
			if i > 0 && state.actor(c.ActorID).HP <= 0 {
				continue
			}
			children, err := state.defenseGemTransitions(e, c, targetID, rng)
			if err != nil {
				return err
			}
			next = append(next, children...)
		}
		states = next
	}
	return nil
}

// These are guard projections, not a second executor: shared target candidates
// own fallback, reflection, cover and confusion; the producer owns only writes.
func (s LifetimeState) defenseGemTransitions(e DefenseGemEntry, c TargetContext, targetID string, rng random.Generator) ([]LifetimeState, error) {
	a := s.actor(c.ActorID)
	if (e.SourceID == 41 || e.SourceID == 42 || e.SourceID == 45) && a.categorySealed(e.Category) {
		// A failed helper would mutate an unrepresented empty source record.
		return nil, ErrUnsupportedEffect
	}
	if e.TargetScope == TargetScopeSingleAlly {
		if err := s.validatePartyTarget(c, e.target(targetID), rng); err != nil {
			return nil, err
		}
	}
	var next []LifetimeState
	if a.categorySealed(e.Category) {
		next = append(next, s.callbackCopy()) // Failed cure: accepted no-effect.
		s = s.callbackCopy()
		s.actor(c.ActorID).Status = ""
	}
	if e.TargetScope != TargetScopeSingleAlly {
		ids := []string{c.ActorID}
		if e.TargetScope == TargetScopeAllAllies {
			ids = c.PartyIDs
		}
		for _, id := range ids {
			if e.TargetScope == TargetScopeSelf || s.actor(id).HP > 0 {
				e.write(&s, c, id)
			}
		}
		return append(next, s), nil
	}
	statuses := []string{s.actor(c.ActorID).Status}
	if c.Item195Activated && statuses[0] == "" {
		statuses = append(statuses, StatusConfusion)
	}
	for _, status := range statuses {
		branch := s.callbackCopy()
		branch.actor(c.ActorID).Status = status
		resolved := c
		resolved.Item195Activated = false // Activation is already branched above.
		for _, target := range branch.partyTargetCandidates(resolved, e.target(targetID), nil) {
			child := branch.callbackCopy()
			e.write(&child, c, target.TargetID)
			next = append(next, child)
		}
	}
	return next, nil
}

// ProjectDefenseGems supplies only an exact, resource-complete transition.
// Uncertain target, seal or absorption paths retain conservative invocation
// admission; a finite multi-slot body is still validated together above.
func (s *LifetimeState) ProjectDefenseGems(ids []int, c TargetContext, targetID string) (bool, error) {
	states, err := s.ProjectDefenseGemBranches(ids, c, targetID)
	if err != nil || len(states) != 1 {
		return false, err
	}
	*s = states[0]
	return true, nil
}

// ProjectDefenseGemBranches retains alternative complete predecessor states.
// Empty means unknown: random MP absorption needs its resource owner's complete
// continuation before later callbacks can rely on these transitions.
func (s *LifetimeState) ProjectDefenseGemBranches(ids []int, c TargetContext, targetID string) ([]LifetimeState, error) {
	if err := s.ValidateDefenseGems(ids, c, targetID, random.Default()); err != nil {
		return nil, err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	states := []LifetimeState{s.callbackCopy()}
	for i, id := range ids {
		e, err := defenseGemEntry(id)
		if err != nil {
			return nil, err
		}
		var next []LifetimeState
		for _, state := range states {
			if i > 0 && state.actor(c.ActorID).HP <= 0 {
				next = append(next, state)
				continue
			}
			if e.TargetScope == TargetScopeSingleAlly && e.Category == CategoryMagic {
				for _, initial := range state.partyInitialTargetIDs(c, e.target(targetID)) {
					if state.actor(initial).Temporary == TemporaryAbsorbMagic {
						return nil, nil
					}
				}
			}
			children, err := state.defenseGemTransitions(e, c, targetID, random.Default())
			if err != nil {
				return nil, err
			}
			next = append(next, children...)
		}
		states = next
	}
	return states, nil
}

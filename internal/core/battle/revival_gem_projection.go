package battle

import "github.com/witchcraze/party2re/internal/core/random"

type RevivalContinuation struct {
	State   LifetimeState
	Context RevivalContext
}

func revivalContinuation(s LifetimeState, c RevivalContext) RevivalContinuation {
	f := cloneCallbackContext(CallbackContext{TargetContext: c.TargetContext, Defeat: c.NativeDefeat})
	c.TargetContext, c.NativeDefeat = f.TargetContext, f.Defeat
	if c.Deaths != nil {
		deaths := *c.Deaths
		c.Deaths = &deaths
	}
	return RevivalContinuation{State: s.callbackCopy(), Context: c}
}

// ProjectRevivalGemBranches certifies63–66 only. Opaque defeat hooks and other
// revival families stay unknown until their owning producer supplies transitions.
func (s *LifetimeState) ProjectRevivalGemBranches(c *RevivalContext, id, targetID string) ([]RevivalContinuation, error) {
	if s == nil || c == nil {
		return nil, ErrInvalidRequest
	}
	if id != "gem-63" && id != "gem-64" && id != "gem-65" && id != "gem-66" {
		return nil, nil
	}
	if id != "gem-66" {
		if err := s.ValidateRevival(c, id, targetID, random.Default()); err != nil {
			return nil, err
		}
	} else if err := s.validateTargetContext(c.TargetContext, random.Default()); err != nil {
		return nil, err
	}
	for _, a := range s.Actors {
		if a.MaxHP < 0 || a.MaxHP > 1e12 || a.MaxMP < 0 || a.MaxMP > 1e12 || a.MP < 0 || a.MP > 1e12 {
			return nil, ErrInvalidParticipant
		}
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	p := s.callbackCopy()
	if id == "gem-66" {
		if c.NativeDefeat == nil {
			return nil, nil
		}
		for _, target := range c.PartyIDs {
			if target != c.ActorID {
				p.actor(target).HP = p.actor(target).MaxHP
			}
		}
		defeat := *c.NativeDefeat
		defeat.TargetContext = c.TargetContext
		states, err := p.projectDefeatBranches(&defeat, c.ActorID, random.Default(), map[string]bool{}, true)
		if err != nil {
			return nil, err
		}
		var results []RevivalContinuation
		for _, state := range states {
			state.State.actor(c.ActorID).MP = 0
			child := *c
			child.NativeDefeat = &state.Context
			results = append(results, revivalContinuation(state.State, child))
		}
		return results, nil
	}
	if id == "gem-65" {
		states := []LifetimeState{p}
		for _, target := range c.PartyIDs {
			var next []LifetimeState
			for _, state := range states {
				next = appendDefeatState(next, state)
				if state.actor(target).HP <= 0 {
					child := state.callbackCopy()
					child.actor(target).HP = child.actor(target).MaxHP / 2
					next = appendDefeatState(next, child)
				}
			}
			states = next
		}
		var results []RevivalContinuation
		for _, state := range states {
			results = append(results, revivalContinuation(state, *c))
		}
		return results, nil
	}
	targets, err := p.projectSymbolicTargets(c.TargetContext, revivalTarget(id, targetID), false)
	if err != nil {
		return nil, err
	}
	var results []RevivalContinuation
	for _, branch := range targets {
		if branch.Resolution.Resolved {
			t := branch.State.actor(branch.Resolution.TargetID)
			if t.HP <= 0 {
				t.HP = t.MaxHP
				if id == "gem-63" {
					t.HP /= 2
				}
			}
		}
		results = append(results, revivalContinuation(branch.State, *c))
	}
	return results, nil
}

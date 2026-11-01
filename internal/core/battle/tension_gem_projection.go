package battle

import (
	"reflect"

	"github.com/witchcraze/party2re/internal/core/random"
)

// RecoveryStatContinuation retains operation counters as well as battle facts.
type RecoveryStatContinuation struct {
	State   LifetimeState
	Context EffectContext
}

// Only TensionCalls changes in these five bodies; other context facts survive.
func appendTensionContinuation(branches []RecoveryStatContinuation, s LifetimeState, c EffectContext) []RecoveryStatContinuation {
	for _, old := range branches {
		if old.Context.TensionCalls == c.TensionCalls && reflect.DeepEqual(old.State, s) {
			return branches
		}
	}
	f := cloneCallbackContext(CallbackContext{Damage: &DamageContext{EffectContext: c}})
	return append(branches, RecoveryStatContinuation{State: s.callbackCopy(), Context: f.Damage.EffectContext})
}

// ProjectRecoveryStatGemBranches supplies complete alternatives for54/67–69/74.
// Other recovery/stat predecessors remain unknown until their owners supply
// complete numeric, resource and equipment-tail transitions.
func (s *LifetimeState) ProjectRecoveryStatGemBranches(ids []int, c *EffectContext, targetID string) ([]RecoveryStatContinuation, error) {
	if s == nil || c == nil {
		return nil, ErrInvalidRequest
	}
	for _, id := range ids {
		switch id {
		case 54, 67, 68, 69, 74:
		default:
			return nil, nil
		}
	}
	if err := s.ValidateRecoveryStatGems(ids, c, targetID, random.Default()); err != nil {
		return nil, err
	}
	if c.Custom && s.actor(targetID) == nil {
		targetID = c.ActorID
	}
	branches := appendTensionContinuation(nil, *s, *c)
	for i, id := range ids {
		var next []RecoveryStatContinuation
		for _, branch := range branches {
			if i > 0 && branch.State.actor(c.ActorID).HP <= 0 {
				next = appendTensionContinuation(next, branch.State, branch.Context)
				continue
			}
			children, err := branch.State.projectTensionGem(id, branch.Context, targetID)
			if err != nil {
				return nil, err
			}
			for _, child := range children {
				next = appendTensionContinuation(next, child.State, child.Context)
			}
		}
		branches = next
	}
	return branches, nil
}

func (s LifetimeState) projectTensionGem(id int, c EffectContext, targetID string) ([]RecoveryStatContinuation, error) {
	switch id {
	case 54:
		for _, id := range c.PartyIDs {
			s.actor(id).Hit = 95
		}
	case 69:
		for i := range s.Actors {
			s.Actors[i].Tension = 1
		}
	case 67:
		first, err := s.projectStatTension(c, c.ActorID, false)
		if err != nil {
			return nil, err
		}
		var next []RecoveryStatContinuation
		for _, branch := range first {
			children, err := branch.State.projectStatTension(branch.Context, c.ActorID, false)
			if err != nil {
				return nil, err
			}
			for _, child := range children {
				next = appendTensionContinuation(next, child.State, child.Context)
			}
		}
		return next, nil
	case 74:
		var next []RecoveryStatContinuation
		for _, branch := range s.tensionConfusionBranches(c) {
			resolved := c.TargetContext
			resolved.Item195Activated = false // Activation alternatives are already isolated.
			e := controlSymbol(targetID, "encourage", CategoryUntyped)
			if err := branch.validatePartyTarget(resolved, e, random.Default()); err != nil {
				return nil, err
			}
			for _, target := range branch.partyTargetCandidates(resolved, e, nil) {
				child := branch.callbackCopy()
				children, err := child.projectStatTension(c, target.TargetID, false)
				if err != nil {
					return nil, err
				}
				for _, result := range children {
					next = appendTensionContinuation(next, result.State, result.Context)
				}
			}
		}
		return next, nil
	case 68:
		return s.projectTensionReset(c, targetID)
	}
	return appendTensionContinuation(nil, s, c), nil
}

func (s LifetimeState) tensionConfusionBranches(c EffectContext) []LifetimeState {
	branches := []LifetimeState{s.callbackCopy()}
	if c.Item195Activated && s.actor(c.ActorID).Status == "" {
		child := s.callbackCopy()
		child.actor(c.ActorID).Status = StatusConfusion
		branches = append(branches, child)
	}
	return branches
}

func (s LifetimeState) projectTensionReset(c EffectContext, targetID string) ([]RecoveryStatContinuation, error) {
	e, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, controlSymbol(targetID, "destroy", CategoryPhysical))
	if err != nil {
		return nil, err
	}
	var next []RecoveryStatContinuation
	for _, state := range s.tensionConfusionBranches(c) {
		if len(c.EnemyIDs) == 0 {
			next = appendTensionContinuation(next, state, c)
			continue
		}
		a := state.actor(c.ActorID)
		if a.categorySealed(e.Category) {
			next = appendTensionContinuation(next, state, c) // Failed cure.
			a.Status = ""
		}
		if a.Status != StatusConfusion {
			ids := []string{targetID}
			if t := state.actor(targetID); t == nil || t.TeamID == a.TeamID {
				ids = c.EnemyIDs
			}
			for _, id := range ids {
				if state.actor(id).HP <= 0 {
					next = appendTensionContinuation(next, state, c)
					break // Selection precedes the fallen guard, without redirection.
				}
			}
		}
		resolved := DamageContext{EffectContext: c}
		resolved.Item195Activated = false
		for _, target := range state.damageCandidates(&resolved, e) {
			child := state.callbackCopy()
			child.actor(target.TargetID).Tension = 1
			next = appendTensionContinuation(next, child, c)
		}
	}
	return next, nil
}

func (s LifetimeState) projectStatTension(c EffectContext, targetID string, recursive bool) ([]RecoveryStatContinuation, error) {
	c.TensionCalls++
	t := s.actor(targetID)
	if t.Status == StatusSabaku {
		return appendTensionContinuation(nil, s, c), nil
	}
	var base []RecoveryStatContinuation
	switch {
	case t.Tension <= 3:
		if err := t.AdvanceTension(0, nil); err != nil {
			return nil, err
		}
	case t.Tension < 8:
		if c.RequesterLevel == nil || *c.RequesterLevel < 0 {
			return nil, ErrUnsupportedEffect
		}
		if *c.RequesterLevel >= 25 {
			if *c.RequesterLevel < 69 {
				base = appendTensionContinuation(base, s, c)
			}
			t.Tension = 8
		}
	}
	base = appendTensionContinuation(base, s, c)
	var next []RecoveryStatContinuation
	for _, branch := range base {
		variants := []RecoveryStatContinuation{branch}
		if !recursive && targetID == c.ActorID && !c.ActorNPC && hasItem(branch.State.actor(c.ActorID).ItemDefinitionIDs, "item-145") {
			child := branch.State.callbackCopy()
			children, err := child.projectStatTension(branch.Context, targetID, true)
			if err != nil {
				return nil, err
			}
			variants = append(variants, children...)
		}
		for _, variant := range variants {
			next = appendTensionContinuation(next, variant.State, variant.Context)
			t := variant.State.actor(targetID)
			if !hasItem(t.ItemDefinitionIDs, "item-161") || t.Temporary == TemporaryDefense || t.Temporary == TemporaryStrongDefense || t.Temporary == TemporaryOneHitDefense {
				continue
			}
			for _, defense := range []TemporaryEffect{TemporaryDefense, TemporaryStrongDefense} {
				if defense == TemporaryStrongDefense && t.Tension < 5 {
					continue
				}
				child := variant.State.callbackCopy()
				child.actor(targetID).Temporary = defense
				next = appendTensionContinuation(next, child, variant.Context)
			}
		}
	}
	return next, nil
}

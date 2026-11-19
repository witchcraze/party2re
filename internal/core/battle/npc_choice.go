package battle

import "github.com/witchcraze/party2re/internal/core/random"

// NPCChoice is a weighted position with the last eligible binding for its name.
type NPCChoice struct {
	CallbackBinding
	Custom      bool
	Invocations []CallbackInvocation
}

func (choice NPCChoice) frame(c *CallbackContext) CallbackContext {
	frame := *c
	frame.Arguments = nil
	frame.Custom = choice.Custom
	if choice.Custom {
		frame.Arguments = []string{c.ActorID}
	}
	return frame
}

func (choice NPCChoice) invocations() []CallbackInvocation {
	if choice.Custom {
		return choice.Invocations
	}
	return []CallbackInvocation{choice.Invocation}
}

func (choice NPCChoice) charge(a *LifetimeActor) {
	a.MP = max(0, a.MP-choice.MPCost)
	if load := a.NPCLoadout; load != nil && (choice.Custom || choice.Name == load.CustomName) {
		a.CMP -= load.CustomCost
	}
}

// ValidateNPCChoices validates each eligible position without selecting one.
func (s *LifetimeState) ValidateNPCChoices(c *CallbackContext, plan NPCActionPlan, rng random.Generator) error {
	return s.validateNPCChoices(c, plan, rng, nil)
}

func (s *LifetimeState) validateNPCChoices(c *CallbackContext, plan NPCActionPlan, rng random.Generator, after func(*LifetimeState, *CallbackContext) error) error {
	if rng == nil || c == nil {
		return ErrInvalidRequest
	}
	if len(plan.Choices) == 0 {
		return ErrUnsupportedEffect
	}
	for _, choice := range plan.Choices {
		frame := choice.frame(c)
		var continuation func(*LifetimeState, *CallbackContext) error
		if after != nil {
			continuation = func(state *LifetimeState, frame *CallbackContext) error {
				choice.charge(state.actor(frame.ActorID))
				return after(state, frame)
			}
		}
		if choice.ID == "npc-normal-attack" {
			if c.Damage == nil || !c.Damage.ActorNPC {
				return ErrUnsupportedEffect
			}
			child := *c.Damage
			child.TargetContext = frame.TargetContext
			if err := s.ValidateNormalAttack(&child, "", rng); err != nil {
				return err
			}
			defeat := child.responseDefeat
			if defeat == nil && continuation == nil {
				continue
			}
			// The default skill calls kougeki once. The critical travels
			// through the direct-power branch.
			power := float64(s.actor(frame.ActorID).Attack)
			for _, hit := range []TargetEffect{
				{Category: CategoryPhysical, Payload: CallbackPayload{Number: power}, Arguments: []string{"0", "1"}},
				{Category: CategoryPhysical, Payload: CallbackPayload{Number: power * .75}, Arguments: []string{"1", "1"}},
			} {
				states, err := s.projectCallbackDamage(&child, defeat, hit, hit.Payload.Number, continuation != nil, rng)
				if err != nil {
					return err
				}
				if continuation != nil {
					for _, branch := range states {
						state, projected := branch.State.callbackCopy(), frame
						projected.Damage, projected.TargetContext = &branch.Context, branch.Context.TargetContext
						projected = cloneCallbackContext(projected)
						if err := continuation(&state, &projected); err != nil {
							return err
						}
					}
				}
			}
			continue
		}
		invocations := choice.invocations()
		arguments := make([][]string, len(invocations))
		for i := range invocations {
			arguments[i] = frame.Arguments
		}
		if err := s.validateSequenceThen(&frame, plan.Catalog, invocations, arguments, rng, continuation); err != nil {
			return err
		}
	}
	return nil
}

// ApplyNPCChoice executes an admitted position, followed by its listed cost.
func (s *LifetimeState) ApplyNPCChoice(c *CallbackContext, d CallbackCatalog, choice NPCChoice, rng random.Generator) error {
	if s == nil || c == nil {
		return ErrInvalidRequest
	}
	if a := s.actor(c.ActorID); a == nil || !a.NPC {
		return ErrInvalidParticipant
	}
	frame := choice.frame(c)
	if choice.ID == "npc-normal-attack" {
		if c.Damage == nil || !c.Damage.ActorNPC {
			return ErrUnsupportedEffect
		}
		c.Damage.TargetContext = frame.TargetContext
		if _, err := s.ApplyNormalAttack(c.Damage, "", rng); err != nil {
			return err
		}
		frame.TargetContext = c.Damage.TargetContext
	} else if err := s.ApplyInvocations(&frame, d, choice.invocations(), rng); err != nil {
		return err
	}
	a := s.actor(frame.ActorID)
	if a == nil || !a.NPC {
		return ErrInvalidParticipant
	}
	*c = frame
	choice.charge(a)
	return nil
}

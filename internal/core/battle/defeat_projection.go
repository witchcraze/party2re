package battle

import (
	"math"
	"reflect"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

// These are reset outcome boundaries, never draws from the caller's stream.
type resetBoundary float64

func (r resetBoundary) Float64() float64 { return float64(r) }
func (r resetBoundary) Intn(n int) int   { return min(n-1, int(float64(n)*float64(r))) }
func (r resetBoundary) IntN(n int) int   { return r.Intn(n) }

func (s LifetimeState) defeatCopy() LifetimeState {
	return s.callbackCopy()
}

func appendDefeatState(states []LifetimeState, next LifetimeState) []LifetimeState {
	for _, state := range states {
		if reflect.DeepEqual(state, next) {
			return states
		}
	}
	return append(states, next)
}

type defeatContinuation struct {
	State   LifetimeState
	Context DefeatContext
}

// Reset retains finite resource/temporary alternatives and independent stat
// branches; continuous revival HP and weapon power retain their shared bounds.
func (s *LifetimeState) projectDefeatResetBranches(c *DefeatContext, id string) ([]defeatContinuation, bool, error) {
	a := s.actor(id)
	if a == nil {
		return nil, false, ErrInvalidParticipant
	}
	branch := automaticRevivalBranch(a)
	values := []float64{0}
	masks := 2
	if a.Gear != nil && a.Gear.AccessoryID == "item-159" {
		masks = 16
	}
	if a.Gear != nil && (a.Gear.AccessoryID == "item-111" || a.Gear.AccessoryID == "item-112") {
		values = nil
		for i := 0; i < 9; i++ {
			values = append(values, (float64(i)+.5)/9)
		}
	}
	if a.Gear != nil && a.Gear.AccessoryID == "item-247" {
		values = []float64{0, .5, math.Nextafter(1, 0)}
	}
	var states []defeatContinuation
	prefixes := []float64{0, math.Nextafter(1, 0)}
	if branch == "temporary" {
		for _, hp := range []int{999, 1000} {
			draw := ((float64(hp)+.5)/float64(a.MaxHP) - .1) / .1
			if draw > 0 && draw < 1 {
				prefixes = append(prefixes, draw)
			}
		}
	}
	for _, prefix := range prefixes {
		for _, value := range values {
			for mask := range masks {
				p := s.defeatCopy()
				context := *c
				child := &context.EquipmentResetContext
				draw := &damageResetDraws{bits: mask}
				if a.Gear != nil && (a.Gear.AccessoryID == "item-111" || a.Gear.AccessoryID == "item-112" || a.Gear.AccessoryID == "item-247") {
					v := value
					draw.first = &v
				}
				reset := func(id string) error {
					return p.ApplyEquipmentReset(child, id, draw)
				}
				auto := AutomaticRevivalContext{TargetContext: c.TargetContext, Reset: reset}
				r, err := p.ApplyAutomaticRevival(&auto, id, resetBoundary(prefix))
				if err != nil {
					return nil, false, err
				}
				if !r.Prevented {
					tmp := p.actor(id).Temporary
					p.actor(id).HP = 0
					if err := reset(id); err != nil {
						return nil, false, err
					}
					if tmp == TemporaryEffect("death_mark") {
						p.actor(id).Temporary = tmp
					}
					if hasItem(p.actor(id).ItemDefinitionIDs, "item-212") {
						e := TargetEffect{Payload: CallbackPayload{Number: float64(p.actor(id).Attack) * 1.5}}
						if err := p.validateRecovery(&EffectContext{TargetContext: c.TargetContext}, e, resetBoundary(value)); err != nil {
							return nil, false, err
						}
					}
				}
				duplicate := false
				for _, state := range states {
					if state.Context.Cursor == context.Cursor && reflect.DeepEqual(state.State, p) {
						duplicate = true
						break
					}
				}
				if !duplicate {
					states = append(states, defeatContinuation{State: p, Context: context})
				}
			}
		}
		if branch != "temporary" {
			break
		}
	}
	return states, branch != "", nil
}

func (s *LifetimeState) projectDefeatContinuation(c *DefeatContext, id string, rng random.Generator, visiting map[string]bool, ordered bool) ([]LifetimeState, error) {
	branches, err := s.projectDefeatBranches(c, id, rng, visiting, ordered)
	var states []LifetimeState
	for _, branch := range branches {
		states = appendDefeatState(states, branch.State)
	}
	return states, err
}

func (s *LifetimeState) projectDefeatBranches(c *DefeatContext, id string, rng random.Generator, visiting map[string]bool, ordered bool) ([]defeatContinuation, error) {
	if visiting[id] {
		return nil, ErrUnsupportedEffect
	}
	visiting[id] = true
	defer delete(visiting, id)
	states, prevented, err := s.projectDefeatResetBranches(c, id)
	if err != nil || prevented {
		return states, err
	}
	if err := s.validateDefeatRewards(c, id); err != nil {
		return nil, err
	}
	var results []defeatContinuation
	for _, branch := range states {
		state, context := branch.State, branch.Context
		continuations := []defeatContinuation{branch}
		if hasItem(state.actor(id).ItemDefinitionIDs, "item-212") {
			if _, err := state.defeatStrikeCandidates(&context, id, rng); err != nil {
				return nil, err
			}
			candidates, err := state.projectSymbolicTargets(context.TargetContext, TargetEffect{Payload: CallbackPayload{Symbol: "last_strike"}}, !slices.Contains(context.EnemyIDs, id))
			if err != nil {
				return nil, err
			}
			continuations = nil
			for _, candidate := range candidates {
				p := candidate.State
				t := p.actor(candidate.Resolution.TargetID)
				if !candidate.Resolution.Resolved {
					return nil, ErrUnsupportedEffect
				}
				power := float64(p.actor(id).Attack) * 1.5
				if t.ConsumeOneHitDefense() || (power == 0 && t.HP > 0) {
					continuations = append(continuations, defeatContinuation{State: p, Context: context})
					continue
				}
				if err := c.ToolCategory.validate(); err != nil {
					return nil, err
				}
				if t.Gear == nil {
					return nil, ErrUnsupportedEffect
				}
				maximum := rollDamage(power, toolDefense(t), false, t.BaseStats.Defense, resetBoundary(math.Nextafter(1, 0)))
				minimum := rollDamage(power, toolDefense(t), false, t.BaseStats.Defense, resetBoundary(0))
				if int((power*.5-toolDefense(t))*.9) < 1 {
					minimum = 1
					maximum = max(2, maximum)
				}
				for _, hp := range damageSurvivorHPs(t.HP, minimum, maximum) {
					alive := p.defeatCopy()
					alive.actor(t.ID).HP = hp
					continuations = append(continuations, defeatContinuation{State: alive, Context: context})
				}
				if t.HP <= maximum {
					t.HP = 0
					next, err := p.projectDefeatBranches(&context, t.ID, rng, visiting, ordered)
					if err != nil {
						return nil, err
					}
					continuations = append(continuations, next...)
				}
			}
		}
		for _, continuation := range continuations {
			continuation.State.applyDefeatInheritance(&continuation.Context, id)
			if ordered {
				tails, err := continuation.State.projectDefeatTail(&continuation.Context, id)
				if err != nil {
					return nil, err
				}
				for _, tail := range tails {
					results = append(results, defeatContinuation{State: tail, Context: continuation.Context})
				}
				continue
			}
			results = append(results, continuation)
		}
	}
	return results, nil
}

func (s *LifetimeState) projectDefeatTail(c *DefeatContext, id string) ([]LifetimeState, error) {
	if s.actor(c.ActorID).NPC || id == c.ActorID {
		return []LifetimeState{*s}, nil
	}
	states := []LifetimeState{*s}
	if c.Rewards.Project != nil {
		var err error
		states, err = c.Rewards.Project(s, c.TargetContext, id)
		if err != nil {
			return nil, err
		}
		if len(states) == 0 {
			return nil, ErrUnsupportedEffect
		}
	}
	var results []LifetimeState
	for _, state := range states {
		if c.FieldAtLoad == "fortress" && state.Fields.AntiTurns == 0 {
			if c.Fortress == nil || c.Fortress.Project == nil {
				return nil, ErrUnsupportedEffect
			}
			tails, err := c.Fortress.Project(&state, c.TargetContext, id)
			if err != nil {
				return nil, err
			}
			if len(tails) == 0 {
				return nil, ErrUnsupportedEffect
			}
			for _, tail := range tails {
				results = appendDefeatState(results, tail)
			}
		} else {
			results = appendDefeatState(results, state)
		}
	}
	return results, nil
}

func (s *LifetimeState) validateDefeatRewards(c *DefeatContext, id string) error {
	if s.actor(c.ActorID).NPC || id == c.ActorID {
		return nil
	}
	if c.Rewards == nil || c.Rewards.Validate == nil || c.Rewards.Apply == nil {
		return ErrUnsupportedEffect
	}
	if err := c.Rewards.Validate(s, c.TargetContext, id); err != nil {
		return err
	}
	if c.FieldAtLoad == "fortress" && s.Fields.AntiTurns == 0 {
		if c.Fortress == nil || c.Fortress.Validate == nil || c.Fortress.Apply == nil {
			return ErrUnsupportedEffect
		}
		return c.Fortress.Validate(s, c.TargetContext, id)
	}
	return nil
}

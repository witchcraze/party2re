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
	s.Actors = slices.Clone(s.Actors)
	for i := range s.Actors {
		s.Actors[i].Abilities = slices.Clone(s.Actors[i].Abilities)
		s.Actors[i].ConsumedItems = slices.Clone(s.Actors[i].ConsumedItems)
	}
	return s
}

func appendDefeatState(states []LifetimeState, next LifetimeState) []LifetimeState {
	for _, state := range states {
		if reflect.DeepEqual(state, next) {
			return states
		}
	}
	return append(states, next)
}

// Reset has finite resource/temporary outcomes; numerical stats are bounded by
// the two extremes (159's independent stat branches need not be correlated).
func (s *LifetimeState) projectDefeatReset(c *DefeatContext, id string) ([]LifetimeState, bool, error) {
	a := s.actor(id)
	if a == nil {
		return nil, false, ErrInvalidParticipant
	}
	branch := automaticRevivalBranch(a)
	values := []float64{0, math.Nextafter(1, 0)}
	if a.Gear != nil && a.Gear.AccessoryID == "item-111" {
		for i := 1; i < 9; i++ {
			values = append(values, (float64(i)+.5)/9)
		}
	}
	if a.Gear != nil && a.Gear.AccessoryID == "item-247" {
		values = append(values, .5)
	}
	var states []LifetimeState
	for _, prefix := range []float64{0, math.Nextafter(1, 0)} {
		for _, value := range values {
			p := s.defeatCopy()
			child := c.EquipmentResetContext
			reset := func(id string) error {
				t := p.actor(id)
				if branch == "temporary" && t.HP > 0 {
					t.HP = max(1, int(float64(t.MaxHP)*.1))
				}
				return p.ApplyEquipmentReset(&child, id, resetBoundary(value))
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
			states = appendDefeatState(states, p)
		}
		if branch != "temporary" {
			break
		}
	}
	attack, defense, hp := states[0].actor(id).Attack, states[0].actor(id).Defense, math.MaxInt
	for i := range states {
		t := states[i].actor(id)
		attack, defense = max(attack, t.Attack), min(defense, t.Defense)
		if t.HP > 0 {
			hp = min(hp, t.HP)
		}
	}
	var bounded []LifetimeState
	for i := range states {
		t := states[i].actor(id)
		t.Attack, t.Defense = attack, defense
		if t.HP > 0 {
			t.HP = hp
		}
		bounded = appendDefeatState(bounded, states[i])
	}
	return bounded, branch != "", nil
}

func (s *LifetimeState) projectDefeatContinuation(c *DefeatContext, id string, rng random.Generator, visiting map[string]bool, ordered bool) ([]LifetimeState, error) {
	if visiting[id] {
		return nil, ErrUnsupportedEffect
	}
	visiting[id] = true
	defer delete(visiting, id)
	states, prevented, err := s.projectDefeatReset(c, id)
	if err != nil || prevented {
		return states, err
	}
	if err := s.validateDefeatRewards(c, id); err != nil {
		return nil, err
	}
	var results []LifetimeState
	for _, state := range states {
		continuations := []LifetimeState{state}
		if hasItem(state.actor(id).ItemDefinitionIDs, "item-212") {
			candidates, err := state.defeatStrikeCandidates(c, id, rng)
			if err != nil {
				return nil, err
			}
			continuations = nil
			for _, candidate := range candidates {
				p := state.defeatCopy()
				t := p.actor(candidate.TargetID)
				power := float64(p.actor(id).Attack) * 1.5
				if t.ConsumeOneHitDefense() || (power == 0 && t.HP > 0) {
					continuations = appendDefeatState(continuations, p)
					continue
				}
				if err := c.ToolCategory.validate(); err != nil {
					return nil, err
				}
				if t.Gear == nil {
					return nil, ErrUnsupportedEffect
				}
				maximum := rollDamage(power, toolDefense(t), false, t.BaseStats.Defense, resetBoundary(math.Nextafter(1, 0)))
				if int((power*.5-toolDefense(t))*.9) < 1 {
					maximum = max(2, maximum)
				}
				if t.HP > 1 {
					alive := p.defeatCopy()
					alive.actor(t.ID).HP = max(1, t.HP-maximum)
					continuations = appendDefeatState(continuations, alive)
				}
				if t.HP <= maximum {
					t.HP = 0
					next, err := p.projectDefeatContinuation(c, t.ID, rng, visiting, ordered)
					if err != nil {
						return nil, err
					}
					for _, n := range next {
						continuations = appendDefeatState(continuations, n)
					}
				}
			}
		}
		for _, continuation := range continuations {
			continuation.applyDefeatInheritance(c, id)
			if ordered {
				if err := continuation.projectDefeatTail(c, id); err != nil {
					return nil, err
				}
			}
			results = appendDefeatState(results, continuation)
		}
	}
	return results, nil
}

func (s *LifetimeState) projectDefeatTail(c *DefeatContext, id string) error {
	if s.actor(c.ActorID).NPC || id == c.ActorID {
		return nil
	}
	if c.Rewards.Project != nil {
		if err := c.Rewards.Project(s, c.TargetContext, id); err != nil {
			return err
		}
	}
	if c.FieldAtLoad == "fortress" && s.Fields.AntiTurns == 0 {
		if c.Fortress == nil || c.Fortress.Project == nil {
			return ErrUnsupportedEffect
		}
		return c.Fortress.Project(s, c.TargetContext, id)
	}
	return nil
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

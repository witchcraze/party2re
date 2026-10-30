package battle

import (
	"math"

	"github.com/witchcraze/party2re/internal/core/random"
)

// Reset draws are independent:159 can mix its three stat branches before71's
// weapon draw. Other accessories add finite resource/extended-state outcomes.
type damageResetDraws struct {
	bits  int
	first *float64
}

func (r *damageResetDraws) Float64() float64 {
	if r.first != nil {
		v := *r.first
		r.first = nil
		return v
	}
	v := float64(r.bits&1) * math.Nextafter(1, 0)
	r.bits >>= 1
	return v
}
func (r *damageResetDraws) Intn(n int) int { return min(n-1, int(r.Float64()*float64(n))) }
func (r *damageResetDraws) IntN(n int) int { return r.Intn(n) }

func (s *LifetimeState) projectDamageReset(defeat *DefeatContext, target TargetContext, id string, keepStatus bool, rng random.Generator) ([]LifetimeState, error) {
	if defeat == nil {
		return nil, ErrUnsupportedEffect
	}
	reset := defeat.EquipmentResetContext
	reset.TargetContext = target
	states, _, err := s.projectEquipmentReset(reset, id, rng)
	if err != nil {
		return nil, err
	}
	for i := range states {
		states[i].actor(id).Tension = s.actor(id).Tension
		if keepStatus {
			states[i].actor(id).Status = s.actor(id).Status
		}
	}
	return states, nil
}

func (s *LifetimeState) projectEquipmentReset(reset EquipmentResetContext, id string, rng random.Generator) ([]LifetimeState, TargetCursor, error) {
	if err := s.ValidateEquipmentReset(&reset, id, rng); err != nil {
		return nil, reset.Cursor, err
	}
	values := []float64{0}
	switch s.actor(id).Gear.AccessoryID {
	case "item-111", "item-112":
		values = nil
		for i := range 9 {
			values = append(values, (float64(i)+.5)/9)
		}
	case "item-247":
		values = []float64{0, .5, math.Nextafter(1, 0)}
	}
	var states []LifetimeState
	cursor := reset.Cursor
	for _, value := range values {
		for mask := range 16 {
			p, child := s.callbackCopy(), reset
			draw := &damageResetDraws{bits: mask}
			if len(values) > 1 {
				v := value
				draw.first = &v
			}
			if err := p.ApplyEquipmentReset(&child, id, draw); err != nil {
				return nil, cursor, err
			}
			cursor = child.Cursor
			states = appendDefeatState(states, p)
		}
	}
	return states, cursor, nil
}

// Interception is a nonlethal direct tool hit: it consumes the actor's one-hit
// defense, otherwise lowers HP, then decrements the interceptor's duration.
func (s *LifetimeState) projectDamageInterception(c *DamageContext, target string, rng random.Generator) ([]LifetimeState, error) {
	p := s.callbackCopy()
	a, t := p.actor(c.ActorID), p.actor(target)
	if a.ConsumeOneHitDefense() {
		t.ExtendedTurns--
		if t.ExtendedTurns <= 0 {
			t.Extended = ""
		}
		return []LifetimeState{p}, nil
	}
	power := float64(t.Attack)*.2 + 5
	if err := p.validateRecovery(&c.EffectContext, controlEffect(c.ActorID, power, CategoryUntyped), rng); err != nil {
		return nil, err
	}
	if err := c.ToolCategory.validate(); err != nil {
		return nil, err
	}
	if power == 0 && a.HP > 0 {
		t.ExtendedTurns--
		if t.ExtendedTurns <= 0 {
			t.Extended = ""
		}
		return []LifetimeState{p}, nil
	}
	minimum := rollDamage(power, toolDefense(a), true, a.BaseStats.Defense, resetBoundary(0))
	maximum := max(2, rollDamage(power, toolDefense(a), true, a.BaseStats.Defense, resetBoundary(math.Nextafter(1, 0))))
	if power*.9 < 1 || a.BaseStats.Defense > 999 && (power*.5-toolDefense(a))*.9 < 1 {
		minimum = 1
	}
	var states []LifetimeState
	for _, hp := range damageSurvivorHPs(a.HP, minimum, maximum) {
		next := p.callbackCopy()
		next.actor(a.ID).HP = hp
		next.actor(target).ExtendedTurns--
		if next.actor(target).ExtendedTurns <= 0 {
			next.actor(target).Extended = ""
		}
		states = appendDefeatState(states, next)
	}
	// Nonlethal tool damage also retains HP1 when every roll reaches zero.
	if a.HP <= maximum {
		p.actor(a.ID).HP = 1
		p.actor(target).ExtendedTurns--
		if p.actor(target).ExtendedTurns <= 0 {
			p.actor(target).Extended = ""
		}
		states = appendDefeatState(states, p)
	}
	return states, nil
}

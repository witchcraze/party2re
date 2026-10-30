package battle

import (
	"math"
	"reflect"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

type damageContinuation struct {
	State    LifetimeState
	Context  DamageContext
	TargetID string
}

func appendDamageContinuation(states []damageContinuation, s LifetimeState, c DamageContext, target string) []damageContinuation {
	for _, old := range states {
		if old.TargetID == target && old.Context.DamageCalls == c.DamageCalls && old.Context.Item195Activated == c.Item195Activated && old.Context.Item185Activated == c.Item185Activated && reflect.DeepEqual(old.Context.End, c.End) && reflect.DeepEqual(old.State, s) {
			return states
		}
	}
	return append(states, damageContinuation{State: s, Context: c, TargetID: target})
}

// Project direct untyped/magic damage and native tails without external Apply
// hooks. upper is the raw interval ceiling before target and shared modifiers.
func (s *LifetimeState) projectDirectDamage(c *DamageContext, defeat *DefeatContext, e TargetEffect, upper float64, followed bool, rng random.Generator) ([]damageContinuation, error) {
	damage := s.nativeDamageContext(*c, defeat, rng)
	if err := s.ValidateDamage(&damage, e, rng); err != nil {
		return nil, err
	}
	converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return nil, err
	}
	if converted.Category != CategoryUntyped && converted.Category != CategoryMagic {
		return nil, ErrUnsupportedEffect
	}
	if len(c.EnemyIDs) == 0 || converted.Payload.Number == 0 {
		return []damageContinuation{{State: s.callbackCopy(), Context: *c}}, nil
	}
	actorStates := []LifetimeState{s.callbackCopy()}
	if c.Item195Activated && s.actor(c.ActorID).Status == "" {
		p := s.callbackCopy()
		p.actor(c.ActorID).Status = StatusConfusion
		actorStates = append(actorStates, p)
	}
	var results []damageContinuation
	for _, state := range actorStates {
		if state.actor(c.ActorID).categorySealed(converted.Category) {
			results = appendDamageContinuation(results, state.callbackCopy(), *c, "")
			state.actor(c.ActorID).Status = ""
		}
		selection := damage
		selection.Item195Activated = false // Activation alternatives are separate states above.
		candidates, err := state.statusTargetCandidates(&selection, converted)
		if err != nil {
			return nil, err
		}
		if upper > e.Payload.Number {
			highEffect := converted
			highEffect.Payload.Number *= upper / e.Payload.Number
			highCandidates, err := state.statusTargetCandidates(&selection, highEffect)
			if err != nil {
				return nil, err
			}
			for _, high := range highCandidates {
				high.Payload.Number *= e.Payload.Number / upper
				candidates = append(candidates, high)
			}
		}
		if len(candidates) == 0 {
			results = appendDamageContinuation(results, state, *c, "")
		}
		for _, candidate := range candidates {
			p, child := state.callbackCopy(), *c
			applyStatusResources(&p, candidate)
			if candidate.Payload.Number == 0 {
				results = appendDamageContinuation(results, p, child, "")
				continue
			}
			child.DamageCalls++
			t := p.actor(candidate.TargetID)
			if t.HP <= 0 {
				t = p.actor(c.ActorID)
				candidate.TargetID = t.ID
			}
			if t.ConsumeOneHitDefense() {
				results = appendDamageContinuation(results, p, child, "")
				continue
			}
			low, high := p.directPowerBounds(&child, t, candidate.TargetEffect, candidate.Payload.Number*upper/e.Payload.Number)
			if low == 0 {
				results = appendDamageContinuation(results, p.callbackCopy(), child, "")
			}
			if high == 0 {
				continue
			}
			beforeHit := []LifetimeState{p}
			if p.actor(c.ActorID).Extended == ExtendedPurification {
				beforeHit, err = p.projectDamageReset(defeat, c.TargetContext, t.ID, true, rng)
				if err != nil {
					return nil, err
				}
			}
			for _, ready := range beforeHit {
				states, err := ready.projectDirectHit(defeat, child, candidate.TargetEffect, low, high, followed, rng)
				if err != nil {
					return nil, err
				}
				for _, next := range states {
					results = appendDamageContinuation(results, next.State, next.Context, next.TargetID)
				}
			}
		}
	}
	return results, nil
}

// Four optional modifier draws affect direct magic. Retain143/144's interior
// threshold extrema as well as interval endpoints and boost/reduction branches.
func (s *LifetimeState) directPowerBounds(c *DamageContext, t *LifetimeActor, e TargetEffect, upper float64) (float64, float64) {
	a := s.actor(c.ActorID)
	items := a.ItemDefinitionIDs
	areaBoost := hasItem(items, "item-144") && c.AreaDamageCalls > 0
	if areaBoost {
		a.ItemDefinitionIDs = slices.DeleteFunc(slices.Clone(items), func(id string) bool { return id == "item-144" })
		defer func() { a.ItemDefinitionIDs = items }()
	}
	low, high := math.Inf(1), 0.0
	values := []float64{e.Payload.Number, upper}
	threshold := 300.0
	if t.Temporary == TemporaryDouble {
		threshold *= 2
	}
	if hasItem(items, "item-149") {
		threshold /= 1.1
	}
	if e.Category == CategoryMagic && hasItem(items, "item-143") && e.Payload.Number < threshold && upper >= threshold {
		values = append(values, threshold, math.Nextafter(threshold, 0))
	}
	for _, value := range values {
		for mask := range 16 {
			bits := mask
			draw := func() float64 { v := float64(bits&1) * math.Nextafter(1, 0); bits >>= 1; return v }
			power := s.modifyDamage(c, t, value, e.Category, true, false, draw)
			low, high = min(low, power), max(high, power)
		}
	}
	if areaBoost {
		switch {
		case high < 300:
			low, high = low*1.3, high*1.3
		case low < 300:
			low, high = min(low*1.3, 300), max(high, 390)
		}
	}
	return low, high
}

// Ordered continuation depends on living/fallen and the HP999 boundary. Retain
// each class plus numerical extremes for subsequent damage and low-HP gear.
func damageSurvivorHPs(hp, minimum, maximum int) []int {
	low, high := max(1, hp-maximum), hp-minimum
	var values []int
	for _, value := range []int{low, high, 999, 1000} {
		if value >= low && value <= high && !slices.Contains(values, value) {
			values = append(values, value)
		}
	}
	return values
}

func (s *LifetimeState) projectDirectHit(defeat *DefeatContext, damage DamageContext, e TargetEffect, low, high float64, followed bool, rng random.Generator) ([]damageContinuation, error) {
	a, t := s.actor(damage.ActorID), s.actor(e.TargetID)
	if !hasItem(a.ItemDefinitionIDs, "item-149") {
		low, high = low*a.Tension, high*a.Tension
		a.Tension = 1
	}
	minus := toolDefense(t)
	if hasItem(a.ItemDefinitionIDs, "item-187") {
		minus -= float64(t.Defense-t.BaseStats.Defense) * .3
	}
	minimum := rollDamage(low, minus, true, t.BaseStats.Defense, resetBoundary(0))
	maximum := rollDamage(high, minus, true, t.BaseStats.Defense, resetBoundary(math.Nextafter(1, 0)))
	if low*.9 < 1 || t.BaseStats.Defense > 999 && (low*.5-minus)*.9 < 1 {
		minimum, maximum = 1, max(2, maximum)
	}
	damage.Item185Activated = damage.Item185Activated || hasItem(t.ItemDefinitionIDs, "item-185")
	damage.Item195Activated = damage.Item195Activated || hasItem(t.ItemDefinitionIDs, "item-195")
	var states []LifetimeState
	for _, hp := range damageSurvivorHPs(t.HP, minimum, maximum) {
		p := s.callbackCopy()
		p.actor(t.ID).HP = hp
		// Direct untyped/magic hits skip physical162 defense growth and can reach interception.
		if t.Extended == ExtendedInterception && t.ID != a.ID {
			nextStates, err := p.projectDamageInterception(&damage, t.ID, rng)
			if err != nil {
				return nil, err
			}
			for _, next := range nextStates {
				states = appendDefeatState(states, next)
			}
		} else {
			states = appendDefeatState(states, p)
		}
	}
	if t.HP <= maximum {
		if defeat == nil {
			return nil, ErrUnsupportedEffect
		}
		p := s.callbackCopy()
		p.actor(t.ID).HP = 0
		child := *defeat
		child.TargetContext = damage.TargetContext
		if err := p.ValidateDefeat(&child, t.ID, rng); err != nil {
			return nil, err
		}
		next, err := p.projectDefeatContinuation(&child, t.ID, rng, map[string]bool{}, followed || hasAbility(p.actor(damage.ActorID).Abilities, "seal_kuu"))
		if err != nil {
			return nil, err
		}
		for _, state := range next {
			states = appendDefeatState(states, state)
		}
	}
	var results []damageContinuation
	for _, state := range states {
		results = appendDamageContinuation(results, state, damage, t.ID)
		if state.actor(t.ID).HP > 0 && hasAbility(state.actor(damage.ActorID).Abilities, "seal_kuu") {
			next, err := state.projectDamageReset(defeat, damage.TargetContext, t.ID, false, rng)
			if err != nil {
				return nil, err
			}
			for _, reset := range next {
				results = appendDamageContinuation(results, reset, damage, t.ID)
			}
		}
	}
	return results, nil
}

func (s *LifetimeState) nativeDamageContext(damage DamageContext, defeat *DefeatContext, rng random.Generator) DamageContext {
	damage.Reset = func(target string) error {
		if defeat == nil {
			return ErrUnsupportedEffect
		}
		defeat.TargetContext = damage.TargetContext
		return s.ApplyEquipmentReset(&defeat.EquipmentResetContext, target, rng)
	}
	damage.Defeat = func(target string) error {
		if defeat == nil {
			return ErrUnsupportedEffect
		}
		defeat.TargetContext = damage.TargetContext
		_, err := s.ApplyDefeat(defeat, target, rng)
		return err
	}
	return damage
}

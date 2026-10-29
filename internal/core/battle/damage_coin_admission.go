package battle

import (
	"math"
	"reflect"
	"slices"

	"github.com/witchcraze/party2re/internal/core/random"
)

type coinDamageContinuation struct {
	State   LifetimeState
	Context DamageContext
}

func appendCoinDamage(states []coinDamageContinuation, s LifetimeState, c DamageContext) []coinDamageContinuation {
	for _, old := range states {
		if old.Context.DamageCalls == c.DamageCalls && old.Context.Item195Activated == c.Item195Activated && old.Context.Item185Activated == c.Item185Activated && reflect.DeepEqual(old.Context.End, c.End) && reflect.DeepEqual(old.State, s) {
			return states
		}
	}
	return append(states, coinDamageContinuation{State: s, Context: c})
}

// Coin has one fixed damage family: direct, untyped, bounded by1800 before
// modifiers. Project its target/HP classes and native tails, using the same
// formula, reset and defeat owners as execution, without external Apply hooks.
func (s *LifetimeState) projectCoinDamage(c *TreatmentChanceContext, e TargetEffect, followed bool, rng random.Generator) ([]coinDamageContinuation, error) {
	damage := s.coinDamageContext(c, rng)
	if err := s.ValidateDamage(&damage, e, rng); err != nil {
		return nil, err
	}
	converted, _, err := s.actor(c.ActorID).ConvertEnemyCategory(c.TargetContext, e)
	if err != nil {
		return nil, err
	}
	if converted.Category != CategoryUntyped {
		return nil, ErrUnsupportedEffect
	}
	actorStates := []LifetimeState{s.callbackCopy()}
	if c.Item195Activated && s.actor(c.ActorID).Status == "" {
		p := s.callbackCopy()
		p.actor(c.ActorID).Status = StatusConfusion
		actorStates = append(actorStates, p)
	}
	var results []coinDamageContinuation
	for _, state := range actorStates {
		selection := damage
		selection.Item195Activated = false // Activation alternatives are separate states above.
		candidates := state.damageCandidates(&selection, converted)
		if len(candidates) == 0 {
			results = appendCoinDamage(results, state, c.DamageContext)
		}
		for _, candidate := range candidates {
			p, child := state.callbackCopy(), c.DamageContext
			if candidate.Payload.Number == 0 {
				results = appendCoinDamage(results, p, child)
				continue
			}
			child.DamageCalls++
			t := p.actor(candidate.TargetID)
			if t.HP <= 0 {
				t = p.actor(c.ActorID)
				candidate.TargetID = t.ID
			}
			if t.ConsumeOneHitDefense() {
				results = appendCoinDamage(results, p, child)
				continue
			}
			low, high := p.coinPowerBounds(&child, t, candidate)
			if low == 0 {
				results = appendCoinDamage(results, p.callbackCopy(), child)
			}
			if high == 0 {
				continue
			}
			beforeHit := []LifetimeState{p}
			if p.actor(c.ActorID).Extended == ExtendedPurification {
				beforeHit, err = p.projectCoinReset(c, t.ID, true, rng)
				if err != nil {
					return nil, err
				}
			}
			for _, ready := range beforeHit {
				states, err := ready.projectCoinHit(c, child, candidate, low, high, followed, rng)
				if err != nil {
					return nil, err
				}
				for _, next := range states {
					results = appendCoinDamage(results, next.State, next.Context)
				}
			}
		}
	}
	return results, nil
}

// At most three optional modifier draws affect untyped coin damage. Bound all
// boost/reduction combinations; handle144's nonmonotone300 threshold explicitly.
func (s *LifetimeState) coinPowerBounds(c *DamageContext, t *LifetimeActor, e TargetEffect) (float64, float64) {
	a := s.actor(c.ActorID)
	items := a.ItemDefinitionIDs
	areaBoost := hasItem(items, "item-144") && c.AreaDamageCalls > 0
	if areaBoost {
		a.ItemDefinitionIDs = slices.DeleteFunc(slices.Clone(items), func(id string) bool { return id == "item-144" })
		defer func() { a.ItemDefinitionIDs = items }()
	}
	low, high := math.Inf(1), 0.0
	for mask := range 8 {
		bits := mask
		draw := func() float64 { v := float64(bits&1) * math.Nextafter(1, 0); bits >>= 1; return v }
		power := s.modifyDamage(c, t, e.Payload.Number, CategoryUntyped, true, false, draw)
		low, high = min(low, power), max(high, power)
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

// Coin continuation depends on living/fallen and the HP999 boundary. Retain
// each class plus numerical extremes for subsequent damage and low-HP gear.
func coinSurvivorHPs(hp, minimum, maximum int) []int {
	low, high := max(1, hp-maximum), hp-minimum
	var values []int
	for _, value := range []int{low, high, 999, 1000} {
		if value >= low && value <= high && !slices.Contains(values, value) {
			values = append(values, value)
		}
	}
	return values
}

func (s *LifetimeState) projectCoinHit(c *TreatmentChanceContext, damage DamageContext, e TargetEffect, low, high float64, followed bool, rng random.Generator) ([]coinDamageContinuation, error) {
	a, t := s.actor(c.ActorID), s.actor(e.TargetID)
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
	for _, hp := range coinSurvivorHPs(t.HP, minimum, maximum) {
		p := s.callbackCopy()
		p.actor(t.ID).HP = hp
		// Untyped hits skip physical162 defense growth and can reach interception.
		if t.Extended == ExtendedInterception && t.ID != a.ID {
			nextStates, err := p.projectCoinInterception(&damage, t.ID, rng)
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
		if c.Status.Defeat == nil {
			return nil, ErrUnsupportedEffect
		}
		p := s.callbackCopy()
		p.actor(t.ID).HP = 0
		defeat := *c.Status.Defeat
		defeat.TargetContext = damage.TargetContext
		if err := p.ValidateDefeat(&defeat, t.ID, rng); err != nil {
			return nil, err
		}
		next, err := p.projectDefeatContinuation(&defeat, t.ID, rng, map[string]bool{}, followed || hasAbility(p.actor(c.ActorID).Abilities, "seal_kuu"))
		if err != nil {
			return nil, err
		}
		for _, state := range next {
			states = appendDefeatState(states, state)
		}
	}
	var results []coinDamageContinuation
	for _, state := range states {
		results = appendCoinDamage(results, state, damage)
		if state.actor(t.ID).HP > 0 && hasAbility(state.actor(c.ActorID).Abilities, "seal_kuu") {
			next, err := state.projectCoinReset(c, t.ID, false, rng)
			if err != nil {
				return nil, err
			}
			for _, reset := range next {
				results = appendCoinDamage(results, reset, damage)
			}
		}
	}
	return results, nil
}

package battle

import (
	"encoding/json"
	"math"

	"github.com/witchcraze/party2re/internal/core/random"
)

func (s *LifetimeState) validateReachedEquipmentTail(c *EquipmentTailContext, id string, rng random.Generator) error {
	p, child := s.callbackCopy(), *c
	a := p.actor(c.ActorID)
	switch id {
	case "item-164-operation-tail":
		_, err := p.gemTensionCeiling(&child.EffectContext, a, 1)
		return err
	case "item-209-operation-tail":
		_, err := p.ApplyToolHealing(c.TargetContext, a.ID, float64(a.MaxHP)*.03, false, resetBoundary(0))
		return err
	case "item-202-npc-tail":
		_, err := p.ApplyToolHealing(c.TargetContext, a.ID, float64(a.MaxHP)*.05, false, resetBoundary(0))
		return err
	case "guardian-mp-tail":
		_, err := p.ApplyToolMPHealing(c.TargetContext, a.ID, float64(a.MaxMP)*.035, false, resetBoundary(0))
		return err
	case "item-206-operation-tail":
		a.Tension = 1
		states, err := p.projectEquipment206Healing(&child, rng)
		if err != nil {
			return err
		}
		for _, state := range states {
			power := float64(state.actor(c.ActorID).Attack)
			if _, err := state.projectCallbackDamage(&child.DamageContext, c.Defeat, dualAgilityEffect("", power, CategoryUntyped, true), power, false, rng); err != nil {
				return err
			}
		}
	case "item-208-combat-tail":
		return p.validateEquipment208(&child, rng)
	}
	return nil
}

// Healing's targets are alternatives. Retain HP bounds, rage clearing and splash
// before admitting206's independent damage branch, including healing above cap.
func (s *LifetimeState) projectEquipment206Healing(c *EquipmentTailContext, rng random.Generator) ([]LifetimeState, error) {
	e := controlEffect(c.ActorID, float64(s.actor(c.ActorID).MaxHP)*.3, CategoryUntyped)
	if err := s.validateRecovery(&c.EffectContext, e, rng); err != nil {
		return nil, err
	}
	if err := s.validatePartyTarget(c.TargetContext, e, rng); err != nil {
		return nil, err
	}
	var states []LifetimeState
	for _, candidate := range s.partyTargetCandidates(c.TargetContext, e, nil) {
		p := s.callbackCopy()
		// Untyped206 has no numeric MP debit/absorption category; item195 can
		// still activate confusion while the explicit self target is retained.
		actorStates := []LifetimeState{p}
		if c.Item195Activated && p.actor(c.ActorID).Status == "" {
			q := p.callbackCopy()
			q.actor(c.ActorID).Status = StatusConfusion
			actorStates = append(actorStates, q)
		}
		for _, state := range actorStates {
			t := state.actor(candidate.TargetID)
			power := state.healingBasePower(&c.EffectContext, candidate)
			modified := candidate
			modified.Payload.Number = power
			power = state.finalEffectPower(&c.EffectContext, state.initialEffectPower(&c.EffectContext, modified, ModifierHPHealing))
			if t.Status == StatusVirulentPoison {
				power = float64(int(power * .1))
			}
			if t.HP <= 0 || candidate.Payload.Number == 0 || power == 0 {
				states = appendDefeatState(states, state)
				continue
			}
			if t.Temporary == TemporaryRage {
				t.Temporary = ""
			}
			for _, draw := range []float64{0, math.Nextafter(1, 0)} {
				q := state.callbackCopy()
				amount := int(power * (.9 + draw*.3)) //206 temporarily fixes tension1.
				q.actor(t.ID).HP = min(t.MaxHP, t.HP+amount)
				if hasItem(q.actor(c.ActorID).ItemDefinitionIDs, "item-256") && c.PartyHealCalls == 0 && c.DamageCalls == 0 {
					splash := min(amount/2, int(float64(t.MaxHP)*.3))
					for _, id := range c.PartyIDs {
						other := q.actor(id)
						if id != t.ID && other.HP > 0 {
							other.HP = min(other.MaxHP, other.HP+splash)
						}
					}
				}
				states = appendDefeatState(states, q)
			}
		}
	}
	return states, nil
}

func (s *LifetimeState) validateEquipment208(c *EquipmentTailContext, rng random.Generator) error {
	states := []LifetimeState{s.callbackCopy()}
	count := len(c.PartyIDs)
	if count >= 3 {
		for _, id := range c.PartyIDs {
			var next []LifetimeState
			for _, state := range states {
				t := state.actor(id)
				if !hasItem(t.ItemDefinitionIDs, "item-208") || t.Status == "" {
					next = appendDefeatState(next, state)
					continue
				}
				e := controlSymbol(id, t.Status, CategoryUntyped)
				if err := state.validatePartyTarget(c.TargetContext, e, rng); err != nil {
					return err
				}
				// A prior cure can enable item195 on this or the next helper.
				actorStates := []LifetimeState{state}
				if c.Item195Activated && state.actor(c.ActorID).Status == "" {
					p := state.callbackCopy()
					p.actor(c.ActorID).Status = StatusConfusion
					actorStates = append(actorStates, p)
				}
				for _, ready := range actorStates {
					for _, candidate := range ready.partyTargetCandidates(c.TargetContext, e, nil) {
						p := ready.callbackCopy()
						target := p.actor(candidate.TargetID)
						if target.HP > 0 && (target.Status == candidate.Payload.Symbol || candidate.Payload.Symbol == StatusDeadlyPoison && (target.Status == StatusPoison || target.Status == StatusVirulentPoison)) {
							target.Status = ""
						}
						next = appendDefeatState(next, p)
					}
				}
			}
			states = next
		}
	}
	var continuations []damageContinuation
	var finished []damageContinuation
	for _, state := range states {
		state.actor(c.ActorID).Tension = 1
		if count >= 4 {
			safe, err := state.equipment208Nonlethal(c, count, rng)
			if err != nil {
				return err
			}
			if safe {
				finished = append(finished, damageContinuation{State: state, Context: c.DamageContext})
				continue
			}
		}
		continuations = appendDamageContinuation(continuations, state, c.DamageContext, "")
	}
	if count >= 4 {
		for hit := range count {
			var next []damageContinuation
			seen := make(map[string]bool)
			for _, current := range continuations {
				power := float64(current.State.actor(c.ActorID).Attack) * .1
				e := dualAgilityEffect("", power, CategoryUntyped, true)
				projected, err := current.State.projectCallbackDamage(&current.Context, c.Defeat, e, power, hit+1 < count || count >= 5, rng)
				if err != nil {
					return err
				}
				for _, result := range projected {
					// Ordered target alternatives converge; index exact facts rather
					// than comparing every prior roster for each additional hit.
					key, err := json.Marshal(struct {
						State            LifetimeState
						DamageCalls      int
						Item185, Item195 bool
						End              EndProcessing
					}{result.State, result.Context.DamageCalls, result.Context.Item185Activated, result.Context.Item195Activated, result.Context.End})
					if err != nil {
						return err
					}
					if !seen[string(key)] {
						seen[string(key)] = true
						next = append(next, result)
					}
				}
			}
			continuations = next
		}
	}
	continuations = append(continuations, finished...)
	for _, current := range continuations {
		p, child := current.State, current.Context
		p.actor(c.ActorID).Tension = s.actor(c.ActorID).Tension
		for _, id := range c.PartyIDs {
			if count >= 5 {
				if _, err := p.ApplyToolHealing(c.TargetContext, id, min(float64(p.actor(id).MaxHP)*.1, 200), false, resetBoundary(0)); err != nil {
					return err
				}
			}
			if count >= 6 {
				if _, err := p.gemTensionCeiling(&child.EffectContext, p.actor(id), 1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

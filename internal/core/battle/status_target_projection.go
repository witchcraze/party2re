package battle

import "github.com/witchcraze/party2re/internal/core/random"

type statusTargetContinuation struct {
	State      LifetimeState
	Resolution TargetResolution
}

// Symbolic target helpers have finite resource outcomes (absorption30–49).
// Activation precedes enemy seal checks, but follows party seal checks.
func (s *LifetimeState) projectSymbolicTargets(c TargetContext, e TargetEffect, enemy bool) ([]statusTargetContinuation, error) {
	if err := s.validateTargetContext(c, random.Default()); err != nil {
		return nil, err
	}
	if err := e.validate(); err != nil {
		return nil, err
	}
	if !enemy {
		if err := s.validatePartyTarget(c, e, random.Default()); err != nil {
			return nil, err
		}
	}
	debit := 0
	if enemy {
		var err error
		e, debit, err = s.actor(c.ActorID).ConvertEnemyCategory(c, e)
		if err != nil {
			return nil, err
		}
	}
	var results []statusTargetContinuation
	unresolved := func(state LifetimeState) {
		results = append(results, statusTargetContinuation{State: state.callbackCopy(), Resolution: TargetResolution{TargetEffect: e, MPDebit: debit}})
	}
	start := s.callbackCopy()
	if !enemy && start.actor(c.ActorID).categorySealed(e.Category) {
		unresolved(start)
		start.actor(c.ActorID).Status = ""
	}
	for _, state := range start.tensionConfusionBranches(EffectContext{TargetContext: c}) {
		if enemy {
			state.actor(c.ActorID).MP = max(0, state.actor(c.ActorID).MP-debit)
			if len(c.EnemyIDs) == 0 {
				unresolved(state)
				continue
			}
			if state.actor(c.ActorID).categorySealed(e.Category) {
				unresolved(state)
				state.actor(c.ActorID).Status = ""
			}
		}
		selection := c
		selection.Item195Activated = false // Activation already has its own branch.
		ids := state.partyInitialTargetIDs(selection, e)
		if enemy {
			ids = state.damageInitialTargetIDs(&DamageContext{EffectContext: EffectContext{TargetContext: selection}}, e)
			if state.actor(c.ActorID).Status != StatusConfusion {
				initial := []string{e.TargetID}
				if target := state.actor(e.TargetID); target == nil || target.TeamID == state.actor(c.ActorID).TeamID {
					initial = c.EnemyIDs
				}
				for _, id := range initial {
					if state.actor(id).HP <= 0 {
						unresolved(state)
						break
					}
				}
			}
		}
		for _, id := range ids {
			targets := []TargetEffect{e}
			targets[0].TargetID = id
			bypass := enemy && state.actor(c.ActorID).Temporary == TemporaryPhantom
			if !bypass {
				targets = state.temporaryTargetCandidates(selection, e, []string{id}, nil)
			}
			if enemy {
				targets = state.phantomTargetCandidates(selection, targets)
			}
			for _, target := range targets {
				values := []int{0}
				if !bypass && state.actor(id).Temporary == TemporaryAbsorbMagic && e.Category == CategoryMagic {
					values = nil
					for value := 30; value < 50; value++ {
						if state.Fields.Current() == "den" {
							values = append(values, int(float64(value)*.025)+1)
						} else {
							values = append(values, value)
						}
					}
				}
				for _, value := range values {
					p := state.callbackCopy()
					r := TargetResolution{TargetEffect: target, Resolved: true, MPDebit: debit}
					if value > 0 {
						r.MPRecoveryTarget, r.MPRecovery = id, value
						applyStatusResources(&p, r)
					}
					results = append(results, statusTargetContinuation{State: p, Resolution: r})
				}
			}
		}
	}
	return results, nil
}

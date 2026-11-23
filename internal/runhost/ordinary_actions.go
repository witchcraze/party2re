package runhost

import (
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func bindOrdinaryPlayerAction(action *parsedAction, frame corebattle.CallbackContext, catalog corebattle.CallbackCatalog) {
	invocations := []corebattle.CallbackInvocation{action.Invocation}
	action.Certify = func(state *corebattle.LifetimeState) error {
		return state.CertifyInvocations(&frame, catalog, invocations, random.Default())
	}
	action.Validate = func(state *corebattle.LifetimeState) error {
		return state.ValidateInvocations(&frame, catalog, invocations, random.Default())
	}
	action.Execute = func(state *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
		return runFacts(frame, func(work *corebattle.CallbackContext) error {
			return state.ApplyInvocations(work, catalog, invocations, rng)
		})
	}
}

// applyPlayerOperation keeps the run's ordered projections aligned after a
// formation callback. The response can use the rebuilt opposing side at once.
func (r *State) applyPlayerOperation(op *corebattle.PlayerOperation, rng random.Generator) (corebattle.PlayerOperationResult, error) {
	result, err := r.Battle.ApplyPlayerOperation(op, rng)
	if err != nil || !result.NormalAction || op.First.EntryID != "front" && op.First.EntryID != "back" {
		return result, err
	}
	r.Enemies = slices.Clone(result.Target.EnemyIDs)
	r.syncFormation()
	return result, nil
}

// syncFormation reorders the roster from the member order a formation wrote.
func (r *State) syncFormation() {
	roster := make([]string, 0, len(r.Roster))
	for _, a := range r.Battle.Actors {
		if slices.Contains(r.Roster, a.ID) {
			roster = append(roster, a.ID)
		}
	}
	r.Roster = roster
}

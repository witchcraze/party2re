package adventure

import (
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func bindOrdinaryPlayerAction(action *ParsedRunAction, frame corebattle.CallbackContext, catalog corebattle.CallbackCatalog) {
	invocations := []corebattle.CallbackInvocation{action.Invocation}
	action.Validate = func(state *corebattle.LifetimeState) error {
		return state.ValidateInvocations(&frame, catalog, invocations, random.Default())
	}
	action.Execute = func(state *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
		work := frame
		var damage corebattle.DamageContext
		if frame.Damage != nil {
			damage = *frame.Damage
		}
		healsBefore := damage.HealCalls
		work.Damage = &damage
		if err := state.ApplyInvocations(&work, catalog, invocations, rng); err != nil {
			return corebattle.PlayerActionFacts{}, err
		}
		facts := corebattle.PlayerActionFacts{HealCalls: work.Damage.HealCalls - healsBefore}
		if invocations[0].EntryID == "front" || invocations[0].EntryID == "back" {
			facts.Target = work.TargetContext
		}
		return facts, nil
	}
}

// applyPlayerOperation keeps the run's ordered projections aligned after a
// formation callback. The response can use the rebuilt opposing side at once.
func (r *NormalRun) applyPlayerOperation(op *corebattle.PlayerOperation, rng random.Generator) (corebattle.PlayerOperationResult, error) {
	result, err := r.Battle.ApplyPlayerOperation(op, rng)
	if err != nil || !result.NormalAction || op.First.EntryID != "front" && op.First.EntryID != "back" {
		return result, err
	}
	roster := make([]string, 0, len(r.Roster))
	for _, a := range r.Battle.Actors {
		if slices.Contains(r.Roster, a.ID) {
			roster = append(roster, a.ID)
		}
	}
	r.Roster, r.Enemies = roster, slices.Clone(result.Target.EnemyIDs)
	return result, nil
}

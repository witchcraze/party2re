package playercontext

import "github.com/witchcraze/party2re/internal/core/scheduling"

// CooldownGate blocks concurrent actions until all unfinished work settles.
type CooldownGate struct{}

func (CooldownGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	if action.RequiredGates.Has(GateCooldownCheck) {
		for _, ongoing := range snapshot.OngoingActions {
			if ongoing.State == scheduling.StatePending || ongoing.State == scheduling.StateProcessing {
				return false
			}
		}
	}
	return true
}

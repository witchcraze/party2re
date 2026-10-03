package playercontext

// FatigueGate excludes strenuous actions at or above 100 percent fatigue.
type FatigueGate struct{}

func (FatigueGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	return !action.RequiredGates.Has(GateFatigueCheck) || !snapshot.Character.IsExhausted()
}

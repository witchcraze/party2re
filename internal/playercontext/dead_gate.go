package playercontext

// DeadGate excludes actions requiring a living character.
type DeadGate struct{}

func (DeadGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	return !action.RequiredGates.Has(GateDeadCheck) || snapshot.Character.Stats.HP > 0
}

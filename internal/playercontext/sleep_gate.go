package playercontext

// SleepGate blocks awake-only actions through sleep and pending wake recovery.
type SleepGate struct{}

func (SleepGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	if action.ID == "home_wake" {
		return snapshot.CanWake && snapshot.SleepRemaining <= 0
	}
	return !action.RequiredGates.Has(GateSleepCheck) || (!snapshot.Sleeping && snapshot.SleepRemaining <= 0)
}

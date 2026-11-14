package playercontext

import "slices"

// LocationGate admits a location-bound action only in a scene that registers
// it. Wake is recovery, and a snapshot without a scene registry is unrestricted.
type LocationGate struct{}

func (LocationGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	if !action.RequiredGates.Has(GateLocationCheck) || action.RequiresWake || snapshot.SceneActions == nil {
		return true
	}
	return slices.Contains(snapshot.SceneActions, action.ID)
}

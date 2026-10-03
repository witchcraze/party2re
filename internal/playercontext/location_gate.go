package playercontext

// LocationGate limits facility entry actions to the initial town scene.
// ponytail: town scene only (#939), extend with facility scene contracts in #947–#949.
type LocationGate struct{}

func (LocationGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	return !action.RequiredGates.Has(GateLocationCheck) || snapshot.LocationID == LocationTown
}

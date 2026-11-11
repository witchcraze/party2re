package playercontext

import "strings"

// LocationGate separates ordinary facility entries from recovery/continuation.
type LocationGate struct{}

func (LocationGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	if !action.RequiredGates.Has(GateLocationCheck) || action.ID == "home_wake" || action.Category == "navigation" {
		return true
	}
	if action.Category == "npc" {
		// NPCs stand in facilities; the HTTP registry decides which have one.
		return snapshot.LocationID != LocationTown
	}
	if snapshot.LocationID == LocationTown {
		return true
	}
	return strings.HasPrefix(action.ID, snapshot.LocationID+"_") || (strings.HasPrefix(snapshot.LocationID, "shop_") && strings.HasPrefix(action.ID, "shop_")) ||
		// Legacy player stores stand in the four towns (town1..town4).
		(strings.HasPrefix(snapshot.LocationID, LocationTown) && strings.HasPrefix(action.ID, "store_"))
}

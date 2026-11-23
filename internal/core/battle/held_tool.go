package battle

import "slices"

// ClearConsumedHeldTool removes the consumed definition's combat identities.
func (a *LifetimeActor) ClearConsumedHeldTool(definitionID string) {
	a.ItemDefinitionIDs = slices.DeleteFunc(slices.Clone(a.ItemDefinitionIDs), func(id string) bool {
		return id == definitionID
	})
	if a.Gear != nil && a.Gear.AccessoryID == definitionID {
		gear := *a.Gear
		gear.AccessoryID = ""
		a.Gear = &gear
	}
	if a.HeldToolID != nil && *a.HeldToolID == definitionID {
		empty := ""
		a.HeldToolID = &empty
	}
}

// setHeldTool updates hand identities without reapplying equipment or resetting
// current resources/buffs. Missing reset facts remain unknown.
func (a *LifetimeActor) setHeldTool(id string) {
	previous := ""
	if a.HeldToolID != nil {
		previous = *a.HeldToolID
	}
	a.ItemDefinitionIDs = slices.DeleteFunc(slices.Clone(a.ItemDefinitionIDs), func(current string) bool {
		return current == previous || a.Gear != nil && current == a.Gear.AccessoryID
	})
	if id != "" && !slices.Contains(a.ItemDefinitionIDs, id) {
		a.ItemDefinitionIDs = append(a.ItemDefinitionIDs, id)
	}
	if a.Gear != nil {
		a.Gear.AccessoryID = id
	}
	if a.HeldToolID == nil {
		a.HeldToolID = &id
	} else {
		*a.HeldToolID = id
	}
}

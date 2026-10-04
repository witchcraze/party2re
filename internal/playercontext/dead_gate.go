package playercontext

// DeadGate excludes actions requiring a living character (e.g. combat quests and dungeon expeditions).
// In legacy Party2 (lib/quest.cgi:479, 902), only quest and combat participation require HP > 0.
type DeadGate struct{}

func (DeadGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	return !action.RequiredGates.Has(GateDeadCheck) || snapshot.Character.Stats.HP > 0
}

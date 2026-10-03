package playercontext

// CurrencyGate checks positive wallet gold for parameter-independent entry.
// Exact costs and alternate currencies remain execution-service prerequisites.
type CurrencyGate struct{}

func (CurrencyGate) Allows(action ActionDefinition, snapshot *Snapshot) bool {
	return !action.RequiredGates.Has(GateCurrencyCheck) || snapshot.Character.Money > 0
}

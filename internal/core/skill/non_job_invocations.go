package skill

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// NonJobInvocationEntry keeps outer custom cost separate from child MP costs.
// Tools have no CMP contribution; their durable consumption is consumer-owned.
type NonJobInvocationEntry struct {
	ID         string
	CMPCost    int
	Invocation corebattle.CallbackInvocation
}

// NonJobInvocationEntries covers ff97fc5:_data.cgi:1255–1267,2171,2174–2180.
// Metadata is not permission: InvocationAction admits the complete descendants.
func NonJobInvocationEntries() []NonJobInvocationEntry {
	return []NonJobInvocationEntry{
		{"tool-84", 0, corebattle.CallbackInvocation{Kind: "tool-84"}},
		{"tool-85", 0, corebattle.CallbackInvocation{Kind: "random"}},
		{"gem-83", 5, corebattle.CallbackInvocation{Kind: "random"}},
		{"gem-86", 20, corebattle.CallbackInvocation{Kind: "gem-86"}},
		{"gem-87", 15, corebattle.CallbackInvocation{Kind: "gem-87"}},
		{"gem-88", 1, corebattle.CallbackInvocation{Kind: "gem-88"}},
		{"gem-89", 15, corebattle.CallbackInvocation{Kind: "gem-89"}},
		{"gem-90", 20, corebattle.CallbackInvocation{Kind: "gem-90"}},
		{"gem-91", 15, corebattle.CallbackInvocation{Kind: "gem-91"}},
		{"gem-92", 40, corebattle.CallbackInvocation{Kind: "gem-92"}},
	}
}

// RegisterNonJobInvocations binds the concrete wrappers to the existing engine.
// Rebuild child catalogs/ports after reload; missing bodies remain admission gates.
func (d *BattleDefinitions) RegisterNonJobInvocations() {
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	for _, entry := range NonJobInvocationEntries() {
		d.Invocations.Entries[entry.ID] = corebattle.CallbackDefinition{Invocation: entry.Invocation}
	}
}

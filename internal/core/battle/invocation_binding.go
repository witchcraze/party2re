package battle

// CallbackBinding is serializable table-construction data. Persist the selected
// Invocation with frozen command input, then rebuild the function catalog.
type CallbackBinding struct {
	CallbackEntry
	Invocation CallbackInvocation
}

// CallbackBindings returns full source order against the actual actor, including
// extended entries. This is metadata, not admission or ordinary cost payment.
func (s *LifetimeState) CallbackBindings(c *CallbackContext, d CallbackCatalog, job int) ([]CallbackBinding, error) {
	if s == nil || c == nil || s.actor(c.ActorID) == nil {
		return nil, ErrInvalidRequest
	}
	entries, err := s.callbackTable(c, d, job)
	if err != nil {
		return nil, err
	}
	bindings := make([]CallbackBinding, 0, len(entries))
	for _, entry := range entries {
		op := CallbackInvocation{Kind: "entry", EntryID: entry.ID}
		if entry.child != nil {
			op.GeneratedChildID, op.GeneratedChildMP = entry.child.ID, entry.child.MPCost
		}
		bindings = append(bindings, CallbackBinding{CallbackEntry: entry.CallbackEntry, Invocation: op})
	}
	return bindings, nil
}

// The table and position share S; job8 also receives the three remapped residues.
func generatedPositionReachable(job, position, size int) bool {
	gcd, n := 87, size
	for n != 0 {
		gcd, n = n, gcd%n
	}
	if (position-job)%gcd == 0 {
		return true
	}
	if job == 8 {
		for _, residue := range []int{0, 73, 82} {
			if (position-residue)%gcd == 0 {
				return true
			}
		}
	}
	return false
}

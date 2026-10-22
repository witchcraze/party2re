package battle

// ProjectRoster applies only exact roster transitions to an admission-owned
// copy. Random stats, damage, recovery and growth remain unknown; their enclosing
// continuation must still cover arbitrary prior effects. No operation port runs.
func (s *LifetimeState) ProjectRoster(c *RosterContext, id string) (bool, error) {
	if s == nil || c == nil {
		return false, ErrInvalidRequest
	}
	job, command, ok := rosterCommand(id)
	if !ok {
		return false, nil
	}
	if command == "summon" {
		return s.rosterIdentity(callingIdentity(job)) != nil, nil
	}
	if command == "duplicate" || command == "distribution" || command == "revive" {
		return false, nil
	}
	helperID := s.FindCallingHelper(c.TargetContext, job)
	if command == "dismiss" {
		helperID = ""
		if a := s.rosterIdentity(callingIdentity(job)); a != nil {
			helperID = a.ID
		}
	}
	h := s.actor(helperID)
	if h == nil || h.HP <= 0 {
		return true, nil
	}
	switch command {
	case "dismiss":
		s.dismissHelper(c, helperID)
	case "merge":
		s.mergeHelper(c, helperID, job)
	case "defend":
		if job == 87 && h.RosterIdentity == callingIdentity(87) && h.SummonForm < 2 {
			return false, nil
		}
		h.Temporary = TemporaryDefense
	case "revive-party":
		s.reviveCallingParty(c)
	default:
		return false, nil
	}
	return true, nil
}

func (s *LifetimeState) reviveCallingParty(c *RosterContext) bool {
	applied := false
	for _, id := range c.PartyIDs {
		a := s.actor(id)
		if a.HP <= 0 {
			a.HP = int(float64(a.MaxHP) * .42)
			a.Attack += 300
			a.Agility += 300
			a.Icon = "assets/images/placeholders/battle/helpers.svg#revived"
			applied = true
		}
	}
	return applied
}

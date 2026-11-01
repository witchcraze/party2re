package battle

// Each positional alternative starts from the same predecessor facts. Unknown
// children invalidate the whole projection, including already collected paths.
func (p *callbackAdmission) projectRandom(kind string, seen map[CallbackInvocation]bool) ([]CallbackContinuation, error) {
	first, last := 1, 87
	if kind == "tool-84" {
		first = callbackJob(p.s.actor(p.c.ActorID).Jobs.JobID)
		last = first
	}
	mp := p.s.actor(p.c.ActorID).MP
	var result []CallbackContinuation
	for job := first; job <= last; job++ {
		entries, err := p.s.callbackTable(p.c, p.d, job)
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 && kind == "random" {
			result = append(result, CallbackContinuation{State: p.s.callbackCopy(), Context: cloneCallbackContext(*p.c)})
			continue
		}
		for _, entry := range entries {
			if kind == "random" && entry.Extended {
				continue
			}
			branches, err := p.projectSelection(entry, false, seen)
			if err != nil || len(branches) == 0 {
				return nil, err
			}
			for i := range branches {
				a := branches[i].State.actor(branches[i].Context.ActorID)
				a.MP = max(mp, a.MP)
			}
			result = append(result, branches...)
		}
	}
	return result, nil
}

func (p *callbackAdmission) projectSelection(entry callbackSelection, explicit bool, seen map[CallbackInvocation]bool) ([]CallbackContinuation, error) {
	// Native projections retain actor identities/records; PruneRoster is a save boundary.
	state := p.s.callbackCopy()
	context := cloneCallbackContext(*p.c)
	if explicit {
		context = explicitCallbackTarget(&context)
	}
	if entry.child != nil {
		entry = *entry.child
		if entry.MPCost > state.actor(context.ActorID).MP {
			return []CallbackContinuation{{State: state, Context: context}}, nil
		}
	}
	child := *p
	child.s, child.c = &state, &context
	branches, err := child.projectAfter(CallbackInvocation{Kind: "entry", EntryID: entry.ID}, seen)
	for i := range branches {
		branches[i].State = branches[i].State.callbackCopy()
		branches[i].Context = cloneCallbackContext(branches[i].Context)
	}
	return branches, err
}

func (p *callbackAdmission) projectExtended(seen map[CallbackInvocation]bool) ([]CallbackContinuation, error) {
	a := p.s.actor(p.c.ActorID)
	for _, job := range []struct {
		id string
		sp int
	}{{a.Jobs.JobID, a.Jobs.SP}, {a.Jobs.OldJobID, a.Jobs.OldSP}} {
		if job.id == "" {
			continue
		}
		if callbackJob(job.id) == 73 {
			if a.MP > 0 {
				if a.Attack < 999 {
					a.Attack += len(p.c.PartyIDs)
				}
				a.MP--
			}
			return []CallbackContinuation{{State: *p.s, Context: *p.c}}, nil
		}
		entries, err := p.s.callbackTable(p.c, p.d, callbackJob(job.id))
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 || !entries[len(entries)-1].Extended {
			continue
		}
		entry := entries[len(entries)-1]
		if entry.RequiredSP > job.sp || entry.MPCost > a.MP {
			return []CallbackContinuation{{State: *p.s, Context: *p.c}}, nil
		}
		branches, err := p.projectSelection(entry, true, seen)
		for i := range branches {
			actor := branches[i].State.actor(branches[i].Context.ActorID)
			actor.MP = max(0, actor.MP-entry.MPCost)
		}
		return branches, err
	}
	return []CallbackContinuation{{State: *p.s, Context: *p.c}}, nil
}

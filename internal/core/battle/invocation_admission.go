package battle

import "github.com/witchcraze/party2re/internal/core/random"

// ValidateInvocations is the shared discovery/submission gate. It traverses the
// original graph (including cycles and generated, tool/book/helper descendants)
// without selecting a random child or changing resources, arguments or counters.
func (s *LifetimeState) ValidateInvocations(c *CallbackContext, d CallbackCatalog, ops []CallbackInvocation, rng random.Generator) error {
	if s == nil || c == nil || len(ops) == 0 {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	p := callbackAdmission{s: s, c: c, d: d, seen: map[callbackVisit]bool{}}
	for i, op := range ops {
		if err := p.visit(op, i > 0); err != nil {
			return err
		}
	}
	return nil
}

type callbackVisit struct {
	CallbackInvocation
	afterEffects   bool
	explicitTarget bool
}

type callbackAdmission struct {
	s              *LifetimeState
	c              *CallbackContext
	d              CallbackCatalog
	seen           map[callbackVisit]bool
	explicitTarget bool
}

func (p *callbackAdmission) visit(op CallbackInvocation, afterEffects bool) error {
	key := callbackVisit{op, afterEffects, p.explicitTarget}
	if p.seen[key] {
		return nil
	}
	p.seen[key] = true
	switch op.Kind {
	case "entry":
		def, ok := p.d.Entries[op.EntryID]
		if !ok {
			return ErrUnsupportedEffect
		}
		c := *p.c
		c.AfterEffects = afterEffects
		if def.Validate != nil {
			if err := def.Validate(p.s, &c); err != nil {
				return err
			}
		}
		if def.Invocation.Kind != "" {
			if def.Execute != nil || def.Invocation.Kind == "entry" {
				return ErrUnsupportedEffect
			}
			if err := p.visit(def.Invocation, afterEffects); err != nil {
				return err
			}
		} else if def.Execute == nil || def.Validate == nil {
			return ErrUnsupportedEffect
		}
		for _, child := range def.Children {
			if err := p.visit(child, true); err != nil {
				return err
			}
		}
		return nil
	case "random":
		for job := 1; job <= 87; job++ {
			if job == 73 {
				continue // Every possible proxy table is already in1..72.
			}
			if err := p.table(job, "random", true); err != nil {
				return err
			}
		}
		return nil
	case "job-82-generated":
		for job := 1; job < 87; job++ {
			if job == 73 || job == 82 {
				continue
			}
			if err := p.table(job, "generated", true); err != nil {
				return err
			}
		}
		return nil
	case "tool-84":
		if afterEffects {
			return p.allTables("tool-84")
		}
		return p.table(callbackJob(p.s.actor(p.c.ActorID).Jobs.JobID), "tool-84", false)
	case "gem-86":
		if afterEffects {
			return p.allTables("gem-86")
		}
		a := p.s.actor(p.c.ActorID)
		for _, job := range []struct {
			id string
			sp int
		}{{a.Jobs.JobID, a.Jobs.SP}, {a.Jobs.OldJobID, a.Jobs.OldSP}} {
			if job.id == "" {
				continue
			}
			id := callbackJob(job.id)
			if id == 73 {
				return nil // MP0 is handled, too; no table/equipment/SP is needed.
			}
			entries, err := p.s.callbackTable(p.c, p.d, id)
			if err != nil {
				return err
			}
			if len(entries) == 0 || !entries[len(entries)-1].Extended {
				continue
			}
			entry := entries[len(entries)-1]
			if entry.MPCost < 0 || entry.RequiredSP < 0 {
				return ErrUnsupportedEffect
			}
			if entry.MPCost > a.MP || entry.RequiredSP > job.sp {
				return nil // Handled without executing: no fallback/child context.
			}
			return p.table(id, "gem-86", false)
		}
		return nil
	default:
		if _, ok := callingCommand(op.Kind); !ok {
			return ErrUnsupportedEffect
		}
		if afterEffects {
			for job := 1; job <= 87; job++ {
				if isCallingJob(job) {
					if err := p.table(job, op.Kind, true); err != nil {
						return err
					}
				}
			}
			return nil
		}
		job := callingJob(p.s.actor(p.c.ActorID).Jobs)
		if job == 0 {
			return nil
		}
		return p.table(job, op.Kind, false)
	}
}

// A nested current-job operation may follow copying, form switching or a helper
// actor switch. Cover every certified context plus the full player/form domain.
func (p *callbackAdmission) allTables(kind string) error {
	for job := 1; job <= 87; job++ {
		if job != 73 {
			if err := p.table(job, kind, true); err != nil {
				return err
			}
		}
	}
	if err := p.table(300, kind, true); err != nil {
		return err
	}
	for job := range p.d.JobTables {
		if job > 87 && job != 300 || job == 0 {
			if err := p.table(job, kind, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *callbackAdmission) table(job int, kind string, afterEffects bool) error {
	if _, calling := callingCommand(kind); calling || kind == "gem-86" {
		child := explicitCallbackTarget(p.c)
		copy := *p
		copy.c, copy.explicitTarget = &child, true
		p = &copy
	}
	if job == 73 {
		a := p.s.actor(p.c.ActorID)
		job = (a.Attack + a.Defense + a.Agility) % 73
		if job <= 0 {
			job = 8
		}
	}
	table, ok := p.d.JobTables[job]
	if !ok || len(table.Variants) == 0 || len(table.Variants) > 1 && table.Select == nil {
		return ErrUnsupportedEffect
	}
	if table.Select != nil {
		variant := table.Select(p.s, p.c)
		if variant < 0 || variant >= len(table.Variants) {
			return ErrUnsupportedEffect
		}
	}
	for _, entries := range table.Variants {
		ordinary := false
		for _, entry := range entries {
			ordinary = ordinary || !entry.Extended
		}
		if kind == "random" && len(entries) > 0 && !ordinary || (kind == "tool-84" || kind == "generated") && len(entries) == 0 {
			return ErrUnsupportedEffect
		}
		if kind == "gem-86" {
			if len(entries) == 0 || !entries[len(entries)-1].Extended {
				continue
			}
			entries = entries[len(entries)-1:]
		} else if name, calling := callingCommand(kind); calling {
			var first []CallbackEntry
			for _, entry := range entries {
				if callingMatch(entry, name) {
					first = []CallbackEntry{entry}
					break
				}
			}
			entries = first
		}
		for _, entry := range entries {
			if kind == "random" && entry.Extended {
				continue
			}
			if entry.ID == "" || entry.RequiredSP < 0 || entry.MPCost < 0 {
				return ErrUnsupportedEffect
			}
			if p.d.Entries[entry.ID].Invocation.Kind == "job-82-generated" && job != 82 {
				return ErrUnsupportedEffect
			}
			if err := p.visit(CallbackInvocation{Kind: "entry", EntryID: entry.ID}, afterEffects); err != nil {
				return err
			}
		}
	}
	return nil
}

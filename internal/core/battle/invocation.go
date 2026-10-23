package battle

import (
	"slices"
	"strconv"
	"strings"

	"github.com/witchcraze/party2re/internal/core/random"
)

// CallbackContext retains the actual actor and remaining arguments. Naked
// invocation shares Arguments, including a child's consumed arguments. Command
// stays the outer command. AfterEffects is set only during conservative preflight.
type CallbackContext struct {
	TargetContext
	Arguments    []string
	AfterEffects bool
	// ArithmeticDivisor is shared by nested callbacks in this operation only.
	ArithmeticDivisor int
	// Invoke executes a declared child in the current argument/actor frame. It
	// is supplied only during Execute, after the enclosing closure was admitted.
	Invoke func(CallbackInvocation) error
}

// CallbackInvocation describes an invocation boundary, not a scalar attack kind.
// Kinds are random, tool-84, gem-86..92, job-82-generated, and entry (by native ID).
type CallbackInvocation struct {
	Kind, EntryID string
}

type CallbackEntry struct {
	ID, Name           string
	RequiredSP, MPCost int
	Extended           bool
}

// CallbackTable contains complete ordered tables, including every context
// variant. Select is pure and chooses a variant against the current actor;
// admission checks them all. A present empty variant differs from a missing job.
type CallbackTable struct {
	Variants [][]CallbackEntry
	Select   func(*LifetimeState, *CallbackContext) int
}

// CallbackDefinition certifies the entire callback and its transitive helper,
// book and tool edges. Validate must be pure and cover earlier reachable effects
// when AfterEffects is true. Invocation supplies a native invoking body instead
// of Execute; neither a kind nor an entry's presence certifies a legacy table.
type CallbackDefinition struct {
	Invocation CallbackInvocation
	Children   []CallbackInvocation
	Validate   func(*LifetimeState, *CallbackContext) error
	Execute    func(*LifetimeState, *CallbackContext, random.Generator) error
	// Project supplies the complete deterministic transition on an isolated
	// copy, without external Apply hooks or RNG. False leaves input unchanged
	// and means unknown; later callbacks must admit arbitrary prior effects.
	// Never sample outcomes or omit operation facts changed by the body.
	Project func(*LifetimeState, *CallbackContext) (bool, error)
}

// CallbackCatalog is server-owned, default-empty certification. Table variants
// must cover the full source context, never a supported-only subset. Functions
// are rebuilt by the consumer; only resulting battle facts belong in run state.
type CallbackCatalog struct {
	JobTables map[int]CallbackTable
	Entries   map[string]CallbackDefinition
}

// ApplyInvocations preflights all slots before executing any of them. Outer
// SP/admission, MP/CMP and item costs belong to the invoking consumer, which must
// call the same preflight before charging. Stop later custom slots after death.
func (s *LifetimeState) ApplyInvocations(c *CallbackContext, d CallbackCatalog, ops []CallbackInvocation, rng random.Generator) error {
	if err := s.ValidateInvocations(c, d, ops, rng); err != nil {
		return err
	}
	for _, op := range ops {
		child := *c
		child.Arguments = slices.Clone(c.Arguments) // Each custom gem receives the original target.
		if err := s.invokeCallback(&child, d, op, rng); err != nil {
			return err
		}
		c.TargetContext = child.TargetContext
		c.ArithmeticDivisor = child.ArithmeticDivisor
		if s.actor(c.ActorID).HP <= 0 {
			break
		}
	}
	return nil
}

type callbackSelection struct {
	CallbackEntry
	child *callbackSelection
}

func (s *LifetimeState) callbackTable(c *CallbackContext, d CallbackCatalog, job int) ([]callbackSelection, error) {
	a := s.actor(c.ActorID)
	if job == 73 {
		job = (a.Attack + a.Defense + a.Agility) % 73
		if job <= 0 {
			job = 8
		}
	}
	table, ok := d.JobTables[job]
	if !ok || len(table.Variants) == 0 {
		return nil, ErrUnsupportedEffect
	}
	variant := 0
	if table.Select != nil {
		variant = table.Select(s, c)
	}
	if variant < 0 || variant >= len(table.Variants) {
		return nil, ErrUnsupportedEffect
	}
	entries := table.Variants[variant]
	selected := make([]callbackSelection, 0, len(entries))
	for _, entry := range entries {
		item := callbackSelection{CallbackEntry: entry}
		if d.Entries[entry.ID].Invocation.Kind == "job-82-generated" {
			child, err := s.generatedCallback(c, d)
			if err != nil {
				return nil, err
			}
			item.MPCost, item.Name, item.child = child.MPCost, "機構技能_"+child.Name, &child
		}
		selected = append(selected, item)
	}
	return selected, nil
}

func (s *LifetimeState) generatedCallback(c *CallbackContext, d CallbackCatalog) (callbackSelection, error) {
	a := s.actor(c.ActorID)
	sum := a.Attack + a.Defense + a.Agility + len(s.Actors)
	job := sum % 87
	if job <= 0 || job == 73 || job == 82 {
		job = 8
	}
	entries, err := s.callbackTable(c, d, job)
	if err != nil {
		return callbackSelection{}, err
	}
	if len(entries) == 0 || sum < 0 {
		return callbackSelection{}, ErrUnsupportedEffect
	}
	return entries[sum%len(entries)], nil
}

func (s *LifetimeState) invokeCallback(c *CallbackContext, d CallbackCatalog, op CallbackInvocation, rng random.Generator) error {
	switch op.Kind {
	case "entry":
		return s.executeCallback(c, d, callbackSelection{CallbackEntry: CallbackEntry{ID: op.EntryID}}, rng)
	case "random", "tool-84":
		job := callbackJob(s.actor(c.ActorID).Jobs.JobID)
		if op.Kind == "random" {
			job = 1 + int(rng.Float64()*87)
		}
		entries, err := s.callbackTable(c, d, job)
		if err != nil || len(entries) == 0 {
			return err // Empty general-random tables are accepted no-effect.
		}
		entry := entries[int(rng.Float64()*float64(len(entries)))]
		for op.Kind == "random" && entry.Extended {
			entry = entries[int(rng.Float64()*float64(len(entries)))]
		}
		mp := s.actor(c.ActorID).MP
		if err := s.executeCallback(c, d, entry, rng); err != nil {
			return err
		}
		s.actor(c.ActorID).MP = max(mp, s.actor(c.ActorID).MP)
		return nil
	case "job-82-generated":
		child, err := s.generatedCallback(c, d)
		if err != nil {
			return err
		}
		return s.executeGenerated(c, d, child, rng)
	case "gem-86":
		a := s.actor(c.ActorID)
		for _, job := range []struct {
			id string
			sp int
		}{{a.Jobs.JobID, a.Jobs.SP}, {a.Jobs.OldJobID, a.Jobs.OldSP}} {
			if job.id == "" {
				continue
			}
			if callbackJob(job.id) == 73 {
				a = s.actor(c.ActorID)
				if a.MP > 0 {
					if a.Attack < 999 {
						a.Attack += len(c.PartyIDs)
					}
					a.MP--
				}
				return nil
			}
			entries, err := s.callbackTable(c, d, callbackJob(job.id))
			if err != nil {
				return err
			}
			if len(entries) == 0 || !entries[len(entries)-1].Extended {
				continue
			}
			entry := entries[len(entries)-1]
			if entry.RequiredSP <= job.sp && entry.MPCost <= s.actor(c.ActorID).MP {
				if err := s.executeExplicitCallback(c, d, entry, rng); err != nil {
					return err
				}
				s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-entry.MPCost)
			}
			return nil
		}
		return nil
	default:
		name, ok := callingCommand(op.Kind)
		if !ok {
			return ErrUnsupportedEffect
		}
		job := callingJob(s.actor(c.ActorID).Jobs)
		if job == 0 {
			return nil
		}
		entries, err := s.callbackTable(c, d, job)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if callingMatch(entry.CallbackEntry, name) {
				return s.executeExplicitCallback(c, d, entry, rng)
			}
		}
		return nil
	}
}

func (s *LifetimeState) executeExplicitCallback(c *CallbackContext, d CallbackCatalog, entry callbackSelection, rng random.Generator) error {
	child := explicitCallbackTarget(c)
	err := s.executeCallback(&child, d, entry, rng)
	c.TargetContext = child.TargetContext
	c.ArithmeticDivisor = child.ArithmeticDivisor
	return err
}

func (s *LifetimeState) executeGenerated(c *CallbackContext, d CallbackCatalog, child callbackSelection, rng random.Generator) error {
	if child.MPCost > s.actor(c.ActorID).MP {
		return nil
	}
	return s.executeCallback(c, d, child, rng)
}

func (s *LifetimeState) executeCallback(c *CallbackContext, d CallbackCatalog, entry callbackSelection, rng random.Generator) error {
	if entry.child != nil {
		return s.executeGenerated(c, d, *entry.child, rng)
	}
	def, ok := d.Entries[entry.ID]
	if !ok {
		return ErrUnsupportedEffect
	}
	if def.Invocation.Kind != "" {
		return s.invokeCallback(c, d, def.Invocation, rng)
	}
	invoke := c.Invoke
	c.Invoke = func(op CallbackInvocation) error {
		if !slices.Contains(def.Children, op) {
			return ErrUnsupportedEffect
		}
		return s.invokeCallback(c, d, op, rng)
	}
	defer func() { c.Invoke = invoke }()
	return def.Execute(s, c, rng)
}

func explicitCallbackTarget(c *CallbackContext) CallbackContext {
	child := *c
	target := ""
	if len(c.Arguments) > 0 {
		target = c.Arguments[0]
	}
	child.Arguments = []string{target}
	return child
}

func callbackJob(id string) int {
	job, err := strconv.Atoi(strings.TrimPrefix(id, "job-"))
	if err != nil {
		return -1
	}
	return job
}

func isCallingJob(job int) bool {
	switch job {
	case 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 69, 71, 80, 87:
		return true
	}
	return false
}

func callingJob(jobs BattleJobs) int {
	if job := callbackJob(jobs.OldJobID); isCallingJob(job) {
		return job
	}
	if job := callbackJob(jobs.JobID); isCallingJob(job) {
		return job
	}
	return 0
}

func callingCommand(kind string) (string, bool) {
	switch kind {
	case "gem-87":
		return "summon", true
	case "gem-88":
		return "dismiss", true
	case "gem-89":
		return "attack", true
	case "gem-90":
		return "special", true
	case "gem-91":
		return "defend", true
	case "gem-92":
		return "", true
	}
	return "", false
}

func callingMatch(entry CallbackEntry, name string) bool {
	if name != "" {
		return entry.Name == name
	}
	switch entry.Name {
	case "summon", "dismiss", "attack", "special", "defend":
		return false
	}
	return !entry.Extended
}

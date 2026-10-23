package skill

import (
	"strconv"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterArithmetic binds the eleven intrinsic bodies and full ordered table
// contributions. Unbound job31 positions still prevent a partial random pool.
func (d *BattleDefinitions) RegisterArithmetic(c *corebattle.ArithmeticContext) {
	if d.ArithmeticSkills == nil {
		d.ArithmeticSkills = make(map[string]bool)
	}
	if d.Invocations.Entries == nil {
		d.Invocations.Entries = make(map[string]corebattle.CallbackDefinition)
	}
	if d.Invocations.JobTables == nil {
		d.Invocations.JobTables = make(map[int]corebattle.CallbackTable)
	}
	for _, job := range []int{31, 72} {
		var entries []corebattle.CallbackEntry
		if job == 31 {
			// _skill.cgi:2608-2715; other bodies retain their own owners.
			entries = []corebattle.CallbackEntry{
				{ID: "job-31-entry-0", Name: "self-destruct", RequiredSP: 11, MPCost: 11},
				{ID: "job-31-entry-1", Name: "death-roulette", RequiredSP: 44, MPCost: 4},
				{ID: "job-31-entry-2", Name: "missing-hp", RequiredSP: 66, MPCost: 18},
				{ID: "job-31-entry-3", Name: "party-guard", RequiredSP: 77, MPCost: 34},
			}
		}
		for _, entry := range corebattle.ArithmeticEntries("job-" + strconv.Itoa(job)) {
			entries = append(entries, corebattle.CallbackEntry(entry))
			if entry.ID == "job-31-gravity" {
				entries = append(entries, corebattle.CallbackEntry{ID: "job-31-entry-5", Name: "party-hp-heal", RequiredSP: 121, MPCost: 36})
			}
			d.ArithmeticSkills[entry.ID] = true
			d.Invocations.Entries[entry.ID] = d.ArithmeticInvocation(c, entry.ID)
		}
		d.Invocations.JobTables[job] = corebattle.CallbackTable{Variants: [][]corebattle.CallbackEntry{entries}}
	}
}

// ArithmeticInvocation retains positional identity and the live argument frame.
// Only selectors use the submitted-text/last-discovered-name lookup.
func (d BattleDefinitions) ArithmeticInvocation(c *corebattle.ArithmeticContext, id string) corebattle.CallbackDefinition {
	context := func(frame *corebattle.CallbackContext) (corebattle.ArithmeticContext, string, error) {
		if c == nil {
			return corebattle.ArithmeticContext{}, "", corebattle.ErrUnsupportedEffect
		}
		child := *c
		child.TargetContext, child.Divisor = frame.TargetContext, frame.ArithmeticDivisor
		target := ""
		if len(frame.Arguments) > 0 {
			target = frame.Arguments[0]
		}
		return child, target, nil
	}
	return corebattle.CallbackDefinition{
		Validate: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) error {
			child, target, err := context(frame)
			if err != nil {
				return err
			}
			selected, _, err := child.ArithmeticSelection(id)
			if err != nil {
				return err
			}
			if selected != "" && selected != id {
				def := d.Invocations.Entries[selected]
				if def.Validate == nil || def.Execute == nil {
					return corebattle.ErrUnsupportedEffect
				}
			}
			if frame.AfterEffects {
				// Earlier unknown effects may change HP, MP, base defense, or
				// the operation divisor, making a previously safe member lethal.
				if id == "job-31-reverse" {
					return corebattle.ErrUnsupportedEffect
				}
				if selected == "job-31-death" || selected == "job-72-death" || selected == "job-72-damage" || id == "job-72-death" || id == "job-72-damage" {
					if child.Defeat == nil {
						return corebattle.ErrUnsupportedEffect
					}
				}
			}
			_, err = d.ArithmeticAction(s, &child, id, target, random.Default())
			return err
		},
		Project: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) (bool, error) {
			child, target, err := context(frame)
			if err != nil {
				return false, err
			}
			known, err := s.ProjectArithmetic(&child, id, target)
			if known && err == nil {
				frame.ArithmeticDivisor = child.Divisor
				if id == "job-31-reverse" && len(frame.Arguments) > 0 {
					frame.Arguments = frame.Arguments[1:]
				}
			}
			return known, err
		},
		Execute: func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext, rng random.Generator) error {
			child, target, err := context(frame)
			if err != nil {
				return err
			}
			if _, err := d.ArithmeticAction(s, &child, id, target, rng); err != nil {
				return err
			}
			result, err := s.ApplyArithmetic(&child, id, target, rng)
			frame.ArithmeticDivisor = child.Divisor
			if result.TargetConsumed && len(frame.Arguments) > 0 {
				frame.Arguments = frame.Arguments[1:]
			}
			return err
		},
	}
}

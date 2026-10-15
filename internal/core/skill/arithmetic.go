package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// ArithmeticAction shares the concrete executor's admission for discovery and
// submission. Register individual source-backed callbacks only; a selector also
// needs its discovered child registered. Random closures retain their own gate.
func (d BattleDefinitions) ArithmeticAction(s *corebattle.LifetimeState, c *corebattle.ArithmeticContext, id, targetID string, rng random.Generator) (corebattle.ArithmeticEntry, error) {
	if s == nil {
		return corebattle.ArithmeticEntry{}, corebattle.ErrInvalidRequest
	}
	if !d.ArithmeticSkills[id] {
		return corebattle.ArithmeticEntry{}, corebattle.ErrUnsupportedEffect
	}
	child, _, err := c.ArithmeticSelection(id)
	if err != nil {
		return corebattle.ArithmeticEntry{}, err
	}
	if child != "" && !d.ArithmeticSkills[child] {
		return corebattle.ArithmeticEntry{}, corebattle.ErrUnsupportedEffect
	}
	if err := s.ValidateArithmetic(c, id, targetID, rng); err != nil {
		return corebattle.ArithmeticEntry{}, err
	}
	for _, job := range []string{"job-31", "job-72"} {
		for _, entry := range corebattle.ArithmeticEntries(job) {
			if entry.ID == id {
				return entry, nil
			}
		}
	}
	return corebattle.ArithmeticEntry{}, corebattle.ErrUnsupportedEffect
}

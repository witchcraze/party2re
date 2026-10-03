package playercontext

import (
	"time"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
)

// LocationTown is the initial observation scene; facility scenes are added later.
const LocationTown = "town"

// Snapshot holds the read state shared by availability evaluation and its caller.
// Sleeping includes pending wake recovery after the sleep duration has elapsed.
type Snapshot struct {
	Character      character.Character
	OngoingActions []scheduling.ScheduledAction
	Sleeping       bool
	CanWake        bool
	SleepRemaining time.Duration
	LocationID     string
}

// Evaluator applies one availability rule, honoring action-specific exemptions.
type Evaluator interface {
	Allows(action ActionDefinition, snapshot *Snapshot) bool
}

func defaultEvaluators() []Evaluator {
	return []Evaluator{DeadGate{}, FatigueGate{}, SleepGate{}, CooldownGate{}, CurrencyGate{}, LocationGate{}}
}

// Evaluate returns eligible top-level ActionIDs in catalog order. Execution
// services still validate request-specific amounts, items and other prerequisites.
func Evaluate(snapshot Snapshot) []string {
	return evaluate(DefaultCatalog, &snapshot, defaultEvaluators())
}

func evaluate(catalog []ActionDefinition, snapshot *Snapshot, evaluators []Evaluator) []string {
	ids := make([]string, 0, len(catalog))
	for _, action := range catalog {
		allowed := true
		for _, evaluator := range evaluators {
			if !evaluator.Allows(action, snapshot) {
				allowed = false
				break
			}
		}
		if allowed {
			ids = append(ids, action.ID)
		}
	}
	return ids
}

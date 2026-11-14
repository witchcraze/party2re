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
	Activities     []Activity
	// SceneActions are the location-bound commands the observed scene admits;
	// nil when no scene registry is configured.
	SceneActions []string
	// ActionWait is the remaining legacy action wait; zero when none.
	ActionWait time.Duration
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
	location := activityLocation(snapshot.ActiveActivities())
	for _, action := range catalog {
		if !(ActionWaitGate{}).Allows(action, snapshot) {
			continue
		}
		if action.ActivityKind != "" {
			if continuationAllowed(action, snapshot, location) {
				ids = append(ids, action.ID)
			}
			continue
		}
		if location != "" && action.ID != "home_wake" && action.ID != "rescue_request" && action.ID != "presence_refresh" {
			continue
		}
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

package playercontext

import (
	"context"
	"slices"

	"github.com/witchcraze/party2re/internal/core/scheduling"
)

// Activity is a whitelist of owned membership/run facts, never raw feature state.
// Actions are service-verified continuation candidates, not execution approval.
type Activity struct {
	Kind    string   `json:"kind"`
	ID      string   `json:"id"`
	Role    string   `json:"role"`
	Phase   string   `json:"phase"`
	Round   int      `json:"round"`
	Actions []string `json:"actions"`
	PartyID string   `json:"party_id,omitempty"`
}

// ActivityReader adapts public feature services at the composition root.
// The query verifies ownership before invoking this required, read-only port.
type ActivityReader func(ctx context.Context, playerID, actorID string) ([]Activity, error)

func WithActivities(read ActivityReader) func(*Service) {
	return func(s *Service) { s.readActivities = read }
}

// ActiveActivities retains every unfinished fact, including elapsed work.
func (s Snapshot) ActiveActivities() []Activity {
	facts := append([]Activity{}, s.Activities...)
	for i := range facts {
		facts[i].Actions = append([]string{}, facts[i].Actions...)
	}
	if s.Sleeping {
		phase := "sleeping"
		if s.CanWake {
			phase = "recovery_pending"
		}
		facts = append(facts, Activity{Kind: "sleep", ID: "sleep", Role: "actor", Phase: phase, Actions: []string{}})
	}
	for _, a := range s.OngoingActions {
		if a.State == scheduling.StatePending || a.State == scheduling.StateProcessing {
			facts = append(facts, Activity{Kind: "scheduled", ID: a.ID, Role: "actor", Phase: string(a.State), Actions: []string{}})
		}
	}
	return facts
}

func activityLocation(facts []Activity) string {
	location := ""
	for _, fact := range facts {
		// An explicitly linked party roster and its run are one activity.
		if fact.Kind == "party" && slices.ContainsFunc(facts, func(run Activity) bool {
			return (run.Kind == "dungeon" || run.Kind == "challenge") && run.PartyID == fact.ID
		}) {
			continue
		}
		if location != "" {
			return "activity_conflict"
		}
		location = fact.Kind
	}
	return location
}

func continuationAllowed(action ActionDefinition, s *Snapshot, location string) bool {
	if !action.Recovery && (s.Sleeping || location != action.ActivityKind) {
		return false
	}
	return slices.ContainsFunc(s.Activities, func(a Activity) bool {
		return a.Kind == action.ActivityKind && slices.Contains(a.Actions, action.ID)
	})
}

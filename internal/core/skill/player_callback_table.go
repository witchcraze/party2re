package skill

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// PlayerCallback retains callback metadata, including source SP0/extended0,
// separately from scalar Definition and its positive-SP constructor.
type PlayerCallback struct {
	corebattle.CallbackBinding
	RequiredJobIDs []string
}

// PlayerCallbackTable supplies full ordered metadata for a canonical persistent
// job ID. Consumers apply ordinary learning/equipment/cost gates and validate
// the selected frozen Invocation before charging or mutating anything.
func (d BattleDefinitions) PlayerCallbackTable(s *corebattle.LifetimeState, c *corebattle.CallbackContext, jobID string) ([]PlayerCallback, error) {
	job, err := strconv.Atoi(strings.TrimPrefix(jobID, "job-"))
	if jobID == "starter" {
		job, err = 0, nil
	}
	canonical := fmt.Sprintf("job-%02d", job)
	if job == 0 {
		canonical = "starter"
	}
	if err != nil || canonical != jobID {
		return nil, ErrUnavailable
	}
	bindings, err := s.CallbackBindings(c, d.Invocations, job)
	if err != nil {
		return nil, err
	}
	entries := make([]PlayerCallback, 0, len(bindings)+2)
	for _, binding := range bindings {
		entries = append(entries, PlayerCallback{CallbackBinding: binding, RequiredJobIDs: []string{jobID}})
	}
	if job != 73 {
		return entries, nil
	}
	for _, a := range s.Actors {
		if a.ID != c.ActorID || a.MP < 1 || !slices.Contains(a.ItemDefinitionIDs, "item-158") {
			continue
		}
		for _, slot := range []struct {
			id   string
			sp   int
			name string
		}{{a.Jobs.JobID, a.Jobs.SP, "current"}, {a.Jobs.OldJobID, a.Jobs.OldSP, "previous"}} {
			if slot.id == jobID && slot.sp != 0 {
				id := "job-73-gear-" + slot.name
				entries = append(entries, PlayerCallback{CallbackBinding: corebattle.CallbackBinding{CallbackEntry: corebattle.CallbackEntry{ID: id, Name: "shift-gear", MPCost: 1}, Invocation: corebattle.CallbackInvocation{Kind: "entry", EntryID: id}}, RequiredJobIDs: []string{jobID}})
			}
		}
	}
	return entries, nil
}

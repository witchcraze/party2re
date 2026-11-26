package adventure_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/runhost"
)

// Observation discovers the operations Command would admit, reveals only
// public encounter facts and leaves the stored run byte-identical.
func TestObserveRunDiscoversWithoutMutation(t *testing.T) {
	f := combatFixture(t, 1)
	cmd := combatRoom(t, f, f.actors[0], nil, runEnemy("foe", 900))
	before, err := json.Marshal(readStockRoom(t, f, cmd.RunID))
	if err != nil {
		t.Fatal(err)
	}
	obs, err := f.svc.ObserveRun(t.Context(), cmd.PlayerID, cmd.ActorID)
	if err != nil || obs == nil {
		t.Fatal(obs, err)
	}
	usable := func(action string) bool {
		return slices.ContainsFunc(obs.Choices, func(c runhost.OperationChoice) bool { return c.Action == action && !c.Gated })
	}
	if obs.RunID != cmd.RunID || obs.Version != cmd.Version || !usable("attack") || !usable("exit") || usable("advance") {
		t.Fatalf("observation %+v", obs)
	}
	if !slices.ContainsFunc(obs.Actors, func(a runhost.ObservedActor) bool { return a.ID == "foe" && a.Enemy && a.HP == 900 }) || obs.ReadyAt.After(f.clock.Now()) {
		t.Fatalf("actors %+v ready %v now %v", obs.Actors, obs.ReadyAt, f.clock.Now())
	}
	after, err := json.Marshal(readStockRoom(t, f, cmd.RunID))
	if err != nil || string(before) != string(after) {
		t.Fatalf("observation changed the run: %v", err)
	}
	if _, err := f.svc.ObserveRun(t.Context(), "foreign-player", cmd.ActorID); !errors.Is(err, runhost.ErrForbidden) {
		t.Fatalf("foreign observation: %v", err)
	}

	r := accepted(t, f, combatCommand(cmd, "strike", "attack", "foe", ""))
	if obs, err = f.svc.ObserveRun(t.Context(), cmd.PlayerID, cmd.ActorID); err != nil || obs.Version != r.Version || !obs.ReadyAt.After(f.clock.Now()) {
		t.Fatalf("after command: %+v %v", obs, err)
	}
}

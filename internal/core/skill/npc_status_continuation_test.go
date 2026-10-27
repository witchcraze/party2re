package skill

import (
	"encoding/json"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestNPCStatusFrozenReloadAndCompletedCallback(t *testing.T) {
	s, c, d := npcStatusFixture()
	c.Command, c.Custom = "frozen-npc-status", true
	s.Actors[0].NPC, c.ActorNPC = true, true
	s.Actors[0].Jobs.JobID = "job-90"
	s.Actors[0].ItemDefinitionIDs = []string{"item-249"} // No player cost modifier here.
	executeNPCStatus(t, &s, c, d, 90, 0, []string{"enemy", "tail"}, 0, 0)
	type checkpoint struct {
		State                    corebattle.LifetimeState
		Target                   corebattle.TargetContext
		Entries                  []corebattle.CallbackEntry
		Arguments                []string
		Completed                bool
		DamageCalls, StatusCalls int
	}
	frozen := checkpoint{State: s, Target: c.TargetContext, Entries: d.Invocations.JobTables[90].Variants[0], Arguments: c.Arguments, Completed: true, DamageCalls: c.DamageCalls, StatusCalls: c.StatusCalls}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if !loaded.Completed || loaded.State.Actors[1].HP != 751 || loaded.State.Actors[1].Status != corebattle.StatusDeadlyPoison || loaded.State.Actors[0].MP != 80 || loaded.DamageCalls != 1 || loaded.StatusCalls != 1 || !reflect.DeepEqual(loaded.Entries, corebattle.NPCJobEntries(90)) || !reflect.DeepEqual(loaded.Arguments, []string{"tail"}) {
		t.Fatal(loaded)
	}
	resumed := &corebattle.NPCStatusContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, ActorNPC: true}}, Death: &corebattle.DefeatContext{}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterNPCStatus(resumed)
	// A completed receipt resumes after the callback. Rebuilding the ports and
	// inspecting the saved facts performs no callback or effect replay.
	unchanged, err := json.Marshal(loaded)
	if err != nil || string(data) != string(unchanged) {
		t.Fatal(err, loaded)
	}
	executeNPCStatus(t, &s, c, d, 93, 3, []string{"enemy", "tail"}, 0)
	executeNPCStatus(t, &loaded.State, resumed, rebuilt, 93, 3, []string{"enemy", "tail"}, 0)
	if !reflect.DeepEqual(s, loaded.State) || loaded.State.Actors[1].HP != 199 || loaded.State.Actors[0].MP != 80 || resumed.Command != "frozen-npc-status" || !resumed.Custom {
		t.Fatal(s, loaded.State)
	}
}

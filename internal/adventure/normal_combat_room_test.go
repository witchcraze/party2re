package adventure

import (
	"errors"
	"slices"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

func choiceActions(choices []runhost.Choice) []string {
	var actions []string
	for _, c := range choices {
		actions = append(actions, c.Action)
	}
	return actions
}

func TestStageRoomChoicesFollowFloors(t *testing.T) {
	var room stageRoom
	r := &NormalRun{State: runhost.State{Round: 1, LeaderID: "a", Roster: []string{"a", "b"}, Battle: corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "a", TeamID: "party", HP: 100, MaxHP: 100}},
		{Participant: corebattle.Participant{ID: "c", TeamID: "enemy", HP: 100, MaxHP: 100}, NPC: true},
	}}}}
	r.Enemies = []string{"c"}
	if got := choiceActions(room.Choices(r, "a")); len(got) != 0 {
		t.Fatalf("living enemy listed %v", got)
	}
	// add_battle_action lists すすむ whenever no enemy remains; susumu rejects
	// a nonleader's first advance and a finished quest when executed.
	r.Enemies = nil
	r.Round = 0
	for _, actor := range []string{"a", "b"} {
		if got := choiceActions(room.Choices(r, actor)); !slices.Equal(got, []string{"advance"}) {
			t.Fatalf("%s's first advance: %v", actor, got)
		}
	}
	// しらべる precedes すすむ; an unopened chest is an enemy.
	r.Round, r.Boxes = TreasureRoomFloor, []runhost.TreasureBox{{ID: "box-1"}}
	if got := choiceActions(room.Choices(r, "a")); !slices.Equal(got, []string{"chest"}) {
		t.Fatalf("treasure floor with an unopened chest: %v", got)
	}
	r.Boxes[0].OpenedBy = "a"
	choices := room.Choices(r, "a")
	if got := choiceActions(choices); !slices.Equal(got, []string{"chest", "advance"}) || choices[1].Check("") != nil {
		t.Fatalf("finished ordinary treasure floor: %v", got)
	}
}

func TestStageRoomChoicesEXTreasureFloors(t *testing.T) {
	var room stageRoom
	r := &NormalRun{State: runhost.State{LeaderID: "a", Roster: []string{"a"}}}
	r.Stage, r.EXUnlocked = Stage{ID: "stage-24", StageTraits: StageTraits{EX: &EXStage{Marker: markerBossEncounter, TreasureItems: []string{"item-104"}}}}, true
	r.Round = EXBossFloor
	choices := room.Choices(r, "a")
	if got := choiceActions(choices); !slices.Equal(got, []string{"advance"}) || choices[0].Check("") != nil {
		t.Fatalf("12→13 advance: %v", got)
	}
	r.Stage.EX.TreasureItems = nil
	if err := room.Choices(r, "a")[0].Check(""); !errors.Is(err, runhost.ErrUnavailable) {
		t.Fatalf("advance into an empty EX treasure room: %v", err)
	}
	r.Round, r.Boxes = EXTreasureFloor, []runhost.TreasureBox{{ID: "box-1", Fixed: true}}
	if got := choiceActions(room.Choices(r, "a")); !slices.Equal(got, []string{"chest"}) {
		t.Fatalf("floor-13 choices: %v", got)
	}
	r.Boxes[0].OpenedBy = "a"
	if got := choiceActions(room.Choices(r, "a")); !slices.Equal(got, []string{"chest", "advance"}) {
		t.Fatalf("finished floor-13 choices: %v", got)
	}
}

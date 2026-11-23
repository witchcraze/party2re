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
	r.Enemies = nil
	r.Round = 0
	if got := choiceActions(room.Choices(r, "b")); len(got) != 0 {
		t.Fatalf("first advance listed for a nonleader: %v", got)
	}
	if got := choiceActions(room.Choices(r, "a")); !slices.Equal(got, []string{"advance"}) {
		t.Fatalf("leader's first advance: %v", got)
	}
	r.Round = TreasureRoomFloor
	if got := choiceActions(room.Choices(r, "a")); !slices.Equal(got, []string{"chest"}) {
		t.Fatalf("ordinary treasure floor without EX: %v", got)
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
}

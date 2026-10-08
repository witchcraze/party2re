package casino

import (
	"reflect"
	"testing"
)

func TestRoomViewControls(t *testing.T) {
	v := RoomView{Room: RoomObservation{RoomSummary: RoomSummary{GameType: GameTypeHighLow, Status: RoomStatusWaiting, ParticipantCount: 2}, LeaderCharacterID: "hero"}, Members: []MemberObservation{{CharacterID: "hero"}, {CharacterID: "peer"}, {CharacterID: "watcher", IsSpectator: true}}}
	c, err := v.Controls("hero")
	if err != nil || c.Role != "leader" || !c.CanStart || !reflect.DeepEqual(c.KickTargets, []string{"peer", "watcher"}) || len(c.Actions) != 0 {
		t.Fatalf("waiting controls: %+v %v", c, err)
	}
	v.Room.Round, v.Room.Status = 1, RoomStatusInProgress
	for _, tc := range []struct {
		actor, role string
		game        GameType
		actions     []string
	}{
		{"hero", "leader", GameTypeIndian, []string{"call", "showdown", "fold"}},
		{"peer", "member", GameTypeHighLow, []string{"call", "high", "fold"}},
		{"hero", "leader", GameTypeDoppel, []string{"★", "●", "◆"}},
		{"watcher", "spectator", GameTypeIndian, []string{}},
	} {
		v.Room.GameType = tc.game
		c, err := v.Controls(tc.actor)
		if err != nil || c.Role != tc.role || c.CanStart || len(c.KickTargets) != 0 || !reflect.DeepEqual(c.Actions, tc.actions) {
			t.Fatalf("%s/%s: %+v %v", tc.game, tc.actor, c, err)
		}
	}
	v.Room.GameType, v.Room.ParticipantCount = GameTypeHighLow, 3
	c, _ = v.Controls("hero")
	if !reflect.DeepEqual(c.Actions, []string{"call", "high", "low", "fold"}) {
		t.Fatalf("low choices: %+v", c)
	}
	v.Members[0].Action = "high"
	c, _ = v.Controls("hero")
	if len(c.Actions) != 0 {
		t.Fatal("acted member can act again")
	}
	v.Room.GameType = GameTypeDoppel
	c, _ = v.Controls("hero")
	if len(c.Actions) != 4 {
		t.Fatal("existing Doppel mark reselection was hidden")
	}
	if _, err := v.Controls("outsider"); err != ErrRoomViewForbidden {
		t.Fatalf("outsider: %v", err)
	}
}

func TestSlotBetRates(t *testing.T) {
	for _, job := range []string{"", "job-01", "job-46", "46"} {
		want := []int64{1, 10, 50, 100}
		if job == "job-46" || job == "46" {
			want = append(want, 200)
		}
		if got := SlotBetRates(job); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %v", job, got)
		}
	}
}

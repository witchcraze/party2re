package casino

import (
	"reflect"
	"testing"
)

func TestRoomViewControls(t *testing.T) {
	v := RoomView{Room: RoomObservation{RoomSummary: RoomSummary{GameType: GameTypeHighLow, Status: RoomStatusWaiting, ParticipantCount: 2}, LeaderCharacterID: "hero", CurrentBet: 10, MaxBet: 50}, Members: []MemberObservation{{CharacterID: "hero"}, {CharacterID: "peer"}, {CharacterID: "watcher", IsSpectator: true}}}
	c, err := v.Controls("hero", 1000)
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
		c, err := v.Controls(tc.actor, 1000)
		if err != nil || c.Role != tc.role || c.CanStart || len(c.KickTargets) != 0 || !reflect.DeepEqual(c.Actions, tc.actions) {
			t.Fatalf("%s/%s: %+v %v", tc.game, tc.actor, c, err)
		}
	}
	v.Room.GameType, v.Room.ParticipantCount = GameTypeHighLow, 3
	c, _ = v.Controls("hero", 1000)
	if !reflect.DeepEqual(c.Actions, []string{"call", "high", "low", "fold"}) {
		t.Fatalf("low choices: %+v", c)
	}
	v.Members[0].Action = "high"
	c, _ = v.Controls("hero", 1000)
	if len(c.Actions) != 0 {
		t.Fatal("acted member can act again")
	}
	v.Room.GameType = GameTypeDoppel
	c, _ = v.Controls("hero", 1000)
	if len(c.Actions) != 4 {
		t.Fatal("existing Doppel mark reselection was hidden")
	}
	if _, err := v.Controls("outsider", 1000); err != ErrRoomViewForbidden {
		t.Fatalf("outsider: %v", err)
	}

	// つづける needs now_bet < max_bet && coin >= now_bet (casino_highlow.cgi:13-16).
	v.Members[0].Action = ""
	v.Room.GameType = GameTypeHighLow
	for _, tc := range []struct {
		bet, coins int64
		call       bool
	}{{10, 10, true}, {10, 9, false}, {50, 1000, false}} {
		v.Room.CurrentBet = tc.bet
		c, _ = v.Controls("hero", tc.coins)
		if got := len(c.Actions) > 0 && c.Actions[0] == "call"; got != tc.call {
			t.Fatalf("bet %d coins %d: %v", tc.bet, tc.coins, c.Actions)
		}
	}

	// A mid-game entrant sits out every game and adds no Doppel mark.
	v.Members = append(v.Members, MemberObservation{CharacterID: "late", Action: "待機中"})
	v.Room.ParticipantCount = 3
	for _, game := range []GameType{GameTypeIndian, GameTypeHighLow, GameTypeDoppel} {
		v.Room.GameType = game
		if c, _ = v.Controls("late", 1000); len(c.Actions) != 0 {
			t.Fatalf("%s waiting entrant offered %v", game, c.Actions)
		}
	}
	if c, _ = v.Controls("hero", 1000); !reflect.DeepEqual(c.Actions, []string{"★", "●", "◆"}) {
		t.Fatalf("waiting entrant widened Doppel marks: %v", c.Actions)
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

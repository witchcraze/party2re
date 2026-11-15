package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestGameplayScenario_CasinoDoppelRoom plays a Doppel round between two
// accounts: both buy coins, one opens a room, the other selects and joins it,
// the leader starts, both choose marks and the round settles.
func TestGameplayScenario_CasinoDoppelRoom(t *testing.T) {
	a := newIsolatedScenarioApp(t)
	leaderToken, leader := a.register()
	childToken, child := a.register()

	// Starting gold 200G buys 10 coins at 20G each (casino.cgi ryougae).
	buyCoins := func(token, characterID string) scenarioContext {
		t.Helper()
		casino := a.enter(token, characterID, a.context(token, characterID), "casino")
		bought := a.offered(token, characterID, casino, "casino_exchange", map[string]any{"coins": 10}, http.StatusOK)
		if bought.Context.Character.Gold != 0 || bought.Context.Scene.Data.Menu.Coins != 10 {
			t.Fatalf("exchange: gold %d coins %d", bought.Context.Character.Gold, bought.Context.Scene.Data.Menu.Coins)
		}
		return bought.Context
	}
	buyCoins(leaderToken, leader)
	buyCoins(childToken, child)

	a.elapseActionWait(leader)
	lobby := a.context(leaderToken, leader)
	opened := a.offered(leaderToken, leader, lobby, "casino_room_create", map[string]any{
		"name": "s" + leader[:7], "game_type": "doppel", "speed": 18, "max_players": 2, "rate": 10, "allow_spectators": false,
	}, http.StatusOK)
	var room struct {
		RoomID string `json:"room_id"`
	}
	if err := json.Unmarshal(opened.Result, &room); err != nil || room.RoomID == "" {
		t.Fatalf("create: %s %v", opened.Result, err)
	}

	a.elapseActionWait(child)
	childLobby := a.context(childToken, child)
	if len(childLobby.Scene.Data.Rooms) != 1 || childLobby.Scene.Data.Rooms[0].ID != room.RoomID {
		t.Fatalf("lobby rooms: %+v", childLobby.Scene.Data.Rooms)
	}
	selected := a.command(childToken, child, "scene_select", childLobby.Scene.Data.Rooms[0].SelectParams, http.StatusOK).Context
	if string(selected.Scene.Data.JoinParams) != `{"room_id":"`+room.RoomID+`"}` {
		t.Fatalf("selected room: %s", selected.Scene.Data.JoinParams)
	}
	a.offered(childToken, child, selected, "casino_room_join", nil, http.StatusOK)

	// The leader's create wait still runs: starting now is rejected and changes nothing.
	waiting := a.context(leaderToken, leader)
	if waiting.Scene.Data.Casino == nil || waiting.Scene.Data.Casino.Controls.Role != "leader" {
		t.Fatalf("leader room: %+v", waiting.Scene)
	}
	rejected := a.command(leaderToken, leader, "casino_room_start", map[string]any{"room_id": room.RoomID}, http.StatusConflict)
	if rejected.Error.Code != "ACTION_WAIT" || rejected.Context.Scene.Data.Casino == nil || len(rejected.Context.Scene.Data.Casino.Controls.Actions) != 0 {
		t.Fatalf("start during wait: %+v", rejected)
	}
	a.elapseActionWait(leader)
	started := a.offered(leaderToken, leader, a.context(leaderToken, leader), "casino_room_start", nil, http.StatusOK).Context
	marks := started.Scene.Data.Casino.Controls.Actions
	if len(marks) != 3 {
		t.Fatalf("two players choose from three marks: %v", marks)
	}
	a.offered(leaderToken, leader, started, "casino_room_action", map[string]any{"action": marks[0]}, http.StatusOK)
	a.elapseActionWait(child)
	childTurn := a.context(childToken, child)
	a.offered(childToken, child, childTurn, "casino_room_action", map[string]any{"action": marks[1]}, http.StatusOK)

	// No child matched the leader's mark, so the leader takes both wagers; the
	// child, left with no coins, is ejected (casino_doppel.cgi).
	settled := a.context(leaderToken, leader)
	if settled.Scene.Data.Casino == nil || settled.Scene.Data.Casino.Coins != 20 {
		t.Fatalf("leader after settlement: %+v", settled.Scene)
	}
	if out := a.context(childToken, child); out.Scene.Data.Casino != nil || out.Scene.Data.Menu.Coins != 0 {
		t.Fatalf("child after settlement: %+v", out.Scene)
	}
	var total int64
	if err := a.db.QueryRowContext(context.Background(), `SELECT SUM(coins) FROM casino_accounts WHERE character_id IN (?, ?)`, leader, child).Scan(&total); err != nil || total != 20 {
		t.Fatalf("persisted coins %d: %v", total, err)
	}

	// Leaving as a participant costs one fatigue and disbands the empty room.
	left := a.offered(leaderToken, leader, settled, "casino_room_leave", nil, http.StatusOK).Context
	if left.Character.Tired != settled.Character.Tired+1 || left.Scene.Data.Casino != nil || len(left.Scene.Data.Rooms) != 0 {
		t.Fatalf("leave: tired %d->%d %+v", settled.Character.Tired, left.Character.Tired, left.Scene.Data)
	}
}

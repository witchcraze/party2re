package http

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func casinoRoomPlace(id string) placelog.Place {
	return placelog.Place{Kind: placeKindCasinoRoom, ID: id}
}

var casinoLobbyPlace = placelog.Place{Kind: placelog.KindFacility, ID: "casino"}

func casinoLogRouter(t *testing.T) (*casinoSceneFixture, *fakePlaceLog, *fakePlaceMembers, http.Handler) {
	t.Helper()
	f := newCasinoSceneFixture(t)
	log := &fakePlaceLog{byPlace: map[placelog.Place][]placelog.Entry{}}
	members := &fakePlaceMembers{byPlace: map[placelog.Place][]placelog.Member{}}
	router := f.router(WithPlaceLog(log), WithPlaceMembers(members), WithCharacterNames(fakeCharacterNames{"peer": "peer"}))
	return f, log, members, router
}

func say(t *testing.T, router http.Handler, text string) (int, string) {
	t.Helper()
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_say","params":{"text":"`+text+`"}}`, nil)
	if status != 200 {
		return status, actionErrorCode(t, raw)
	}
	return status, ""
}

func logTexts(rows []PlaceLogEntry) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, r.Text)
	}
	return out
}

func roomUpdatedAt(t *testing.T, f *casinoSceneFixture, id string) time.Time {
	t.Helper()
	room, err := f.rooms.GetRoom(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return room.UpdatedAt
}

func ageRoom(t *testing.T, f *casinoSceneFixture, id string, age time.Duration) {
	t.Helper()
	room, err := f.rooms.GetRoom(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	room.UpdatedAt = time.Now().UTC().Add(-age)
	if err := f.rooms.UpdateRoom(context.Background(), *room); err != nil {
		t.Fatal(err)
	}
}

// A participant speaks into its own room's log, which only that room observes.
// Speech renews the room (log mtime, casino.cgi:124-129); observation does not.
func TestCasinoRoomSpeechWritesAndObservesOnlyItsRoom(t *testing.T) {
	f, log, members, router := casinoLogRouter(t)
	f.room("mine", casino.GameTypeIndian, true)
	f.room("other", casino.GameTypeIndian, false)
	log.byPlace[casinoRoomPlace("other")] = []placelog.Entry{{SpeakerKind: placelog.SpeakerCharacter, Name: "X", Text: "other room"}}
	log.byPlace[casinoLobbyPlace] = []placelog.Entry{{SpeakerKind: placelog.SpeakerCharacter, Name: "Y", Text: "lobby"}}
	ageRoom(t, f, "mine", 20*time.Minute)
	before := roomUpdatedAt(t, f, "mine")

	if status, code := say(t, router, "＠ささやき>peer hi"); status != 200 {
		t.Fatalf("room speech: %d %s", status, code)
	}
	rows := log.byPlace[casinoRoomPlace("mine")]
	if len(rows) != 1 || rows[0].Text != "＠ささやき>peer hi" || rows[0].RecipientCharacterID != "" {
		t.Fatalf("room rows: %+v", rows)
	}
	if !roomUpdatedAt(t, f, "mine").After(before) {
		t.Fatal("room speech did not renew the room")
	}
	// Rooms keep no member list (_casino.cgi read_member), and room commands leave the lobby's alone.
	if len(members.refreshed) != 0 {
		t.Fatalf("room command refreshed a member list: %+v", members.refreshed)
	}

	renewed := roomUpdatedAt(t, f, "mine")
	status, response := navigationGET(t, router)
	if status != 200 || response.Scene.Kind != "activity" || !slices.Equal(logTexts(response.Scene.PlaceLog), []string{"＠ささやき>peer hi"}) || len(response.Scene.Members) != 0 {
		t.Fatalf("room observation: %d %+v", status, response.Scene)
	}
	if !roomUpdatedAt(t, f, "mine").Equal(renewed) {
		t.Fatal("observation renewed the room")
	}
	if !slices.ContainsFunc(response.AvailableActions, func(a ContextAction) bool { return a.Action == "place_say" }) ||
		slices.ContainsFunc(response.AvailableActions, func(a ContextAction) bool { return a.Action == "place_whisper" }) {
		t.Fatalf("room speech actions: %+v", response.AvailableActions)
	}

	// Rooms register no ＠ささやき (_casino.cgi:10-33).
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_whisper","params":{"recipient_name":"peer","text":"x"}}`, nil)
	if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_UNAVAILABLE" || len(log.byPlace[casinoRoomPlace("mine")]) != 1 {
		t.Fatalf("room whisper: %d %s", status, raw["error"])
	}
}

func TestCasinoSpectatorSpeaksIntoTheRoomAndOutsidersStayInTheLobby(t *testing.T) {
	f, log, _, router := casinoLogRouter(t)
	f.room("watched", casino.GameTypeIndian, false)

	// Outsider: lobby speech, no room rows.
	if status, code := say(t, router, "lobby"); status != 200 {
		t.Fatalf("lobby speech: %d %s", status, code)
	}
	if status, response := navigationGET(t, router); status != 200 || response.Scene.Kind == "activity" || !slices.Equal(logTexts(response.Scene.PlaceLog), []string{"lobby"}) {
		t.Fatalf("lobby observation: %d %+v", status, response.Scene)
	}

	if _, err := f.service.SpectateRoom(context.Background(), "watched", "hero", "secret"); err != nil {
		t.Fatal(err)
	}
	if status, code := say(t, router, "watching"); status != 200 {
		t.Fatalf("spectator speech: %d %s", status, code)
	}
	if rows := log.byPlace[casinoRoomPlace("watched")]; len(rows) != 1 || rows[0].Text != "watching" {
		t.Fatalf("spectator rows: %+v", rows)
	}
	if status, response := navigationGET(t, router); status != 200 || !slices.Equal(logTexts(response.Scene.PlaceLog), []string{"watching"}) {
		t.Fatalf("spectator observation: %d %+v", status, response.Scene.PlaceLog)
	}
}

// A room that ended, or one the speaker no longer belongs to, takes no row.
func TestCasinoRoomSpeechRejectsEndedRoomsAndFormerMembers(t *testing.T) {
	f, log, _, router := casinoLogRouter(t)
	f.room("gone", casino.GameTypeIndian, true)
	if status, code := say(t, router, "before"); status != 200 {
		t.Fatalf("speech: %d %s", status, code)
	}
	if err := f.rooms.DeleteRoom(context.Background(), "gone"); err != nil {
		t.Fatal(err)
	}
	// The deleted room's log is no longer reachable.
	if status, response := navigationGET(t, router); status != 200 || response.Scene.Kind == "activity" || len(response.Scene.PlaceLog) != 0 {
		t.Fatalf("after deletion: %d %+v", status, response.Scene)
	}

	// A stale activity fact (the room ended or the member left after preflight).
	f.room("left", casino.GameTypeIndian, false)
	for _, room := range []string{"gone", "left"} {
		f.facts = []playercontext.Activity{{Kind: "casino", ID: room, Role: "member", Phase: string(casino.RoomStatusWaiting), Actions: []string{"casino_room_leave", "place_say"}}}
		if status, code := say(t, router, "late"); status != http.StatusNotFound || (code != "CASINO_ROOM_NOT_FOUND" && code != "CASINO_MEMBER_NOT_FOUND") {
			t.Fatalf("%s: %d %s", room, status, code)
		}
	}
	ageRoom(t, f, "left", casino.IdleRoomTimeout+time.Minute)
	if err := f.rooms.AddMember(context.Background(), casino.RoomMember{RoomID: "left", CharacterID: "hero"}); err != nil {
		t.Fatal(err)
	}
	if status, code := say(t, router, "late"); status != http.StatusNotFound || code != "CASINO_ROOM_NOT_FOUND" {
		t.Fatalf("expired room: %d %s", status, code)
	}
	if len(log.byPlace[casinoRoomPlace("gone")]) != 1 || len(log.byPlace[casinoRoomPlace("left")]) != 0 {
		t.Fatalf("rejected speech wrote: %+v", log.byPlace)
	}
}

// Room entry leaves the lobby member list (casino.cgi:376,469,513).
func TestCasinoRoomEntryLeavesTheLobbyMembers(t *testing.T) {
	f, _, members, router := casinoLogRouter(t)
	f.room("open", casino.GameTypeIndian, false)
	f.store.selection = playercontext.Selection{Destination: "casino", Subject: playercontext.Subject{Kind: "room", ID: "open"}}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"casino_room_join","params":{"room_id":"open","password":"secret"}}`, nil)
	if status != 200 {
		t.Fatalf("join: %d %s", status, raw["error"])
	}
	if !slices.Equal(members.refreshed, []placelog.Place{casinoLobbyPlace}) || !slices.Equal(members.left, []string{"hero"}) || len(members.byPlace[casinoLobbyPlace]) != 0 {
		t.Fatalf("lobby members after entry: refreshed %+v left %+v rows %+v", members.refreshed, members.left, members.byPlace)
	}
}

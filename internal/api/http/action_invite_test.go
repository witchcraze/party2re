package http

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

var questLobbyPlace = placelog.Place{Kind: placelog.KindFacility, ID: "quest"}

func invite(t *testing.T, router http.Handler, action, params string) (int, map[string]json.RawMessage) {
	t.Helper()
	return gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+action+`","params":`+params+`}`, nil)
}

func inviteCandidates(t *testing.T, raw map[string]json.RawMessage) InviteCandidates {
	t.Helper()
	var c InviteCandidates
	if err := json.Unmarshal(raw["result"], &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// pvpInviteRouter has a pre-start PvP activity and no PvP service, so a
// successful invitation cannot have touched the room (_skill.cgi:769-779).
func pvpInviteRouter(t *testing.T, round int) (*fakePlaceLog, *fakePlaceMembers, http.Handler) {
	t.Helper()
	f := newBankGatewayFixture(t)
	timers := timer.NewService(nil)
	actions := []string{"pvp_leave"}
	if round == 0 {
		actions = append(actions, "pvp_invite")
	}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timers,
		playercontext.WithActionWait(timers, playercontext.DefaultActionWait),
		playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) {
			return []playercontext.Activity{{Kind: "pvp", ID: "ROOM", Phase: "recruiting", Round: round, Role: "member", Actions: actions}}, nil
		}))
	log := &fakePlaceLog{byPlace: map[placelog.Place][]placelog.Entry{}}
	members := &fakePlaceMembers{byPlace: map[placelog.Place][]placelog.Member{questLobbyPlace: {{CharacterID: "bob", Name: "Bob", Color: "#123456"}}}}
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithPlayerContext(pc), WithPlaceLog(log), WithPlaceMembers(members), WithCharacterNames(fakeCharacterNames{"Bob": "bob", "Carol": "carol"}))
	if err != nil {
		t.Fatal(err)
	}
	return log, members, h.Router()
}

func TestPvPInviteListsTheQuestLobbyAndWhispersIntoItsLog(t *testing.T) {
	log, _, router := pvpInviteRouter(t, 0)

	status, raw := invite(t, router, "pvp_invite", `{"room_id":"ROOM"}`)
	if c := inviteCandidates(t, raw); status != 200 || c.Message != "誰をさそいますか？" || len(c.Candidates) != 1 || c.Candidates[0] != (PlaceMember{Kind: "character", Name: "Bob", Color: "#123456"}) || len(log.byPlace) != 0 {
		t.Fatalf("listing: %d %s %+v", status, raw["error"], log.byPlace)
	}

	// Any existing character may be invited, present in the lobby or not.
	status, raw = invite(t, router, "pvp_invite", `{"room_id":"ROOM","recipient_name":"Carol","text":"come"}`)
	if status != 200 || waitMS(t, raw) != 0 || speechResult(t, raw) != (SpeechResult{Text: "＠さそう>Carol come", Private: true}) {
		t.Fatalf("invite: %d %s %s", status, raw["result"], raw["error"])
	}
	rows := log.byPlace[questLobbyPlace]
	if len(log.byPlace) != 1 || len(rows) != 1 || rows[0].RecipientCharacterID != "carol" || rows[0].SpeakerCharacterID != "hero" {
		t.Fatalf("quest lobby rows: %+v", log.byPlace)
	}

	status, raw = invite(t, router, "pvp_invite", `{"room_id":"OTHER","recipient_name":"Bob"}`)
	if status != http.StatusForbidden || actionErrorCode(t, raw) != "PVP_NOT_IN_ROOM" {
		t.Fatalf("stale room: %d %s", status, raw["error"])
	}
	status, raw = invite(t, router, "pvp_invite", `{"room_id":"ROOM","recipient_name":"Nobody"}`)
	if status != http.StatusNotFound || actionErrorCode(t, raw) != "RECIPIENT_NOT_FOUND" || len(log.byPlace[questLobbyPlace]) != 1 {
		t.Fatalf("unknown recipient: %d %s", status, raw["error"])
	}
}

func TestPvPInviteIsUnavailableAfterTheMatchStarts(t *testing.T) {
	log, _, router := pvpInviteRouter(t, 1)
	status, raw := invite(t, router, "pvp_invite", `{"room_id":"ROOM","recipient_name":"Bob"}`)
	if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_UNAVAILABLE" || len(log.byPlace) != 0 {
		t.Fatalf("started room: %d %s", status, raw["error"])
	}
}

// The casino invitation writes the lobby log, not the room's, and does not
// renew the room (_casino.cgi:122-133).
func TestCasinoRoomInviteWhispersIntoTheLobbyWithoutRenewingTheRoom(t *testing.T) {
	f, log, members, router := casinoLogRouter(t)
	members.byPlace[casinoLobbyPlace] = []placelog.Member{{CharacterID: "peer", Name: "peer"}}

	// Outside a room the invitation is not offered.
	status, raw := invite(t, router, "casino_room_invite", `{"room_id":"mine"}`)
	if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_UNAVAILABLE" {
		t.Fatalf("outsider: %d %s", status, raw["error"])
	}

	f.room("mine", casino.GameTypeIndian, true)
	ageRoom(t, f, "mine", 20*time.Minute)
	before := roomUpdatedAt(t, f, "mine")
	status, raw = invite(t, router, "casino_room_invite", `{"room_id":"mine"}`)
	// The fixture seats the actor without room entry, so its lobby entry from the outsider request remains.
	if c := inviteCandidates(t, raw); status != 200 || len(c.Candidates) != 2 || c.Candidates[0].Name != "peer" {
		t.Fatalf("listing: %d %s", status, raw["result"])
	}
	status, raw = invite(t, router, "casino_room_invite", `{"room_id":"mine","recipient_name":"peer","text":"play"}`)
	if status != 200 || waitMS(t, raw) != 0 || speechResult(t, raw).Text != "＠さそう>peer play" {
		t.Fatalf("invite: %d %s %s", status, raw["result"], raw["error"])
	}
	if rows := log.byPlace[casinoLobbyPlace]; len(rows) != 1 || rows[0].RecipientCharacterID != "peer" || len(log.byPlace[casinoRoomPlace("mine")]) != 0 {
		t.Fatalf("rows: %+v", log.byPlace)
	}
	if !roomUpdatedAt(t, f, "mine").Equal(before) {
		t.Fatal("invitation renewed the room")
	}
	status, raw = invite(t, router, "casino_room_invite", `{"room_id":"other","recipient_name":"peer"}`)
	if status == 200 || len(log.byPlace[casinoLobbyPlace]) != 1 {
		t.Fatalf("stale room: %d %s", status, raw["error"])
	}
}

package http

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestHomePlaceLogAndMembersAreScopedToObservedOwner(t *testing.T) {
	log := &fakePlaceLog{byPlace: map[placelog.Place][]placelog.Entry{}}
	members := &fakePlaceMembers{byPlace: map[placelog.Place][]placelog.Member{}}
	options := []Option{WithPlaceLog(log), WithPlaceMembers(members), WithCharacterNames(fakeCharacterNames{"Hero": "hero"})}
	f, store, router := homeSceneRouter(t, options...)
	f.char.Color = "#123456"
	f.pets = map[string][]home.HomePet{
		"hero":  {{CustomName: "ポチ"}, {MonsterID: "スライム"}},
		"other": {{CustomName: "タマ"}},
	}
	f.talk = map[string]home.CompanionTalkResult{"hero": {PetName: "ポチ", Phrase: "ワン"}, "other": {PetName: "タマ", Phrase: "ニャー"}}
	command := func(action string) {
		t.Helper()
		if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", action, nil); status != 200 {
			t.Fatalf("%s: %d %s", action, status, raw)
		}
	}
	command(`{"action":"place_say","params":{"text":"home A"}}`)
	command(`{"action":"npc_talk"}`)
	status, owner := navigationGET(t, router)
	if status != 200 || len(owner.Scene.PlaceLog) != 3 {
		t.Fatalf("owner log: %d %+v", status, owner.Scene.PlaceLog)
	}
	wantMembers := []PlaceMember{
		{Kind: "npc", Name: "ポチ", Color: npcColor},
		{Kind: "npc", Name: "スライム", Color: npcColor},
		{Kind: "character", Name: "Hero", Color: "#123456", IconURL: "avatar"},
	}
	if !slices.Equal(owner.Scene.Members, wantMembers) {
		t.Fatalf("owner members: %+v", owner.Scene.Members)
	}
	for _, action := range []string{"place_say", "place_whisper", "npc_talk"} {
		if !hasHomeAction(owner, action) {
			t.Fatalf("home lacks %s", action)
		}
	}

	// A second owned character visits A. Public rows and residents are shared;
	// whisper visibility still depends on the viewer, including the homeowner.
	guest, guestStore, guestRouter := homeSceneRouter(t, options...)
	guest.char.ID, guest.char.Name = "guest", "Guest"
	guest.pets = f.pets
	guestStore.selection.Subject = playercontext.Subject{Kind: "home", ID: "hero"}
	if status, visitor := navigationGETForActor(t, guestRouter, "guest"); status != 200 || decodeHomeData(t, visitor).View.IsOwner || !reflect.DeepEqual(visitor.Scene.PlaceLog, owner.Scene.PlaceLog) || !slices.Equal(visitor.Scene.Members, wantMembers) {
		t.Fatalf("visitor: %d %+v", status, visitor.Scene)
	}
	if status, raw := gatewayRequest(t, guestRouter, "guest", "session", "application/json", `{"action":"place_whisper","params":{"recipient_name":"Hero","text":"private in A"}}`, nil); status != 200 {
		t.Fatalf("visitor whisper: %d %s", status, raw)
	}
	if status, owner = navigationGET(t, router); status != 200 || len(owner.Scene.PlaceLog) != 4 || !owner.Scene.PlaceLog[0].Private {
		t.Fatalf("homeowner whisper: %d %+v", status, owner.Scene.PlaceLog)
	}
	if status, visitor := navigationGETForActor(t, guestRouter, "guest"); status != 200 || !reflect.DeepEqual(visitor.Scene.PlaceLog, owner.Scene.PlaceLog) {
		t.Fatalf("whisper sender: %d %+v", status, visitor.Scene.PlaceLog)
	}
	guest.char.ID = "bystander"
	if status, bystander := navigationGETForActor(t, guestRouter, "bystander"); status != 200 || len(bystander.Scene.PlaceLog) != 3 || slices.ContainsFunc(bystander.Scene.PlaceLog, func(row PlaceLogEntry) bool { return row.Private }) {
		t.Fatalf("other visitor saw whisper: %d %+v", status, bystander.Scene.PlaceLog)
	}
	guest.char.ID = "guest"

	// Switching to B must not reuse A's log, residents or live characters.
	store.selection.Subject = playercontext.Subject{Kind: "home", ID: "other"}
	status, other := navigationGET(t, router)
	if status != 200 || len(other.Scene.PlaceLog) != 0 || !slices.Equal(other.Scene.Members, []PlaceMember{{Kind: "npc", Name: "タマ", Color: npcColor}}) {
		t.Fatalf("other home: %d %+v", status, other.Scene)
	}
	command(`{"action":"place_say","params":{"text":"home B"}}`)
	command(`{"action":"place_whisper","params":{"recipient_name":"Hero","text":"private in B"}}`)
	command(`{"action":"npc_talk"}`)
	if status, other = navigationGET(t, router); status != 200 || len(other.Scene.PlaceLog) != 4 || other.Scene.PlaceLog[0].Name != "タマ" || other.Scene.PlaceLog[0].Text != "ニャー" || len(other.Scene.Members) != 2 || other.Scene.Members[1].Name != "Hero" {
		t.Fatalf("B after commands: %d %+v", status, other.Scene)
	}
	if status, visitor := navigationGETForActor(t, guestRouter, "guest"); status != 200 || !reflect.DeepEqual(visitor.Scene.PlaceLog, owner.Scene.PlaceLog) || len(visitor.Scene.Members) != 3 || visitor.Scene.Members[2].Name != "Guest" {
		t.Fatalf("A after B commands: %d %+v", status, visitor.Scene)
	}
	if !slices.Contains(members.refreshed, placelog.Place{Kind: "home", ID: "hero"}) || !slices.Contains(members.refreshed, placelog.Place{Kind: "home", ID: "other"}) {
		t.Fatalf("membership identities: %+v", members.refreshed)
	}

	_, _, _, bankRouter := bankSceneRouter(t, options...)
	if status, bank := navigationGET(t, bankRouter); status != 200 || len(bank.Scene.PlaceLog) != 0 || len(bank.Scene.Members) != 1 {
		t.Fatalf("fixed facility: %d %+v", status, bank.Scene)
	}
	// Repeated observations must not append rows, refresh/leave members or save navigation.
	writes, refreshes, leaves, selections := log.writes, len(members.refreshed), len(members.left), store.writes
	for range 2 {
		if status, _ := navigationGET(t, router); status != 200 {
			t.Fatalf("GET: %d", status)
		}
	}
	if log.writes != writes || len(members.refreshed) != refreshes || len(members.left) != leaves || store.writes != selections {
		t.Fatal("GET wrote place state")
	}
}

func TestHomeTalkLogOmitsEmptyReplies(t *testing.T) {
	log := &fakePlaceLog{}
	f, _, router := homeSceneRouter(t, WithPlaceLog(log))
	talk := func() {
		t.Helper()
		if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk"}`, nil); status != 200 {
			t.Fatalf("talk: %d %s", status, raw)
		}
	}
	talk()
	if len(log.rows) != 0 {
		t.Fatalf("nobody home: %+v", log.rows)
	}
	f.talk = map[string]home.CompanionTalkResult{"hero": {PetName: "ポチ"}}
	talk()
	if len(log.rows) != 1 || log.rows[0].SpeakerKind != placelog.SpeakerCharacter || log.rows[0].Text != "＠はなす" {
		t.Fatalf("silent resident: %+v", log.rows)
	}
	f.talk["hero"] = home.CompanionTalkResult{PetName: "ポチ", Phrase: "ワン"}
	talk()
	if len(log.rows) != 3 || log.rows[0].SpeakerKind != placelog.SpeakerNPC || log.rows[0].Name != "ポチ" || log.rows[0].Color != npcColor || log.rows[0].Text != "ワン" || log.rows[1].Text != "＠はなす" {
		t.Fatalf("resident reply: %+v", log.rows)
	}
}

func TestHomeSelectionLeavesOnlyWhenPlaceChanges(t *testing.T) {
	for _, tc := range []struct {
		target                   string
		status, leaves, retained int
	}{
		{"hero", 200, 0, 1},
		{"other", 200, 1, 0},
		{"missing", 404, 0, 1},
	} {
		t.Run(tc.target, func(t *testing.T) {
			members := &fakePlaceMembers{byPlace: map[placelog.Place][]placelog.Member{}}
			_, _, router := homeSceneRouter(t, WithPlaceMembers(members))
			status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"home","target_id":"`+tc.target+`"}}`, nil)
			if status != tc.status || len(members.left) != tc.leaves || len(members.byPlace[placelog.Place{Kind: "home", ID: "hero"}]) != tc.retained {
				t.Fatalf("selection: %d, leaves %v, members %v", status, members.left, members.byPlace)
			}
		})
	}
}

func TestHomeSelectionKeepsAcceptedOutcomeOnMemberLeaveFailure(t *testing.T) {
	members := &fakePlaceMembers{leaveErr: errors.New("member cleanup unavailable")}
	_, store, router := homeSceneRouter(t, WithPlaceMembers(members))
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"home","target_id":"other"}}`, nil)
	if status != 200 || store.selection.Subject.ID != "other" || len(members.left) != 1 || homeObservation(t, raw).Scene.Subject.ID != "other" {
		t.Fatalf("selection after cleanup failure: %d %s", status, raw)
	}
}

func TestHomeObservationFailsOnPlaceStoreErrors(t *testing.T) {
	for _, visitor := range []bool{false, true} {
		for _, failed := range []string{"log", "members", "refresh"} {
			t.Run(fmt.Sprintf("%s/visitor=%t", failed, visitor), func(t *testing.T) {
				log, members := &fakePlaceLog{}, &fakePlaceMembers{}
				_, store, router := homeSceneRouter(t, WithPlaceLog(log), WithPlaceMembers(members))
				if visitor {
					store.selection.Subject = playercontext.Subject{Kind: "home", ID: "other"}
				}
				err := errors.New("place store unavailable")
				switch failed {
				case "log":
					log.err = err
				case "members":
					members.readErr = err
				case "refresh":
					members.writeErr = err
				}
				if failed == "refresh" {
					status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"place_say","params":{"text":"not written"}}`, nil)
					if status != 500 || actionErrorCode(t, raw) != "ACTION_PREFLIGHT_FAILED" || len(log.rows) != 0 {
						t.Fatalf("refresh failure: %d %s", status, raw)
					}
				} else if status, _ := navigationGET(t, router); status != 500 {
					t.Fatalf("failed observation: %d", status)
				}
			})
		}
	}
}

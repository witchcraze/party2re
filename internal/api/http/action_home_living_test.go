package http

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/home"
)

func (f *homeSceneFixture) record(call string) error {
	f.living = append(f.living, call)
	return f.livingErr
}
func (f *homeSceneFixture) SetCharacterColor(_ context.Context, actor, color string) error {
	return f.record("color " + actor + " " + color)
}
func (f *homeSceneFixture) UseHomeItem(_ context.Context, actor, instance, source string) (*home.UseHomeItemResult, error) {
	return &home.UseHomeItemResult{Action: "consumed", Consumed: true}, f.record("use " + actor + " " + instance + "/" + source)
}
func (f *homeSceneFixture) TeachCompanionPhrase(_ context.Context, actor, phrase string) (home.CompanionPhrase, error) {
	return home.CompanionPhrase{ID: "p1", Phrase: phrase}, f.record("teach " + actor + " " + phrase)
}
func (f *homeSceneFixture) ForgetCompanionPhrase(_ context.Context, phraseID, actor string) error {
	return f.record("forget " + actor + " " + phraseID)
}
func (f *homeSceneFixture) ClearDeliveryNotices(_ context.Context, actor string, ids []string) error {
	return f.record("ack " + actor + " " + strings.Join(ids, ","))
}
func (f *homeSceneFixture) TalkToCompanion(_ context.Context, owner string) (home.CompanionTalkResult, error) {
	talk, ok := f.talk[owner]
	if !ok {
		return home.CompanionTalkResult{}, home.ErrNoPetsAtHome
	}
	return talk, nil
}

var homeLivingCommands = []string{"home_color", "home_item_use", "home_phrase_teach", "home_phrase_forget", "home_notice_ack"}

func TestHomeOwnerLivingCommandsAndVisitorBoundary(t *testing.T) {
	f, _, router := homeSceneRouter(t)
	f.repo.phrases = []home.CompanionPhrase{{ID: "p1", CharacterID: "hero", Phrase: "クエッ"}}
	f.repo.notices = []home.DeliveryNotice{{ID: "n1", CharacterID: "hero", Message: "届きました"}}

	status, response := navigationGET(t, router)
	data := decodeHomeData(t, response)
	if status != 200 || data.View.Private == nil || len(data.View.Private.Phrases) != 1 || len(data.View.Private.Notices) != 1 || data.View.Private.Items == nil || len(data.ColorSamples) != 11 {
		t.Fatalf("owned observation: %d %+v", status, data)
	}
	for _, id := range append([]string{"home_sleep", "npc_talk"}, homeLivingCommands...) {
		if !hasHomeAction(response, id) {
			t.Fatalf("owner lacks %s", id)
		}
	}
	// Observing never acknowledges notices or runs a command.
	if len(f.living) != 0 || f.executions != 0 {
		t.Fatalf("GET mutated: %v", f.living)
	}

	for _, tc := range []struct{ action, params, call string }{
		{"home_color", `{"color":"#FF3333"}`, "color hero #FF3333"},
		{"home_item_use", `{"instance_id":"i1","source":"depot"}`, "use hero i1/depot"},
		{"home_phrase_teach", `{"phrase":"こんにちは"}`, "teach hero こんにちは"},
		{"home_phrase_forget", `{"phrase_id":"p1"}`, "forget hero p1"},
		{"home_notice_ack", `{"notice_ids":["n1"]}`, "ack hero n1"},
	} {
		f.living = nil
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+tc.action+`","params":`+tc.params+`}`, nil)
		if status != 200 || !slices.Equal(f.living, []string{tc.call}) {
			t.Fatalf("%s: %d %s %v", tc.action, status, raw, f.living)
		}
	}
	f.living, f.livingErr = nil, corecharacter.ErrInvalidColor
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"home_color","params":{"color":"red"}}`, nil)
	if status != 400 {
		t.Fatalf("invalid colour: %d %s", status, raw)
	}
	assertGatewayError(t, raw, "HOME_INVALID_COLOR")
	f.livingErr = nil

	// A visitor keeps Sleep and talk but sees and controls none of the owner's facts.
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"home","target_id":"other"}}`, nil); status != 200 {
		t.Fatalf("select: %d %s", status, raw)
	}
	status, response = navigationGET(t, router)
	data = decodeHomeData(t, response)
	if status != 200 || data.View.Private != nil || len(data.ColorSamples) != 0 || !hasHomeAction(response, "home_sleep") || !hasHomeAction(response, "npc_talk") {
		t.Fatalf("visitor observation: %+v", response)
	}
	f.living = nil
	for _, id := range homeLivingCommands {
		if hasHomeAction(response, id) {
			t.Fatalf("visitor offered %s", id)
		}
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+id+`","params":{"color":"#FFFFFF","instance_id":"i1","phrase":"x","phrase_id":"p1","notice_ids":["n1"]}}`, nil)
		if status == 200 {
			t.Fatalf("visitor executed %s: %s", id, raw)
		}
	}
	if len(f.living) != 0 {
		t.Fatalf("visitor reached the service: %v", f.living)
	}

	// Selecting one's own home is the owned home again.
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_back"}`, nil); status != 200 {
		t.Fatalf("back: %d %s", status, raw)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"home","target_id":"hero"}}`, nil); status != 200 {
		t.Fatalf("select own: %d %s", status, raw)
	}
	if _, response = navigationGET(t, router); !hasHomeAction(response, "home_notice_ack") {
		t.Fatalf("own subject lost owner commands: %+v", response.AvailableActions)
	}
}

func TestHomeTalkSpeaksForTheObservedHome(t *testing.T) {
	f, _, router := homeSceneRouter(t)
	talk := func() NPCResult {
		t.Helper()
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk"}`, nil)
		if status != 200 {
			t.Fatalf("talk: %d %s", status, raw)
		}
		var result NPCResult
		if err := json.Unmarshal(raw["result"], &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if got := talk(); got.Dialogue != homeNobodyHere || !got.Private {
		t.Fatalf("empty home: %+v", got)
	}
	f.talk = map[string]home.CompanionTalkResult{"hero": {PetName: "ポチ"}, "other": {PetName: "タマ", Phrase: "ニャー"}}
	if got := talk(); got != (NPCResult{}) {
		t.Fatalf("a resident without phrases stays silent: %+v", got)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"home","target_id":"other"}}`, nil); status != 200 {
		t.Fatalf("select: %d %s", status, raw)
	}
	if got := talk(); got.NPCName != "タマ" || got.Dialogue != "ニャー" || got.Private {
		t.Fatalf("visited home: %+v", got)
	}
}

func TestHomeOwnedObservationFailsOnRequiredRead(t *testing.T) {
	f, _, router := homeSceneRouter(t)
	f.repo.privateErr = errors.New("phrase store down")
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatalf("partial owned observation: %d", status)
	}
}

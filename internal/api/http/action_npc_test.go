package http

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/bank"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/secretshop"
)

func npcResult(t *testing.T, raw map[string]json.RawMessage) NPCResult {
	t.Helper()
	var r NPCResult
	if err := json.Unmarshal(raw["result"], &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNPCTalkLogsActorAndReplyInTheCurrentFacility(t *testing.T) {
	log := &fakePlaceLog{}
	f, store, _, router := bankSceneRouter(t, WithPlaceLog(log))
	f.char.Name, f.char.Color = "Hero", "#00ff00"
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{}}`, nil)
	if status != 200 {
		t.Fatalf("npc_talk: %d %s", status, raw["error"])
	}
	r := npcResult(t, raw)
	if r.NPCName != bank.NPCName || r.Private || !slices.Contains(bank.NPCDialogues, r.Dialogue) {
		t.Fatalf("talk result: %+v", r)
	}
	want := []placelog.Entry{
		{SpeakerKind: placelog.SpeakerNPC, Name: bank.NPCName, Color: npcColor, Text: r.Dialogue},
		{SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: "hero", Name: "Hero", Color: "#00ff00", Text: "＠はなす"},
	}
	// places records the append and the refresh read.
	if len(log.rows) != 2 || len(log.places) == 0 || log.places[0] != (placelog.Place{Kind: placelog.KindFacility, ID: "bank"}) {
		t.Fatalf("log: %+v at %+v", log.rows, log.places)
	}
	for i := range want {
		got := log.rows[i]
		got.OccurredAt = want[i].OccurredAt
		if got != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, got, want[i])
		}
	}
	if store.writes != 0 || f.executions != 0 {
		t.Fatalf("talk changed navigation or Bank state: writes %d executions %d", store.writes, f.executions)
	}
	// The refreshed observation shows the new rows.
	var observation PlayerContextResponse
	if err := json.Unmarshal(raw["context"], &observation); err != nil || len(observation.Scene.PlaceLog) != 2 || observation.Scene.PlaceLog[0].Text != r.Dialogue {
		t.Fatalf("refreshed place_log: %+v %v", observation.Scene.PlaceLog, err)
	}
}

func TestNPCInspectIsPrivateAndUsesTheLegacyDefault(t *testing.T) {
	log := &fakePlaceLog{}
	_, _, _, router := bankSceneRouter(t, WithPlaceLog(log))
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_inspect","params":{}}`, nil)
	if r := npcResult(t, raw); status != 200 || r != (NPCResult{NPCName: bank.NPCName, Dialogue: npcInspectDefault, Private: true}) {
		t.Fatalf("npc_inspect: %d %+v", status, r)
	}
	if len(log.rows) != 0 {
		t.Fatalf("inspect wrote the log: %+v", log.rows)
	}
}

func TestNPCCommandsRejectedOutsideFacilitiesAndWithParams(t *testing.T) {
	log := &fakePlaceLog{}
	_, store, _, router := bankSceneRouter(t, WithPlaceLog(log))
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{"npc":"x"}}`, nil); status != http.StatusBadRequest {
		t.Fatalf("extra params: %d %s", status, raw["error"])
	}
	store.selection = playercontext.Selection{Destination: "town"}
	for _, action := range []string{"npc_talk", "npc_inspect"} {
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+action+`","params":{}}`, nil)
		if status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_UNAVAILABLE" {
			t.Fatalf("%s in town: %d %s", action, status, raw["error"])
		}
	}
	if len(log.rows) != 0 {
		t.Fatalf("rejected NPC commands wrote the log: %+v", log.rows)
	}
}

func TestNPCTalkStartsTheActionWaitButInspectDoesNot(t *testing.T) {
	_, _, router := actionWaitRouter(t, "bank")
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_inspect","params":{}}`, nil); status != 200 || waitMS(t, raw) != 0 {
		t.Fatalf("inspect: %d %s", status, raw["error"])
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{}}`, nil); status != 200 || waitMS(t, raw) <= 0 {
		t.Fatalf("talk: %d %s", status, raw["error"])
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_inspect","params":{}}`, nil); status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
		t.Fatalf("inspect during the wait: %d %s", status, raw["error"])
	}
}

func TestEveryNPCProfileHasLegacyFacts(t *testing.T) {
	for id, p := range npcProfiles {
		if p.name == "" || len(p.words) == 0 {
			t.Errorf("%s: incomplete NPC profile %+v", id, p)
		}
	}
}

func TestNPCRequiresTheFacilityQualification(t *testing.T) {
	f, store, _, router := secretShopSceneRouter(t)
	store.selection = playercontext.Selection{Destination: "secretshop"}
	f.char.JobLevel = 7
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_inspect","params":{}}`, nil)
	if r := npcResult(t, raw); status != 200 || r.NPCName != secretshop.NPCName || r.Dialogue != secretshop.InspectDialogue || !r.Private {
		t.Fatalf("qualified inspect: %d %+v", status, r)
	}
	f.char.JobLevel = 6
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{}}`, nil)
	if status != http.StatusConflict || actionErrorCode(t, raw) != "NPC_UNAVAILABLE" {
		t.Fatalf("unqualified talk: %d %s", status, raw["error"])
	}
}

package http

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/bank"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/park"
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
	// ＠うらない belongs to the park (the town hub) only.
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"park_divinate","params":{}}`, nil); status != http.StatusConflict || actionErrorCode(t, raw) != "NPC_UNAVAILABLE" {
		t.Fatalf("park_divinate in bank: %d %s", status, raw["error"])
	}
	store.selection = playercontext.Selection{Destination: "home"}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{}}`, nil); status == 200 {
		t.Fatalf("npc_talk outside a registered scene: %d %s", status, raw["result"])
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
		if p.name == "" || (len(p.words) == 0) == (p.wordsOf == nil) {
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

func TestSecretShopPuffPuffLogsAPrivateRowPairForTheActor(t *testing.T) {
	log := &fakePlaceLog{}
	f, store, _, router := secretShopSceneRouterWith(t, []Option{WithPlaceLog(log)})
	store.selection = playercontext.Selection{Destination: "secretshop"}
	f.char.JobLevel, f.char.Name, f.char.Color = 7, "Hero", "#00ff00"
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"secretshop_puffpuff"}`, nil)
	word := "パフパフ♥ パフパフ♥ パフパフ♥ ……… どうだ Hero わしのパフパフは気持ちいいだろう"
	if r := npcResult(t, raw); status != 200 || r != (NPCResult{NPCName: secretshop.NPCName, Dialogue: word}) {
		t.Fatalf("puffpuff: %d %+v %s", status, r, raw["error"])
	}
	want := []placelog.Entry{
		{SpeakerKind: placelog.SpeakerNPC, Name: secretshop.NPCName, Color: npcColor, Text: word, RecipientCharacterID: "hero"},
		{SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: "hero", Name: "Hero", Color: "#00ff00", Text: "＠ぱふぱふ", RecipientCharacterID: "hero"},
	}
	if len(log.rows) != 2 || log.places[0] != (placelog.Place{Kind: placelog.KindFacility, ID: "secretshop"}) {
		t.Fatalf("log: %+v at %+v", log.rows, log.places)
	}
	for i := range want {
		got := log.rows[i]
		got.OccurredAt = want[i].OccurredAt
		if got != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, got, want[i])
		}
	}
	if store.writes != 0 || f.executions != 0 || f.writes != 0 {
		t.Fatalf("puffpuff changed state: navigation %d executions %d writes %d", store.writes, f.executions, f.writes)
	}
}

func TestSecretShopPuffPuffOnlyInTheQualifiedSecretShop(t *testing.T) {
	log := &fakePlaceLog{}
	f, store, _, router := secretShopSceneRouterWith(t, []Option{WithPlaceLog(log)})
	f.char.JobLevel = 7
	for _, tc := range []struct {
		destination string
		jobLevel    int
		code        string
	}{{"town", 7, "NPC_UNAVAILABLE"}, {"secretshop", 6, "NPC_UNAVAILABLE"}} {
		store.selection, f.char.JobLevel = playercontext.Selection{Destination: tc.destination}, tc.jobLevel
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"secretshop_puffpuff"}`, nil)
		if status != http.StatusConflict || actionErrorCode(t, raw) != tc.code {
			t.Fatalf("%s job %d: %d %s", tc.destination, tc.jobLevel, status, raw["error"])
		}
	}
	store.selection, f.char.JobLevel = playercontext.Selection{Destination: "secretshop"}, 7
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"secretshop_puffpuff","params":{"target":"x"}}`, nil); status != http.StatusBadRequest {
		t.Fatalf("extra params: %d %s", status, raw["error"])
	}
	if len(log.rows) != 0 {
		t.Fatalf("rejected puffpuff wrote the log: %+v", log.rows)
	}
}

func TestSecretShopPuffPuffStartsTheActionWait(t *testing.T) {
	def, ok := playercontext.GetAction("secretshop_puffpuff")
	if !ok || !playercontext.StartsActionWait(def) || def.Category != "npc" {
		t.Fatalf("puffpuff must be a wait-starting NPC act: %+v", def)
	}
}

type townJobs struct{ JobService }

func (townJobs) ListDefinitions() []corejob.Definition {
	return []corejob.Definition{{ID: "warrior", Name: "戦士"}}
}

// The town hub is the legacy default place, lib/park.cgi with @町娘.
func TestTownHubIsTheLegacyPark(t *testing.T) {
	log := &fakePlaceLog{}
	f, store, _, router := bankSceneRouter(t, WithPlaceLog(log), WithJob(townJobs{}))
	store.selection = playercontext.Selection{Destination: "town"}
	f.char.Name, f.char.JobID = "Hero", "warrior"

	status, observation := navigationGET(t, router)
	if status != 200 || observation.Scene.Title != "交流広場" || observation.Scene.Speaker == nil || observation.Scene.Speaker.Name != park.NPCName ||
		len(observation.Scene.Members) == 0 || observation.Scene.Members[0] != (PlaceMember{Kind: "npc", Name: park.NPCName, Color: npcColor}) {
		t.Fatalf("town scene: %d %+v", status, observation.Scene)
	}
	for _, action := range []string{"npc_talk", "npc_inspect", "park_divinate", "place_say"} {
		if !slices.ContainsFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == action }) {
			t.Fatalf("town lacks %s: %+v", action, observation.AvailableActions)
		}
	}

	words := map[string]bool{}
	for _, w := range park.Words {
		words[strings.NewReplacer("{name}", "Hero", "{job}", "戦士").Replace(w)] = true
	}
	jobLine := "Heroさんの職業は戦士ですね？どうですか？当たりですか？"
	for i := 0; i < 300 && !slices.ContainsFunc(log.rows, func(e placelog.Entry) bool { return e.Text == jobLine }); i++ {
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk"}`, nil)
		if r := npcResult(t, raw); status != 200 || r.NPCName != park.NPCName || r.Private || !words[r.Dialogue] {
			t.Fatalf("talk: %d %+v", status, r)
		}
	}
	if !slices.ContainsFunc(log.rows, func(e placelog.Entry) bool { return e.Text == jobLine }) {
		t.Fatal("the job line never named the actor's job")
	}

	// しらべる>@町娘 answers through ＠はなす, so it is logged publicly.
	log.rows = nil
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_inspect"}`, nil)
	if r := npcResult(t, raw); status != 200 || r.Private || !slices.Contains(park.InspectWords, r.Dialogue) ||
		len(log.rows) != 2 || log.rows[1].Text != "＠しらべる>@町娘" || log.rows[1].RecipientCharacterID != "" || log.rows[0].Text != r.Dialogue {
		t.Fatalf("inspect: %d %+v %+v", status, r, log.rows)
	}

	log.rows = nil
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"park_divinate"}`, nil)
	r := npcResult(t, raw)
	if status != 200 || r.NPCName != park.NPCName || r.Private || !strings.HasPrefix(r.Dialogue, "今日のHeroさんの運勢はずばり…") ||
		len(log.rows) != 2 || log.rows[1].Text != "＠うらない" || log.rows[0] != (placelog.Entry{OccurredAt: log.rows[0].OccurredAt, SpeakerKind: placelog.SpeakerNPC, Name: park.NPCName, Color: npcColor, Text: r.Dialogue}) {
		t.Fatalf("divinate: %d %+v %+v", status, r, log.rows)
	}
}

func TestParkDivinationAndLoggedInspectStartTheActionWait(t *testing.T) {
	for _, action := range []string{"park_divinate", "npc_inspect"} {
		_, _, router := actionWaitRouter(t, "town")
		if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+action+`"}`, nil); status != 200 || waitMS(t, raw) <= 0 {
			t.Fatalf("%s: %d %s", action, status, raw["error"])
		}
		if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk"}`, nil); status != http.StatusConflict || actionErrorCode(t, raw) != "ACTION_WAIT" {
			t.Fatalf("talk after %s: %d %s", action, status, raw["error"])
		}
	}
}

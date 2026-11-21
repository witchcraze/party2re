package http

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func homeProfileRouter(t *testing.T) (*homeSceneFixture, *gatewayNavigationStore, func(string, string) (int, map[string]json.RawMessage), func() (int, PlayerContextResponse)) {
	f, store, router := homeSceneRouter(t)
	f.bios = map[string]map[string]string{"hero": {"hobby": "釣り"}, "other": {"hobby": "昼寝", "message": "よろしく"}}
	f.chronicles = map[string]adventure.AdventureChronicle{"other": {Stages: []adventure.StageClearStat{
		{StageID: "stage-01", StageName: "平原", ClearCount: 3, TotalAttempts: 5},
		{StageID: "stage-02", StageName: "森", TotalAttempts: 2},
		{StageID: "stage-26", StageName: "四季", Season: "spring", SeasonalSubID: 1, ClearCount: 2},
	}}}
	act := func(action, params string) (int, map[string]json.RawMessage) {
		return gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+action+`","params":`+params+`}`, nil)
	}
	return f, store, act, func() (int, PlayerContextResponse) { return navigationGET(t, router) }
}

func TestHomeProfileObservesTheOwnerAndOnlyTheOwnerEdits(t *testing.T) {
	for _, tc := range []struct {
		name, owner string
		editable    bool
	}{{"owner", "hero", true}, {"visitor", "other", false}} {
		t.Run(tc.name, func(t *testing.T) {
			f, store, act, get := homeProfileRouter(t)
			if tc.owner != "hero" {
				store.selection.Subject = playercontext.Subject{Kind: "home", ID: tc.owner}
			}
			home := store.selection
			if status, raw := act("scene_enter", `{"destination":"home_profile"}`); status != 200 {
				t.Fatalf("enter: %d %s", status, raw)
			}
			f.profileReads = nil
			status, response := get()
			data := decodeSceneData[HomeProfileSceneData](t, response)
			if status != 200 || data.OwnerID != tc.owner || !slices.Contains(f.profileReads, tc.owner) || len(data.Fields) != len(character.ProfileFields) {
				t.Fatalf("%d reads=%v %+v", status, f.profileReads, data)
			}
			for i, field := range data.Fields {
				if field.Key != character.ProfileFields[i] || field.Label == "" || field.Value != f.bios[tc.owner][field.Key] {
					t.Fatalf("field %d = %+v", i, field)
				}
			}
			if hasHomeAction(response, "home_profile_update") != tc.editable || hasHomeAction(response, "scene_select") {
				t.Fatalf("actions = %+v", response.AvailableActions)
			}
			status, raw := act("home_profile_update", `{"bio_data":{"hobby":"散歩"}}`)
			if (status == 200) != tc.editable || len(f.profileWrites) != map[bool]int{true: 1, false: 0}[tc.editable] {
				t.Fatalf("update: %d %s writes=%v", status, raw, f.profileWrites)
			}
			if tc.editable && f.profileWrites[0] != "hero" {
				t.Fatalf("wrote %v", f.profileWrites)
			}
			if status, raw := act("scene_back", `{}`); status != 200 || store.selection != home {
				t.Fatalf("back: %d %s %+v", status, raw, store.selection)
			}
		})
	}
}

func TestHomeProfileUpdateRejections(t *testing.T) {
	f, _, act, _ := homeProfileRouter(t)
	if status, _ := act("home_profile_update", `{"bio_data":{}}`); status == 200 {
		t.Fatal("home_profile_update ran outside the profile scene")
	}
	if status, raw := act("scene_enter", `{"destination":"home_profile"}`); status != 200 {
		t.Fatalf("enter: %d %s", status, raw)
	}
	for params, code := range map[string]string{
		`{"bio_data":{"comment":"x"}}`:                              "PROFILE_UNKNOWN_FIELD",
		`{"bio_data":{"hobby":"a;b"}}`:                              "PROFILE_FIELD_INVALID",
		`{"bio_data":{"hobby":"` + strings.Repeat("あ", 161) + `"}}`: "PROFILE_FIELD_TOO_LONG",
		`{}`: "", // bio_data is required by the schema
	} {
		status, raw := act("home_profile_update", params)
		var detail ErrorDetail
		if status != 400 || (code != "" && (json.Unmarshal(raw["error"], &detail) != nil || detail.Code != code)) {
			t.Fatalf("%s: %d %s", params, status, raw)
		}
	}
	if len(f.profileWrites) != 0 {
		t.Fatalf("rejected update wrote %v", f.profileWrites)
	}
}

func TestHomeAdventureRecordObservesTheOwnersClears(t *testing.T) {
	f, store, act, get := homeProfileRouter(t)
	store.selection.Subject = playercontext.Subject{Kind: "home", ID: "other"}
	if status, raw := act("scene_enter", `{"destination":"home_adventure_record"}`); status != 200 {
		t.Fatalf("enter: %d %s", status, raw)
	}
	writes := store.writes
	status, response := get()
	data := decodeSceneData[HomeAdventureRecordSceneData](t, response)
	want := []HomeAdventureRecordStage{{StageID: "stage-01", StageName: "平原", ClearCount: 3}, {StageID: "stage-26", StageName: "四季", Season: "spring", SeasonalSubID: 1, ClearCount: 2}}
	if status != 200 || data.OwnerID != "other" || !slices.Equal(data.Stages, want) || data.TotalClears != 5 || store.writes != writes || len(f.profileWrites) != 0 {
		t.Fatalf("%d %+v", status, data)
	}
	if status, raw := act("scene_back", `{}`); status != 200 || store.selection != (playercontext.Selection{Destination: "home", Subject: playercontext.Subject{Kind: "home", ID: "other"}}) {
		t.Fatalf("back: %d %s %+v", status, raw, store.selection)
	}
	store.selection.Destination = "home_adventure_record"
	// An owner with no clears gets an empty record.
	store.selection.Subject = playercontext.Subject{}
	if status, response := get(); status != 200 || len(decodeSceneData[HomeAdventureRecordSceneData](t, response).Stages) != 0 {
		t.Fatalf("empty record: %d %+v", status, response.Scene.Data)
	}
}

func TestHomeProfileAndAdventureRecordReadFailures(t *testing.T) {
	for _, scene := range []string{"home_profile", "home_adventure_record"} {
		f, store, act, get := homeProfileRouter(t)
		store.selection.Subject = playercontext.Subject{Kind: "home", ID: "other"}
		if status, raw := act("scene_enter", `{"destination":"`+scene+`"}`); status != 200 {
			t.Fatalf("enter %s: %d %s", scene, status, raw)
		}
		f.recordFail = "other"
		if status, _ := get(); status != 500 {
			t.Fatalf("%s hid a required read failure: %d", scene, status)
		}
	}
}

package http

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/collection"
	"github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/customskill"
	jobapp "github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// homeRecordStub serves every record source and logs whose records were read.
type homeRecordStub struct {
	JobService
	reads []string
	sets  []string
	err   error
	skill *customskill.CustomSkill
}

func (s *homeRecordStub) ListDefinitions() []corejob.Definition { return nil }
func (s *homeRecordStub) read(kind, id string) error {
	s.reads = append(s.reads, kind+" "+id)
	return s.err
}
func (s *homeRecordStub) GetMonsterBook(_ context.Context, id string) ([]collection.MonsterBookEntry, collection.CompletionProgress, error) {
	return []collection.MonsterBookEntry{{MonsterID: "mon_001"}}, collection.CompletionProgress{DiscoveredCount: 1}, s.read("monsters", id)
}
func (s *homeRecordStub) GetItemCollection(_ context.Context, id, category string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error) {
	return []collection.ItemCollectionEntry{{ItemID: "item-1"}}, collection.CompletionProgress{}, s.read(category, id)
}
func (s *homeRecordStub) GetWeaponCollection(_ context.Context, id string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error) {
	return nil, collection.CompletionProgress{}, s.read("weapon", id)
}
func (s *homeRecordStub) GetArmorCollection(_ context.Context, id string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error) {
	return nil, collection.CompletionProgress{}, s.read("armor", id)
}
func (s *homeRecordStub) GetJobMastery(_ context.Context, id string) (jobapp.CharacterJobMastery, error) {
	return jobapp.CharacterJobMastery{CharacterID: id}, s.read("mastery", id)
}
func (s *homeRecordStub) GetCustomSkill(_ context.Context, id string) (*customskill.CustomSkill, error) {
	return s.skill, s.read("custom", id)
}
func (s *homeRecordStub) SetCustomSkill(_ context.Context, id, name, comment string, gems [3]string) (*customskill.CustomSkill, error) {
	if name == "" {
		return nil, customskill.ErrInvalidSkillName
	}
	s.sets = append(s.sets, id+" "+name+" "+comment+" "+gems[0])
	return &customskill.CustomSkill{CharacterID: id, Name: name, Comment: comment, Gems: gems}, nil
}

func homeRecordRouter(t *testing.T) (*homeRecordStub, *gatewayNavigationStore, func(string, string) (int, map[string]json.RawMessage), func() (int, PlayerContextResponse)) {
	stub := &homeRecordStub{}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	_, store, router := homeSceneRouter(t, WithCollection(stub, catalog), WithJob(stub), WithCustomSkill(stub))
	act := func(action, params string) (int, map[string]json.RawMessage) {
		return gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+action+`","params":`+params+`}`, nil)
	}
	return stub, store, act, func() (int, PlayerContextResponse) { return navigationGET(t, router) }
}

func homeDestinationIDs(t *testing.T, response PlayerContextResponse) []string {
	var ids []string
	for _, d := range decodeSceneData[HomeSceneData](t, response).Destinations {
		ids = append(ids, d.ID)
	}
	return ids
}

func TestHomeRecordScenesObserveTheHomeOwner(t *testing.T) {
	stub, store, act, get := homeRecordRouter(t)
	for _, tc := range []struct {
		name, owner  string
		destinations []string
	}{
		{"owner", "hero", []string{"home_collection", "home_monster_book", "home_job_master", "home_profile", "home_adventure_record", "home_inbox", "home_outbox", "home_notices", "home_custom_skill"}},
		{"visitor", "other", []string{"home_collection", "home_monster_book", "home_job_master", "home_profile", "home_adventure_record"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store.selection = playercontext.Selection{Destination: "home"}
			if tc.owner != "hero" {
				store.selection.Subject = playercontext.Subject{Kind: "home", ID: tc.owner}
			}
			home := store.selection
			_, response := get()
			if got := homeDestinationIDs(t, response); !slices.Equal(got, tc.destinations) {
				t.Fatalf("destinations = %v", got)
			}
			for scene, want := range map[string][]string{
				"home_collection":   {"weapon " + tc.owner, "armor " + tc.owner, "item " + tc.owner},
				"home_monster_book": {"monsters " + tc.owner},
				"home_job_master":   {"mastery " + tc.owner},
			} {
				stub.reads = nil
				if status, raw := act("scene_enter", `{"destination":"`+scene+`"}`); status != 200 {
					t.Fatalf("enter %s: %d %s", scene, status, raw)
				}
				writes := store.writes
				stub.reads = nil
				status, response := get()
				if status != 200 || response.Scene.Kind == "selection_unavailable" || !slices.Equal(stub.reads, want) || store.writes != writes || hasHomeAction(response, "scene_select") {
					t.Fatalf("%s: %d reads=%v %+v", scene, status, stub.reads, response.Scene)
				}
				if status, raw := act("scene_back", `{}`); status != 200 || store.selection != home {
					t.Fatalf("back from %s: %d %s %+v", scene, status, raw, store.selection)
				}
			}
		})
	}
	if len(stub.sets) != 0 {
		t.Fatal("observation mutated a custom skill")
	}
}

func TestHomeRecordRequiredReadFailures(t *testing.T) {
	for _, scene := range []string{"home_collection", "home_monster_book", "home_job_master", "home_custom_skill"} {
		stub, _, act, get := homeRecordRouter(t)
		if status, raw := act("scene_enter", `{"destination":"`+scene+`"}`); status != 200 {
			t.Fatalf("enter %s: %d %s", scene, status, raw)
		}
		stub.err = errors.New("record read failed")
		if status, _ := get(); status != 500 {
			t.Fatalf("%s hid a required read failure: %d", scene, status)
		}
	}
}

type homeCollectionRepo struct {
	collection.Repository
}

type homeCollectionCatalogFunc func(string) (item.Definition, error)

func (f homeCollectionCatalogFunc) FindByID(id string) (item.Definition, error) {
	return f(id)
}

func (homeCollectionRepo) GetItemCollection(_ context.Context, id, category string) ([]collection.ItemCollectionEntry, error) {
	ids := map[string][]string{
		"weapon": {"weapon-08", "weapon-02", "retired-weapon"},
		"armor":  {"armor-04", "armor-01"},
		"item":   {"item-142", "item-071", "item-001"},
	}
	var entries []collection.ItemCollectionEntry
	for i, itemID := range ids[category] {
		entries = append(entries, collection.ItemCollectionEntry{
			CharacterID: id, ItemID: itemID, ItemName: itemID, Category: category,
			DiscoveredAt: time.Unix(int64(i), 0).UTC(),
		})
	}
	return entries, nil
}

func TestHomeCollectionShowsCatalogColumnsInNumberOrder(t *testing.T) {
	service, err := collection.NewService(homeCollectionRepo{}, collection.DefaultTotalMonsters, collection.DefaultTotalItems)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var lookupErr error
	lookup := homeCollectionCatalogFunc(func(id string) (item.Definition, error) {
		if lookupErr != nil {
			return item.Definition{}, lookupErr
		}
		return catalog.FindByID(id)
	})
	_, store, router := homeSceneRouter(t, WithCollection(service, lookup))
	store.selection = playercontext.Selection{Destination: "home_collection"}
	status, response := navigationGET(t, router)
	if status != 200 || response.Scene.Kind != "facility" {
		t.Fatalf("observation: %d %+v", status, response.Scene)
	}
	type section struct {
		Entries []map[string]json.RawMessage `json:"entries"`
	}
	data := decodeSceneData[struct {
		Weapons section `json:"weapons"`
		Armors  section `json:"armors"`
		Items   section `json:"items"`
	}](t, response)
	for _, tc := range []struct {
		entries []map[string]json.RawMessage
		want    [][4]string
	}{
		{data.Weapons.Entries, [][4]string{{`"weapon-02"`, "4", "2", "30"}, {`"weapon-08"`, "14", "9", "400"}, {`"retired-weapon"`, "", "", ""}}},
		{data.Armors.Entries, [][4]string{{`"armor-01"`, "3", "0", "20"}, {`"armor-04"`, "12", "4", "150"}}},
		{data.Items.Entries, [][4]string{{`"item-001"`, "", "", "30"}, {`"item-071"`, "", "", "5000"}, {`"item-142"`, "", "", "150"}}},
	} {
		var got [][4]string
		for _, entry := range tc.entries {
			got = append(got, [4]string{string(entry["item_id"]), string(entry["power"]), string(entry["weight"]), string(entry["price"])})
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("catalog columns (id, power, weight, price) = %v, want %v", got, tc.want)
		}
	}
	lookupErr = errors.New("catalog read failed")
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatalf("observation hid a required catalog read failure: %d", status)
	}
}

func TestCustomSkillSetInTheOwnedCustomSkillScene(t *testing.T) {
	stub, _, act, get := homeRecordRouter(t)
	if status, _ := act("custom_skill_set", `{"name":"ひっさつ"}`); status == 200 || len(stub.sets) != 0 {
		t.Fatal("custom_skill_set ran outside the custom skill scene")
	}
	if status, raw := act("scene_enter", `{"destination":"home_custom_skill"}`); status != 200 {
		t.Fatalf("enter: %d %s", status, raw)
	}
	stub.reads = nil
	status, response := get()
	if status != 200 || !hasHomeAction(response, "custom_skill_set") || !slices.Equal(stub.reads, []string{"custom hero"}) {
		t.Fatalf("observation: %d %v %+v", status, stub.reads, response.AvailableActions)
	}
	for _, params := range []string{`{"name":"x","gem1":"g"}`, `{"comment":"c"}`} {
		if status, raw := act("custom_skill_set", params); status != 400 || len(stub.sets) != 0 {
			t.Fatalf("schema accepted %s: %d %s", params, status, raw)
		}
	}
	if status, raw := act("custom_skill_set", `{"name":""}`); status != 400 || !json.Valid(raw["error"]) || len(stub.sets) != 0 {
		t.Fatalf("invalid name: %d %s", status, raw)
	}
	status, raw := act("custom_skill_set", `{"name":"ひっさつ","comment":"くらえ","gems":["gem-1","",""]}`)
	if status != 200 || !slices.Equal(stub.sets, []string{"hero ひっさつ くらえ gem-1"}) {
		t.Fatalf("set: %d %s %v", status, raw, stub.sets)
	}
	status, raw = act("custom_skill_set", `{"name":"必\n殺","comment":"a\r\nb\u0301","gems":["gem-1","",""]}`)
	if status != 200 || !slices.Equal(stub.sets, []string{"hero ひっさつ くらえ gem-1", "hero 必殺 ab gem-1"}) {
		t.Fatalf("decode_pt set: %d %s %v", status, raw, stub.sets)
	}
}

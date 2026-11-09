package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestCharacterProfilePublicProjection(t *testing.T) {
	for _, tc := range []struct {
		name string
		bio  map[string]string
	}{
		{"nil bio", nil},
		{"empty bio", map[string]string{}},
		{"written bio", map[string]string{"job": "Writer", "sex": "Self-described", "hobby": "Fishing"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := character.ProfileView{
				Character: corecharacter.Character{
					ID: "private-character-id", PlayerID: "private-player-id", Name: "Hero",
					JobID: "private-job", Gender: "private-gender", Level: 87,
					Money: 81234567, Deposit: 92345678901, Crystal: 734567,
					SmallMedals: 62345, PendingWake: true,
					Stats:    corecharacter.Stats{HP: 512, Attack: 234},
					OldJobID: "private-old-job", JobMemory: &corecharacter.JobMemory{JobID: "private-memory-job"},
				},
				Profile: character.Profile{
					CharacterID: "private-character-id", UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
					Comment: "Mighty adventurer", AvatarURL: "https://i.imgur.com/AvATest.gif", AuraEffect: 3, BioData: tc.bio,
				},
			}
			service := &stubCharacterServiceExtended{getProfileFn: func(ctx context.Context, id string) (character.ProfileView, error) {
				if id != view.Character.ID {
					t.Fatalf("unexpected profile ID %q", id)
				}
				return view, nil
			}}
			h, err := apihttp.NewHandler(&stubPlayerService{}, service, &stubAdventureService{}, &stubShopService{})
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/characters/"+view.Character.ID+"/profile", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			var got map[string]any
			decoder := json.NewDecoder(strings.NewReader(rec.Body.String()))
			decoder.UseNumber()
			if err := decoder.Decode(&got); err != nil {
				t.Fatal(err)
			}
			profile := map[string]any{"comment": "Mighty adventurer", "avatar_url": "https://i.imgur.com/AvATest.gif", "aura_effect": json.Number("3")}
			if len(tc.bio) != 0 {
				profile["bio_data"] = map[string]any{"job": "Writer", "sex": "Self-described", "hobby": "Fishing"}
			}
			want := map[string]any{"character": map[string]any{"name": "Hero"}, "profile": profile}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("public response = %v, want exactly %v", got, want)
			}
			for _, private := range []string{"private-", "81234567", "92345678901", "734567", "62345", "2026-01-02"} {
				if strings.Contains(rec.Body.String(), private) {
					t.Errorf("response leaked private value %q", private)
				}
			}

			var spec map[string]any
			decoder = json.NewDecoder(strings.NewReader(string(apihttp.OpenAPISpec())))
			decoder.UseNumber()
			if err := decoder.Decode(&spec); err != nil {
				t.Fatal(err)
			}
			schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
			operation := spec["paths"].(map[string]any)["/characters/{id}/profile"].(map[string]any)["get"].(map[string]any)
			schema := operation["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
			if err := checkOpenAPIPayload(schemas, schema, want); err != nil {
				t.Fatalf("public response violates OpenAPI: %v", err)
			}
			for _, object := range []string{"character", "profile"} {
				want[object].(map[string]any)["Money"] = json.Number("81234567")
				if checkOpenAPIPayload(schemas, schema, want) == nil {
					t.Errorf("OpenAPI accepted private %s state", object)
				}
				delete(want[object].(map[string]any), "Money")
			}
		})
	}
}

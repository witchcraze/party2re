package http_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	gamehttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

func TestOpenAPIResponsesMatchHandlers(t *testing.T) {
	var root map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(gamehttp.OpenAPISpec())))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		t.Fatal(err)
	}
	schemas := root["components"].(map[string]any)["schemas"].(map[string]any)
	paths := root["paths"].(map[string]any)
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 10}
	h, err := gamehttp.NewHandler(
		&stubPlayerService{authenticateFn: alwaysAuthPlayer(coreplayer.Player{ID: "p1"})},
		&stubCharacterService{getFn: func(context.Context, string) (corecharacter.Character, error) { return char, nil }},
		&stubAdventureService{}, &stubShopService{},
		gamehttp.WithBank(&stubBankService{}),
		gamehttp.WithDepot(&stubDepotService{}), gamehttp.WithBlacksmith(&stubBlacksmithService{}),
		gamehttp.WithPvP(&stubColosseumPvPService{}), gamehttp.WithGvG(&stubGvGService{}),
		gamehttp.WithHome(&mockHomeService{}),
		gamehttp.WithParty(&stubPartyService{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	router := h.Router()
	for _, tc := range []struct {
		method, path, body, token string
		status                    int
	}{
		{"GET", "/health", "", "", 200},
		{"GET", "/pvp/rooms", "", "", 200},
		{"GET", "/pvp/rooms/{room_id}", "", "", 200},
		{"GET", "/gvg/rooms", "", "", 200},
		{"GET", "/gvg/rooms/{room_id}", "", "", 200},
		{"GET", "/gvg/leaderboard", "", "", 200},
		{"GET", "/parties/{id}", "", "", 200},
		{"GET", "/homes/{id}", "", "", 200},
	} {
		t.Run(tc.method+" "+tc.path+"/"+strconv.Itoa(tc.status), func(t *testing.T) {
			url := strings.NewReplacer("{id}", "c1", "{town_id}", "town1", "{room_id}", "room-1").Replace(tc.path)
			if strings.HasPrefix(url, "/gvg/") {
				url = strings.ReplaceAll(url, "room-1", "gvg-room-1")
			}
			req := httptest.NewRequest(tc.method, url, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			op := paths[tc.path].(map[string]any)[strings.ToLower(tc.method)].(map[string]any)
			response := op["responses"].(map[string]any)[strconv.Itoa(rec.Code)].(map[string]any)
			if ref, ok := response["$ref"].(string); ok {
				response = root["components"].(map[string]any)["responses"].(map[string]any)[strings.TrimPrefix(ref, "#/components/responses/")].(map[string]any)
			}
			if tc.status == 204 {
				if rec.Body.Len() != 0 || response["content"] != nil {
					t.Fatal("204 must have no payload or content schema")
				}
				return
			}
			schema := response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
			var payload any
			decoder := json.NewDecoder(rec.Body)
			decoder.UseNumber()
			if err := decoder.Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if err := checkOpenAPIPayload(schemas, schema, payload); err != nil {
				t.Fatal(err)
			}
			if ref, ok := schema["$ref"].(string); ok {
				schema = schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any)
			}
			if obj, ok := payload.(map[string]any); ok && schema["required"] != nil {
				key := schema["required"].([]any)[0].(string)
				delete(obj, key)
				if checkOpenAPIPayload(schemas, schema, obj) == nil {
					t.Fatal("schema accepted a missing required response property")
				}
			}
		})
	}
}

func TestOpenAPIConstraintBoundaries(t *testing.T) {
	var root map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(gamehttp.OpenAPISpec())))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		t.Fatal(err)
	}
	schemas := root["components"].(map[string]any)["schemas"].(map[string]any)
	for _, tc := range []struct {
		schema, field, payload string
		valid                  bool
	}{
		{"AlchemyCompendium", "entries", `null`, true},
		{"AlchemyCompendium", "entries", `[]`, true},
		{"AlchemyCompendium", "entries", `[{}]`, false},
		{"CasinoRoomView", "room", `{"password_hash":"secret"}`, false},
	} {
		t.Run(tc.schema+"/"+tc.field+"/"+tc.payload, func(t *testing.T) {
			schema := schemas[tc.schema].(map[string]any)
			if tc.field != "" {
				schema = schema["properties"].(map[string]any)[tc.field].(map[string]any)
			}
			var value any
			decoder := json.NewDecoder(strings.NewReader(tc.payload))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
			if err := checkOpenAPIPayload(schemas, schema, value); (err == nil) != tc.valid {
				t.Fatalf("valid=%t: %v", tc.valid, err)
			}
		})
	}
}

// minLength counts Unicode characters, independently of any nonblank pattern.
func TestOpenAPIPayloadMinLengthCountsCharacters(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{{"", false}, {"あ", false}, {"あい", true}} {
		schema := map[string]any{"type": "string", "minLength": json.Number("2")}
		if err := checkOpenAPIPayload(nil, schema, tc.value); (err == nil) != tc.valid {
			t.Errorf("minLength for %q: valid=%t: %v", tc.value, tc.valid, err)
		}
	}
}

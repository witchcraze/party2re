package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	gamehttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/plantation"
)

type openAPICasinoService struct{ stubCasinoService }

func (s *openAPICasinoService) GetRoomView(_ context.Context, roomID, _, characterID string) (*casino.RoomView, error) {
	return &casino.RoomView{
		Room: casino.RoomObservation{RoomSummary: casino.RoomSummary{
			ID: roomID, Name: "Test", GameType: casino.GameTypeIndian, Speed: casino.SpeedNormal,
			MaxPlayers: 8, Rate: 10, Status: casino.RoomStatusWaiting,
		}},
		Members: []casino.MemberObservation{{CharacterID: characterID, Card: -1}},
	}, nil
}

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
		gamehttp.WithCasino(&openAPICasinoService{}), gamehttp.WithBank(&stubBankService{}),
		gamehttp.WithDepot(&stubDepotService{}), gamehttp.WithBlacksmith(&stubBlacksmithService{}),
		gamehttp.WithPvP(&stubColosseumPvPService{}), gamehttp.WithGvG(&stubGvGService{}),
		gamehttp.WithAlchemy(&stubAlchemyService{}), gamehttp.WithHome(&mockHomeService{}),
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
		{"GET", "/casino/prizes", "", "", 200},
		{"GET", "/casino/rooms", "", "", 200},
		{"GET", "/casino/rooms/{roomId}", "", "session", 200},
		{"GET", "/characters/{id}/casino", "", "session", 200},
		{"POST", "/characters/{id}/casino/exchange", `{"coins":10}`, "session", 200},
		{"POST", "/characters/{id}/casino/slot", `{"bet":1}`, "session", 200},
		{"POST", "/characters/{id}/casino/rooms", `{"name":"Test","game_type":"indian","rate":10,"speed":18,"max_players":8}`, "session", 201},
		{"POST", "/characters/{id}/bank/inspect", "", "session", 200},
		{"POST", "/characters/{id}/bank/talk", "", "session", 200},
		{"POST", "/characters/{id}/depot/deposit", `{"item_id":"sword"}`, "session", 200},
		{"POST", "/characters/{id}/depot/send-money", `{"recipient_character_id":"c2","amount":10}`, "session", 200},
		{"GET", "/blacksmith/seals", "", "", 200},
		{"GET", "/characters/{id}/blacksmith/storage", "", "session", 200},
		{"POST", "/characters/{id}/blacksmith/seal", `{"seal_id":1}`, "session", 200},
		{"GET", "/pvp/rooms", "", "", 200},
		{"GET", "/pvp/rooms/{room_id}", "", "", 200},
		{"GET", "/gvg/rooms", "", "", 200},
		{"GET", "/gvg/rooms/{room_id}", "", "", 200},
		{"GET", "/gvg/leaderboard", "", "", 200},
		{"GET", "/parties/{id}", "", "", 200},
		{"GET", "/towns/{town_id}/houses", "", "", 200},
		{"POST", "/towns/{town_id}/houses", `{"character_id":"c1","house_style":"001"}`, "session", 201},
		{"GET", "/characters/{id}/alchemy", "", "session", 200},
		{"POST", "/characters/{id}/alchemy/synthesize", `{"recipe_id":"recipe"}`, "session", 200},
		{"POST", "/characters/{id}/alchemy/claim", "", "session", 200},
		{"POST", "/characters/{id}/home/wake", "", "session", 200},
		{"GET", "/homes/{id}", "", "", 200},
		{"GET", "/characters/{id}/casino", "", "", 401},
		{"POST", "/characters/{id}/casino/exchange", `{"coins":0}`, "session", 400},
		{"DELETE", "/letters/{id}", "", "session", 204},
	} {
		t.Run(tc.method+" "+tc.path+"/"+strconv.Itoa(tc.status), func(t *testing.T) {
			url := strings.NewReplacer("{id}", "c1", "{town_id}", "town1", "{room_id}", "room-1", "{roomId}", "r1").Replace(tc.path)
			if strings.HasPrefix(url, "/gvg/") {
				url = strings.ReplaceAll(url, "room-1", "gvg-room-1")
			}
			if tc.path == "/letters/{id}" {
				url += "?character_id=c1"
			}
			if tc.path == "/casino/rooms/{roomId}" {
				url += "?character_id=c1"
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
			if tc.path == "/casino/rooms/{roomId}" {
				room := payload.(map[string]any)["room"].(map[string]any)["room"].(map[string]any)
				room["password_hash"] = "secret"
				if checkOpenAPIPayload(schemas, schema, payload) == nil {
					t.Fatal("viewer schema accepted a private room field")
				}
				delete(room, "password_hash")
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
		{"CasinoAccount", "coins", `9223372036854775807`, true},
		{"CasinoAccount", "coins", `9223372036854775808`, false},
		{"CasinoAccount", "coins", `-1`, false},
		{"CasinoAccount", "coins", `0.5`, false},
		{"CasinoSpinResult", "reels", `["Cherry","Note","Seven"]`, true},
		{"CasinoSpinResult", "reels", `["Cherry","Note"]`, false},
		{"CasinoSpinResult", "reels", `["Cherry","Note","unknown"]`, false},
		{"CasinoSpinResult", "reels", `null`, false},
		{"AlchemyCompendium", "entries", `null`, true},
		{"AlchemyCompendium", "entries", `[]`, true},
		{"AlchemyCompendium", "entries", `[{}]`, false},
		{"CasinoRoomView", "room", `{"password_hash":"secret"}`, false},
		{"BuildStoreRequest", "", `{"character_id":"c1","house_style":"001"}`, true},
		{"UseHomeItemRequest", "", `{"instance_id":"item-1"}`, true},
		{"CasinoRoomActionRequest", "", `{"mark":"0"}`, true},
		{"PlantationSowRequest", "", `{"seed_id":"red"}`, true},
		{"PlantationSowRequest", "", `{"seed_id":""}`, false},
		{"PlantationSowRequest", "", `{}`, false},
		{"PlantationFertilizeRequest", "", `{"fertilizer_id":"speed"}`, true},
		{"PlantationFertilizeRequest", "", `{"fertilizer_id":""}`, false},
		{"PlantationFertilizeRequest", "", `{}`, false},
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

func TestOpenAPILetterAndCasinoRequestContracts(t *testing.T) {
	var root map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(gamehttp.OpenAPISpec())))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		t.Fatal(err)
	}
	schemas := root["components"].(map[string]any)["schemas"].(map[string]any)
	paths := root["paths"].(map[string]any)
	letter := `{"sender_character_id":"c1","recipient_character_id":"c2","content":"こんにちは\nHello"}`
	for _, tc := range []struct {
		name, path, payload string
		valid               bool
	}{
		{"letter without color", "/letters", letter, true},
		{"letter with empty color", "/letters", strings.Replace(letter, "}", `,"color":""}`, 1), true},
		{"letter with non-hex color", "/letters", strings.Replace(letter, "}", `,"color":"arbitrary color"}`, 1), true},
		{"letter trimmed content at limit", "/letters", strings.Replace(letter, `こんにちは\nHello`, " "+strings.Repeat("あ", 1000)+" ", 1), true},
		{"missing sender", "/letters", strings.Replace(letter, `"sender_character_id":"c1",`, "", 1), false},
		{"missing recipient", "/letters", strings.Replace(letter, `"recipient_character_id":"c2",`, "", 1), false},
		{"missing content", "/letters", `{"sender_character_id":"c1","recipient_character_id":"c2"}`, false},
		{"empty sender", "/letters", strings.Replace(letter, `"c1"`, `""`, 1), false},
		{"blank sender", "/letters", strings.Replace(letter, `"c1"`, `" \t\u3000"`, 1), false},
		{"empty recipient", "/letters", strings.Replace(letter, `"c2"`, `""`, 1), false},
		{"blank recipient", "/letters", strings.Replace(letter, `"c2"`, `"\u0085\u00a0"`, 1), false},
		{"empty content", "/letters", strings.Replace(letter, `"こんにちは\nHello"`, `""`, 1), false},
		{"blank content", "/letters", strings.Replace(letter, `"こんにちは\nHello"`, `" \t\r\n\u3000"`, 1), false},
		{"omitted direction", "/characters/{id}/casino/exchange", `{"coins":10}`, true},
		{"empty direction", "/characters/{id}/casino/exchange", `{"direction":"","coins":10}`, true},
		{"gold to coins", "/characters/{id}/casino/exchange", `{"direction":"gold_to_coins","coins":10}`, true},
		{"reverse direction", "/characters/{id}/casino/exchange", `{"direction":"coins_to_gold","coins":10}`, false},
		{"unknown direction", "/characters/{id}/casino/exchange", `{"direction":"other","coins":10}`, false},
		{"missing coins", "/characters/{id}/casino/exchange", `{}`, false},
		{"zero coins", "/characters/{id}/casino/exchange", `{"coins":0}`, false},
		{"negative coins", "/characters/{id}/casino/exchange", `{"coins":-1}`, false},
		{"fractional coins", "/characters/{id}/casino/exchange", `{"coins":0.5}`, false},
		{"maximum coins", "/characters/{id}/casino/exchange", `{"coins":` + strconv.FormatInt(int64(casino.MaxExchangeCoins), 10) + `}`, true},
		{"overflowing coins", "/characters/{id}/casino/exchange", `{"coins":` + strconv.FormatInt(int64(casino.MaxExchangeCoins)+1, 10) + `}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := paths[tc.path].(map[string]any)["post"].(map[string]any)["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
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
	// Check minLength independently of the nonblank pattern, counting Unicode characters.
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

func TestPostReviewPlantationRequestSchema(t *testing.T) {
	var root map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(gamehttp.OpenAPISpec())))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		t.Fatal(err)
	}
	schemas := root["components"].(map[string]any)["schemas"].(map[string]any)
	paths := root["paths"].(map[string]any)

	sowSchema := schemas["PlantationSowRequest"].(map[string]any)
	fertSchema := schemas["PlantationFertilizeRequest"].(map[string]any)

	// Schema-level validation: missing, empty, whitespace, and valid values
	schemaTests := []struct {
		name   string
		schema map[string]any
		body   string
		valid  bool
	}{
		{"sow missing seed_id", sowSchema, `{}`, false},
		{"sow empty seed_id", sowSchema, `{"seed_id":""}`, false},
		{"sow spaces seed_id", sowSchema, `{"seed_id":"   "}`, false},
		{"sow whitespace seed_id", sowSchema, `{"seed_id":" \t\r\n\u3000"}`, false},
		{"sow non-ascii whitespace seed_id", sowSchema, "{\"seed_id\":\"\u0085\u00a0\"}", false},
		{"sow valid canonical id", sowSchema, `{"seed_id":"red"}`, true},
		{"sow valid alias", sowSchema, `{"seed_id":"トマトの種"}`, true},
		{"sow valid gold", sowSchema, `{"seed_id":"gold"}`, true},

		{"fertilize missing fertilizer_id", fertSchema, `{}`, false},
		{"fertilize empty fertilizer_id", fertSchema, `{"fertilizer_id":""}`, false},
		{"fertilize spaces fertilizer_id", fertSchema, `{"fertilizer_id":"   "}`, false},
		{"fertilize whitespace fertilizer_id", fertSchema, `{"fertilizer_id":" \t\r\n\u3000"}`, false},
		{"fertilize non-ascii whitespace fertilizer_id", fertSchema, "{\"fertilizer_id\":\"\u0085\u00a0\"}", false},
		{"fertilize valid canonical id", fertSchema, `{"fertilizer_id":"speed"}`, true},
		{"fertilize valid alias", fertSchema, `{"fertilizer_id":"速効肥料"}`, true},
		{"fertilize none id schema valid", fertSchema, `{"fertilizer_id":"none"}`, true},
	}
	for _, tc := range schemaTests {
		t.Run("schema/"+tc.name, func(t *testing.T) {
			var value any
			dec := json.NewDecoder(strings.NewReader(tc.body))
			dec.UseNumber()
			if err := dec.Decode(&value); err != nil {
				t.Fatal(err)
			}
			err := checkOpenAPIPayload(schemas, tc.schema, value)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t: %v", tc.valid, err)
			}
		})
	}

	// Verify paths in OpenAPI point to these schemas
	sowPathSchema := paths["/characters/{id}/plantation/sow"].(map[string]any)["post"].(map[string]any)["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	fertPathSchema := paths["/characters/{id}/plantation/fertilize"].(map[string]any)["post"].(map[string]any)["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	if sowPathSchema["$ref"] != "#/components/schemas/PlantationSowRequest" {
		t.Fatalf("unexpected sow path schema ref: %v", sowPathSchema["$ref"])
	}
	if fertPathSchema["$ref"] != "#/components/schemas/PlantationFertilizeRequest" {
		t.Fatalf("unexpected fertilize path schema ref: %v", fertPathSchema["$ref"])
	}

	// HTTP Handler validation with zero-value real plantation service
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 10}
	h, err := gamehttp.NewHandler(
		&stubPlayerService{authenticateFn: alwaysAuthPlayer(coreplayer.Player{ID: "p1"})},
		&stubCharacterService{getFn: func(context.Context, string) (corecharacter.Character, error) { return char, nil }},
		&stubAdventureService{}, &stubShopService{},
		gamehttp.WithPlantation(&plantation.Service{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	router := h.Router()

	httpTests := []struct {
		name       string
		path       string
		body       string
		wantStatus int
	}{
		{"sow empty object", "/characters/c1/plantation/sow", `{}`, http.StatusBadRequest},
		{"sow empty string", "/characters/c1/plantation/sow", `{"seed_id":""}`, http.StatusBadRequest},
		{"sow whitespace", "/characters/c1/plantation/sow", `{"seed_id":"   "}`, http.StatusBadRequest},
		{"sow unicode whitespace", "/characters/c1/plantation/sow", "{\"seed_id\":\" \t\r\n\u3000\"}", http.StatusBadRequest},

		{"fertilize empty object", "/characters/c1/plantation/fertilize", `{}`, http.StatusBadRequest},
		{"fertilize empty string", "/characters/c1/plantation/fertilize", `{"fertilizer_id":""}`, http.StatusBadRequest},
		{"fertilize whitespace", "/characters/c1/plantation/fertilize", `{"fertilizer_id":"   "}`, http.StatusBadRequest},
		{"fertilize unicode whitespace", "/characters/c1/plantation/fertilize", "{\"fertilizer_id\":\" \t\r\n\u3000\"}", http.StatusBadRequest},
		{"fertilize none rejection", "/characters/c1/plantation/fertilize", `{"fertilizer_id":"none"}`, http.StatusBadRequest},
	}
	for _, tc := range httpTests {
		t.Run("http/"+tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer session")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status %d, want %d; body: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

package http_test

import (
	"bytes"
	"encoding/json"
	"testing"

	gamehttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestOpenAPIPlayerInspection(t *testing.T) {
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(gamehttp.OpenAPISpec()))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		t.Fatal(err)
	}
	schemas := root["components"].(map[string]any)["schemas"].(map[string]any)
	schema := schemas["PlayerInspection"].(map[string]any)
	for _, location := range []*gamehttp.PlayerInspectionLocation{
		nil,
		{PublicLocation: playercontext.PublicLocation{Kind: "quest"}, Title: "クエスト中"},
		{PublicLocation: playercontext.PublicLocation{Kind: "scene", SceneID: "bank"}, Title: "銀行"},
		{PublicLocation: playercontext.PublicLocation{Kind: "home", HomeOwnerCharacterID: "host", HomeOwnerName: "家主"}, Title: "家主の家"},
	} {
		result := gamehttp.PlayerInspection{Gems: []string{"gem-1", "gem-1"}, GemNames: []string{"宝石", "宝石"}, Location: location}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if err := checkOpenAPIPayload(schemas, schema, payload); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if public, ok := payload["location"].(map[string]any); ok {
			for _, private := range []string{"id", "phase", "members", "target_id"} {
				public[private] = "private"
				if checkOpenAPIPayload(schemas, schema, payload) == nil {
					t.Fatalf("schema accepts private location %s", private)
				}
				delete(public, private)
			}
		}
		delete(payload, "gem_names")
		if checkOpenAPIPayload(schemas, schema, payload) == nil {
			t.Fatal("schema accepts missing gem names")
		}
	}
}

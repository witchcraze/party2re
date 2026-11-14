package http_test

import (
	"bytes"
	"encoding/json"
	"testing"

	gamehttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/shop"
)

func TestOpenAPIShopSynthesisRecipeSchema(t *testing.T) {
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(gamehttp.OpenAPISpec()))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		t.Fatal(err)
	}
	schemas := root["components"].(map[string]any)["schemas"].(map[string]any)
	schema := schemas["ShopSynthesisRecipe"].(map[string]any)

	for _, recipe := range shop.AllSynthesisRecipes() {
		raw, err := json.Marshal(recipe)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if err := checkOpenAPIPayload(schemas, schema, payload); err != nil {
			t.Fatalf("recipe %s (%s): %v", recipe.ID, raw, err)
		}
		payload["legacy_name"] = "legacy"
		if checkOpenAPIPayload(schemas, schema, payload) == nil {
			t.Fatalf("schema accepted legacy_name for %s", recipe.ID)
		}
	}
}

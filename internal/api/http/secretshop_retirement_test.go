package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRetiredSecretShopRoutesDoNotReadOrExecute(t *testing.T) {
	for _, configured := range []bool{true, false} {
		f, store, reads, router := secretShopSceneRouter(t)
		if !configured {
			h, err := NewHandler(f.gatewayFixture, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithSecretShop(nil))
			if err != nil {
				t.Fatal(err)
			}
			router = h.Router()
		}
		for _, route := range []struct{ method, suffix string }{
			{http.MethodGet, ""},
			{http.MethodPost, "/purchase"},
		} {
			for _, actor := range []struct{ id, token string }{{"hero", ""}, {"hero", "session"}, {"other", "session"}, {"missing", "session"}} {
				path := "/characters/" + actor.id + "/secretshop" + route.suffix
				r := httptest.NewRequest(route.method, path, strings.NewReader(`{"item_id":"secret_item_herbal_root","quantity":1}`))
				r.Header.Set("Authorization", "Bearer "+actor.token)
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != http.StatusNotFound || f.executions != 0 || reads.reads != 0 || store.writes != 0 || f.writes != 0 || f.char.Money != 100000 || len(f.inv.Items) != 0 || len(f.dep.Items) != 0 {
					t.Fatalf("configured=%t retired %s %s: %d %s", configured, route.method, path, w.Code, w.Body.String())
				}
			}
		}
	}
}

func TestOpenAPISecretShopRetirementKeepsNPCAndGateway(t *testing.T) {
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(OpenAPISpec(), &spec); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/characters/{id}/secretshop", "/characters/{id}/secretshop/purchase"} {
		if _, exists := spec.Paths[path]; exists {
			t.Errorf("retired path remains in OpenAPI: %s", path)
		}
	}
	for _, route := range []struct{ method, path string }{
		{"post", "/characters/{id}/secretshop/talk"},
		{"post", "/characters/{id}/secretshop/inspect"},
		{"post", "/characters/{id}/secretshop/puffpuff"},
		{"post", "/api/v1/characters/{id}/actions"},
		{"get", "/api/v1/characters/{id}/context"},
	} {
		if len(spec.Paths[route.path][route.method]) == 0 {
			t.Errorf("required route missing from OpenAPI: %s %s", route.method, route.path)
		}
	}
}

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRetiredDepotRoutesDoNotReadOrExecute(t *testing.T) {
	for _, configured := range []bool{true, false} {
		f, store, _, router := depotExpansionRouter(t)
		if !configured {
			h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithDepot(nil))
			if err != nil {
				t.Fatal(err)
			}
			router = h.Router()
		}
		for _, route := range []struct{ method, suffix string }{
			{http.MethodGet, ""},
			{http.MethodPost, "/expand"},
		} {
			for _, actor := range []struct{ id, token string }{{"hero", ""}, {"hero", "session"}, {"other", "session"}, {"missing", "session"}} {
				path := "/characters/" + actor.id + "/depot" + route.suffix
				r := httptest.NewRequest(route.method, path, strings.NewReader(`{}`))
				r.Header.Set("Authorization", "Bearer "+actor.token)
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != http.StatusNotFound || f.executions != 0 || f.reads != 0 || f.queryCalls != 0 || store.writes != 0 {
					t.Fatalf("configured=%t retired %s %s: %d %s", configured, route.method, path, w.Code, w.Body.String())
				}
			}
		}
	}
}

func TestOpenAPIDepotRetirementKeepsRemainingOperationsAndGateway(t *testing.T) {
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(OpenAPISpec(), &spec); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/characters/{id}/depot", "/characters/{id}/depot/expand"} {
		if _, exists := spec.Paths[path]; exists {
			t.Errorf("retired path remains in OpenAPI: %s", path)
		}
	}
	for _, suffix := range []string{"deposit", "withdraw", "sell", "sell-batch", "send-money", "send-item", "sort"} {
		path := "/characters/{id}/depot/" + suffix
		if len(spec.Paths[path]["post"]) == 0 {
			t.Errorf("retained Depot operation missing: POST %s", path)
		}
	}
	for _, route := range []struct{ method, path string }{
		{"post", "/api/v1/characters/{id}/actions"},
		{"get", "/api/v1/characters/{id}/context"},
	} {
		if len(spec.Paths[route.path][route.method]) == 0 {
			t.Errorf("required route missing: %s %s", route.method, route.path)
		}
	}
}

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
		f, store, _, router := depotSaleRouter(t)
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
			{http.MethodPost, "/sell"},
			{http.MethodPost, "/sell-batch"},
			{http.MethodPost, "/sort"},
			{http.MethodPost, "/deposit"},
			{http.MethodPost, "/send-item"},
			{http.MethodPost, "/withdraw"},
			{http.MethodPost, "/send-money"},
		} {
			for _, actor := range []struct{ id, token string }{{"hero", ""}, {"hero", "session"}, {"other", "session"}, {"missing", "session"}} {
				path := "/characters/" + actor.id + "/depot" + route.suffix
				r := httptest.NewRequest(route.method, path, strings.NewReader(`{}`))
				r.Header.Set("Authorization", "Bearer "+actor.token)
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != http.StatusNotFound || f.executions != 0 || f.reads != 0 || f.characterCalls != 0 || f.profileCalls != 0 || f.queryCalls != 0 || f.sleepCalls != 0 || store.writes != 0 {
					t.Fatalf("configured=%t retired %s %s: %d %s", configured, route.method, path, w.Code, w.Body.String())
				}
			}
		}
	}
}

func TestOpenAPIDepotRetirementKeepsGateway(t *testing.T) {
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(OpenAPISpec(), &spec); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "/expand", "/sell", "/sell-batch", "/sort", "/deposit", "/send-item", "/withdraw", "/send-money"} {
		if _, exists := spec.Paths["/characters/{id}/depot"+suffix]; exists {
			t.Errorf("retired path remains in OpenAPI: /characters/{id}/depot%s", suffix)
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

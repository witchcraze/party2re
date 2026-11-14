package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestRetiredRescueRoutesDoNotReadOrExecute(t *testing.T) {
	for _, configured := range []bool{false, true} {
		var service RescueService
		if configured {
			service = &struct{ RescueService }{}
		}
		// Nil embedded methods panic if a retired path reaches auth or any service.
		h, err := NewHandler(&struct{ PlayerService }{}, &struct{ CharacterService }{},
			&struct{ AdventureService }{}, &struct{ ShopService }{}, WithRescue(service))
		if err != nil {
			t.Fatal(err)
		}
		router := h.Router()
		for _, actor := range []struct{ id, token string }{{"hero", ""}, {"hero", "session"}, {"other", "session"}, {"missing", "session"}} {
			for _, route := range []struct{ method, path, body string }{
				{http.MethodPost, "/rescues/request", `{"character_id":"` + actor.id + `","reason":"stuck"}`},
				{http.MethodGet, "/rescues/penalty?character_id=" + actor.id, ""},
			} {
				r := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				r.Header.Set("Authorization", "Bearer "+actor.token)
				if route.body != "" {
					r.Header.Set("Content-Type", "application/json")
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != http.StatusNotFound {
					t.Fatalf("configured=%t %s %s actor=%s: status=%d body=%s", configured, route.method, route.path, actor.id, w.Code, w.Body.String())
				}
			}
		}
	}
}

func TestOpenAPIRescueRetirementKeepsGateway(t *testing.T) {
	var spec struct {
		Paths      map[string]map[string]json.RawMessage
		Components struct{ Schemas map[string]json.RawMessage }
	}
	if err := json.Unmarshal(OpenAPISpec(), &spec); err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{"/rescues/request", "/rescues/penalty"} {
		if _, exists := spec.Paths[retired]; exists {
			t.Errorf("retired Rescue path remains in OpenAPI: %s", retired)
		}
	}
	if _, exists := spec.Components.Schemas["RescuePenaltyResponse"]; exists {
		t.Error("retired RescuePenaltyResponse schema remains in OpenAPI")
	}
	for _, route := range []struct{ method, path string }{
		{"get", "/helpers/quests"},
		{"post", "/helpers/complete"},
		{"post", "/admin/characters/{id}/unstuck"},
		{"get", "/api/v1/characters/{id}/context"},
		{"post", "/api/v1/characters/{id}/actions"},
	} {
		if len(spec.Paths[route.path][route.method]) == 0 {
			t.Errorf("required route missing: %s %s", route.method, route.path)
		}
	}
	var result struct {
		Required    []string
		Description string
	}
	if err := json.Unmarshal(spec.Components.Schemas["RescueRescueRecord"], &result); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Required, []string{"id", "character_id", "reason", "penalty_seconds", "created_at"}) || !strings.Contains(result.Description, "rescue_request") {
		t.Fatalf("missing Gateway RescueRecord contract: %+v", result)
	}
}

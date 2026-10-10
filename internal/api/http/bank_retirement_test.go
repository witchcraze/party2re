package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRetiredBankRoutesDoNotReadOrExecute(t *testing.T) {
	f, store, reads, router := bankSceneRouter(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/characters/hero/bank"},
		{http.MethodPost, "/characters/hero/bank/deposit"},
		{http.MethodPost, "/characters/hero/bank/withdraw"},
	} {
		for _, token := range []string{"", "session"} {
			r := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"amount":40}`))
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != http.StatusNotFound || f.executions != 0 || reads.reads != 0 || store.writes != 0 || f.char.Money != 100 || f.char.Deposit != 1000 {
				t.Fatalf("retired %s %s: %d %s", route.method, route.path, w.Code, w.Body.String())
			}
		}
	}
}

func TestOpenAPIBankRetirementKeepsDialogueAndGateway(t *testing.T) {
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(OpenAPISpec(), &spec); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/characters/{id}/bank", "/characters/{id}/bank/deposit", "/characters/{id}/bank/withdraw"} {
		if _, exists := spec.Paths[path]; exists {
			t.Errorf("retired path remains in OpenAPI: %s", path)
		}
	}
	for _, route := range []struct{ method, path string }{
		{"post", "/characters/{id}/bank/talk"},
		{"post", "/characters/{id}/bank/inspect"},
		{"post", "/api/v1/characters/{id}/actions"},
		{"get", "/api/v1/characters/{id}/context"},
	} {
		if len(spec.Paths[route.path][route.method]) == 0 {
			t.Errorf("required route missing from OpenAPI: %s %s", route.method, route.path)
		}
	}
}

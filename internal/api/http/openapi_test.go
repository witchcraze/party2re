package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gamehttp "github.com/witchcraze/party2re/internal/api/http"
)

type openAPISpec struct {
	OpenAPI    string                            `json:"openapi"`
	Info       map[string]interface{}            `json:"info"`
	Paths      map[string]map[string]interface{} `json:"paths"`
	Components map[string]interface{}            `json:"components"`
}

func TestOpenAPIEndpoint(t *testing.T) {
	h, err := gamehttp.NewHandler(
		&stubPlayerService{},
		&stubCharacterService{},
		&stubAdventureService{},
		&stubShopService{},
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	h.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", contentType)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if parsed["openapi"] != "3.1.0" {
		t.Errorf("expected openapi '3.1.0', got %v", parsed["openapi"])
	}
}

package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

func getRepoRootDir(t *testing.T) string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to determine runtime caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func TestOpenAPISpecificationValidity(t *testing.T) {
	rootDir := getRepoRootDir(t)
	docsPath := filepath.Join(rootDir, "docs", "api", "openapi.json")

	docsData, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatalf("failed to read docs/api/openapi.json: %v", err)
	}

	embeddedData := gamehttp.OpenAPISpec()
	if len(embeddedData) == 0 {
		t.Fatal("embedded OpenAPI spec is empty")
	}

	if !bytes.Equal(bytes.TrimSpace(docsData), bytes.TrimSpace(embeddedData)) {
		t.Error("docs/api/openapi.json and internal/api/http/openapi.json must be identical; please sync them")
	}

	var spec openAPISpec
	if err := json.Unmarshal(embeddedData, &spec); err != nil {
		t.Fatalf("embedded OpenAPI spec is not valid JSON: %v", err)
	}

	if !strings.HasPrefix(spec.OpenAPI, "3.1.") {
		t.Errorf("expected OpenAPI 3.1.x, got %q", spec.OpenAPI)
	}

	if spec.Info["title"] == "" || spec.Info["title"] == nil {
		t.Error("spec.Info.title must not be empty")
	}

	if spec.Info["version"] == "" || spec.Info["version"] == nil {
		t.Error("spec.Info.version must not be empty")
	}

	if len(spec.Paths) == 0 {
		t.Error("spec.Paths must not be empty")
	}

	if len(spec.Components) == 0 {
		t.Error("spec.Components must not be empty")
	}
}

func TestOpenAPIRouteCoverage(t *testing.T) {
	specData := gamehttp.OpenAPISpec()
	var spec openAPISpec
	if err := json.Unmarshal(specData, &spec); err != nil {
		t.Fatalf("failed to unmarshal OpenAPI spec: %v", err)
	}

	expectedRoutes := []struct {
		Method string
		Path   string
	}{
		{"GET", "/health"},
		{"GET", "/openapi.json"},
		{"GET", "/maintenance"},
		{"POST", "/admin/maintenance"},
		{"PUT", "/admin/maintenance"},
		{"POST", "/players"},
		{"DELETE", "/players/me"},
		{"DELETE", "/players/{id}"},
		{"POST", "/sessions"},
		{"DELETE", "/sessions"},
		{"POST", "/characters"},
		{"GET", "/characters/{id}"},
		{"DELETE", "/characters/{id}"},
		{"POST", "/adventures"},
		{"POST", "/adventures/{id}/claim"},
		{"GET", "/characters/{id}/adventures"},
		{"GET", "/characters/{id}/adventure-chronicle"},
		{"POST", "/shop/purchase"},
		{"POST", "/shop/sell"},
		{"GET", "/park/posts"},
		{"POST", "/park/posts"},
		{"POST", "/park/npc/talk"},
		{"POST", "/park/npc/divinate"},
		{"GET", "/park/npc/inspect"},
		{"GET", "/medals/rewards"},
		{"POST", "/medals/claim"},
		{"GET", "/helpers/quests"},
		{"POST", "/helpers/complete"},
		{"GET", "/rescues/penalty"},
		{"POST", "/rescues/request"},
		{"GET", "/news"},
		{"GET", "/news/{id}"},
		{"POST", "/news"},
		{"GET", "/notifications"},
		{"GET", "/notifications/unread-count"},
		{"POST", "/notifications/{id}/read"},
		{"POST", "/notifications/read-all"},
		{"DELETE", "/notifications/{id}"},
		{"GET", "/homes/{id}"},
		{"POST", "/homes/{id}/settings"},
		{"POST", "/homes/{id}/companion/phrases"},
		{"DELETE", "/homes/{id}/companion/phrases/{phrase_id}"},
		{"GET", "/homes/{id}/companion/talk"},
		{"GET", "/homes/{id}/notices"},
		{"POST", "/homes/{id}/notices/clear"},
		{"POST", "/letters"},
		{"GET", "/letters/inbox"},
		{"GET", "/letters/outbox"},
		{"GET", "/letters/unread-count"},
		{"POST", "/letters/{id}/read"},
		{"DELETE", "/letters/{id}"},
		{"GET", "/rankings/levels"},
		{"GET", "/rankings/wealth"},
		{"GET", "/rankings/characters-wealth"},
		{"GET", "/rankings/battles"},
		{"GET", "/rankings/job-mastery"},
		{"GET", "/rankings/job-popularity"},
		{"GET", "/rankings/helpers"},
		{"GET", "/rankings/rebirths"},
		{"GET", "/rankings/medals"},
		{"GET", "/rankings/{type}"},
		{"POST", "/rankings/refresh"},
		{"GET", "/jobs"},
		{"POST", "/characters/{id}/change-job"},
		{"POST", "/characters/{id}/rebirth"},
		{"POST", "/characters/{id}/inn"},
		{"GET", "/characters/{id}/custom-skills"},
		{"POST", "/characters/{id}/custom-skills"},
		{"DELETE", "/characters/{id}/custom-skills/{slot}"},
		{"GET", "/characters/{id}/chapel"},
		{"POST", "/characters/{id}/chapel/pray"},
		{"POST", "/characters/{id}/chapel/donate"},
		{"GET", "/characters/{id}/secretshop"},
		{"POST", "/characters/{id}/secretshop/talk"},
		{"POST", "/characters/{id}/secretshop/inspect"},
		{"POST", "/characters/{id}/secretshop/puffpuff"},
		{"POST", "/characters/{id}/secretshop/purchase"},
		{"GET", "/characters/{id}/farm"},
		{"POST", "/characters/{id}/farm/plant"},
		{"POST", "/characters/{id}/farm/water"},
		{"POST", "/characters/{id}/farm/fertilize"},
		{"POST", "/characters/{id}/farm/harvest"},
		{"POST", "/characters/{id}/farm/clear"},
		{"GET", "/characters/{id}/collections/monsters"},
		{"GET", "/characters/{id}/collections/items"},
		{"GET", "/characters/{id}/lottery/tickets"},
		{"POST", "/characters/{id}/lottery/buy-raffle"},
		{"POST", "/characters/{id}/lottery/raffle"},
		{"POST", "/characters/{id}/lottery/buy-ticket"},
		{"POST", "/characters/{id}/lottery/claim"},
		{"GET", "/characters/{id}/casino"},
		{"POST", "/characters/{id}/casino/exchange"},
		{"POST", "/characters/{id}/casino/slot"},
		{"POST", "/characters/{id}/casino/highlow"},
		{"POST", "/characters/{id}/casino/doppel"},
		{"POST", "/characters/{id}/casino/poker"},
		{"GET", "/challenges/tiers"},
		{"GET", "/characters/{id}/challenges/records"},
		{"POST", "/characters/{id}/challenges/start"},
		{"POST", "/characters/{id}/challenges/advance"},
		{"POST", "/characters/{id}/challenges/retire"},
		{"GET", "/characters/{id}/bosses"},
		{"POST", "/characters/{id}/bosses/fight"},
		{"GET", "/characters/{id}/dungeons"},
		{"POST", "/characters/{id}/dungeons/start"},
		{"POST", "/characters/{id}/dungeons/move"},
		{"POST", "/characters/{id}/dungeons/escape"},
		{"GET", "/characters/{id}/pvp"},
		{"GET", "/characters/{id}/pvp/opponents"},
		{"POST", "/characters/{id}/pvp/fight"},
		{"GET", "/auctions"},
		{"GET", "/auctions/{id}"},
		{"POST", "/auctions"},
		{"POST", "/auctions/{id}/bid"},
		{"POST", "/auctions/{id}/buyout"},
		{"POST", "/auctions/{id}/cancel"},
		{"GET", "/eventplaza"},
		{"GET", "/eventplaza/merchant/items"},
		{"POST", "/eventplaza/merchant/purchase"},
		{"GET", "/eventplaza/banquets"},
		{"POST", "/eventplaza/banquets/{id}/toast"},
		{"GET", "/characters/{id}/secretshop"},
		{"POST", "/characters/{id}/secretshop/talk"},
		{"POST", "/characters/{id}/secretshop/inspect"},
		{"POST", "/characters/{id}/secretshop/puffpuff"},
		{"POST", "/characters/{id}/secretshop/purchase"},
		{"GET", "/tavern/menu"},
		{"GET", "/characters/{id}/tavern"},
		{"POST", "/characters/{id}/tavern/order"},
		{"POST", "/characters/{id}/tavern/delivery"},
		{"GET", "/characters/{id}/tavern/delivery"},
		{"DELETE", "/characters/{id}/tavern/delivery"},
		{"POST", "/characters/{id}/tavern/delivery/claim"},
		{"POST", "/characters/{id}/tavern/talk"},
		{"GET", "/characters/{id}/blackmarket"},
		{"POST", "/characters/{id}/blackmarket/purchase"},
		{"POST", "/characters/{id}/blackmarket/sell"},
		{"POST", "/characters/{id}/blackmarket/talk"},
		{"POST", "/characters/{id}/blackmarket/rumors"},
		{"GET", "/characters/{id}/delivery/quests"},
		{"GET", "/characters/{id}/delivery/active"},
		{"POST", "/characters/{id}/delivery/accept"},
		{"POST", "/characters/{id}/delivery/complete"},
		{"POST", "/characters/{id}/delivery/cancel"},
		{"POST", "/characters/{id}/delivery/parcels/send"},
		{"GET", "/characters/{id}/delivery/parcels/incoming"},
		{"POST", "/characters/{id}/delivery/parcels/claim"},
		{"POST", "/characters/{id}/delivery/parcels/cancel"},
		{"GET", "/fleamarket/listings"},
		{"GET", "/fleamarket/listings/{listing_id}"},
		{"GET", "/characters/{id}/fleamarket/listings"},
		{"POST", "/characters/{id}/fleamarket/listings"},
		{"POST", "/characters/{id}/fleamarket/listings/{listing_id}/purchase"},
		{"DELETE", "/characters/{id}/fleamarket/listings/{listing_id}"},
	}

	for _, route := range expectedRoutes {
		pathItem, exists := spec.Paths[route.Path]
		if !exists {
			t.Errorf("missing path in OpenAPI spec: %s", route.Path)
			continue
		}

		methodKey := strings.ToLower(route.Method)
		op, hasMethod := pathItem[methodKey]
		if !hasMethod || op == nil {
			t.Errorf("missing %s method for path %s in OpenAPI spec", route.Method, route.Path)
			continue
		}

		opMap, ok := op.(map[string]interface{})
		if !ok {
			t.Errorf("operation object for %s %s is invalid", route.Method, route.Path)
			continue
		}

		if summary, ok := opMap["summary"].(string); !ok || summary == "" {
			t.Errorf("operation %s %s missing summary", route.Method, route.Path)
		}

		if opID, ok := opMap["operationId"].(string); !ok || opID == "" {
			t.Errorf("operation %s %s missing operationId", route.Method, route.Path)
		}

		if responses, ok := opMap["responses"].(map[string]interface{}); !ok || len(responses) == 0 {
			t.Errorf("operation %s %s missing responses", route.Method, route.Path)
		}
	}
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

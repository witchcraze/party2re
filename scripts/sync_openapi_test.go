package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractRegisteredRoutes(t *testing.T) {
	tempDir := t.TempDir()
	handlerFile := filepath.Join(tempDir, "handler.go")

	content := `package testpkg

import "net/http"

type Handler struct{}

func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("POST /characters/{id}/items", h.handleAddItem)
	return mux
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {}
func (h *Handler) handleAddItem(w http.ResponseWriter, r *http.Request) {}
`
	if err := os.WriteFile(handlerFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write dummy handler: %v", err)
	}

	routes, err := extractRegisteredRoutes(handlerFile)
	if err != nil {
		t.Fatalf("extractRegisteredRoutes failed: %v", err)
	}

	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	if routes[0].Method != "GET" || routes[0].Path != "/health" {
		t.Errorf("unexpected route 0: %+v", routes[0])
	}
	if routes[1].Method != "POST" || routes[1].Path != "/characters/{id}/items" {
		t.Errorf("unexpected route 1: %+v", routes[1])
	}
}

func TestScaffoldAndBundle(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "base.json")
	pathsDir := filepath.Join(tempDir, "paths")
	if err := os.MkdirAll(pathsDir, 0755); err != nil {
		t.Fatalf("failed to create paths dir: %v", err)
	}

	baseContent := `{
  "openapi": "3.1.0",
  "info": {"title": "Test API", "version": "1.0.0"},
  "components": {}
}`
	if err := os.WriteFile(basePath, []byte(baseContent), 0644); err != nil {
		t.Fatalf("failed to write base.json: %v", err)
	}

	// Scaffold missing route
	testRoutes := []Route{
		{Method: "POST", Path: "/characters/{id}/tokens"},
	}

	count, err := scaffoldMissingRoutes(pathsDir, testRoutes)
	if err != nil {
		t.Fatalf("scaffoldMissingRoutes failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 scaffolded route, got %d", count)
	}

	// Verify character.json was created
	charFile := filepath.Join(pathsDir, "character.json")
	charData, err := os.ReadFile(charFile)
	if err != nil {
		t.Fatalf("failed to read %s: %v", charFile, err)
	}

	var charPaths map[string]map[string]interface{}
	if err := json.Unmarshal(charData, &charPaths); err != nil {
		t.Fatalf("failed to unmarshal character.json: %v", err)
	}

	postOp, exists := charPaths["/characters/{id}/tokens"]["post"]
	if !exists {
		t.Fatalf("missing POST /characters/{id}/tokens in character.json")
	}

	postMap, ok := postOp.(map[string]interface{})
	if !ok {
		t.Fatalf("operation is not a map")
	}

	if postMap["operationId"] != "postCharactersIdTokens" {
		t.Errorf("expected operationId 'postCharactersIdTokens', got %q", postMap["operationId"])
	}

	tags, ok := postMap["tags"].([]interface{})
	if !ok || len(tags) == 0 || tags[0] != "Character" {
		t.Errorf("expected tag ['Character'], got %v", postMap["tags"])
	}

	responses, ok := postMap["responses"].(map[string]interface{})
	if !ok || len(responses) < 5 {
		t.Errorf("expected standard responses, got %v", postMap["responses"])
	}

	// Test bundling
	bundled, pathCount, routeCount, err := bundleOpenAPISpec(basePath, pathsDir)
	if err != nil {
		t.Fatalf("bundleOpenAPISpec failed: %v", err)
	}

	if pathCount != 1 || routeCount != 1 {
		t.Errorf("expected 1 path and 1 route, got %d and %d", pathCount, routeCount)
	}

	if !strings.Contains(string(bundled), "/characters/{id}/tokens") {
		t.Errorf("bundled output missing /characters/{id}/tokens: %s", string(bundled))
	}
}

func TestDuplicatePathDetection(t *testing.T) {
	tempDir := t.TempDir()
	basePath := filepath.Join(tempDir, "base.json")
	pathsDir := filepath.Join(tempDir, "paths")
	if err := os.MkdirAll(pathsDir, 0755); err != nil {
		t.Fatalf("failed to create paths dir: %v", err)
	}

	baseContent := `{"openapi": "3.1.0", "info": {"title": "Test"}, "components": {}}`
	if err := os.WriteFile(basePath, []byte(baseContent), 0644); err != nil {
		t.Fatalf("failed to write base.json: %v", err)
	}

	file1 := filepath.Join(pathsDir, "mod1.json")
	file2 := filepath.Join(pathsDir, "mod2.json")

	content1 := `{"/shared/path": {"get": {"summary": "Mod 1"}}}`
	content2 := `{"/shared/path": {"post": {"summary": "Mod 2"}}}`

	_ = os.WriteFile(file1, []byte(content1), 0644)
	_ = os.WriteFile(file2, []byte(content2), 0644)

	_, _, _, err := bundleOpenAPISpec(basePath, pathsDir)
	if err == nil {
		t.Fatal("expected error on duplicate path, got nil")
	}

	if !strings.Contains(err.Error(), "duplicate path") {
		t.Errorf("expected duplicate path error, got: %v", err)
	}
}

func TestCheckRouteCoverageMissing(t *testing.T) {
	bundledData := []byte(`{
  "openapi": "3.1.0",
  "paths": {
    "/health": {
      "get": {"summary": "Health"}
    }
  }
}`)

	routes := []Route{
		{Method: "GET", Path: "/health"},
		{Method: "POST", Path: "/missing"},
	}

	missing := checkRouteCoverage(bundledData, routes)
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing route, got %d", len(missing))
	}
	if missing[0].Method != "POST" || missing[0].Path != "/missing" {
		t.Errorf("unexpected missing route: %+v", missing[0])
	}
}

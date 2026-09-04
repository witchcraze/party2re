package http_test

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
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
		t.Error("docs/api/openapi.json and internal/api/http/openapi.json must be identical; please sync them with 'make openapi-sync'")
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

func TestOpenAPIModularSpecifications(t *testing.T) {
	rootDir := getRepoRootDir(t)
	basePath := filepath.Join(rootDir, "docs", "api", "base.json")
	pathsDir := filepath.Join(rootDir, "docs", "api", "paths")

	baseData, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("failed to read docs/api/base.json: %v", err)
	}

	var baseSpec map[string]interface{}
	if err := json.Unmarshal(baseData, &baseSpec); err != nil {
		t.Fatalf("docs/api/base.json is not valid JSON: %v", err)
	}

	if v, _ := baseSpec["openapi"].(string); !strings.HasPrefix(v, "3.1.") {
		t.Errorf("expected base.json openapi version 3.1.x, got %q", v)
	}

	entries, err := os.ReadDir(pathsDir)
	if err != nil {
		t.Fatalf("failed to read docs/api/paths: %v", err)
	}

	pathCount := 0
	seenPaths := make(map[string]string)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(pathsDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("failed to read path file %s: %v", entry.Name(), err)
			continue
		}

		var filePaths map[string]map[string]interface{}
		if err := json.Unmarshal(data, &filePaths); err != nil {
			t.Errorf("invalid JSON in path file %s: %v", entry.Name(), err)
			continue
		}

		if len(filePaths) == 0 {
			t.Errorf("path file %s is empty", entry.Name())
		}

		for pathKey, methodMap := range filePaths {
			if existingFile, exists := seenPaths[pathKey]; exists {
				t.Errorf("duplicate path %q found in %s and %s", pathKey, existingFile, entry.Name())
			}
			seenPaths[pathKey] = entry.Name()
			pathCount++

			for method, opRaw := range methodMap {
				op, ok := opRaw.(map[string]interface{})
				if !ok {
					t.Errorf("operation for %s %s in %s is invalid", method, pathKey, entry.Name())
					continue
				}

				if summary, ok := op["summary"].(string); !ok || summary == "" {
					t.Errorf("operation %s %s in %s missing summary", method, pathKey, entry.Name())
				}
				if opID, ok := op["operationId"].(string); !ok || opID == "" {
					t.Errorf("operation %s %s in %s missing operationId", method, pathKey, entry.Name())
				}
				if responses, ok := op["responses"].(map[string]interface{}); !ok || len(responses) == 0 {
					t.Errorf("operation %s %s in %s missing responses", method, pathKey, entry.Name())
				}
			}
		}
	}

	if pathCount == 0 {
		t.Fatal("no path operations found in docs/api/paths/*.json")
	}
}

func TestOpenAPIRouteCoverage(t *testing.T) {
	specData := gamehttp.OpenAPISpec()
	var spec openAPISpec
	if err := json.Unmarshal(specData, &spec); err != nil {
		t.Fatalf("failed to unmarshal OpenAPI spec: %v", err)
	}

	rootDir := getRepoRootDir(t)
	handlerPath := filepath.Join(rootDir, "internal", "api", "http", "handler.go")

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, handlerPath, nil, 0)
	if err != nil {
		t.Fatalf("failed to parse handler.go: %v", err)
	}

	type routeItem struct {
		Method string
		Path   string
	}
	var registeredRoutes []routeItem

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Router" {
			return true
		}

		ast.Inspect(fn.Body, func(bn ast.Node) bool {
			call, ok := bn.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "HandleFunc" {
				return true
			}

			if len(call.Args) >= 1 {
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				pattern := strings.Trim(lit.Value, "\"")
				parts := strings.SplitN(pattern, " ", 2)
				if len(parts) == 2 {
					registeredRoutes = append(registeredRoutes, routeItem{
						Method: parts[0],
						Path:   parts[1],
					})
				}
			}
			return true
		})
		return false
	})

	if len(registeredRoutes) == 0 {
		t.Fatal("no routes extracted from handler.go Router()")
	}

	for _, route := range registeredRoutes {
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

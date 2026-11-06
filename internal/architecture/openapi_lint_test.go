package architecture_test

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type openAPISpec struct {
	OpenAPI    string                            `json:"openapi"`
	Info       map[string]interface{}            `json:"info"`
	Paths      map[string]map[string]interface{} `json:"paths"`
	Components map[string]interface{}            `json:"components"`
}

// readEmbeddedOpenAPISpec reads the file internal/api/http embeds with go:embed.
func readEmbeddedOpenAPISpec(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../api/http/openapi.json")
	if err != nil {
		t.Fatalf("failed to read internal/api/http/openapi.json: %v", err)
	}
	return data
}

func TestOpenAPISpecificationValidity(t *testing.T) {
	rootDir := "../.."
	docsPath := filepath.Join(rootDir, "docs", "api", "openapi.json")

	docsData, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatalf("failed to read docs/api/openapi.json: %v", err)
	}

	embeddedData := readEmbeddedOpenAPISpec(t)
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
	rootDir := "../.."
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
	specData := readEmbeddedOpenAPISpec(t)
	var spec openAPISpec
	if err := json.Unmarshal(specData, &spec); err != nil {
		t.Fatalf("failed to unmarshal OpenAPI spec: %v", err)
	}

	rootDir := "../.."
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

func TestOpenAPISecurityMatchesHandlers(t *testing.T) {
	packages, err := parser.ParseDir(token.NewFileSet(), "../api/http", func(file os.FileInfo) bool {
		return !strings.HasSuffix(file.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg := packages["http"]
	functions := map[string]*ast.FuncDecl{}
	for _, file := range pkg.Files {
		for _, declaration := range file.Decls {
			if fn, ok := declaration.(*ast.FuncDecl); ok {
				functions[fn.Name.Name] = fn
			}
		}
	}
	var spec openAPISpec
	if err := json.Unmarshal(readEmbeddedOpenAPISpec(t), &spec); err != nil {
		t.Fatal(err)
	}
	for _, route := range extractRegisteredRoutes(t, pkg) {
		seen := map[string]bool{}
		scheme := ""
		var inspect func(string)
		inspect = func(name string) {
			if seen[name] {
				return
			}
			seen[name] = true
			for _, call := range extractFunctionCalls(functions[name]) {
				switch {
				case call == "authenticateAdmin":
					scheme = "AdminApiKey"
				case strings.HasPrefix(call, "withAuthenticated"), call == "authenticatePlayer", call == "sessionIDFromRequest":
					if scheme == "" {
						scheme = "BearerAuth"
					}
				default:
					inspect(call)
				}
			}
		}
		inspect(route.HandlerName)
		want := []any{}
		if scheme != "" {
			want = append(want, map[string]any{scheme: []any{}})
		}
		// These handlers also have an explicit anonymous read path.
		if route.HandlerName == "handleGetHomeView" || route.HandlerName == "handleGetReplay" {
			want = append([]any{map[string]any{}}, want...)
		}
		op := spec.Paths[route.Path][strings.ToLower(route.Method)].(map[string]any)
		if !reflect.DeepEqual(op["security"], want) {
			t.Errorf("%s %s (%s): security %v, want %v", route.Method, route.Path, route.HandlerName, op["security"], want)
		}
	}
}

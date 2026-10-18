package http_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"

	gamehttp "github.com/witchcraze/party2re/internal/api/http"
)

func TestOpenAPIOperationContractsAreExplicit(t *testing.T) {
	var spec openAPISpec
	if err := json.Unmarshal(gamehttp.OpenAPISpec(), &spec); err != nil {
		t.Fatal(err)
	}
	for path, methods := range spec.Paths {
		for method, raw := range methods {
			op := raw.(map[string]any)
			label := method + " " + path
			if _, ok := op["security"]; !ok {
				t.Errorf("%s: explicitly declare protected or public security", label)
			}
			if strings.HasSuffix(op["summary"].(string), " endpoint") {
				t.Errorf("%s: replace scaffold summary", label)
			}
			for status, raw := range op["responses"].(map[string]any) {
				response := raw.(map[string]any)
				if status == "204" || status == "304" || response["$ref"] != nil {
					continue
				}
				content, ok := response["content"].(map[string]any)
				if !ok || len(content) == 0 {
					t.Errorf("%s %s: describe response payload", label, status)
				}
				for media, raw := range content {
					if raw.(map[string]any)["schema"] == nil {
						t.Errorf("%s %s %s: missing response schema", label, status, media)
					}
				}
			}
		}
	}
}

func TestOpenAPINoObjectScaffolds(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal(gamehttp.OpenAPISpec(), &root); err != nil {
		t.Fatal(err)
	}
	var walk func(any, string)
	walk = func(value any, path string) {
		switch node := value.(type) {
		case map[string]any:
			if node["type"] == "object" && node["properties"] == nil && node["additionalProperties"] == nil {
				t.Errorf("%s: object needs named properties or an explicit map contract", path)
			}
			for key, child := range node {
				walk(child, path+"/"+key)
			}
		case []any:
			for _, child := range node {
				walk(child, path)
			}
		}
	}
	walk(root, "")
}

func TestOpenAPISecurityMatchesHandlers(t *testing.T) {
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(file os.FileInfo) bool {
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
	if err := json.Unmarshal(gamehttp.OpenAPISpec(), &spec); err != nil {
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

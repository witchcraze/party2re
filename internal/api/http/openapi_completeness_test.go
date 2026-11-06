package http_test

import (
	"encoding/json"
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

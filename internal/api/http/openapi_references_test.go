package http_test

import (
	"encoding/json"
	"strings"
	"testing"

	gamehttp "github.com/witchcraze/party2re/internal/api/http"
)

func TestOpenAPIReferencesAndSecuritySchemes(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal(gamehttp.OpenAPISpec(), &root); err != nil {
		t.Fatal(err)
	}
	components := root["components"].(map[string]any)
	schemes := components["securitySchemes"].(map[string]any)
	var walk func(any)
	walk = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			if ref, ok := node["$ref"].(string); ok {
				if !strings.HasPrefix(ref, "#/") {
					t.Errorf("OpenAPI reference must resolve inside the bundled specification: %s", ref)
				} else {
					var target any = root
					for _, part := range strings.Split(ref[2:], "/") {
						key := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
						object, ok := target.(map[string]any)
						if !ok {
							t.Errorf("unresolved OpenAPI reference: %s", ref)
							break
						}
						target, ok = object[key]
						if !ok {
							t.Errorf("unresolved OpenAPI reference: %s", ref)
							break
						}
					}
				}
			}
			if requirements, ok := node["security"].([]any); ok {
				for _, requirement := range requirements {
					for name := range requirement.(map[string]any) {
						if _, ok := schemes[name]; !ok {
							t.Errorf("undefined OpenAPI security scheme: %s", name)
						}
					}
				}
			}
			for _, child := range node {
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(root)
}

package playercontext_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/validation"
)

// TestCatalog_TextKindsCoverStringParams requires a declared text kind for
// exactly the Gateway parameters whose schema holds a string.
func TestCatalog_TextKindsCoverStringParams(t *testing.T) {
	spec := loadOpenAPISpec(t)
	strings := map[string]map[string]bool{}
	for _, condition := range spec.Components.Schemas["CharacterActionRequest"].AllOf {
		var id string
		if condition.If == nil || condition.Then == nil || json.Unmarshal(condition.If.Properties["action"].Const, &id) != nil {
			t.Fatalf("malformed Gateway condition: %+v", condition)
		}
		then, err := resolveSchema(spec, *condition.Then)
		if err != nil {
			t.Fatal(err)
		}
		params, err := resolveSchema(spec, then.Properties["params"])
		if err != nil {
			t.Fatal(err)
		}
		for name, schema := range params.Properties {
			if holdsString(t, spec, schema) {
				if strings[id] == nil {
					strings[id] = map[string]bool{}
				}
				strings[id][name] = true
			}
		}
	}
	for id, names := range strings {
		for name := range names {
			if kind := playercontext.TextKinds[id][name]; kind != validation.TextLine && kind != validation.TextComment {
				t.Errorf("%s.%s: string parameter declares no text kind", id, name)
			}
		}
	}
	for id, kinds := range playercontext.TextKinds {
		for name := range kinds {
			if !strings[id][name] {
				t.Errorf("%s.%s: text kind declared for a non-string or unknown parameter", id, name)
			}
		}
	}
}

func holdsString(t *testing.T, spec *openAPISpec, schema openAPISchema) bool {
	t.Helper()
	schema, err := resolveSchema(spec, schema)
	if err != nil {
		t.Fatal(err)
	}
	if schema.Type == "string" {
		return true
	}
	if types, ok := schema.Type.([]any); ok && slices.Contains(types, any("string")) {
		return true
	}
	if schema.Items != nil && holdsString(t, spec, *schema.Items) {
		return true
	}
	for _, child := range schema.Properties {
		if holdsString(t, spec, child) {
			return true
		}
	}
	for _, child := range schema.AllOf {
		if holdsString(t, spec, child) {
			return true
		}
	}
	return false
}

package playercontext_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/playercontext"
)

// openAPISpec models the subset of OpenAPI 3.1 needed for schema drift detection.
type openAPISpec struct {
	Paths      map[string]map[string]openAPIOperation `json:"paths"`
	Components openAPIComponents                      `json:"components"`
}

type openAPIComponents struct {
	Schemas map[string]openAPISchema `json:"schemas"`
}

type openAPIOperation struct {
	OperationID string              `json:"operationId"`
	RequestBody *openAPIRequestBody `json:"requestBody"`
}

type openAPIRequestBody struct {
	Content map[string]openAPIMediaType `json:"content"`
}

type openAPIMediaType struct {
	Schema openAPISchema `json:"schema"`
}

type openAPISchema struct {
	Ref                  string                   `json:"$ref"`
	Type                 string                   `json:"type"`
	Items                *openAPISchema           `json:"items"`
	Properties           map[string]openAPISchema `json:"properties"`
	Required             []string                 `json:"required"`
	AllOf                []openAPISchema          `json:"allOf"`
	If                   *openAPISchema           `json:"if"`
	Then                 *openAPISchema           `json:"then"`
	Const                json.RawMessage          `json:"const"`
	AdditionalProperties json.RawMessage          `json:"additionalProperties"`
	Format               string                   `json:"format"`
	Minimum              json.Number              `json:"minimum"`
	Maximum              json.Number              `json:"maximum"`
	MinLength            int                      `json:"minLength"`
	MaxLength            int                      `json:"maxLength"`
	Pattern              string                   `json:"pattern"`
	Enum                 []any                    `json:"enum"`
}

var actionIDPattern = regexp.MustCompile(`^[a-z0-9]+_[a-z0-9_]+$`)

// findRepoRoot finds the repository root by locating go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found in parent directories")
		}
		dir = parent
	}
}

// loadOpenAPISpec loads and parses the authoritative openapi.json.
func loadOpenAPISpec(t *testing.T) *openAPISpec {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("failed to find repo root: %v", err)
	}

	specPath := filepath.Join(root, "docs", "api", "openapi.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("failed to read openapi.json at %s: %v", specPath, err)
	}

	var spec openAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("failed to parse openapi.json: %v", err)
	}
	return &spec
}

// resolveSchema follows local references and fails closed on missing/cyclic refs.
func resolveSchema(spec *openAPISpec, s openAPISchema) (openAPISchema, error) {
	seen := make(map[string]bool)
	for s.Ref != "" {
		if seen[s.Ref] {
			return openAPISchema{}, fmt.Errorf("cyclic schema reference %q", s.Ref)
		}
		seen[s.Ref] = true
		resolved, ok := spec.Components.Schemas[strings.TrimPrefix(s.Ref, "#/components/schemas/")]
		if !strings.HasPrefix(s.Ref, "#/components/schemas/") || !ok {
			return openAPISchema{}, fmt.Errorf("unresolved schema reference %q", s.Ref)
		}
		s = resolved
	}
	return s, nil
}

// gatewayParams selects one documented action condition, never the shared envelope.
func gatewayParams(act playercontext.ActionDefinition, spec *openAPISpec, envelope openAPISchema) (openAPISchema, error) {
	var matches []openAPISchema
	for i, condition := range envelope.AllOf {
		if condition.If == nil {
			return openAPISchema{}, fmt.Errorf("condition %d: malformed action discriminator", i)
		}
		selector, err := resolveSchema(spec, *condition.If)
		if err != nil {
			return openAPISchema{}, err
		}
		var id string
		if selector.Type != "object" || len(selector.Properties) != 1 || len(selector.Required) != 1 || selector.Required[0] != "action" || json.Unmarshal(selector.Properties["action"].Const, &id) != nil || !actionIDPattern.MatchString(id) {
			return openAPISchema{}, fmt.Errorf("condition %d: malformed action discriminator", i)
		}
		if id == act.ID {
			matches = append(matches, condition)
		}
	}
	if len(matches) != 1 {
		return openAPISchema{}, fmt.Errorf("found %d command conditions; require exactly one", len(matches))
	}
	if matches[0].Then == nil {
		return openAPISchema{}, fmt.Errorf("missing object then schema")
	}
	then, err := resolveSchema(spec, *matches[0].Then)
	if err != nil {
		return openAPISchema{}, err
	}
	if then.Type != "object" {
		return openAPISchema{}, fmt.Errorf("missing object then schema")
	}
	params, ok := then.Properties["params"]
	if !ok {
		return openAPISchema{}, fmt.Errorf("missing params schema")
	}
	if slices.Contains(then.Required, "params") != (len(act.RequiredParams) > 0) {
		return openAPISchema{}, fmt.Errorf("params envelope requirement does not match required_params %v", act.RequiredParams)
	}
	params, err = resolveSchema(spec, params)
	if err != nil {
		return openAPISchema{}, err
	}
	if params.Type != "object" || string(params.AdditionalProperties) != "false" {
		return openAPISchema{}, fmt.Errorf("params must be a strict object (additionalProperties: false)")
	}
	for name := range params.Properties {
		if strings.EqualFold(name, "character_id") || strings.EqualFold(name, "player_id") {
			return openAPISchema{}, fmt.Errorf("actor identity input %q is forbidden in params", name)
		}
	}
	for _, name := range params.Required {
		if strings.EqualFold(name, "character_id") || strings.EqualFold(name, "player_id") {
			return openAPISchema{}, fmt.Errorf("actor identity input %q is forbidden in params", name)
		}
	}
	return params, nil
}

// extractOperations indexes all OpenAPI operations by operationId.
func extractOperations(spec *openAPISpec) map[string]openAPIOperation {
	ops := make(map[string]openAPIOperation)
	for _, methods := range spec.Paths {
		for _, op := range methods {
			if op.OperationID != "" {
				ops[op.OperationID] = op
			}
		}
	}
	return ops
}

// verifyActionDrift inspects an ActionDefinition against known OpenAPI operations.
// Returns a slice of descriptive error messages if any schema drift or naming violations are found.
func verifyActionDrift(act playercontext.ActionDefinition, ops map[string]openAPIOperation, spec *openAPISpec) []string {
	var errs []string

	// 1. ActionID Naming Check
	if !actionIDPattern.MatchString(act.ID) {
		errs = append(errs, fmt.Sprintf("action %q has invalid ID format: must match %s", act.ID, actionIDPattern.String()))
	}

	// 2. OpenAPI Operation Existence Check
	op, ok := ops[act.OperationID]
	if !ok {
		errs = append(errs, fmt.Sprintf("action %q references OperationID %q which does not exist in openapi.json", act.ID, act.OperationID))
		return errs
	}

	// 3. Request Body Schema & Parameter Completeness Check
	if op.RequestBody == nil {
		if len(act.RequiredParams) > 0 || act.OperationID == "executeCharacterAction" {
			errs = append(errs, fmt.Sprintf("action %q (operationId: %q) specifies required_params %v, but OpenAPI operation has no requestBody defined",
				act.ID, act.OperationID, act.RequiredParams))
		}
		return errs
	}

	mediaType, hasJSON := op.RequestBody.Content["application/json"]
	if !hasJSON {
		if len(act.RequiredParams) > 0 || act.OperationID == "executeCharacterAction" {
			errs = append(errs, fmt.Sprintf("action %q (operationId: %q) specifies required_params %v, but OpenAPI requestBody missing application/json content",
				act.ID, act.OperationID, act.RequiredParams))
		}
		return errs
	}

	schema, err := resolveSchema(spec, mediaType.Schema)
	if err == nil && act.OperationID == "executeCharacterAction" {
		schema, err = gatewayParams(act, spec, schema)
	}
	if err != nil {
		return append(errs, fmt.Sprintf("action %q (operationId: %q): %v", act.ID, act.OperationID, err))
	}

	// Verify all schema-required properties (excluding server-injected character_id) are declared.
	for _, reqField := range schema.Required {
		if reqField == "character_id" {
			continue
		}
		if !slices.Contains(act.RequiredParams, reqField) {
			errs = append(errs, fmt.Sprintf("action %q (operationId: %q) missing required_param %q declared in OpenAPI requestBody schema",
				act.ID, act.OperationID, reqField))
		}
	}

	definedProps := make([]string, 0, len(schema.Properties))
	for p := range schema.Properties {
		definedProps = append(definedProps, p)
	}
	sort.Strings(definedProps)

	for _, reqParam := range act.RequiredParams {
		// Server-injected identity must not be declared as a user-supplied command parameter
		if reqParam == "character_id" {
			errs = append(errs, fmt.Sprintf("action %q (operationId: %q) specifies server-injected identity parameter %q in required_params",
				act.ID, act.OperationID, reqParam))
			continue
		}

		// Check if property is defined in requestBody schema
		if _, exists := schema.Properties[reqParam]; !exists {
			errs = append(errs, fmt.Sprintf("action %q (operationId: %q) specifies required_param %q, but OpenAPI requestBody schema only defines properties %v",
				act.ID, act.OperationID, reqParam, definedProps))
			continue
		}

		// Check if property is marked as required in schema
		if !slices.Contains(schema.Required, reqParam) {
			errs = append(errs, fmt.Sprintf("action %q (operationId: %q) specifies required_param %q, but it is not marked as required in OpenAPI requestBody schema (required: %v)",
				act.ID, act.OperationID, reqParam, schema.Required))
		}
	}

	return errs
}

// TestCatalog_OpenAPIDrift verifies that the canonical action catalog in catalog.go strictly
// matches OpenAPI 3.1 specifications without schema drift, missing operations, or mismatched required parameters.
func TestCatalog_OpenAPIDrift(t *testing.T) {
	spec := loadOpenAPISpec(t)
	ops := extractOperations(spec)
	actions := playercontext.AllActions()

	seenIDs := make(map[string]bool)

	for _, act := range actions {
		if seenIDs[act.ID] {
			t.Errorf("duplicate action ID: %s", act.ID)
		}
		seenIDs[act.ID] = true

		errs := verifyActionDrift(act, ops, spec)
		for _, errStr := range errs {
			t.Errorf("[Drift Detected] %s", errStr)
		}
	}
}

// TestCatalog_DriftDetection_Diagnostics tests that the drift verification logic
// accurately catches various schema discrepancies with clear diagnostic messages.
func TestCatalog_DriftDetection_Diagnostics(t *testing.T) {
	mockSpec := &openAPISpec{
		Paths: map[string]map[string]openAPIOperation{
			"/test/deposit": {
				"post": {
					OperationID: "testDeposit",
					RequestBody: &openAPIRequestBody{
						Content: map[string]openAPIMediaType{
							"application/json": {
								Schema: openAPISchema{
									Type: "object",
									Properties: map[string]openAPISchema{
										"amount": {Type: "integer"},
									},
									Required: []string{"amount"},
								},
							},
						},
					},
				},
			},
			"/test/optional": {
				"post": {
					OperationID: "testOptional",
					RequestBody: &openAPIRequestBody{
						Content: map[string]openAPIMediaType{
							"application/json": {
								Schema: openAPISchema{
									Type: "object",
									Properties: map[string]openAPISchema{
										"amount": {Type: "integer"},
									},
									Required: []string{}, // amount is optional
								},
							},
						},
					},
				},
			},
			"/test/multi": {
				"post": {
					OperationID: "testMulti",
					RequestBody: &openAPIRequestBody{
						Content: map[string]openAPIMediaType{
							"application/json": {
								Schema: openAPISchema{
									Type: "object",
									Properties: map[string]openAPISchema{
										"photo_id": {Type: "string"},
										"title":    {Type: "string"},
									},
									Required: []string{"photo_id", "title"},
								},
							},
						},
					},
				},
			},
			"/test/identity": {
				"post": {
					OperationID: "testIdentity",
					RequestBody: &openAPIRequestBody{
						Content: map[string]openAPIMediaType{
							"application/json": {
								Schema: openAPISchema{
									Type: "object",
									Properties: map[string]openAPISchema{
										"character_id": {Type: "string"},
										"target_id":    {Type: "string"},
									},
									Required: []string{"character_id", "target_id"},
								},
							},
						},
					},
				},
			},
		},
		Components: openAPIComponents{
			Schemas: map[string]openAPISchema{},
		},
	}

	ops := extractOperations(mockSpec)

	tests := []struct {
		name        string
		act         playercontext.ActionDefinition
		wantSubstrs []string
	}{
		{
			name: "Valid action with matched parameters",
			act: playercontext.ActionDefinition{
				ID:             "bank_deposit",
				OperationID:    "testDeposit",
				RequiredParams: []string{"amount"},
			},
			wantSubstrs: nil,
		},
		{
			name: "Invalid action ID format",
			act: playercontext.ActionDefinition{
				ID:             "InvalidNamingPattern",
				OperationID:    "testDeposit",
				RequiredParams: []string{"amount"},
			},
			wantSubstrs: []string{"has invalid ID format: must match"},
		},
		{
			name: "Missing OperationID in OpenAPI",
			act: playercontext.ActionDefinition{
				ID:             "bank_missing",
				OperationID:    "nonExistentOperation",
				RequiredParams: []string{"amount"},
			},
			wantSubstrs: []string{"which does not exist in openapi.json"},
		},
		{
			name: "Parameter typo or mismatch",
			act: playercontext.ActionDefinition{
				ID:             "bank_deposit",
				OperationID:    "testDeposit",
				RequiredParams: []string{"amt"},
			},
			wantSubstrs: []string{`specifies required_param "amt", but OpenAPI requestBody schema only defines properties [amount]`},
		},
		{
			name: "Parameter defined but not required in OpenAPI",
			act: playercontext.ActionDefinition{
				ID:             "bank_optional",
				OperationID:    "testOptional",
				RequiredParams: []string{"amount"},
			},
			wantSubstrs: []string{`specifies required_param "amount", but it is not marked as required in OpenAPI requestBody schema`},
		},
		{
			name: "Empty required parameters when schema requires fields",
			act: playercontext.ActionDefinition{
				ID:             "bank_empty",
				OperationID:    "testDeposit",
				RequiredParams: []string{},
			},
			wantSubstrs: []string{`missing required_param "amount" declared in OpenAPI requestBody schema`},
		},
		{
			name: "Partially incomplete required parameters",
			act: playercontext.ActionDefinition{
				ID:             "contest_partial",
				OperationID:    "testMulti",
				RequiredParams: []string{"photo_id"},
			},
			wantSubstrs: []string{`missing required_param "title" declared in OpenAPI requestBody schema`},
		},
		{
			name: "Valid action with server-injected identity parameter excluded",
			act: playercontext.ActionDefinition{
				ID:             "action_identity_valid",
				OperationID:    "testIdentity",
				RequiredParams: []string{"target_id"},
			},
			wantSubstrs: nil,
		},
		{
			name: "Action erroneously declares server-injected identity parameter",
			act: playercontext.ActionDefinition{
				ID:             "action_identity_invalid",
				OperationID:    "testIdentity",
				RequiredParams: []string{"character_id", "target_id"},
			},
			wantSubstrs: []string{`specifies server-injected identity parameter "character_id" in required_params`},
		},
		{
			name: "Valid action with empty parameters when schema requires nothing",
			act: playercontext.ActionDefinition{
				ID:             "action_no_params",
				OperationID:    "testOptional",
				RequiredParams: []string{},
			},
			wantSubstrs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := verifyActionDrift(tt.act, ops, mockSpec)
			if len(tt.wantSubstrs) == 0 {
				if len(errs) > 0 {
					t.Fatalf("expected no errors, got: %v", errs)
				}
				return
			}

			if len(errs) == 0 {
				t.Fatalf("expected errors containing %v, but got none", tt.wantSubstrs)
			}

			combined := strings.Join(errs, "\n")
			for _, substr := range tt.wantSubstrs {
				if !strings.Contains(combined, substr) {
					t.Errorf("error output %q does not contain expected substring %q", combined, substr)
				}
			}
		})
	}
}

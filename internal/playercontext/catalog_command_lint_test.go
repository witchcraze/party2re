package playercontext_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/playercontext"
)

func gatewayContract(t *testing.T, spec *openAPISpec, id string) (openAPISchema, openAPISchema) {
	t.Helper()
	for _, condition := range spec.Components.Schemas["CharacterActionRequest"].AllOf {
		if condition.If != nil && string(condition.If.Properties["action"].Const) == `"`+id+`"` && condition.Then != nil {
			return *condition.Then, condition.Then.Properties["params"]
		}
	}
	t.Fatalf("missing Gateway condition for %s", id)
	return openAPISchema{}, openAPISchema{}
}

func TestCatalog_GatewayContracts(t *testing.T) {
	spec := loadOpenAPISpec(t)
	tests := []struct {
		id, field, kind string
		required        bool
	}{
		{"bank_deposit", "amount", "integer", true},
		{"bank_withdraw", "amount", "integer", true},
		{"adventure_start", "stage_id", "string", true},
		{"rescue_request", "reason", "string", true},
		{"home_sleep", "target_home_id", "string", false},
		{"home_wake", "", "", false},
		{"depot_expand", "", "", false},
		{"depot_sort", "", "", false},
		{"depot_sell", "item_id", "string", true},
		{"depot_sell_batch", "item_ids", "array", true},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			act, ok := playercontext.GetAction(tc.id)
			if !ok || act.OperationID != "executeCharacterAction" {
				t.Fatalf("command is not linked to Gateway: %+v", act)
			}
			then, params := gatewayContract(t, spec, tc.id)
			if params.Type != "object" || string(params.AdditionalProperties) != "false" || slices.Contains(then.Required, "params") != tc.required {
				t.Fatalf("incorrect strict params/envelope: %+v / %+v", params, then)
			}
			if tc.field == "" {
				if len(params.Properties) != 0 || len(params.Required) != 0 {
					t.Fatal("command must accept only empty params")
				}
				return
			}
			field := params.Properties[tc.field]
			if len(params.Properties) != 1 || field.Type != tc.kind || field.MinLength != 0 || slices.Contains(params.Required, tc.field) != tc.required {
				t.Fatalf("incorrect input contract: %+v", params)
			}
			if tc.kind == "integer" && (field.Format != "int64" || field.Minimum.String() != "-9223372036854775808" || field.Maximum.String() != "9223372036854775807") {
				t.Fatalf("amount must cover precisely signed int64: %+v", field)
			}
			if tc.kind == "array" && (field.Items == nil || field.Items.Type != "string") {
				t.Fatalf("sale IDs must be non-null strings: %+v", field)
			}
		})
	}
}

func TestCatalog_GatewayRetirementIndependent(t *testing.T) {
	spec := loadOpenAPISpec(t)
	retired := []string{"startAdventure", "requestEmergencyRescue", "homeSleep", "homeWake", "postCharactersIdBankDeposit", "postCharactersIdBankWithdraw", "purchaseSecretShopItem", "postCharactersIdDepotSell", "postCharactersIdDepotSellBatch", "postCharactersIdDepotSort"}
	for _, methods := range spec.Paths {
		for method, op := range methods {
			if slices.Contains(retired, op.OperationID) {
				delete(methods, method)
			}
		}
	}
	ops := extractOperations(spec)
	for _, id := range retired {
		if _, exists := ops[id]; exists {
			t.Fatalf("former REST operation %s still exists", id)
		}
	}
	for _, act := range playercontext.AllActions() {
		if errs := verifyActionDrift(act, ops, spec); len(errs) != 0 {
			t.Errorf("retirement broke %s: %v", act.ID, errs)
		}
		if act.OperationID != "executeCharacterAction" {
			withoutOperation := extractOperations(spec)
			delete(withoutOperation, act.OperationID)
			if errs := verifyActionDrift(act, withoutOperation, spec); len(errs) == 0 {
				t.Errorf("unmigrated %s escaped operation validation", act.ID)
			}
		}
	}
}

func TestCatalog_SecretShopGatewayContract(t *testing.T) {
	act, ok := playercontext.GetAction("secretshop_purchase")
	if !ok || act.OperationID != "executeCharacterAction" || !slices.Equal(act.RequiredParams, []string{"item_id", "quantity"}) {
		t.Fatalf("purchase not bound to explicit Gateway inputs: %+v", act)
	}
	then, params := gatewayContract(t, loadOpenAPISpec(t), act.ID)
	quantity := params.Properties["quantity"]
	if params.Type != "object" || string(params.AdditionalProperties) != "false" || !slices.Equal(then.Required, []string{"params"}) || !slices.Equal(params.Required, act.RequiredParams) || len(params.Properties) != 2 || params.Properties["item_id"].Type != "string" || quantity.Type != "integer" || quantity.Format != "int64" || quantity.Minimum.String() != "-9223372036854775808" || quantity.Maximum.String() != "9223372036854775807" {
		t.Fatalf("incorrect purchase contract: %+v / %+v", then, params)
	}
}

func TestCatalog_GatewayDriftDiagnostics(t *testing.T) {
	tests := []struct {
		name, id, want string
		mutate         func(*openAPISpec, *openAPISchema)
	}{
		{"missing condition", "bank_deposit", "0 command conditions", func(_ *openAPISpec, s *openAPISchema) { s.AllOf = nil }},
		{"duplicate condition", "bank_deposit", "2 command conditions", func(_ *openAPISpec, s *openAPISchema) { s.AllOf = append(s.AllOf, s.AllOf[0]) }},
		{"missing discriminator", "bank_deposit", "malformed action discriminator", func(_ *openAPISpec, s *openAPISchema) { delete(s.AllOf[0].If.Properties, "action") }},
		{"wrong discriminator type", "bank_deposit", "malformed action discriminator", func(_ *openAPISpec, s *openAPISchema) {
			s.AllOf[0].If.Properties["action"] = openAPISchema{Const: json.RawMessage(`123`)}
		}},
		{"optional discriminator", "bank_deposit", "malformed action discriminator", func(_ *openAPISpec, s *openAPISchema) { s.AllOf[0].If.Required = nil }},
		{"missing if", "bank_deposit", "malformed action discriminator", func(_ *openAPISpec, s *openAPISchema) { s.AllOf[0].If = nil }},
		{"missing then", "bank_deposit", "missing object then", func(_ *openAPISpec, s *openAPISchema) { s.AllOf[0].Then = nil }},
		{"missing params", "bank_deposit", "missing params schema", func(_ *openAPISpec, s *openAPISchema) { delete(s.AllOf[0].Then.Properties, "params") }},
		{"unresolved params", "bank_deposit", "unresolved schema reference", func(_ *openAPISpec, s *openAPISchema) {
			s.AllOf[0].Then.Properties["params"] = openAPISchema{Ref: "#/components/schemas/Missing"}
		}},
		{"cyclic params", "bank_deposit", "cyclic schema reference", func(spec *openAPISpec, s *openAPISchema) {
			spec.Components.Schemas["Cycle"] = openAPISchema{Ref: "#/components/schemas/Cycle"}
			s.AllOf[0].Then.Properties["params"] = spec.Components.Schemas["Cycle"]
		}},
		{"nonobject params", "bank_deposit", "params must be a strict object", func(_ *openAPISpec, s *openAPISchema) {
			p := s.AllOf[0].Then.Properties["params"]
			p.Type = "string"
			s.AllOf[0].Then.Properties["params"] = p
		}},
		{"unknown fields allowed", "bank_deposit", "params must be a strict object", func(_ *openAPISpec, s *openAPISchema) {
			p := s.AllOf[0].Then.Properties["params"]
			p.AdditionalProperties = nil
			s.AllOf[0].Then.Properties["params"] = p
		}},
		{"actor property", "bank_deposit", "actor identity input", func(_ *openAPISpec, s *openAPISchema) {
			s.AllOf[0].Then.Properties["params"].Properties["character_id"] = openAPISchema{Type: "string"}
		}},
		{"player property", "bank_deposit", "actor identity input", func(_ *openAPISpec, s *openAPISchema) {
			s.AllOf[0].Then.Properties["params"].Properties["Player_ID"] = openAPISchema{Type: "string"}
		}},
		{"required actor", "bank_deposit", "actor identity input", func(_ *openAPISpec, s *openAPISchema) {
			p := s.AllOf[0].Then.Properties["params"]
			p.Required = append(p.Required, "player_id")
			s.AllOf[0].Then.Properties["params"] = p
		}},
		{"optional envelope", "bank_deposit", "params envelope requirement", func(_ *openAPISpec, s *openAPISchema) { s.AllOf[0].Then.Required = nil }},
		{"required Home envelope", "home_wake", "params envelope requirement", func(_ *openAPISpec, s *openAPISchema) { s.AllOf[0].Then.Required = []string{"params"} }},
		{"required field removed", "bank_deposit", `not marked as required`, func(_ *openAPISpec, s *openAPISchema) {
			p := s.AllOf[0].Then.Properties["params"]
			p.Required = nil
			s.AllOf[0].Then.Properties["params"] = p
		}},
		{"required field added", "bank_deposit", `missing required_param "extra"`, func(_ *openAPISpec, s *openAPISchema) {
			p := s.AllOf[0].Then.Properties["params"]
			p.Required = append(p.Required, "extra")
			s.AllOf[0].Then.Properties["params"] = p
		}},
		{"required property removed", "bank_deposit", `only defines properties []`, func(_ *openAPISpec, s *openAPISchema) {
			delete(s.AllOf[0].Then.Properties["params"].Properties, "amount")
		}},
		{"missing requestBody for Wake", "home_wake", "no requestBody", func(spec *openAPISpec, _ *openAPISchema) {
			for _, methods := range spec.Paths {
				for method, op := range methods {
					if op.OperationID == "executeCharacterAction" {
						op.RequestBody = nil
						methods[method] = op
					}
				}
			}
		}},
		{"missing JSON for Wake", "home_wake", "missing application/json", func(spec *openAPISpec, _ *openAPISchema) {
			for _, op := range extractOperations(spec) {
				if op.OperationID == "executeCharacterAction" {
					delete(op.RequestBody.Content, "application/json")
				}
			}
		}},
		{"valid params ref", "bank_deposit", "", func(spec *openAPISpec, s *openAPISchema) {
			spec.Components.Schemas["DepositParams"] = s.AllOf[0].Then.Properties["params"]
			s.AllOf[0].Then.Properties["params"] = openAPISchema{Ref: "#/components/schemas/DepositParams"}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := loadOpenAPISpec(t)
			schema := spec.Components.Schemas["CharacterActionRequest"]
			for _, condition := range schema.AllOf {
				if condition.If != nil && string(condition.If.Properties["action"].Const) == `"`+tc.id+`"` {
					schema.AllOf = []openAPISchema{condition}
					break
				}
			}
			if len(schema.AllOf) != 1 {
				t.Fatal("missing command contract", tc.id)
			}
			tc.mutate(spec, &schema)
			spec.Components.Schemas["CharacterActionRequest"] = schema
			act, ok := playercontext.GetAction(tc.id)
			if !ok {
				t.Fatal("missing action", tc.id)
			}
			errs := verifyActionDrift(act, extractOperations(spec), spec)
			combined := strings.Join(errs, "\n")
			if tc.want == "" {
				if len(errs) != 0 {
					t.Fatal(errs)
				}
				return
			}
			if !strings.Contains(combined, tc.want) || !strings.Contains(combined, tc.id) {
				t.Fatalf("expected diagnostic %q for %s, got %v", tc.want, tc.id, errs)
			}
		})
	}
}

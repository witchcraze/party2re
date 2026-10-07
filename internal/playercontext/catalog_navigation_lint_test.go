package playercontext_test

import (
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestCatalogNavigationContracts(t *testing.T) {
	spec := loadOpenAPISpec(t)
	for _, id := range []string{"scene_enter", "scene_select", "scene_page", "scene_back"} {
		act, ok := playercontext.GetAction(id)
		if !ok || act.OperationID != "executeCharacterAction" {
			t.Fatalf("missing Gateway navigation: %+v", act)
		}
		then, params := gatewayContract(t, spec, id)
		if params.Type != "object" || string(params.AdditionalProperties) != "false" || slices.Contains(then.Required, "params") != (id != "scene_back") {
			t.Fatalf("non-strict navigation contract %s: %+v", id, params)
		}
		if !slices.Equal(params.Required, act.RequiredParams) {
			t.Fatalf("required inputs for %s: %v", id, params.Required)
		}
		switch id {
		case "scene_enter":
			if !slices.Equal(params.Properties["destination"].Enum, []any{"town", "bank", "home", "shop_weapon", "shop_armor", "shop_item", "shop_accessory"}) {
				t.Fatal("destination registry drift")
			}
		case "scene_select":
			if !slices.Equal(params.Properties["target_kind"].Enum, []any{"item"}) || params.Properties["target_id"].Pattern != `^[a-zA-Z0-9_-]{1,128}$` {
				t.Fatal("subject registry/bound drift")
			}
		case "scene_page":
			if params.Properties["offset"].Type != "integer" || params.Properties["offset"].Minimum.String() != "0" || params.Properties["offset"].Maximum.String() != "1000000" || params.Properties["limit"].Minimum.String() != "1" || params.Properties["limit"].Maximum.String() != "100" {
				t.Fatal("page bounds drift")
			}
		case "scene_back":
			if len(params.Properties) != 0 {
				t.Fatal("back must not accept a caller-supplied parent")
			}
		}
	}
	if spec.Components.Schemas["NavigationSubject"].Properties["target_id"].MaxLength != 128 {
		t.Fatal("unbounded saved target")
	}
}

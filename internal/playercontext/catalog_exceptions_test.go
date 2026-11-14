package playercontext

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/scheduling"
)

func TestEvaluate_CatalogExceptionSnapshots(t *testing.T) {
	cases := map[string]Snapshot{}
	ordinary := healthySnapshot()
	cases["ordinary"] = ordinary
	sleeping := ordinary
	sleeping.Sleeping, sleeping.SleepRemaining = true, time.Minute
	cases["sleeping"] = sleeping
	wakeable := ordinary
	wakeable.Sleeping, wakeable.CanWake, wakeable.SceneActions = true, true, []string{}
	cases["wakeable outside home"] = wakeable
	active := ordinary
	active.Activities = []Activity{{Kind: "gvg", ID: "room", Actions: []string{"gvg_advance", "gvg_leave"}}}
	cases["active"] = active
	conflict := active
	conflict.OngoingActions = []scheduling.ScheduledAction{{State: scheduling.StateProcessing}}
	cases["conflict"] = conflict
	var want map[string][]string
	raw, err := os.ReadFile("testdata/catalog_exception_snapshots.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(cases) {
		t.Fatal("snapshot case mismatch")
	}
	for name, snapshot := range cases {
		t.Run(name, func(t *testing.T) {
			if got := Evaluate(snapshot); !reflect.DeepEqual(got, want[name]) {
				t.Fatalf("catalog actions = %v, want %v", got, want[name])
			}
		})
	}
}

func TestCatalog_ExceptionAttributes(t *testing.T) {
	for _, def := range DefaultCatalog {
		wake := def.ID == "home_wake"
		recovery := wake || def.ID == "rescue_request"
		exempt := recovery || def.ID == "presence_refresh"
		global := exempt || def.ID == "player_inspect"
		if def.ActivityExempt != exempt || def.GlobalSupport != global || def.RequiresWake != wake || (def.ActivityKind == "" && def.Recovery != recovery) {
			t.Errorf("incorrect exception attributes: %+v", def)
		}
		if exempt && def.RequiredGates.Has(GateSleepCheck) {
			t.Errorf("%s must bypass the ordinary sleep gate", def.ID)
		}
		raw, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"ActivityExempt", "GlobalSupport", "RequiresWake"} {
			if fields[name] != nil {
				t.Errorf("%s exposes internal attribute %s", def.ID, name)
			}
		}
	}
}

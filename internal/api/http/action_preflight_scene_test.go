package http

import (
	"testing"

	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestSceneCommandsQueryOnlyForPreflightAndRefresh(t *testing.T) {
	for _, tc := range []struct {
		action, destination, params string
		rows                        int
	}{
		{"place_say", "town", `{"text":"hello"}`, 1},
		{"place_whisper", "town", `{"text":"hello","recipient_name":"Bob"}`, 1},
		{"npc_talk", "town", `{}`, 2},
		{"npc_inspect", "town", `{}`, 2},
		{"park_divinate", "town", `{}`, 2},
		{"secretshop_puffpuff", "secretshop", `{}`, 2},
	} {
		t.Run(tc.action, func(t *testing.T) {
			log := &fakePlaceLog{}
			f, store, _, router := secretShopSceneRouterWith(t, []Option{WithPlaceLog(log), WithCharacterNames(fakeCharacterNames{"Bob": "bob"})})
			store.selection = playercontext.Selection{Destination: tc.destination}
			f.char.JobLevel = 7
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+tc.action+`","params":`+tc.params+`}`, nil)
			if status != 200 || len(log.rows) != tc.rows {
				t.Fatalf("status=%d rows=%d: %s", status, len(log.rows), raw["error"])
			}
			if store.loads != 2 {
				t.Fatalf("Query calls=%d, want preflight + response refresh only", store.loads)
			}
		})
	}
}

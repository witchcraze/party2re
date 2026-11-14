package http

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestSceneExceptionSupport(t *testing.T) {
	type offer struct {
		mode  string
		entry bool
	}
	for _, state := range []string{"ordinary", "sleeping", "wakeable", "active", "conflict"} {
		t.Run(state, func(t *testing.T) {
			f := newGatewayFixture(t)
			timers := timer.NewService(nil)
			var facts []playercontext.Activity
			want := map[string]offer{"player_inspect": {entry: true}, "home_wake": {}, "rescue_request": {entry: true}, "presence_refresh": {entry: true}}
			if state == "sleeping" || state == "wakeable" {
				if err := timers.SetLock(context.Background(), timer.CategoryAsleep, "hero", time.Hour); err != nil {
					t.Fatal(err)
				}
				want = map[string]offer{"rescue_request": {mode: "recovery"}, "presence_refresh": {entry: true}}
				if state == "sleeping" {
					if err := timers.SetLock(context.Background(), timer.CategorySleep, "hero", time.Hour); err != nil {
						t.Fatal(err)
					}
				} else {
					want["home_wake"] = offer{mode: "recovery"}
				}
			}
			if state == "active" || state == "conflict" {
				facts = []playercontext.Activity{{Kind: "gvg", ID: "room", Actions: []string{"gvg_advance", "gvg_leave"}}}
				want = map[string]offer{"rescue_request": {mode: "recovery"}, "presence_refresh": {entry: true}, "gvg_leave": {mode: "recovery"}}
				if state == "conflict" {
					f.actions = []scheduling.ScheduledAction{{State: scheduling.StateProcessing}}
				} else {
					want["gvg_advance"] = offer{mode: "continuation"}
				}
			}
			pc := playercontext.NewService(f, f, timers,
				playercontext.WithNavigation(&gatewayNavigationStore{}, playercontext.SceneDefinition{ID: "town"}),
				playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) { return facts, nil }))
			status, observed := navigationGET(t, f.router("rescue_request", timers, WithPlayerContext(pc)))
			if status != 200 {
				t.Fatalf("GET = %d", status)
			}
			got := map[string]offer{}
			for _, support := range observed.Scene.Support.Actions {
				// Navigation commands are outside the shared support contract.
				if state == "ordinary" && support.Action != "player_inspect" && support.Action != "home_wake" && support.Action != "rescue_request" && support.Action != "presence_refresh" {
					continue
				}
				got[support.Action] = offer{mode: support.Mode, entry: support.EntryEligible}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("support = %+v, want %+v", got, want)
			}
		})
	}
}

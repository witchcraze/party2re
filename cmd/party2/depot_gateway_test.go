package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

type depotGatewayPlayer struct {
	apihttp.PlayerService
	owner string
}

func (p depotGatewayPlayer) Authenticate(_ context.Context, token string) (coreplayer.Player, error) {
	if token != "session" {
		return coreplayer.Player{}, errors.New("invalid session")
	}
	return coreplayer.Player{ID: p.owner}, nil
}

type depotGatewayObservation struct {
	playercontext.NavigationRepository
}

func (depotGatewayObservation) Load(context.Context, string) (playercontext.Selection, error) {
	return playercontext.Selection{Destination: "depot"}, nil
}

func (depotGatewayObservation) FindPendingByActorID(context.Context, string) ([]scheduling.ScheduledAction, error) {
	return []scheduling.ScheduledAction{}, nil
}

func (f depotEquipmentFixture) gateway(t *testing.T) http.Handler {
	t.Helper()
	observation := depotGatewayObservation{}
	pc := playercontext.NewService(f.core.charRepo, observation, timer.NewService(nil),
		playercontext.WithNavigation(observation, playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "depot", Parent: "town", Pageable: true}))
	chars, err := character.NewService(f.core.charRepo)
	if err != nil {
		t.Fatal(err)
	}
	h, err := apihttp.NewHandler(depotGatewayPlayer{owner: f.sender.PlayerID}, chars,
		&struct{ apihttp.AdventureService }{}, &struct{ apihttp.ShopService }{}, apihttp.WithDepot(f.econ.depot), apihttp.WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return h.Router()
}

func (f depotEquipmentFixture) gatewayMove(router http.Handler, send bool) *httptest.ResponseRecorder {
	action, params := "depot_deposit", `{"item_id":"`+f.instance.ID+`"}`
	if send {
		action, params = "depot_send_item", `{"item_id":"`+f.instance.ID+`","recipient_character_id":"`+f.recipient.ID+`"}`
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/characters/"+f.sender.ID+"/actions", strings.NewReader(`{"action":"`+action+`","params":`+params+`}`))
	r.Header.Set("Authorization", "Bearer session")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestWireDepotGatewayMovement_Integration(t *testing.T) {
	for _, send := range []bool{false, true} {
		for _, failSave := range []bool{false, true} {
			t.Run(fmt.Sprintf("send=%t/failSave=%t", send, failSave), func(t *testing.T) {
				f := newDepotEquipmentFixture(t, item.SlotMainHand, true)
				if failSave {
					var err error
					f.econ.depot, err = depot.NewServiceWithTransaction(depotMovementStorageFailure{f.econ.depotRepo}, f.core.charRepo, f.core.invRepo, f.core.txProvider,
						depot.WithItemDefinitionProvider(f.core.itemCatalog), depot.WithEquipmentRepository(f.econ.equipRepo))
					if err != nil {
						t.Fatal(err)
					}
				}
				before := f.state(t)
				w := f.gatewayMove(f.gateway(t), send)
				if failSave {
					if w.Code != 500 || !strings.Contains(w.Body.String(), `"EXECUTION_FAILED"`) || strings.Contains(w.Body.String(), errDepotEquipmentPersistence.Error()) {
						t.Fatalf("save failure: %d %s", w.Code, w.Body.String())
					}
					if after := f.state(t); !reflect.DeepEqual(before, after) {
						t.Fatal("failed Gateway move changed assets or equipment")
					}
					return
				}
				var response struct {
					Success bool
					Context *apihttp.PlayerContextResponse
				}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if w.Code != 200 || !response.Success || response.Context == nil || response.Context.Scene.LocationID != "depot" {
					t.Fatalf("Gateway move: %d %s", w.Code, w.Body.String())
				}
				f.assertMoved(t, item.SlotMainHand, true, send)
			})
		}
	}
}

func TestWireDepotGatewayMovementConcurrency_Integration(t *testing.T) {
	f := newDepotEquipmentFixture(t, item.SlotMainHand, true)
	router := f.gateway(t)
	var deposits, sends atomic.Int64
	result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		send := (worker+op)%2 == 0
		w := f.gatewayMove(router, send)
		if w.Code == 404 && strings.Contains(w.Body.String(), `"DEPOT_ITEM_NOT_FOUND"`) {
			return nil
		}
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"success":true`) {
			return fmt.Errorf("Gateway movement: %d %s", w.Code, w.Body.String())
		}
		if send {
			sends.Add(1)
		} else {
			deposits.Add(1)
		}
		return nil
	})
	if result.Failures != 0 || deposits.Load()+sends.Load() != 1 {
		t.Fatalf("result=%+v deposits=%d sends=%d", result, deposits.Load(), sends.Load())
	}
	f.assertMoved(t, item.SlotMainHand, true, sends.Load() == 1)
}

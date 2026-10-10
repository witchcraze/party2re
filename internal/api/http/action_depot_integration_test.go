package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

type expansionDBPlayer struct {
	PlayerService
	owner string
}

func (p expansionDBPlayer) Authenticate(_ context.Context, token string) (coreplayer.Player, error) {
	if token != "session" {
		return coreplayer.Player{}, errors.New("invalid session")
	}
	return coreplayer.Player{ID: p.owner}, nil
}

type expansionFailingSave struct{ *database.DepotRepository }

func (expansionFailingSave) Save(context.Context, depot.Depot) error {
	return errors.New("private depot save failure")
}

func expansionDBRouter(t *testing.T, count, funds int, failSave bool) (http.Handler, corecharacter.Character, *database.CharacterRepository, *database.DepotRepository, *gatewayNavigationStore, item.Instance) {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	characters, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inventories, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := database.CreateTestCharacterWithFunds(context.Background(), db, "Expansion Gateway", funds)
	if err != nil {
		t.Fatal(err)
	}
	storedItem, err := item.NewInstance("weapon-01", 2)
	if err != nil {
		t.Fatal(err)
	}
	storedItem.EnhancementLevel = 6
	if count >= 0 {
		if _, err := database.CreateTestDepot(context.Background(), db, actor.ID, count, []item.Instance{storedItem}); err != nil {
			t.Fatal(err)
		}
	} else {
		// Character creation initializes storage; remove it to exercise recovery.
		if _, err := db.ExecContext(context.Background(), "DELETE FROM character_depots WHERE character_id = ?", actor.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := depots.FindByCharacterID(context.Background(), actor.ID); !errors.Is(err, depot.ErrNotFound) {
			t.Fatalf("missing storage fixture: %v", err)
		}
	}
	var storage depot.Repository = depots
	if failSave {
		storage = expansionFailingSave{depots}
	}
	service, err := depot.NewServiceWithTransaction(storage, characters, inventories, depots)
	if err != nil {
		t.Fatal(err)
	}
	characterService, err := character.NewService(characters)
	if err != nil {
		t.Fatal(err)
	}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "depot"}}
	pc := playercontext.NewService(characters, emptyActionsReader{}, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "depot", Parent: "town", Pageable: true}))
	h, err := NewHandler(expansionDBPlayer{owner: actor.PlayerID}, characterService, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithDepot(service), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return h.Router(), actor, characters, depots, store, storedItem
}

func TestDepotExpansionGatewayPersistence(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		count, funds, status int
		failSave             bool
	}{
		{"create missing", -1, 200000, 200, false},
		{"first", 0, 200000, 200, false}, {"second", 1, 200000, 200, false},
		{"third", 2, 400000, 200, false}, {"fourth", 3, 400000, 200, false},
		{"fifth", 4, 600000, 200, false}, {"sixth", 5, 600000, 200, false},
		{"seventh", 6, 800000, 200, false}, {"eighth", 7, 800000, 200, false},
		{"ninth", 8, 999999, 200, false}, {"last", 19, 999999, 200, false},
		{"maximum", 20, 999999, 400, false}, {"short", 0, 199999, 400, false},
		{"empty wallet", 0, 0, 400, false}, {"rollback", 1, 200000, 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, actor, chars, depots, store, storedItem := expansionDBRouter(t, tc.count, tc.funds, tc.failSave)
			status, got := gatewayRequest(t, router, actor.ID, "session", "application/json", `{"action":"depot_expand","params":{}}`, nil)
			if status != tc.status {
				t.Fatalf("status=%d: %s", status, got)
			}
			wantCount, wantMoney := max(tc.count, 0), tc.funds
			if status == 200 {
				cost, err := depot.ExpansionCost(wantCount)
				if err != nil {
					t.Fatal(err)
				}
				wantMoney -= cost
				wantCount++
			} else if status == 500 {
				assertGatewayError(t, got, "EXECUTION_FAILED")
			} else if tc.count == 20 {
				assertGatewayError(t, got, "DEPOT_MAX_EXPANDED")
			} else {
				assertGatewayError(t, got, "DEPOT_INSUFFICIENT_FUNDS")
			}
			persisted, err := chars.FindByID(context.Background(), actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			dep, err := depots.FindByCharacterID(context.Background(), actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if persisted.Money != wantMoney || dep.ExDepot != wantCount || dep.Capacity != depot.CalculateCapacity(actor.JobLevel, wantCount, actor.OverDepot) {
				t.Fatalf("wrong persisted charge/capacity: %+v %+v", persisted, dep)
			}
			if tc.count >= 0 && !reflect.DeepEqual(dep.Items, []item.Instance{storedItem}) {
				t.Fatalf("expansion changed stored assets: %+v", dep.Items)
			}
			if status == 200 {
				var result depotResponse
				if err := json.Unmarshal(got["result"], &result); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(result, toDepotResponse(dep)) {
					t.Fatalf("wrong result: %+v", result)
				}
			}
			if status != 500 {
				if _, failed := got["context_error"]; failed {
					t.Fatalf("unexpected refresh failure: %s", got)
				}
				var refreshed PlayerContextResponse
				if err := json.Unmarshal(got["context"], &refreshed); err != nil {
					t.Fatal(err)
				}
				if refreshed.Character.ID != actor.ID || refreshed.Character.Gold != wantMoney || refreshed.Scene.LocationID != "depot" {
					t.Fatalf("wrong refreshed actor: %+v", refreshed.Character)
				}
			}
			r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+actor.ID+"/context", nil)
			r.Header.Set("Authorization", "Bearer session")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			var observation PlayerContextResponse
			if err := json.Unmarshal(w.Body.Bytes(), &observation); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || observation.Character.Gold != wantMoney || store.writes != 0 {
				t.Fatalf("GET recovery: %d %s", w.Code, w.Body.String())
			}
			facts := decodeShopScene[DepotSceneData](t, observation)
			if facts.ExDepot != wantCount || facts.Capacity != dep.Capacity {
				t.Fatalf("stale scene: %+v", facts)
			}
			after, err := chars.FindByID(context.Background(), actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			afterDepot, err := depots.FindByCharacterID(context.Background(), actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after != persisted || !reflect.DeepEqual(afterDepot, dep) {
				t.Fatal("GET changed persisted state")
			}
		})
	}
}

func TestDepotExpansionGatewayConcurrentPurchases(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		count, funds, cost, purchases int
		rejection                     string
	}{
		{"one affordable", 0, 250000, 200000, 1, "DEPOT_INSUFFICIENT_FUNDS"},
		{"cross price tier", 0, 999999, 800000, 3, "DEPOT_INSUFFICIENT_FUNDS"},
		{"one remaining", 19, 999999, 999999, 1, "DEPOT_MAX_EXPANDED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, actor, chars, depots, store, storedItem := expansionDBRouter(t, tc.count, tc.funds, false)
			var purchases atomic.Int64
			result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(_, _ int) error {
				r := httptest.NewRequest(http.MethodPost, "/api/v1/characters/"+actor.ID+"/actions", strings.NewReader(`{"action":"depot_expand"}`))
				r.Header.Set("Authorization", "Bearer session")
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				var response struct {
					Success bool
					Error   ErrorDetail
				}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					return err
				}
				if w.Code == 200 && response.Success {
					purchases.Add(1)
					return nil
				}
				if w.Code == 400 && !response.Success && response.Error.Code == tc.rejection {
					return nil
				}
				return fmt.Errorf("unexpected expansion (including transaction/lock failure): %d %s", w.Code, w.Body.String())
			})
			if result.Failures != 0 || purchases.Load() != int64(tc.purchases) || store.writes != 0 {
				t.Fatalf("concurrent outcomes: %+v purchases=%d", result, purchases.Load())
			}
			persisted, err := chars.FindByID(context.Background(), actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			dep, err := depots.FindByCharacterID(context.Background(), actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if dep.ExDepot != tc.count+tc.purchases || dep.ExDepot > depot.MaxExDepot || persisted.Money+tc.cost != tc.funds || !reflect.DeepEqual(dep.Items, []item.Instance{storedItem}) {
				t.Fatalf("lost gold/expansion/items: %+v %+v", persisted, dep)
			}
		})
	}
}

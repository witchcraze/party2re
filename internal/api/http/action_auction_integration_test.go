package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

type auctionDBFixture struct {
	db          *sql.DB
	chars       *database.CharacterRepository
	inv         *database.InventoryRepository
	equip       *database.EquipmentRepository
	depots      *database.DepotRepository
	actor, peer corecharacter.Character
}

func newAuctionDBFixture(t *testing.T) *auctionDBFixture {
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
	f := &auctionDBFixture{db: db}
	f.chars, err = database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f.inv, err = database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f.equip, err = database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f.depots, err = database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f.actor, err = database.CreateTestCharacterWithFunds(context.Background(), db, "AuctionSender", 10000)
	if err != nil {
		t.Fatal(err)
	}
	f.peer, err = database.CreateTestCharacterWithFunds(context.Background(), db, "AuctionRecipient", 10000)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *auctionDBFixture) router(t *testing.T, actor corecharacter.Character, failSave bool) http.Handler {
	t.Helper()
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var storage auction.DepotRepository = f.depots
	if failSave {
		storage = expansionFailingSave{f.depots}
	}
	service, err := auction.NewService(database.NewTransactionProvider(f.db), f.chars, f.equip, f.inv, storage, catalog)
	if err != nil {
		t.Fatal(err)
	}
	characters, err := character.NewService(f.chars)
	if err != nil {
		t.Fatal(err)
	}
	pc := playercontext.NewService(f.chars, emptyActionsReader{}, timer.NewService(nil))
	h, err := NewHandler(expansionDBPlayer{owner: actor.PlayerID}, characters, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithAuction(service), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return h.Router()
}

func TestAuctionGatewayPersistence(t *testing.T) {
	for _, mode := range []string{"gold by name", "equipped slot", "owned instance", "rollback", "negative", "empty"} {
		t.Run(mode, func(t *testing.T) {
			f := newAuctionDBFixture(t)
			ctx := context.Background()
			inst, err := item.NewInstance("weapon-01", 1)
			if err != nil {
				t.Fatal(err)
			}
			inst.EnhancementLevel = 6
			if _, err := database.CreateTestInventoryWithItems(ctx, f.db, f.actor.ID, []item.Instance{inst}); err != nil {
				t.Fatal(err)
			}
			equipment, err := f.equip.FindByCharacterID(ctx, f.actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			catalog, err := item.DefaultCatalog()
			if err != nil {
				t.Fatal(err)
			}
			definition, err := catalog.FindByID(inst.DefinitionID)
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := f.inv.FindByCharacterID(ctx, f.actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := equipment.Equip(&inventory, definition, inst.ID); err != nil {
				t.Fatal(err)
			}
			if err := f.equip.Save(ctx, equipment); err != nil {
				t.Fatal(err)
			}
			params := fmt.Sprintf(`{"target_character_id":%q,"slot":"weapon"}`, f.peer.ID)
			want := 200
			switch mode {
			case "gold by name":
				params = fmt.Sprintf(`{"target_character_name":%q,"gold":7}`, f.peer.Name)
			case "owned instance":
				params = fmt.Sprintf(`{"target_character_id":%q,"instance_id":%q}`, f.peer.ID, inst.ID)
			case "rollback":
				want = 500
			case "negative":
				params, want = fmt.Sprintf(`{"target_character_id":%q,"gold":-1}`, f.peer.ID), 400
			case "empty":
				params, want = `{}`, 400
			}
			router := f.router(t, f.actor, mode == "rollback")
			status, got := gatewayRequest(t, router, f.actor.ID, "session", "application/json", `{"action":"auction_send","params":`+params+`}`, nil)
			if status != want {
				t.Fatalf("status=%d: %s", status, got)
			}
			actor, err := f.chars.FindByID(ctx, f.actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			peer, err := f.chars.FindByID(ctx, f.peer.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantGold := 10000
			if mode == "gold by name" {
				wantGold -= 7
			}
			if actor.Money != wantGold || actor.Money+peer.Money != 20000 {
				t.Fatalf("lost gold: %d/%d", actor.Money, peer.Money)
			}
			inventory, err = f.inv.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			equipment, err = f.equip.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			dep, err := f.depots.FindByCharacterID(ctx, peer.ID)
			if err != nil {
				t.Fatal(err)
			}
			transferred := mode == "equipped slot" || mode == "owned instance"
			_, held := inventory.Find(inst.ID)
			_, equipped := equipment.Equipped(item.SlotMainHand)
			wantQuantity := 0
			if transferred {
				wantQuantity = 1
			}
			if held == transferred || equipped == transferred || dep.Quantity(inst.DefinitionID) != wantQuantity {
				t.Fatalf("lost/duplicated equipment: %+v %+v %+v", inventory, equipment, dep)
			}
			if transferred {
				var result auction.SendResult
				if err := json.Unmarshal(got["result"], &result); err != nil {
					t.Fatal(err)
				}
				if result.TransferredItem == nil || len(dep.Items) != 1 || dep.Items[0].ID != result.TransferredItem.InstanceID || dep.Items[0].EnhancementLevel != 6 {
					t.Fatalf("lost item facts: %+v %+v", result, dep)
				}
			}
			if mode == "rollback" {
				assertGatewayError(t, got, "EXECUTION_FAILED")
			}
		})
	}
}

func TestAuctionGatewayConcurrentGoldConservation(t *testing.T) {
	f := newAuctionDBFixture(t)
	routers := []http.Handler{f.router(t, f.actor, false), f.router(t, f.peer, false)}
	actors := []corecharacter.Character{f.actor, f.peer}
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, _ int) error {
		i := worker % 2
		body := fmt.Sprintf(`{"action":"auction_send","params":{"target_character_id":%q,"gold":1}}`, actors[1-i].ID)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/characters/"+actors[i].ID+"/actions", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer session")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		routers[i].ServeHTTP(w, r)
		if w.Code != 200 {
			return fmt.Errorf("status=%d: %s", w.Code, w.Body.String())
		}
		return nil
	})
	if res.Failures != 0 || res.Successes != res.TotalOps {
		t.Fatalf("concurrent transfers: %+v", res)
	}
	actor, err := f.chars.FindByID(context.Background(), f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := f.chars.FindByID(context.Background(), f.peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if actor.Money+peer.Money != 20000 {
		t.Fatalf("lost/duplicated gold: %d/%d", actor.Money, peer.Money)
	}
	// Each successful response must correspond to exactly one debit and credit.
	cfg := testutil.GetStressConfig()
	wantActor := 10000 - ((cfg.Workers+1)/2-cfg.Workers/2)*cfg.OpsPerWorker
	if actor.Money != wantActor {
		t.Fatalf("successful commands were lost or replayed: want=%d got=%d", wantActor, actor.Money)
	}
}

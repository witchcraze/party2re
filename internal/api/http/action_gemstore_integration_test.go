package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

type gemStoreGatewayBoxes struct {
	*database.GemBoxRepository
	db   *sql.DB
	fail bool
}

func (r *gemStoreGatewayBoxes) Save(ctx context.Context, box gemstore.GemBox) error {
	if r.fail {
		return errors.New("private gem-box save failure")
	}
	return r.GemBoxRepository.Save(ctx, box)
}

func gemStoreDBRouter(t *testing.T, ctx context.Context) (http.Handler, corecharacter.Character, *database.CharacterRepository, *gemStoreGatewayBoxes, []item.Instance) {
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
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	boxes, err := database.NewGemBoxRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := database.CreateTestCharacterWithFunds(ctx, db, "Gem Gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := chars.Delete(cleanupCtx, actor.ID); err != nil {
			t.Error(err)
		}
	})
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	itemCatalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]item.Instance, 2)
	for i, def := range []string{"gem_atk_2", "gem_atk_1"} {
		items[i], err = item.NewInstance(def, 1)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := boxes.Save(ctx, gemstore.GemBox{CharacterID: actor.ID, Capacity: 5, Items: items}); err != nil {
		t.Fatal(err)
	}
	storage := &gemStoreGatewayBoxes{GemBoxRepository: boxes, db: db}
	svc, err := gemstore.NewService(catalog, chars, gemstore.WithGemBoxRepository(storage), gemstore.WithTransactionProvider(database.NewTransactionProvider(db)), gemstore.WithItemDefinitionProvider(itemCatalog), gemstore.WithDepotRepository(depots))
	if err != nil {
		t.Fatal(err)
	}
	characters, err := character.NewService(chars)
	if err != nil {
		t.Fatal(err)
	}
	pc := playercontext.NewService(chars, emptyActionsReader{}, timer.NewService(nil))
	h, err := NewHandler(expansionDBPlayer{owner: actor.PlayerID}, characters, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithGemStore(svc), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return h.Router(), actor, chars, storage, items
}

func TestGemStoreGatewayPersistenceAndRollback(t *testing.T) {
	for _, input := range []struct{ action string }{{"gemstore_sell"}, {"gemstore_sort"}} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%t", input.action, fail), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				router, actor, chars, boxes, items := gemStoreDBRouter(t, ctx)
				before, err := boxes.FindByCharacterID(ctx, actor.ID)
				if err != nil {
					t.Fatal(err)
				}
				boxes.fail = fail
				params := `{}`
				if input.action == "gemstore_sell" {
					params = fmt.Sprintf(`{"item_id":%q}`, items[0].ID)
				}
				status, got := gatewayRequest(t, router, actor.ID, "session", "application/json", saleGatewayBody(input.action, params), ctx)
				after, err := boxes.FindByCharacterID(ctx, actor.ID)
				if err != nil {
					t.Fatal(err)
				}
				char, err := chars.FindByID(ctx, actor.ID)
				if err != nil {
					t.Fatal(err)
				}
				if fail {
					assertGatewayError(t, got, "EXECUTION_FAILED")
					if status != 500 || !reflect.DeepEqual(before, after) || char.Money != actor.Money {
						t.Fatalf("partial mutation: %d %+v %+v", status, after, char)
					}
					return
				}
				if status != 200 || string(got["success"]) != "true" {
					t.Fatalf("command failed: %d %s", status, got)
				}
				if input.action == "gemstore_sort" {
					if len(after.Items) != 2 || after.Items[0].ID != items[1].ID || after.Items[1].ID != items[0].ID || char.Money != 0 {
						t.Fatalf("sort lost order/assets: %+v", after)
					}
				} else {
					var result gemstore.SellResult
					if err := json.Unmarshal(got["result"], &result); err != nil {
						t.Fatal(err)
					}
					if len(after.Items) != 1 || after.Items[0].ID != items[1].ID || char.Money != 50 || result.Payout != 50 || result.Gem.ID != "gem_atk_2" {
						t.Fatalf("sale lost assets: %+v %+v", after, result)
					}
					status, got = gatewayRequest(t, router, actor.ID, "session", "application/json", saleGatewayBody(input.action, params), ctx)
					if status != 400 {
						t.Fatalf("duplicate sale succeeded: %d %s", status, got)
					}
					assertGatewayError(t, got, "GEMSTORE_ITEM_NOT_OWNED")
				}
			})
		}
	}
}

func TestGemStoreGatewaySynthesisWithRenamedItemRollsBack(t *testing.T) {
	for _, outcome := range []string{"success", "save failure", "missing material", "hand only"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			router, actor, _, boxes, _ := gemStoreDBRouter(t, ctx)
			gem, err := item.NewInstance("gem_sky_atk_2", 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := boxes.GemBoxRepository.Save(ctx, gemstore.GemBox{CharacterID: actor.ID, Capacity: 5, Items: []item.Instance{gem}}); err != nil {
				t.Fatal(err)
			}
			depots, err := database.NewDepotRepository(boxes.db)
			if err != nil {
				t.Fatal(err)
			}
			dep, err := depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			reins, err := item.NewInstance("item-059", 1)
			if err != nil {
				t.Fatal(err)
			}
			inventories, err := database.NewInventoryRepository(boxes.db)
			if err != nil {
				t.Fatal(err)
			}
			invBefore, err := inventories.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if outcome == "hand only" {
				if err := invBefore.Add(reins); err != nil {
					t.Fatal(err)
				}
				if err := inventories.Save(ctx, invBefore); err != nil {
					t.Fatal(err)
				}
			} else if outcome != "missing material" {
				if err := dep.AddItem(reins); err != nil {
					t.Fatal(err)
				}
			}
			if err := depots.Save(ctx, dep); err != nil {
				t.Fatal(err)
			}
			boxBefore, err := boxes.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			boxes.fail = outcome == "save failure"
			status, got := gatewayRequest(t, router, actor.ID, "session", "application/json", saleGatewayBody("gemstore_synthesize", `{"recipe_id":"recipe_burst_sky"}`), ctx)
			boxAfter, err := boxes.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			depAfter, err := depots.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			invAfter, err := inventories.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(invBefore, invAfter) {
				t.Fatalf("synthesis changed hand slots: before=%+v after=%+v", invBefore, invAfter)
			}
			if outcome != "success" {
				wantStatus, wantCode := 400, "GEMSTORE_INSUFFICIENT_MATERIALS"
				if boxes.fail {
					wantStatus, wantCode = 500, "EXECUTION_FAILED"
				}
				assertGatewayError(t, got, wantCode)
				if status != wantStatus || !reflect.DeepEqual(boxBefore, boxAfter) || !reflect.DeepEqual(dep.Items, depAfter.Items) {
					t.Fatalf("partial mutation: %d box=%+v depot=%+v", status, boxAfter, depAfter)
				}
				return
			}
			if status != 200 || len(boxAfter.Items) != 1 || boxAfter.Items[0].DefinitionID != "gem_burst_sky" || len(depAfter.Items) != 0 {
				t.Fatalf("synthesis lost assets: %d %s box=%+v depot=%+v", status, got, boxAfter, depAfter)
			}
		})
	}
}

func TestGemStoreGatewayConcurrentSaleAndSort(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	router, actor, chars, boxes, items := gemStoreDBRouter(t, ctx)
	statuses := make([]int, 3)
	request := func(index int, action, params string) func() error {
		return func() error {
			status, got := gatewayRequest(t, router, actor.ID, "session", "application/json", saleGatewayBody(action, params), ctx)
			statuses[index] = status
			if status != 200 && !(action == "gemstore_sell" && status == 400) {
				return fmt.Errorf("%s: %d %s", action, status, got)
			}
			return nil
		}
	}
	params := fmt.Sprintf(`{"item_id":%q}`, items[0].ID)
	for _, err := range testutil.RunRace(request(0, "gemstore_sell", params), request(1, "gemstore_sell", params), request(2, "gemstore_sort", `{}`)) {
		if err != nil {
			t.Fatal(err)
		}
	}
	if statuses[2] != 200 || !((statuses[0] == 200 && statuses[1] == 400) || (statuses[0] == 400 && statuses[1] == 200)) {
		t.Fatalf("duplicate mutation: %v", statuses)
	}
	char, err := chars.FindByID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	box, err := boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if char.Money != 50 || len(box.Items) != 1 || box.Items[0].ID != items[1].ID {
		t.Fatalf("assets not conserved: %+v %+v", char, box)
	}
}

func TestGemStoreGatewayBuyPersistsAndRejectsWithoutFunds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	router, actor, chars, boxes, _ := gemStoreDBRouter(t, ctx)
	// One gold passes the wallet gate, so the service owns the price rejection.
	poor := actor
	poor.Money = 1
	if err := chars.Update(ctx, poor); err != nil {
		t.Fatal(err)
	}
	body := saleGatewayBody("gemstore_buy", `{"gem_id":"gem_atk_1"}`)
	status, got := gatewayRequest(t, router, actor.ID, "session", "application/json", body, ctx)
	assertGatewayError(t, got, "GEMSTORE_INSUFFICIENT_FUNDS")
	if box, err := boxes.FindByCharacterID(ctx, actor.ID); status != 400 || err != nil || len(box.Items) != 2 {
		t.Fatalf("rejected purchase mutated: %d %+v %v", status, box, err)
	}
	funded := actor
	funded.Money = 1000
	if err := chars.Update(ctx, funded); err != nil {
		t.Fatal(err)
	}
	status, got = gatewayRequest(t, router, actor.ID, "session", "application/json", body, ctx)
	var result gemstore.BuyResult
	if err := json.Unmarshal(got["result"], &result); status != 200 || err != nil {
		t.Fatalf("purchase failed: %d %s %v", status, got, err)
	}
	char, err := chars.FindByID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	box, err := boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if char.Money != 1000-result.Cost || result.Cost <= 0 || len(box.Items) != 3 || box.Items[2].ID != result.ItemInstance.ID {
		t.Fatalf("purchase not persisted: money=%d cost=%d %+v", char.Money, result.Cost, box)
	}
}

func TestGemStoreGatewayConcurrentSendConservesGem(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	router, actor, chars, boxes, items := gemStoreDBRouter(t, ctx)
	recipient, err := database.CreateTestCharacterWithFunds(ctx, boxes.db, "Gem Recipient", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := chars.Delete(cleanupCtx, recipient.ID); err != nil {
			t.Error(err)
		}
	})
	body := saleGatewayBody("gemstore_send", fmt.Sprintf(`{"recipient_character_id":%q,"item_id":%q}`, recipient.ID, items[0].ID))
	statuses := make([]int, 2)
	request := func(index int) func() error {
		return func() error {
			status, got := gatewayRequest(t, router, actor.ID, "session", "application/json", body, ctx)
			statuses[index] = status
			if status != 200 && status != 400 {
				return fmt.Errorf("send: %d %s", status, got)
			}
			return nil
		}
	}
	for _, err := range testutil.RunRace(request(0), request(1)) {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !((statuses[0] == 200 && statuses[1] == 400) || (statuses[0] == 400 && statuses[1] == 200)) {
		t.Fatalf("duplicate send: %v", statuses)
	}
	sender, err := boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	received, err := boxes.FindByCharacterID(ctx, recipient.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sender.Items) != 1 || sender.Items[0].ID != items[1].ID || len(received.Items) != 1 || received.Items[0].DefinitionID != "gem_atk_2" {
		t.Fatalf("gem not conserved: %+v %+v", sender, received)
	}
}

func TestGemStoreGatewayAppraisesDepotNotEquippedHandSlot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	router, actor, _, boxes, _ := gemStoreDBRouter(t, ctx)
	held, err := item.NewInstance("item-251", 1)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := item.NewInstance("item-251", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, boxes.db, actor.ID, []item.Instance{held}); err != nil {
		t.Fatal(err)
	}
	equip, err := database.NewEquipmentRepository(boxes.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := equip.Save(ctx, coreequipment.Equipment{CharacterID: actor.ID, Slots: map[item.Slot]string{item.SlotAccessory: held.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, boxes.db, actor.ID, 0, []item.Instance{stored}); err != nil {
		t.Fatal(err)
	}
	inv, err := database.NewInventoryRepository(boxes.db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(boxes.db)
	if err != nil {
		t.Fatal(err)
	}
	assertHandKept := func() {
		t.Helper()
		owned, err := inv.FindByCharacterID(ctx, actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		eq, err := equip.FindByCharacterID(ctx, actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(owned.Items) != 1 || owned.Items[0].ID != held.ID || eq.Slots[item.SlotAccessory] != held.ID {
			t.Fatalf("hand slot changed: %+v %+v", owned.Items, eq.Slots)
		}
	}

	// Legacy kantei never reads the hand slots.
	status, got := gatewayRequest(t, router, actor.ID, "session", "application/json", saleGatewayBody("gemstore_appraise", fmt.Sprintf(`{"item_id":%q}`, held.ID)), ctx)
	if status != 400 {
		t.Fatalf("hand-slot appraisal: %d %s", status, got)
	}
	assertGatewayError(t, got, "GEMSTORE_ITEM_NOT_OWNED")
	assertHandKept()

	before, err := boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	status, got = gatewayRequest(t, router, actor.ID, "session", "application/json", saleGatewayBody("gemstore_appraise", `{"item_id":"光る宝珠"}`), ctx)
	if status != 200 {
		t.Fatalf("depot appraisal by name: %d %s", status, got)
	}
	assertHandKept()
	dep, err := depots.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := boxes.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dep.Items) != 0 || len(after.Items) != len(before.Items)+1 {
		t.Fatalf("depot orb not appraised: depot=%+v box=%+v", dep.Items, after.Items)
	}
}

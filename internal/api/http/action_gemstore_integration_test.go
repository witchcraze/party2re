package http

import (
	"context"
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
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

type gemStoreGatewayBoxes struct {
	*database.GemBoxRepository
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
	inv, err := database.NewInventoryRepository(db)
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
	storage := &gemStoreGatewayBoxes{GemBoxRepository: boxes}
	svc, err := gemstore.NewService(catalog, chars, inv, gemstore.WithGemBoxRepository(storage), gemstore.WithTransactionProvider(database.NewTransactionProvider(db)))
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
	for _, input := range gemStoreInputs {
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

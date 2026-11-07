package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/blackmarket"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

type blackMarketGatewayPoints struct {
	*database.BlackMarketRepository
	fail bool
}

func (r *blackMarketGatewayPoints) SaveCharacterPoints(ctx context.Context, points blackmarket.CharacterPoints) error {
	if r.fail {
		return errors.New("private points save failure")
	}
	return r.BlackMarketRepository.SaveCharacterPoints(ctx, points)
}

type blackMarketDB struct {
	router http.Handler
	actor  corecharacter.Character
	inv    *database.InventoryRepository
	depots *database.DepotRepository
	points *blackMarketGatewayPoints
	rare   item.Instance
	common item.Instance
}

func newBlackMarketDB(t *testing.T, ctx context.Context) *blackMarketDB {
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
	return buildBlackMarketDB(t, ctx, db)
}

func buildBlackMarketDB(t *testing.T, ctx context.Context, db *sql.DB) *blackMarketDB {
	t.Helper()
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewBlackMarketRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := database.CreateTestCharacterWithFunds(ctx, db, "Black Market Gateway", 0)
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
	actor.JobLevel = blackmarket.MinAccessJobLevel
	if err := chars.Update(ctx, actor); err != nil {
		t.Fatal(err)
	}
	rare, err := item.NewInstance("weapon-29", 1)
	if err != nil {
		t.Fatal(err)
	}
	common, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := coreinventory.New(actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	owned.Items = []item.Instance{rare, common}
	if err := inv.Save(ctx, owned); err != nil {
		t.Fatal(err)
	}
	catalog, err := blackmarket.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	points := &blackMarketGatewayPoints{BlackMarketRepository: repo}
	svc, err := blackmarket.NewService(chars, inv, points, catalog, blackmarket.WithDepotRepository(depots), blackmarket.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	characters, err := character.NewService(chars)
	if err != nil {
		t.Fatal(err)
	}
	pc := playercontext.NewService(chars, emptyActionsReader{}, timer.NewService(nil))
	h, err := NewHandler(expansionDBPlayer{owner: actor.PlayerID}, characters, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithBlackMarket(svc), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return &blackMarketDB{router: h.Router(), actor: actor, inv: inv, depots: depots, points: points, rare: rare, common: common}
}

func (f *blackMarketDB) request(t *testing.T, ctx context.Context, action, params string) (int, map[string]json.RawMessage) {
	t.Helper()
	return gatewayRequest(t, f.router, f.actor.ID, "session", "application/json", saleGatewayBody(action, params), ctx)
}

func (f *blackMarketDB) rarePoints(t *testing.T, ctx context.Context) int {
	t.Helper()
	points, err := f.points.GetCharacterPoints(ctx, f.actor.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return points.RarePoints
}

func (f *blackMarketDB) inventoryCount(t *testing.T, ctx context.Context) int {
	t.Helper()
	owned, err := f.inv.FindByCharacterID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	return len(owned.Items)
}

func (f *blackMarketDB) depotItems(t *testing.T, ctx context.Context) []item.Instance {
	t.Helper()
	dep, err := f.depots.FindByCharacterID(ctx, f.actor.ID)
	if errors.Is(err, depot.ErrNotFound) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return dep.Items
}

func TestBlackMarketGatewaySacrificeAndTradePersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newBlackMarketDB(t, ctx)

	status, got := f.request(t, ctx, "blackmarket_sacrifice", fmt.Sprintf(`{"item_instance_id":%q}`, f.common.ID))
	if status != 400 {
		t.Fatalf("ineligible sacrifice: %d %s", status, got)
	}
	assertGatewayError(t, got, "BLACKMARKET_ITEM_NOT_ELIGIBLE")
	if f.inventoryCount(t, ctx) != 2 || f.rarePoints(t, ctx) != 0 {
		t.Fatal("ineligible sacrifice moved assets")
	}

	status, got = f.request(t, ctx, "blackmarket_trade", `{"prize_id":"bm_prize_087"}`)
	if status != 400 {
		t.Fatalf("unaffordable trade: %d %s", status, got)
	}
	assertGatewayError(t, got, "BLACKMARKET_INSUFFICIENT_RARE_POINTS")

	status, got = f.request(t, ctx, "blackmarket_sacrifice", fmt.Sprintf(`{"item_instance_id":%q}`, f.rare.ID))
	if status != 200 || f.inventoryCount(t, ctx) != 1 || f.rarePoints(t, ctx) != 1 {
		t.Fatalf("sacrifice: %d %s", status, got)
	}

	status, got = f.request(t, ctx, "blackmarket_trade", `{"prize_id":"bm_prize_087"}`)
	var result blackmarket.TradeResult
	if err := json.Unmarshal(got["result"], &result); err != nil {
		t.Fatal(err)
	}
	delivered := f.depotItems(t, ctx)
	if status != 200 || f.rarePoints(t, ctx) != 0 || len(delivered) != 1 || delivered[0].ID != result.DepotInstanceID || delivered[0].DefinitionID != "item-087" {
		t.Fatalf("trade: %d %s %+v", status, got, delivered)
	}
}

func TestBlackMarketGatewayRollsBackAssetsWhenPointsFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newBlackMarketDB(t, ctx)
	f.points.fail = true
	status, got := f.request(t, ctx, "blackmarket_sacrifice", fmt.Sprintf(`{"item_instance_id":%q}`, f.rare.ID))
	if status != 500 {
		t.Fatalf("sacrifice failure: %d %s", status, got)
	}
	assertGatewayError(t, got, "EXECUTION_FAILED")
	if f.inventoryCount(t, ctx) != 2 || f.rarePoints(t, ctx) != 0 {
		t.Fatal("failed sacrifice consumed the item")
	}

	f.points.fail = false
	if err := f.points.SaveCharacterPoints(ctx, blackmarket.CharacterPoints{CharacterID: f.actor.ID, RarePoints: 1}); err != nil {
		t.Fatal(err)
	}
	f.points.fail = true
	status, got = f.request(t, ctx, "blackmarket_trade", `{"prize_id":"bm_prize_087"}`)
	if status != 500 {
		t.Fatalf("trade failure: %d %s", status, got)
	}
	if len(f.depotItems(t, ctx)) != 0 || f.rarePoints(t, ctx) != 1 {
		t.Fatal("failed trade delivered the prize or spent points")
	}
}

func TestBlackMarketGatewayConcurrentAssetsAreConserved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newBlackMarketDB(t, ctx)
	if err := f.points.SaveCharacterPoints(ctx, blackmarket.CharacterPoints{CharacterID: f.actor.ID, RarePoints: 1}); err != nil {
		t.Fatal(err)
	}
	statuses := make([]int, 4)
	request := func(index int, action, params string) func() error {
		return func() error {
			status, got := f.request(t, ctx, action, params)
			statuses[index] = status
			if status != 200 && status != 400 {
				return fmt.Errorf("%s: %d %s", action, status, got)
			}
			return nil
		}
	}
	sacrifice := fmt.Sprintf(`{"item_instance_id":%q}`, f.rare.ID)
	trade := `{"prize_id":"bm_prize_087"}`
	for _, err := range testutil.RunRace(request(0, "blackmarket_sacrifice", sacrifice), request(1, "blackmarket_sacrifice", sacrifice), request(2, "blackmarket_trade", trade), request(3, "blackmarket_trade", trade)) {
		if err != nil {
			t.Fatal(err)
		}
	}
	sacrificed, traded := 0, 0
	for i, status := range statuses {
		if status == 200 && i < 2 {
			sacrificed++
		}
		if status == 200 && i >= 2 {
			traded++
		}
	}
	// Exactly one sacrifice succeeds; points earned minus prizes delivered is conserved.
	delivered := 0
	for _, prize := range f.depotItems(t, ctx) {
		delivered += prize.Quantity // same-definition prizes stack in one depot row
	}
	if sacrificed != 1 || f.inventoryCount(t, ctx) != 1 || delivered != traded || f.rarePoints(t, ctx) != 1+sacrificed-traded {
		t.Fatalf("assets not conserved: statuses=%v inventory=%d delivered=%d points=%d", statuses, f.inventoryCount(t, ctx), delivered, f.rarePoints(t, ctx))
	}
}

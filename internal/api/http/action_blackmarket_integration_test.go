package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/blackmarket"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
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
	chars  *database.CharacterRepository
	equip  *database.EquipmentRepository
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
	equip, err := database.NewEquipmentRepository(db)
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
	svc, err := blackmarket.NewService(chars, inv, points, catalog, blackmarket.WithDepotRepository(depots), blackmarket.WithEquipmentRepository(equip), blackmarket.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	characters, err := character.NewService(chars)
	if err != nil {
		t.Fatal(err)
	}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "blackmarket"}}
	pc := playercontext.NewService(chars, emptyActionsReader{}, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"},
		playercontext.SceneDefinition{ID: "blackmarket", Parent: "town", Pageable: true, CanEnter: blackmarket.CheckEligibility}))
	h, err := NewHandler(expansionDBPlayer{owner: actor.PlayerID}, characters, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithBlackMarket(svc), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return &blackMarketDB{router: h.Router(), actor: actor, chars: chars, equip: equip, inv: inv, depots: depots, points: points, rare: rare, common: common}
}

func (f *blackMarketDB) request(t *testing.T, ctx context.Context, action, params string) (int, map[string]json.RawMessage) {
	t.Helper()
	return gatewayRequest(t, f.router, f.actor.ID, "session", "application/json", saleGatewayBody(action, params), ctx)
}

// equipRare wields the rare weapon with weapon and armor customization set.
func (f *blackMarketDB) equipRare(t *testing.T, ctx context.Context) {
	t.Helper()
	f.actor.WeaponCustomName, f.actor.WeaponSeal, f.actor.ArmorCustomName = "Named blade", 4, "Named mail"
	if err := f.chars.Update(ctx, f.actor); err != nil {
		t.Fatal(err)
	}
	if err := f.equip.Save(ctx, coreequipment.Equipment{CharacterID: f.actor.ID, Slots: map[item.Slot]string{item.SlotMainHand: f.rare.ID}}); err != nil {
		t.Fatal(err)
	}
}

func (f *blackMarketDB) slots(t *testing.T, ctx context.Context) map[item.Slot]string {
	t.Helper()
	eq, err := f.equip.FindByCharacterID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	return eq.Slots
}

func (f *blackMarketDB) persistedActor(t *testing.T, ctx context.Context) corecharacter.Character {
	t.Helper()
	actor, err := f.chars.FindByID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	return actor
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
	f.equipRare(t, ctx)
	f.points.fail = true
	status, got := f.request(t, ctx, "blackmarket_sacrifice", fmt.Sprintf(`{"item_instance_id":%q}`, f.rare.ID))
	if status != 500 {
		t.Fatalf("sacrifice failure: %d %s", status, got)
	}
	assertGatewayError(t, got, "EXECUTION_FAILED")
	if f.inventoryCount(t, ctx) != 2 || f.rarePoints(t, ctx) != 0 {
		t.Fatal("failed sacrifice consumed the item")
	}
	if slots := f.slots(t, ctx); slots[item.SlotMainHand] != f.rare.ID || f.persistedActor(t, ctx) != f.actor {
		t.Fatalf("failed sacrifice changed equipment or customization: %+v", slots)
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
	f.equipRare(t, ctx)
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
	if slots := f.slots(t, ctx); len(slots) != 0 {
		t.Fatalf("sacrificed weapon stays equipped: %+v", slots)
	}
}

func TestBlackMarketGatewaySacrificesEquippedItem(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newBlackMarketDB(t, ctx)
	spare, err := item.NewInstance("armor-35", 1)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := f.inv.FindByCharacterID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	owned.Items = append([]item.Instance{spare}, owned.Items...)
	if err := f.inv.Save(ctx, owned); err != nil {
		t.Fatal(err)
	}
	f.equipRare(t, ctx)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+f.actor.ID+"/context", nil)
	r.Header.Set("Authorization", "Bearer session")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	var observation PlayerContextResponse
	if err := json.Unmarshal(w.Body.Bytes(), &observation); err != nil {
		t.Fatal(err)
	}
	candidates := decodeShopScene[BlackMarketSceneData](t, observation).SacrificeCandidates
	if w.Code != 200 || len(candidates) != 2 || candidates[0].ItemInstanceID != f.rare.ID || !candidates[0].Equipped || candidates[1].ItemInstanceID != spare.ID || candidates[1].Equipped {
		t.Fatalf("equipped candidate must be first and marked: %d %+v", w.Code, candidates)
	}

	status, got := f.request(t, ctx, "blackmarket_sacrifice", fmt.Sprintf(`{"item_instance_id":%q}`, f.rare.ID))
	if status != 200 || f.inventoryCount(t, ctx) != 2 || f.rarePoints(t, ctx) != 1 {
		t.Fatalf("equipped sacrifice: %d %s", status, got)
	}
	if slots := f.slots(t, ctx); len(slots) != 0 {
		t.Fatalf("sacrificed weapon stays equipped: %+v", slots)
	}
	// Legacy write_user drops the weapon name and seal with the weapon; the armor keeps its name.
	want := f.actor
	want.ClearWeaponCustomization()
	if actor := f.persistedActor(t, ctx); actor != want {
		t.Fatalf("customization: got %+v", actor)
	}
	var refreshed PlayerContextResponse
	if err := json.Unmarshal(got["context"], &refreshed); err != nil {
		t.Fatal(err)
	}
	candidates = decodeShopScene[BlackMarketSceneData](t, refreshed).SacrificeCandidates
	if len(candidates) != 1 || candidates[0].ItemInstanceID != spare.ID {
		t.Fatalf("refreshed candidates: %+v", candidates)
	}
}

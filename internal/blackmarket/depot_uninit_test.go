package blackmarket_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type uninitDepotRepo struct {
	depots map[string]depot.Depot
}

func newUninitDepotRepo() *uninitDepotRepo {
	return &uninitDepotRepo{depots: make(map[string]depot.Depot)}
}

func (m *uninitDepotRepo) FindByCharacterID(_ context.Context, id string) (depot.Depot, error) {
	if d, ok := m.depots[id]; ok {
		return d, nil
	}
	return depot.Depot{}, depot.ErrNotFound
}

func (m *uninitDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, id string) (depot.Depot, error) {
	return m.FindByCharacterID(ctx, id)
}

func (m *uninitDepotRepo) Save(_ context.Context, d depot.Depot) error {
	m.depots[d.CharacterID] = d
	return nil
}

func TestSacrificeItem_UninitializedDepot(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	depotRepo := newUninitDepotRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()

	char := corecharacter.Character{ID: "char-uninit", Name: "Tester", JobLevel: 5}
	_ = charRepo.Update(ctx, char)

	// Add sacrifice-eligible item to inventory
	inv, _ := invRepo.FindByCharacterID(ctx, "char-uninit")
	inst, _ := coreitem.NewInstance("weapon-29", 1) // はやぶさの剣: sacrifice eligible
	_ = inv.Add(inst)
	_ = invRepo.Save(ctx, inv)

	svc, err := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		catalog,
		blackmarket.WithDepotRepository(depotRepo),
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.SacrificeItem(ctx, "char-uninit", inst.ID)
	if err != nil {
		t.Fatalf("SacrificeItem failed with uninitialized depot: %v", err)
	}
	if res.RarePointsGained <= 0 {
		t.Errorf("expected positive rare points, got %d", res.RarePointsGained)
	}
}

func TestTradePrize_UninitializedDepot(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()
	depotRepo := newUninitDepotRepo()
	bmRepo := newMockBlackMarketRepo()
	catalog, _ := blackmarket.LoadDefaultCatalog()

	char := corecharacter.Character{ID: "char-uninit-trade", Name: "Trader", JobLevel: 3}
	_ = charRepo.Update(ctx, char)

	// Give enough rare points for bm_prize_087 (cost 1)
	bmRepo.points["char-uninit-trade"] = blackmarket.CharacterPoints{
		CharacterID: "char-uninit-trade",
		RarePoints:  100,
	}

	svc, err := blackmarket.NewService(
		charRepo,
		invRepo,
		bmRepo,
		catalog,
		blackmarket.WithDepotRepository(depotRepo),
		blackmarket.WithTransactionProvider(&mockTxProvider{}),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.TradePrize(ctx, "char-uninit-trade", "bm_prize_087")
	if err != nil {
		t.Fatalf("TradePrize failed with uninitialized depot: %v", err)
	}
	if res.RemainingRare != 99 {
		t.Errorf("expected remaining points 99, got %d", res.RemainingRare)
	}

	// Verify depot was created and has the traded item
	dep, err := depotRepo.FindByCharacterID(ctx, "char-uninit-trade")
	if err != nil {
		t.Fatalf("depot not created: %v", err)
	}
	if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-087" {
		t.Errorf("expected 1 item with ID item-087 in depot, got %+v", dep.Items)
	}
}

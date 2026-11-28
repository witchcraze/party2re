package shop_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/shop"
)

type errCollectionRecorder struct {
	err error
}

func (m *errCollectionRecorder) RecordItemDiscovered(_ context.Context, _, _, _, _ string) error {
	return m.err
}

type categoryCollectionRecorder struct {
	calls []struct {
		itemID   string
		category string
	}
}

func (m *categoryCollectionRecorder) RecordItemDiscovered(_ context.Context, _, itemID, _, category string) error {
	m.calls = append(m.calls, struct {
		itemID   string
		category string
	}{itemID: itemID, category: category})
	return nil
}

func TestPurchase_PropagatesCollectionError(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _, _, _, _ := setupParityTest(t)

	colErr := errors.New("shop collection failure")
	recorder := &errCollectionRecorder{err: colErr}
	svc.SetCollectionRecorder(recorder)

	char := character.Character{
		ID:    "char-buyer",
		Name:  "Buyer",
		Money: 5000,
	}
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	_, err := svc.PurchaseInShop(ctx, char.ID, shop.ShopTypeWeapon, "weapon-01")
	if !errors.Is(err, colErr) {
		t.Fatalf("expected collection error %v, got: %v", colErr, err)
	}
}

func TestPurchase_RecordsLegacyCollectionCategory(t *testing.T) {
	ctx := context.Background()
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatalf("DefaultCatalog failed: %v", err)
	}
	charRepo := newCharacterRepoStub()
	invRepo := newInventoryRepoStub()
	depotRepo := newDepotRepoStub()
	recorder := &categoryCollectionRecorder{}

	svc, err := shop.NewService(
		charRepo,
		invRepo,
		catalog,
		shop.WithDepotRepository(depotRepo),
		shop.WithCollectionRecorder(recorder),
		shop.WithTransactionProvider(&mockTxProvider{}),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	char := character.Character{
		ID:       "char-cat-buyer",
		Name:     "Buyer",
		JobLevel: 50,
		Money:    100000,
	}
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	// Weapon -> "weapon", Armor -> "armor", Accessory (SlotAccessory) -> "item"
	for _, tc := range []struct {
		shopType shop.ShopType
		itemID   string
		wantCat  string
	}{
		{shop.ShopTypeWeapon, "weapon-01", "weapon"},
		{shop.ShopTypeArmor, "armor-01", "armor"},
		{shop.ShopTypeAccessory, "item-145", "item"},
	} {
		if _, err := svc.PurchaseInShop(ctx, char.ID, tc.shopType, tc.itemID); err != nil {
			t.Fatalf("PurchaseInShop(%s, %s) failed: %v", tc.shopType, tc.itemID, err)
		}
	}

	if len(recorder.calls) != 3 {
		t.Fatalf("expected 3 collection calls, got %+v", recorder.calls)
	}
	want := []struct {
		itemID   string
		category string
	}{
		{"weapon-01", "weapon"},
		{"armor-01", "armor"},
		{"item-145", "item"},
	}
	for i, w := range want {
		if recorder.calls[i] != w {
			t.Errorf("call[%d] = %+v, want %+v", i, recorder.calls[i], w)
		}
	}
}

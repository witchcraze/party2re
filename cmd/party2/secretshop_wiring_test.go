package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/secretshop"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestSecretShopProductionHelperWiring(t *testing.T) {
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
	ctx := context.Background()
	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "SecretHelper_"+id.New()[:8], 50000)
	if err != nil {
		t.Fatal(err)
	}
	char.JobLevel = 7
	if err := core.charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, db, char.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, db, char.ID, 0, nil); err != nil {
		t.Fatal(err)
	}
	questRepo, err := database.NewHelperRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	q := helperquest.Quest{ID: id.New(), Title: "Secret exclusion", Kind: helperquest.KindItem, TargetID: "item-010", TargetName: "Herbal root", RequiredCount: 1, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := questRepo.Save(ctx, q); err != nil {
		t.Fatal(err)
	}
	questID := q.ID
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM helper_quests WHERE id = ?", questID); err != nil {
			t.Error(err)
		}
	})
	assertAssets := func(wantGold, wantCount int) {
		t.Helper()
		got, err := core.charRepo.FindByID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		inv, err := core.invRepo.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		dep, err := econ.depotRepo.FindByCharacterID(ctx, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		count := dep.Quantity("item-010")
		for _, it := range inv.Items {
			if it.DefinitionID == "item-010" {
				count += it.Quantity
			}
		}
		if got.Money != wantGold || count != wantCount {
			t.Fatalf("assets: gold=%d items=%d; want %d/%d", got.Money, count, wantGold, wantCount)
		}
	}
	status, err := misc.secretshop.GetShopStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range status.Items {
		if it.ItemDefinitionID == q.TargetID {
			t.Fatal("production catalog exposes active helper target")
		}
	}
	res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(_, _ int) error {
		_, err := misc.secretshop.PurchaseItem(ctx, char.ID, "secret_item_herbal_root", 1)
		if !errors.Is(err, secretshop.ErrItemUnavailableInHelperQuest) {
			return fmt.Errorf("active target purchase: %v", err)
		}
		return nil
	})
	if res.Failures != 0 {
		t.Fatalf("concurrent exclusions: %+v", res)
	}
	assertAssets(char.Money, 0)
	q.CompletedAt, q.CompletedBy = &now, char.ID
	if err := questRepo.Save(ctx, q); err != nil {
		t.Fatal(err)
	}
	status, err = misc.secretshop.GetShopStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range status.Items {
		found = found || it.ItemDefinitionID == q.TargetID
	}
	if !found {
		t.Fatal("completed target remains excluded")
	}
	for _, err := range testutil.RunRace(
		func() error {
			_, err := misc.secretshop.PurchaseItem(ctx, char.ID, "secret_item_herbal_root", 1)
			return err
		},
		func() error {
			_, err := misc.secretshop.PurchaseItem(ctx, char.ID, "secret_item_herbal_root", 1)
			return err
		},
	) {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertAssets(char.Money-2*750, 2)
	q.ID, q.CompletedAt, q.CompletedBy, q.ExpiresAt = id.New(), nil, "", now.Add(-time.Hour)
	if err := questRepo.Save(ctx, q); err != nil {
		t.Fatal(err)
	}
	expiredID := q.ID
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM helper_quests WHERE id = ?", expiredID); err != nil {
			t.Error(err)
		}
	})
	status, err = misc.secretshop.GetShopStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, it := range status.Items {
		found = found || it.ItemDefinitionID == q.TargetID
	}
	if !found {
		t.Fatal("expired target remains excluded")
	}
	if _, err := misc.secretshop.PurchaseItem(ctx, char.ID, "secret_item_herbal_root", 1); err != nil {
		t.Fatal(err)
	}
	assertAssets(char.Money-3*750, 3)
}

type secretShopFailingQuests struct {
	helperquest.QuestRepository
	err error
	ctx context.Context
	now time.Time
}

func (r *secretShopFailingQuests) ListActive(ctx context.Context, now time.Time) ([]helperquest.Quest, error) {
	r.ctx, r.now = ctx, now
	return nil, r.err
}

type secretShopTransactionKey struct{}

type secretShopTestTransaction struct{}

func (secretShopTestTransaction) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, secretShopTransactionKey{}, true))
}

func TestSecretShopHelperReadFailure(t *testing.T) {
	failure := errors.New("helper read failed")
	quests := &secretShopFailingQuests{err: failure}
	chars := &wireMockCharRepo{char: corecharacter.Character{ID: "hero", JobLevel: 7, Money: 50000}}
	helper := helperquest.NewService(quests, chars, nil, nil)
	catalog, err := secretshop.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	// Any inventory/depot access would panic: failed helper reads must precede it.
	service, err := secretshop.NewService(chars, &secretShopUntouchedInventory{}, catalog,
		secretshop.WithHelperFilter(secretShopHelperAdapter{helper: helper}),
		secretshop.WithTransactionProvider(secretShopTestTransaction{}),
		secretshop.WithDepotRepository(&secretShopUntouchedDepot{}))
	if err != nil {
		t.Fatal(err)
	}
	type requestKey struct{}
	ctx := context.WithValue(context.Background(), requestKey{}, "request")
	before := time.Now().UTC()
	status, err := service.GetShopStatus(ctx, "hero")
	if status != nil || !errors.Is(err, failure) || quests.ctx != ctx {
		t.Fatalf("status=%+v err=%v ctx=%v", status, err, quests.ctx)
	}
	result, err := service.PurchaseItem(ctx, "hero", "secret_item_herbal_root", 1)
	if result != nil || !errors.Is(err, failure) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if quests.ctx.Value(requestKey{}) != "request" || quests.ctx.Value(secretShopTransactionKey{}) != true {
		t.Fatal("helper read lost request/transaction context")
	}
	if quests.now.Before(before) || quests.now.After(time.Now().UTC()) {
		t.Fatalf("helper reader time=%v", quests.now)
	}
	if chars.char.Money != 50000 {
		t.Fatalf("wallet changed on helper failure: %+v", chars.char)
	}
}

type secretShopUntouchedInventory struct{ secretshop.InventoryRepository }
type secretShopUntouchedDepot struct{ secretshop.DepotRepository }

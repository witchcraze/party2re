package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/secretshop"
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
	status, err := misc.secretshop.GetShopStatus(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range status.Items {
		if it.ItemDefinitionID == q.TargetID {
			t.Fatal("production catalog exposes active helper target")
		}
	}
	if _, err := misc.secretshop.PurchaseItem(ctx, char.ID, "secret_item_herbal_root", 1); !errors.Is(err, secretshop.ErrItemUnavailableInHelperQuest) {
		t.Fatalf("production helper purchase filter: %v", err)
	}
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

}

type secretShopFailingQuests struct {
	helperquest.QuestRepository
	err error
	ctx context.Context
}

type secretShopUntouchedInventory struct{ secretshop.InventoryRepository }

func (r *secretShopFailingQuests) ListUncompleted(ctx context.Context) ([]helperquest.Quest, error) {
	r.ctx = ctx
	return nil, r.err
}

func TestSecretShopHelperAdapterError(t *testing.T) {
	failure := errors.New("helper read failed")
	quests := &secretShopFailingQuests{err: failure}
	helper := helperquest.NewService(quests, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ids, err := (secretShopHelperAdapter{helper: helper}).GetActiveHelperItemIDs(ctx)
	if ids != nil || !errors.Is(err, failure) || quests.ctx != ctx {
		t.Fatalf("adapter ids=%v err=%v ctx=%v", ids, err, quests.ctx)
	}
}

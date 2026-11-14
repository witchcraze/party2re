package auction_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/testutil"
)

type failingSaveDepotRepo struct {
	*database.DepotRepository
}

func (r failingSaveDepotRepo) Save(context.Context, depot.Depot) error {
	return errors.New("simulated depot save failure")
}

type auctionDBFixture struct {
	db      *sql.DB
	chars   *database.CharacterRepository
	inv     *database.InventoryRepository
	equip   *database.EquipmentRepository
	depots  *database.DepotRepository
	catalog *item.Catalog
	actor   corecharacter.Character
	peer    corecharacter.Character
}

func newAuctionDBFixture(t *testing.T) *auctionDBFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN not set")
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	f := &auctionDBFixture{db: db}
	if f.chars, err = database.NewCharacterRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.inv, err = database.NewInventoryRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.equip, err = database.NewEquipmentRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.depots, err = database.NewDepotRepository(db); err != nil {
		t.Fatal(err)
	}
	if f.catalog, err = item.DefaultCatalog(); err != nil {
		t.Fatal(err)
	}

	f.actor, err = database.CreateTestCharacterWithFunds(ctx, db, "AuctionS", 10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if err := f.chars.Delete(cleanupCtx, f.actor.ID); err != nil {
			t.Error(err)
		}
	})

	f.peer, err = database.CreateTestCharacterWithFunds(ctx, db, "AuctionR", 10000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if err := f.chars.Delete(cleanupCtx, f.peer.ID); err != nil {
			t.Error(err)
		}
	})

	return f
}

func (f *auctionDBFixture) service(t *testing.T, failSave bool) *auction.Service {
	t.Helper()
	var storage auction.DepotRepository = f.depots
	if failSave {
		storage = failingSaveDepotRepo{DepotRepository: f.depots}
	}
	svc, err := auction.NewService(database.NewTransactionProvider(f.db), f.chars, f.equip, f.inv, storage, f.catalog)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func (f *auctionDBFixture) equipWeapon(t *testing.T, ctx context.Context, defID string, enhancement int) item.Instance {
	t.Helper()
	inst, err := item.NewInstance(defID, 1)
	if err != nil {
		t.Fatal(err)
	}
	inst.EnhancementLevel = enhancement
	inv, err := database.CreateTestInventoryWithItems(ctx, f.db, f.actor.ID, []item.Instance{inst})
	if err != nil {
		t.Fatal(err)
	}
	equipment, err := f.equip.FindByCharacterID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	def, err := f.catalog.FindByID(defID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := equipment.Equip(&inv, def, inst.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.equip.Save(ctx, equipment); err != nil {
		t.Fatal(err)
	}
	return inst
}

func TestAuctionSendPersistence_Integration(t *testing.T) {
	t.Run("gold by target name", func(t *testing.T) {
		f := newAuctionDBFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		svc := f.service(t, false)
		res, err := svc.Send(ctx, auction.SendRequest{
			SenderCharacterID:   f.actor.ID,
			TargetCharacterName: f.peer.Name,
			Gold:                250,
		})
		if err != nil {
			t.Fatalf("Send gold failed: %v", err)
		}
		if res.TransferredGold != 250 || res.TargetCharacterID != f.peer.ID {
			t.Fatalf("unexpected result: %+v", res)
		}

		sender, err := f.chars.FindByID(ctx, f.actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		peer, err := f.chars.FindByID(ctx, f.peer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if sender.Money != 9750 || peer.Money != 10250 {
			t.Fatalf("gold balances: sender=%d peer=%d", sender.Money, peer.Money)
		}
	})

	t.Run("equipped slot", func(t *testing.T) {
		f := newAuctionDBFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		inst := f.equipWeapon(t, ctx, "weapon-01", 6)
		svc := f.service(t, false)

		res, err := svc.Send(ctx, auction.SendRequest{
			SenderCharacterID: f.actor.ID,
			TargetCharacterID: f.peer.ID,
			Slot:              "weapon",
		})
		if err != nil {
			t.Fatalf("Send equipped slot failed: %v", err)
		}
		if res.TransferredItem == nil || res.TransferredItem.DefinitionID != "weapon-01" || res.TransferredItem.EnhancementLevel != 6 {
			t.Fatalf("unexpected transferred item: %+v", res.TransferredItem)
		}

		eq, err := f.equip.FindByCharacterID(ctx, f.actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, equipped := eq.Equipped(item.SlotMainHand); equipped {
			t.Fatal("expected sender weapon to be unequipped")
		}
		inv, err := f.inv.FindByCharacterID(ctx, f.actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, found := inv.Find(inst.ID); found {
			t.Fatal("expected sent weapon removed from sender inventory")
		}
		peerDepot, err := f.depots.FindByCharacterID(ctx, f.peer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(peerDepot.Items) != 1 || peerDepot.Items[0].DefinitionID != "weapon-01" || peerDepot.Items[0].EnhancementLevel != 6 {
			t.Fatalf("peer depot = %+v", peerDepot.Items)
		}
	})

	t.Run("equipped instance", func(t *testing.T) {
		f := newAuctionDBFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		inst := f.equipWeapon(t, ctx, "weapon-01", 6)
		svc := f.service(t, false)

		res, err := svc.Send(ctx, auction.SendRequest{
			SenderCharacterID: f.actor.ID,
			TargetCharacterID: f.peer.ID,
			InstanceID:        inst.ID,
		})
		if err != nil {
			t.Fatalf("Send equipped instance failed: %v", err)
		}
		if res.TransferredItem == nil || res.TransferredItem.InstanceID == "" || res.TransferredItem.DefinitionID != "weapon-01" || res.TransferredItem.EnhancementLevel != 6 {
			t.Fatalf("unexpected transferred item: %+v", res.TransferredItem)
		}

		eq, err := f.equip.FindByCharacterID(ctx, f.actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, equipped := eq.Equipped(item.SlotMainHand); equipped {
			t.Fatal("expected sender weapon to be unequipped")
		}
		inv, err := f.inv.FindByCharacterID(ctx, f.actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, found := inv.Find(inst.ID); found {
			t.Fatal("expected sent weapon removed from sender inventory")
		}
		peerDepot, err := f.depots.FindByCharacterID(ctx, f.peer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(peerDepot.Items) != 1 || peerDepot.Items[0].ID != res.TransferredItem.InstanceID || peerDepot.Items[0].DefinitionID != "weapon-01" || peerDepot.Items[0].EnhancementLevel != 6 {
			t.Fatalf("peer depot = %+v", peerDepot.Items)
		}
	})

	t.Run("rollback on depot save failure", func(t *testing.T) {
		f := newAuctionDBFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		inst := f.equipWeapon(t, ctx, "weapon-01", 6)
		failingSvc := f.service(t, true)
		if _, err := failingSvc.Send(ctx, auction.SendRequest{
			SenderCharacterID: f.actor.ID,
			TargetCharacterID: f.peer.ID,
			InstanceID:        inst.ID,
		}); err == nil {
			t.Fatal("expected DepotRepository.Save failure")
		}

		sender, err := f.chars.FindByID(ctx, f.actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		peer, err := f.chars.FindByID(ctx, f.peer.ID)
		if err != nil {
			t.Fatal(err)
		}
		eq, err := f.equip.FindByCharacterID(ctx, f.actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		eqID, equipped := eq.Equipped(item.SlotMainHand)
		inv, err := f.inv.FindByCharacterID(ctx, f.actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, stillInInv := inv.Find(inst.ID)
		peerDepot, err := f.depots.FindByCharacterID(ctx, f.peer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if sender.Money != 10000 || peer.Money != 10000 || !equipped || eqID != inst.ID || !stillInInv || len(peerDepot.Items) != 0 {
			t.Fatalf("rollback leaked state: sender=%d peer=%d equipped=%v(%s) stillInInv=%v depot=%+v", sender.Money, peer.Money, equipped, eqID, stillInInv, peerDepot.Items)
		}

		// Retry without fault injection succeeds cleanly.
		svc := f.service(t, false)
		if _, err := svc.Send(ctx, auction.SendRequest{
			SenderCharacterID: f.actor.ID,
			TargetCharacterID: f.peer.ID,
			InstanceID:        inst.ID,
		}); err != nil {
			t.Fatalf("retry Send failed: %v", err)
		}
	})

	t.Run("rejected requests leave DB unchanged", func(t *testing.T) {
		f := newAuctionDBFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		svc := f.service(t, false)
		if _, err := svc.Send(ctx, auction.SendRequest{
			SenderCharacterID: f.actor.ID,
			TargetCharacterID: f.peer.ID,
			Gold:              -1,
		}); !errors.Is(err, auction.ErrInvalidSendAmount) {
			t.Fatalf("expected ErrInvalidSendAmount, got %v", err)
		}
		if _, err := svc.Send(ctx, auction.SendRequest{
			SenderCharacterID: f.actor.ID,
			TargetCharacterID: f.peer.ID,
		}); !errors.Is(err, auction.ErrNothingToSend) {
			t.Fatalf("expected ErrNothingToSend, got %v", err)
		}

		sender, err := f.chars.FindByID(ctx, f.actor.ID)
		if err != nil {
			t.Fatal(err)
		}
		peer, err := f.chars.FindByID(ctx, f.peer.ID)
		if err != nil {
			t.Fatal(err)
		}
		if sender.Money != 10000 || peer.Money != 10000 {
			t.Fatalf("rejected send mutated funds: sender=%d peer=%d", sender.Money, peer.Money)
		}
	})
}

func TestAuctionConcurrentGoldConservation_Integration(t *testing.T) {
	f := newAuctionDBFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	svc := f.service(t, false)
	var netDelta atomic.Int64
	const transferAmount = 10

	res := testutil.RunConcurrentStressTest(t, testutil.ConcurrencyStressConfig{
		Workers:      8,
		OpsPerWorker: 5,
	}, func(workerID, op int) error {
		opCtx, opCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer opCancel()
		if (workerID+op)%2 == 0 {
			_, err := svc.Send(opCtx, auction.SendRequest{
				SenderCharacterID: f.actor.ID,
				TargetCharacterID: f.peer.ID,
				Gold:              transferAmount,
			})
			if err == nil {
				netDelta.Add(transferAmount)
			}
			return err
		}
		_, err := svc.Send(opCtx, auction.SendRequest{
			SenderCharacterID: f.peer.ID,
			TargetCharacterID: f.actor.ID,
			Gold:              transferAmount,
		})
		if err == nil {
			netDelta.Add(-transferAmount)
		}
		return err
	})
	if res.Successes == 0 {
		t.Fatalf("expected successful concurrent transfers, got failures=%d", res.Failures)
	}

	sender, err := f.chars.FindByID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := f.chars.FindByID(ctx, f.peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if total := sender.Money + peer.Money; total != 20000 {
		t.Fatalf("gold not conserved: sender=%d peer=%d total=%d", sender.Money, peer.Money, total)
	}
	if int64(peer.Money-10000) != netDelta.Load() {
		t.Fatalf("delta mismatch: peer=%d netDelta=%d", peer.Money, netDelta.Load())
	}
}

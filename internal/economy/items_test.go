package economy_test

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/economy"
)

type trackingInvRepo struct {
	invs     map[string]coreinventory.Inventory
	readErr  error
	saveErr  error
	saveHits int
}

func newTrackingInvRepo() *trackingInvRepo {
	return &trackingInvRepo{
		invs: make(map[string]coreinventory.Inventory),
	}
}

func (r *trackingInvRepo) FindByCharacterID(_ context.Context, characterID string) (coreinventory.Inventory, error) {
	if r.readErr != nil {
		return coreinventory.Inventory{}, r.readErr
	}
	if inv, ok := r.invs[characterID]; ok {
		return inv, nil
	}
	return coreinventory.New(characterID)
}

func (r *trackingInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, characterID)
}

func (r *trackingInvRepo) Save(_ context.Context, inv coreinventory.Inventory) error {
	r.saveHits++
	if r.saveErr != nil {
		return r.saveErr
	}
	r.invs[inv.CharacterID] = inv
	return nil
}

func TestFindInventory_DatabaseReadErrorPropagation(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	char := corecharacter.Character{
		ID:    "char-1",
		Name:  "Hero",
		Money: 500,
	}
	charRepo.chars[char.ID] = char

	dbErr := errors.New("simulated database query failure or connection drop")

	t.Run("GrantItem aborts and does not Save on read error", func(t *testing.T) {
		invRepo := newTrackingInvRepo()
		invRepo.readErr = dbErr

		svc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(&mockTxProvider{}))
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		_, _, err = svc.GrantItem(ctx, char.ID, "item-001", 1)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected error to wrap %v, got %v", dbErr, err)
		}
		if invRepo.saveHits != 0 {
			t.Fatalf("expected 0 Save calls on read error, got %d (risk of wiping inventory)", invRepo.saveHits)
		}
	})

	t.Run("ConsumeItemInstance aborts on read error", func(t *testing.T) {
		invRepo := newTrackingInvRepo()
		invRepo.readErr = dbErr

		svc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(&mockTxProvider{}))
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		_, err = svc.ConsumeItemInstance(ctx, char.ID, "inst-001", 1)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected error to wrap %v, got %v", dbErr, err)
		}
		if invRepo.saveHits != 0 {
			t.Fatalf("expected 0 Save calls, got %d", invRepo.saveHits)
		}
	})

	t.Run("ConsumeItemDefinition aborts on read error", func(t *testing.T) {
		invRepo := newTrackingInvRepo()
		invRepo.readErr = dbErr

		svc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(&mockTxProvider{}))
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		_, err = svc.ConsumeItemDefinition(ctx, char.ID, "item-001", 1)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected error to wrap %v, got %v", dbErr, err)
		}
		if invRepo.saveHits != 0 {
			t.Fatalf("expected 0 Save calls, got %d", invRepo.saveHits)
		}
	})

	t.Run("Exchange aborts on read error without saving", func(t *testing.T) {
		invRepo := newTrackingInvRepo()
		invRepo.readErr = dbErr

		svc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(&mockTxProvider{}))
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		_, err = svc.Exchange(ctx, economy.ExchangeRequest{
			CharacterID:       char.ID,
			GrantDefinitionID: "item-001",
			GrantQuantity:     1,
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected error to wrap %v, got %v", dbErr, err)
		}
		if invRepo.saveHits != 0 {
			t.Fatalf("expected 0 Save calls on read error, got %d", invRepo.saveHits)
		}
	})

	t.Run("ExecuteTransaction with inventory aborts on read error without saving", func(t *testing.T) {
		invRepo := newTrackingInvRepo()
		invRepo.readErr = dbErr

		svc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(&mockTxProvider{}))
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		_, err = svc.ExecuteTransaction(ctx, economy.TransactionRequest{
			CharacterID:   char.ID,
			LockInventory: true,
		}, func(_ *economy.TxContext) error {
			return nil
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected error to wrap %v, got %v", dbErr, err)
		}
		if invRepo.saveHits != 0 {
			t.Fatalf("expected 0 Save calls on read error, got %d", invRepo.saveHits)
		}
	})

	t.Run("Happy path creates empty inventory when repository returns nil error with 0 items", func(t *testing.T) {
		invRepo := newTrackingInvRepo()

		svc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(&mockTxProvider{}))
		if err != nil {
			t.Fatalf("NewService failed: %v", err)
		}

		inv, inst, err := svc.GrantItem(ctx, char.ID, "item-001", 1)
		if err != nil {
			t.Fatalf("GrantItem failed: %v", err)
		}
		if len(inv.Items) != 1 {
			t.Fatalf("expected 1 item in inventory, got %d", len(inv.Items))
		}
		if inst.DefinitionID != "item-001" {
			t.Fatalf("expected item-001, got %s", inst.DefinitionID)
		}
		if invRepo.saveHits != 1 {
			t.Fatalf("expected 1 Save call, got %d", invRepo.saveHits)
		}
	})
}

func TestGrantItem_InventoryFull(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	invRepo := newTrackingInvRepo()

	char := corecharacter.Character{ID: "char-full"}
	charRepo.chars[char.ID] = char

	svc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(&mockTxProvider{}))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1st grant succeeds
	_, _, err = svc.GrantItem(ctx, char.ID, "item-001", 1)
	if err != nil {
		t.Fatalf("first GrantItem failed: %v", err)
	}

	// 2nd grant into full inventory must fail with ErrInventoryFull
	_, _, err = svc.GrantItem(ctx, char.ID, "item-002", 1)
	if !errors.Is(err, economy.ErrInventoryFull) {
		t.Fatalf("expected ErrInventoryFull, got %v", err)
	}
}

package economy_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/event"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

type runnerTestEvent struct {
	name        string
	characterID string
}

func (e runnerTestEvent) EventName() string {
	return e.name
}

func TestExecuteTransaction_InvalidInputs(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	svc, _ := economy.NewService(charRepo, invRepo)

	// Empty CharacterID
	_, err := svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "",
	}, nil)
	if !errors.Is(err, economy.ErrInvalidCharacterID) {
		t.Errorf("expected ErrInvalidCharacterID, got %v", err)
	}

	// Character not found
	_, err = svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "non-existent",
	}, nil)
	if !errors.Is(err, economy.ErrCharacterNotFound) {
		t.Errorf("expected ErrCharacterNotFound, got %v", err)
	}

	// Negative cost
	char := corecharacter.Character{ID: "char-1", Money: 100, SmallMedals: 5}
	_ = charRepo.Update(context.Background(), char)

	_, err = svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-1",
		Cost:        economy.ResourceCost{Gold: -10},
	}, nil)
	if !errors.Is(err, economy.ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount for negative gold, got %v", err)
	}

	_, err = svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-1",
		Cost:        economy.ResourceCost{SmallMedals: -5},
	}, nil)
	if !errors.Is(err, economy.ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount for negative medals, got %v", err)
	}
}

func TestExecuteTransaction_StaticCost_CurrencyDeduction(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	txProv := &mockTxProvider{}
	svc, _ := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))

	char := corecharacter.Character{ID: "char-1", Money: 500, SmallMedals: 20}
	_ = charRepo.Update(context.Background(), char)

	res, err := svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-1",
		Cost: economy.ResourceCost{
			Gold:        200,
			SmallMedals: 5,
		},
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Character.Money != 300 {
		t.Errorf("expected 300 money, got %d", res.Character.Money)
	}
	if res.Character.SmallMedals != 15 {
		t.Errorf("expected 15 medals, got %d", res.Character.SmallMedals)
	}
	if !txProv.executed {
		t.Error("expected transaction provider to execute")
	}

	// Insufficient gold
	_, err = svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-1",
		Cost:        economy.ResourceCost{Gold: 500},
	}, nil)
	if !errors.Is(err, economy.ErrInsufficientGold) {
		t.Errorf("expected ErrInsufficientGold, got %v", err)
	}

	// Insufficient medals
	_, err = svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-1",
		Cost:        economy.ResourceCost{SmallMedals: 50},
	}, nil)
	if !errors.Is(err, economy.ErrInsufficientMedals) {
		t.Errorf("expected ErrInsufficientMedals, got %v", err)
	}
}

func TestExecuteTransaction_DynamicCostFunc(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	svc, _ := economy.NewService(charRepo, invRepo)

	char := corecharacter.Character{ID: "char-level5", Level: 5, Money: 100}
	_ = charRepo.Update(context.Background(), char)

	// CostFunc computes fee based on level: level * 10 = 50
	res, err := svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-level5",
		CostFunc: func(c corecharacter.Character) (economy.ResourceCost, error) {
			return economy.ResourceCost{Gold: c.Level * 10}, nil
		},
	}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Character.Money != 50 {
		t.Errorf("expected 50 remaining gold, got %d", res.Character.Money)
	}

	// CostFunc returns custom error
	customErr := errors.New("cannot calculate fee")
	_, err = svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-level5",
		CostFunc: func(c corecharacter.Character) (economy.ResourceCost, error) {
			return economy.ResourceCost{}, customErr
		},
	}, nil)
	if !errors.Is(err, customErr) {
		t.Errorf("expected customErr, got %v", err)
	}
}

func TestExecuteTransaction_ItemCostAndGrants(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	svc, _ := economy.NewService(charRepo, invRepo)

	char := corecharacter.Character{ID: "char-crafter", Money: 100}
	_ = charRepo.Update(context.Background(), char)

	inv, _ := coreinventory.New("char-crafter")
	wood, _ := coreitem.NewInstance("wood", 5)
	iron, _ := coreitem.NewInstance("iron", 2)
	_ = inv.Add(wood)
	_ = inv.Add(iron)
	_ = invRepo.Save(context.Background(), inv)

	// Consume 3 wood by definition, and iron by instance ID, grant sword
	res, err := svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-crafter",
		Cost: economy.ResourceCost{
			Gold:              30,
			ItemDefinitionID:  "wood",
			ItemDefinitionQty: 3,
			ItemInstanceID:    iron.ID,
			ItemInstanceQty:   1,
		},
		Grant: economy.ResourceGrant{
			ItemDefinitionID: "sword_bronze",
			ItemQuantity:     1,
		},
	}, func(tc *economy.TxContext) error {
		// Verify in-context inventory state
		if tc.Inventory.Quantity("wood") != 2 {
			t.Errorf("expected 2 wood remaining in txCtx, got %d", tc.Inventory.Quantity("wood"))
		}
		// Add additional gold grant inside callback
		tc.AddGrant(economy.ResourceGrant{Gold: 10})
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Initial 100 - 30 + 10 = 80
	if res.Character.Money != 80 {
		t.Errorf("expected 80 gold, got %d", res.Character.Money)
	}
	if res.Inventory.Quantity("wood") != 2 {
		t.Errorf("expected 2 wood remaining, got %d", res.Inventory.Quantity("wood"))
	}
	if res.Inventory.Quantity("sword_bronze") != 1 {
		t.Errorf("expected 1 sword_bronze granted, got %d", res.Inventory.Quantity("sword_bronze"))
	}
	if res.GrantedItem == nil || res.GrantedItem.DefinitionID != "sword_bronze" {
		t.Errorf("expected GrantedItem sword_bronze, got %v", res.GrantedItem)
	}
}

func TestExecuteTransaction_CallbackMutationAndRollback(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	svc, _ := economy.NewService(charRepo, invRepo)

	char := corecharacter.Character{
		ID:    "char-sleepy",
		Money: 50,
		Stats: corecharacter.Stats{
			MaxHP: 100,
			MaxMP: 50,
			HP:    10,
			MP:    5,
		},
	}
	_ = charRepo.Update(context.Background(), char)

	// Successful callback mutation (Inn pattern)
	res, err := svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-sleepy",
		Cost:        economy.ResourceCost{Gold: 20},
	}, func(tc *economy.TxContext) error {
		tc.Character.Stats.HP = tc.Character.Stats.MaxHP
		tc.Character.Stats.MP = tc.Character.Stats.MaxMP
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Character.Stats.HP != 100 || res.Character.Stats.MP != 50 {
		t.Errorf("expected HP=100 MP=50, got HP=%d MP=%d", res.Character.Stats.HP, res.Character.Stats.MP)
	}
	if res.Character.Money != 30 {
		t.Errorf("expected 30 gold, got %d", res.Character.Money)
	}

	// Callback error triggers rollback
	errCallback := errors.New("something went wrong inside business logic")
	_, err = svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-sleepy",
		Cost:        economy.ResourceCost{Gold: 10},
	}, func(tc *economy.TxContext) error {
		tc.Character.Money = 0 // Attempted mutation
		return errCallback
	})

	if !errors.Is(err, errCallback) {
		t.Errorf("expected errCallback, got %v", err)
	}

	// Verify character in repo was not modified by the failed transaction
	savedChar, _ := charRepo.FindByID(context.Background(), "char-sleepy")
	if savedChar.Money != 30 {
		t.Errorf("expected money to remain 30 after rollback, got %d", savedChar.Money)
	}
}

func TestExecuteTransaction_TwoPhaseEventDispatching(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	dispatcher := event.NewDispatcher()
	svc, _ := economy.NewService(charRepo, invRepo, economy.WithEventDispatcher(dispatcher))

	char := corecharacter.Character{ID: "char-event", Money: 100}
	_ = charRepo.Update(context.Background(), char)

	var syncHandled int64
	var asyncHandled int64

	dispatcher.SubscribeSync("test.action", func(ctx context.Context, evt event.Event) error {
		atomic.AddInt64(&syncHandled, 1)
		return nil
	})
	dispatcher.SubscribeAsync("test.action", func(ctx context.Context, evt event.Event) error {
		atomic.AddInt64(&asyncHandled, 1)
		return nil
	})

	res, err := svc.ExecuteTransaction(context.Background(), economy.TransactionRequest{
		CharacterID: "char-event",
		Cost:        economy.ResourceCost{Gold: 25},
	}, func(tc *economy.TxContext) error {
		tc.EmitEvent(runnerTestEvent{name: "test.action", characterID: tc.Character.ID})
		if len(tc.Events()) != 1 {
			t.Errorf("expected 1 event in tc.Events(), got %d", len(tc.Events()))
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Character.Money != 75 {
		t.Errorf("expected 75 money, got %d", res.Character.Money)
	}

	dispatcher.WaitAsync()

	if atomic.LoadInt64(&syncHandled) != 1 {
		t.Errorf("expected syncHandled 1, got %d", atomic.LoadInt64(&syncHandled))
	}
	if atomic.LoadInt64(&asyncHandled) != 1 {
		t.Errorf("expected asyncHandled 1, got %d", atomic.LoadInt64(&asyncHandled))
	}
}

func TestRun_GenericHelper(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	svc, _ := economy.NewService(charRepo, invRepo)

	char := corecharacter.Character{ID: "char-generic", Money: 100}
	_ = charRepo.Update(context.Background(), char)

	type LodgingReceipt struct {
		RoomName string
		Restored int
	}

	receipt, txRes, err := economy.Run(context.Background(), svc, economy.TransactionRequest{
		CharacterID: "char-generic",
		Cost:        economy.ResourceCost{Gold: 40},
	}, func(tc *economy.TxContext) (LodgingReceipt, error) {
		return LodgingReceipt{
			RoomName: "Deluxe Suite",
			Restored: 50,
		}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receipt.RoomName != "Deluxe Suite" || receipt.Restored != 50 {
		t.Errorf("unexpected receipt outcome: %+v", receipt)
	}
	if txRes.Character.Money != 60 {
		t.Errorf("expected 60 money, got %d", txRes.Character.Money)
	}
}

package depot

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

var errTestBoom = errors.New("database connection failed")

type stubDepotRepo struct {
	findFn          func(ctx context.Context, characterID string) (Depot, error)
	findForUpdateFn func(ctx context.Context, characterID string) (Depot, error)
	saveFn          func(ctx context.Context, value Depot) error
}

func (s *stubDepotRepo) FindByCharacterID(ctx context.Context, characterID string) (Depot, error) {
	if s.findFn != nil {
		return s.findFn(ctx, characterID)
	}
	return Depot{}, ErrNotFound
}

func (s *stubDepotRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (Depot, error) {
	if s.findForUpdateFn != nil {
		return s.findForUpdateFn(ctx, characterID)
	}
	return s.FindByCharacterID(ctx, characterID)
}

func (s *stubDepotRepo) Save(ctx context.Context, value Depot) error {
	if s.saveFn != nil {
		return s.saveFn(ctx, value)
	}
	return nil
}

type stubCharRepo struct {
	findFn          func(ctx context.Context, id string) (corecharacter.Character, error)
	findForUpdateFn func(ctx context.Context, id string) (corecharacter.Character, error)
	updateFn        func(ctx context.Context, character corecharacter.Character) error
}

func (s *stubCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if s.findFn != nil {
		return s.findFn(ctx, id)
	}
	return corecharacter.Character{ID: id, Money: 1000}, nil
}

func (s *stubCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	if s.findForUpdateFn != nil {
		return s.findForUpdateFn(ctx, id)
	}
	return s.FindByID(ctx, id)
}

func (s *stubCharRepo) Update(ctx context.Context, character corecharacter.Character) error {
	if s.updateFn != nil {
		return s.updateFn(ctx, character)
	}
	return nil
}

type stubInvRepo struct {
	findFn          func(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	findForUpdateFn func(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	saveFn          func(ctx context.Context, inventory coreinventory.Inventory) error
}

func (s *stubInvRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if s.findFn != nil {
		return s.findFn(ctx, characterID)
	}
	return coreinventory.New(characterID)
}

func (s *stubInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if s.findForUpdateFn != nil {
		return s.findForUpdateFn(ctx, characterID)
	}
	return s.FindByCharacterID(ctx, characterID)
}

func (s *stubInvRepo) Save(ctx context.Context, inventory coreinventory.Inventory) error {
	if s.saveFn != nil {
		return s.saveFn(ctx, inventory)
	}
	return nil
}

type stubTxProvider struct {
	runInTxFn func(ctx context.Context, fn func(ctx context.Context) error) error
}

func (s *stubTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.runInTxFn != nil {
		return s.runInTxFn(ctx, fn)
	}
	return fn(ctx)
}

type stubTransactionRunner struct {
	executeFn func(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error)
}

func (s *stubTransactionRunner) ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
	if s.executeFn != nil {
		return s.executeFn(ctx, req, fn)
	}
	return nil, nil
}

func TestService_Constructors(t *testing.T) {
	depotRepo := &stubDepotRepo{}
	charRepo := &stubCharRepo{}
	invRepo := &stubInvRepo{}
	txProvider := &stubTxProvider{}

	// NewService validation
	if _, err := NewService(nil, charRepo, invRepo); err == nil {
		t.Fatal("expected error when depotRepo is nil")
	}
	if _, err := NewService(depotRepo, nil, invRepo); err == nil {
		t.Fatal("expected error when charRepo is nil")
	}
	if _, err := NewService(depotRepo, charRepo, nil); err == nil {
		t.Fatal("expected error when invRepo is nil")
	}
	svc, err := NewService(depotRepo, charRepo, invRepo)
	if err != nil || svc == nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// NewServiceWithTransaction validation
	if _, err := NewServiceWithTransaction(nil, charRepo, invRepo, txProvider); err == nil {
		t.Fatal("expected error when depotRepo is nil")
	}
	if _, err := NewServiceWithTransaction(depotRepo, nil, invRepo, txProvider); err == nil {
		t.Fatal("expected error when charRepo is nil")
	}
	if _, err := NewServiceWithTransaction(depotRepo, charRepo, nil, txProvider); err == nil {
		t.Fatal("expected error when invRepo is nil")
	}
	if _, err := NewServiceWithTransaction(depotRepo, charRepo, invRepo, nil); err == nil {
		t.Fatal("expected error when txProvider is nil")
	}
	svcWithTx, err := NewServiceWithTransaction(depotRepo, charRepo, invRepo, txProvider)
	if err != nil || svcWithTx == nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Options validation
	runner := &stubTransactionRunner{}
	svcWithRunner, err := NewService(depotRepo, charRepo, invRepo, WithTransactionRunner(runner))
	if err != nil || svcWithRunner == nil {
		t.Fatalf("unexpected error with runner option: %v", err)
	}
	svcWithProv, err := NewService(depotRepo, charRepo, invRepo, WithTransactionProvider(txProvider))
	if err != nil || svcWithProv == nil {
		t.Fatalf("unexpected error with provider option: %v", err)
	}
	eco, err := economy.NewService(charRepo, invRepo)
	if err != nil {
		t.Fatal(err)
	}
	svcWithEco, err := NewService(depotRepo, charRepo, invRepo, WithEconomy(eco))
	if err != nil || svcWithEco == nil {
		t.Fatalf("unexpected error with economy option: %v", err)
	}
}

func TestGetDepot_Extended(t *testing.T) {
	ctx := context.Background()

	// 1. Invalid CharacterID
	svc, _ := NewService(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{})
	if _, err := svc.GetDepot(ctx, ""); !errors.Is(err, ErrInvalidCharacterID) {
		t.Fatalf("expected ErrInvalidCharacterID, got %v", err)
	}

	// 2. Depot not found -> returns empty depot
	depotRepoNotFound := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, ErrNotFound
		},
	}
	svcNotFound, _ := NewService(depotRepoNotFound, &stubCharRepo{}, &stubInvRepo{})
	dep, err := svcNotFound.GetDepot(ctx, "char-1")
	if err != nil {
		t.Fatalf("unexpected error on not found: %v", err)
	}
	if dep.CharacterID != "char-1" || dep.Capacity != DefaultDepotCapacity || dep.Gold != 0 {
		t.Fatalf("unexpected depot: %#v", dep)
	}

	// 3. Depot found -> returns existing depot
	existingDepot, _ := NewDepot("char-1")
	existingDepot.Gold = 500
	depotRepoFound := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return existingDepot, nil
		},
	}
	svcFound, _ := NewService(depotRepoFound, &stubCharRepo{}, &stubInvRepo{})
	dep, err = svcFound.GetDepot(ctx, "char-1")
	if err != nil {
		t.Fatalf("unexpected error on found: %v", err)
	}
	if dep.Gold != 500 {
		t.Fatalf("expected gold 500, got %d", dep.Gold)
	}

	// 4. Depot repository error
	repoErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	}
	svcErr, _ := NewService(repoErr, &stubCharRepo{}, &stubInvRepo{})
	_, err = svcErr.GetDepot(ctx, "char-1")
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}
}

func TestDepositGold_ValidationAndExecutionErrors(t *testing.T) {
	ctx := context.Background()

	char, _ := corecharacter.New("Hero")
	char.Money = 1000

	baseService := func(depotRepo Repository, charRepo CharacterRepository) *Service {
		svc, _ := NewService(depotRepo, charRepo, &stubInvRepo{})
		return svc
	}

	// Invalid character ID
	svc := baseService(&stubDepotRepo{}, &stubCharRepo{})
	if _, err := svc.DepositGold(ctx, "", 100); !errors.Is(err, ErrInvalidCharacterID) {
		t.Fatalf("expected ErrInvalidCharacterID, got %v", err)
	}

	// Invalid amount
	if _, err := svc.DepositGold(ctx, "char-1", 0); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}
	if _, err := svc.DepositGold(ctx, "char-1", -50); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}

	// CharacterRepo.FindByID error
	charRepoErr := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{}, errTestBoom
		},
	}
	svc = baseService(&stubDepotRepo{}, charRepoErr)
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// Insufficient funds
	poorChar, _ := corecharacter.New("Hero")
	poorChar.Money = 50
	charRepoPoor := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return poorChar, nil
		},
	}
	svc = baseService(&stubDepotRepo{}, charRepoPoor)
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	// DepotRepo.FindByCharacterID returns generic error
	depotRepoErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	}
	charRepoOK := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
	}
	svc = baseService(depotRepoErr, charRepoOK)
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// DepotRepo.FindByCharacterID returns ErrNotFound -> creates new depot successfully
	depotRepoNotFound := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, ErrNotFound
		},
		saveFn: func(ctx context.Context, value Depot) error {
			if value.Gold != 100 {
				t.Fatalf("expected depot gold 100, got %d", value.Gold)
			}
			return nil
		},
	}
	var updatedMoney int
	charRepoUpdateCheck := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
		updateFn: func(ctx context.Context, character corecharacter.Character) error {
			updatedMoney = character.Money
			return nil
		},
	}
	svc = baseService(depotRepoNotFound, charRepoUpdateCheck)
	dep, err := svc.DepositGold(ctx, "char-1", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dep.Gold != 100 {
		t.Fatalf("expected depot gold 100, got %d", dep.Gold)
	}
	if updatedMoney != 900 {
		t.Fatalf("expected character money 900, got %d", updatedMoney)
	}

	// CharacterRepo.Update error
	charRepoUpdateErr := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
		updateFn: func(ctx context.Context, character corecharacter.Character) error {
			return errTestBoom
		},
	}
	svc = baseService(&stubDepotRepo{}, charRepoUpdateErr)
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on update, got %v", err)
	}

	// DepotRepo.Save error
	depotRepoSaveErr := &stubDepotRepo{
		saveFn: func(ctx context.Context, value Depot) error {
			return errTestBoom
		},
	}
	svc = baseService(depotRepoSaveErr, charRepoOK)
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on save, got %v", err)
	}
}

func TestWithdrawGold_ValidationAndExecutionErrors(t *testing.T) {
	ctx := context.Background()

	char, _ := corecharacter.New("Hero")
	char.Money = 500

	depotVal, _ := NewDepot("char-1")
	depotVal.Gold = 300

	baseService := func(depotRepo Repository, charRepo CharacterRepository) *Service {
		svc, _ := NewService(depotRepo, charRepo, &stubInvRepo{})
		return svc
	}

	// Invalid character ID
	svc := baseService(&stubDepotRepo{}, &stubCharRepo{})
	if _, err := svc.WithdrawGold(ctx, "", 100); !errors.Is(err, ErrInvalidCharacterID) {
		t.Fatalf("expected ErrInvalidCharacterID, got %v", err)
	}

	// Invalid amount
	if _, err := svc.WithdrawGold(ctx, "char-1", 0); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}
	if _, err := svc.WithdrawGold(ctx, "char-1", -10); !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got %v", err)
	}

	// DepotRepo.FindByCharacterID error
	depotRepoFindErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	}
	svc = baseService(depotRepoFindErr, &stubCharRepo{})
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// Insufficient depot gold
	depotRepoOK := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
	}
	svc = baseService(depotRepoOK, &stubCharRepo{})
	if _, err := svc.WithdrawGold(ctx, "char-1", 500); !errors.Is(err, ErrInsufficientDepotGold) {
		t.Fatalf("expected ErrInsufficientDepotGold, got %v", err)
	}

	// CharacterRepo.FindByID error
	charRepoFindErr := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{}, errTestBoom
		},
	}
	svc = baseService(depotRepoOK, charRepoFindErr)
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on char find, got %v", err)
	}

	// DepotRepo.Save error
	charRepoOK := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
	}
	depotRepoSaveErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		saveFn: func(ctx context.Context, value Depot) error {
			return errTestBoom
		},
	}
	svc = baseService(depotRepoSaveErr, charRepoOK)
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on depot save, got %v", err)
	}

	// CharacterRepo.Update error
	charRepoUpdateErr := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
		updateFn: func(ctx context.Context, character corecharacter.Character) error {
			return errTestBoom
		},
	}
	svc = baseService(depotRepoOK, charRepoUpdateErr)
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on char update, got %v", err)
	}
}

func TestDepositItem_ValidationAndExecutionErrors(t *testing.T) {
	ctx := context.Background()

	potion, _ := item.NewInstance("potion", 2)
	inv, _ := coreinventory.New("char-1")
	_ = inv.Add(potion)

	baseService := func(depotRepo Repository, invRepo InventoryRepository) *Service {
		svc, _ := NewService(depotRepo, &stubCharRepo{}, invRepo)
		return svc
	}

	// Invalid character ID
	svc := baseService(&stubDepotRepo{}, &stubInvRepo{})
	if _, err := svc.DepositItem(ctx, "", potion.ID); !errors.Is(err, ErrInvalidCharacterID) {
		t.Fatalf("expected ErrInvalidCharacterID, got %v", err)
	}

	// Invalid itemInstanceID
	if _, err := svc.DepositItem(ctx, "char-1", ""); !errors.Is(err, ErrInvalidItemInstanceID) {
		t.Fatalf("expected ErrInvalidItemInstanceID, got %v", err)
	}

	// Transaction runner error
	runnerErr := &stubTransactionRunner{
		executeFn: func(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
			return nil, errTestBoom
		},
	}
	svcWithRunner, _ := NewService(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, WithTransactionRunner(runnerErr))
	if _, err := svcWithRunner.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// DepotRepo.FindByCharacterID returns generic error
	depotRepoErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	}
	invRepoOK := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return inv, nil
		},
	}
	svc = baseService(depotRepoErr, invRepoOK)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// Depot is full
	fullDepot, _ := NewDepot("char-1")
	fullDepot.Capacity = 1
	dummyItem, _ := item.NewInstance("dummy", 1)
	_ = fullDepot.AddItem(dummyItem)
	depotRepoFull := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return fullDepot, nil
		},
	}
	svc = baseService(depotRepoFull, invRepoOK)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got %v", err)
	}

	// Item not found in inventory
	emptyInv, _ := coreinventory.New("char-1")
	invRepoEmpty := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return emptyInv, nil
		},
	}
	svc = baseService(&stubDepotRepo{}, invRepoEmpty)
	if _, err := svc.DepositItem(ctx, "char-1", "non-existent-id"); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}

	// InvRepo.Save error
	invRepoSaveErr := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			testInv, _ := coreinventory.New("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = testInv.Add(p)
			return testInv, nil
		},
		saveFn: func(ctx context.Context, inventory coreinventory.Inventory) error {
			return errTestBoom
		},
	}
	svc = baseService(&stubDepotRepo{}, invRepoSaveErr)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on inv save, got %v", err)
	}

	// DepotRepo.Save error
	invRepoWithPotion := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			testInv, _ := coreinventory.New("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = testInv.Add(p)
			return testInv, nil
		},
	}
	depotRepoSaveErr := &stubDepotRepo{
		saveFn: func(ctx context.Context, value Depot) error {
			return errTestBoom
		},
	}
	svc = baseService(depotRepoSaveErr, invRepoWithPotion)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on depot save, got %v", err)
	}
}

func TestWithdrawItem_ValidationAndExecutionErrors(t *testing.T) {
	ctx := context.Background()

	potion, _ := item.NewInstance("potion", 2)
	depotWithItem, _ := NewDepot("char-1")
	_ = depotWithItem.AddItem(potion)

	baseService := func(depotRepo Repository, invRepo InventoryRepository) *Service {
		svc, _ := NewService(depotRepo, &stubCharRepo{}, invRepo)
		return svc
	}

	// Invalid character ID
	svc := baseService(&stubDepotRepo{}, &stubInvRepo{})
	if _, err := svc.WithdrawItem(ctx, "", potion.ID); !errors.Is(err, ErrInvalidCharacterID) {
		t.Fatalf("expected ErrInvalidCharacterID, got %v", err)
	}

	// Invalid itemInstanceID
	if _, err := svc.WithdrawItem(ctx, "char-1", ""); !errors.Is(err, ErrInvalidItemInstanceID) {
		t.Fatalf("expected ErrInvalidItemInstanceID, got %v", err)
	}

	// DepotRepo.FindByCharacterID error
	depotRepoErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	}
	svc = baseService(depotRepoErr, &stubInvRepo{})
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// Transaction runner error
	depotRepoOK := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotWithItem, nil
		},
	}
	runnerErr := &stubTransactionRunner{
		executeFn: func(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
			return nil, errTestBoom
		},
	}
	svcWithRunner, _ := NewService(depotRepoOK, &stubCharRepo{}, &stubInvRepo{}, WithTransactionRunner(runnerErr))
	if _, err := svcWithRunner.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on runner error, got %v", err)
	}

	// Item not found in depot
	svc = baseService(depotRepoOK, &stubInvRepo{})
	if _, err := svc.WithdrawItem(ctx, "char-1", "non-existent-item"); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}

	// Inventory full (inv.Add fails because item ID already in inv)
	invWithSameItem, _ := coreinventory.New("char-1")
	_ = invWithSameItem.Add(potion)
	invRepoWithSameItem := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return invWithSameItem, nil
		},
	}
	svc = baseService(depotRepoOK, invRepoWithSameItem)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, ErrInventoryFull) {
		t.Fatalf("expected ErrInventoryFull, got %v", err)
	}

	// DepotRepo.Save error
	depotRepoSaveErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			d, _ := NewDepot("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = d.AddItem(p)
			return d, nil
		},
		saveFn: func(ctx context.Context, value Depot) error {
			return errTestBoom
		},
	}
	svc = baseService(depotRepoSaveErr, &stubInvRepo{})
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on depot save, got %v", err)
	}

	// InvRepo.Save error
	invRepoSaveErr := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return coreinventory.New("char-1")
		},
		saveFn: func(ctx context.Context, inventory coreinventory.Inventory) error {
			return errTestBoom
		},
	}
	svc = baseService(depotRepoOK, invRepoSaveErr)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on inv save, got %v", err)
	}
}

func TestTransactionPaths_DepositGold(t *testing.T) {
	ctx := context.Background()

	char, _ := corecharacter.New("Hero")
	char.Money = 500

	depotVal, _ := NewDepot("char-1")
	depotVal.Gold = 100

	// 1. Transaction runner error
	runnerErr := &stubTransactionRunner{
		executeFn: func(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
			return nil, errTestBoom
		},
	}
	svc, _ := NewService(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, WithTransactionRunner(runnerErr))
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 2. Character not found error
	charRepoNotFound := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	svc, _ = NewService(&stubDepotRepo{}, charRepoNotFound, &stubInvRepo{})
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("expected corecharacter.ErrNotFound, got %v", err)
	}

	// 3. Insufficient funds error
	charPoor := char
	charPoor.Money = 50
	charRepoPoor := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return charPoor, nil
		},
	}
	svc, _ = NewService(&stubDepotRepo{}, charRepoPoor, &stubInvRepo{})
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	// 4. DepotRepo FindByCharacterIDForUpdate generic error
	depotRepoErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	}
	charRepoOK := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
	}
	svc, _ = NewService(depotRepoErr, charRepoOK, &stubInvRepo{})
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 5. DepotRepo FindByCharacterIDForUpdate ErrNotFound -> creates new depot and continues
	var savedDepotGold int
	depotRepoNotFound := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, ErrNotFound
		},
		saveFn: func(ctx context.Context, depot Depot) error {
			savedDepotGold = depot.Gold
			return nil
		},
	}
	svc, _ = NewService(depotRepoNotFound, charRepoOK, &stubInvRepo{})
	res, err := svc.DepositGold(ctx, "char-1", 100)
	if err != nil {
		t.Fatalf("unexpected error when depot ErrNotFound: %v", err)
	}
	if res.Gold != 100 || savedDepotGold != 100 {
		t.Fatalf("expected saved depot gold 100, got res=%d, saved=%d", res.Gold, savedDepotGold)
	}

	// 6. DepotRepo Save error
	depotRepoSaveErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		saveFn: func(ctx context.Context, depot Depot) error {
			return errTestBoom
		},
	}
	svc, _ = NewService(depotRepoSaveErr, charRepoOK, &stubInvRepo{})
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 7. CharacterRepo Update error
	charRepoUpdateErr := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
		updateFn: func(ctx context.Context, character corecharacter.Character) error {
			return errTestBoom
		},
	}
	depotRepoOK := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
	}
	svc, _ = NewService(depotRepoOK, charRepoUpdateErr, &stubInvRepo{})
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 8. WithTransactionProvider error
	txProviderErr := &stubTxProvider{
		runInTxFn: func(ctx context.Context, fn func(ctx context.Context) error) error {
			return errTestBoom
		},
	}
	svc, _ = NewServiceWithTransaction(depotRepoOK, charRepoOK, &stubInvRepo{}, txProviderErr)
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom with txProvider, got %v", err)
	}
}

func TestTransactionPaths_WithdrawGold(t *testing.T) {
	ctx := context.Background()

	char, _ := corecharacter.New("Hero")
	char.Money = 500

	depotVal, _ := NewDepot("char-1")
	depotVal.Gold = 300

	// 1. Transaction runner error
	runnerErr := &stubTransactionRunner{
		executeFn: func(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
			return nil, errTestBoom
		},
	}
	svc, _ := NewService(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, WithTransactionRunner(runnerErr))
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 2. Character not found error
	charRepoNotFound := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	svc, _ = NewService(&stubDepotRepo{}, charRepoNotFound, &stubInvRepo{})
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("expected corecharacter.ErrNotFound, got %v", err)
	}

	// 3. DepotRepo FindByCharacterIDForUpdate error
	depotRepoFindErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	}
	charRepoOK := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
	}
	svc, _ = NewService(depotRepoFindErr, charRepoOK, &stubInvRepo{})
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 4. Insufficient depot gold
	depotRepoOK := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
	}
	svc, _ = NewService(depotRepoOK, charRepoOK, &stubInvRepo{})
	if _, err := svc.WithdrawGold(ctx, "char-1", 500); !errors.Is(err, ErrInsufficientDepotGold) {
		t.Fatalf("expected ErrInsufficientDepotGold, got %v", err)
	}

	// 5. DepotRepo Save error
	depotRepoSaveErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		saveFn: func(ctx context.Context, value Depot) error {
			return errTestBoom
		},
	}
	svc, _ = NewService(depotRepoSaveErr, charRepoOK, &stubInvRepo{})
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 6. CharacterRepo Update error
	charRepoUpdateErr := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
		updateFn: func(ctx context.Context, character corecharacter.Character) error {
			return errTestBoom
		},
	}
	svc, _ = NewService(depotRepoOK, charRepoUpdateErr, &stubInvRepo{})
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on char update, got %v", err)
	}

	// 7. WithTransactionProvider error
	txProviderErr := &stubTxProvider{
		runInTxFn: func(ctx context.Context, fn func(ctx context.Context) error) error {
			return errTestBoom
		},
	}
	svc, _ = NewServiceWithTransaction(depotRepoOK, charRepoOK, &stubInvRepo{}, txProviderErr)
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom with txProvider, got %v", err)
	}

	// 8. Happy path
	svc, _ = NewService(depotRepoOK, charRepoOK, &stubInvRepo{})
	res, err := svc.WithdrawGold(ctx, "char-1", 100)
	if err != nil {
		t.Fatalf("unexpected error on happy path: %v", err)
	}
	if res.Gold != 200 {
		t.Fatalf("expected depot gold 200, got %d", res.Gold)
	}
}

func TestTransactionPaths_DepositItem(t *testing.T) {
	ctx := context.Background()

	potion, _ := item.NewInstance("potion", 2)
	inv, _ := coreinventory.New("char-1")
	_ = inv.Add(potion)

	char, _ := corecharacter.New("Hero")
	depotVal, _ := NewDepot("char-1")

	charRepoOK := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
	}
	invRepoOK := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return inv, nil
		},
	}
	depotRepoOK := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
	}

	// 1. Transaction runner error
	runnerErr := &stubTransactionRunner{
		executeFn: func(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
			return nil, errTestBoom
		},
	}
	svc, _ := NewService(depotRepoOK, charRepoOK, invRepoOK, WithTransactionRunner(runnerErr))
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 2. Character not found error
	charRepoNotFound := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	svc, _ = NewService(depotRepoOK, charRepoNotFound, invRepoOK)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("expected corecharacter.ErrNotFound, got %v", err)
	}

	// 3. CharacterRepo Update error
	charRepoUpdateErr := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
		updateFn: func(ctx context.Context, character corecharacter.Character) error {
			return errTestBoom
		},
	}
	svc, _ = NewService(depotRepoOK, charRepoUpdateErr, invRepoOK)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on char update, got %v", err)
	}

	// 4. Depot full
	fullDepot, _ := NewDepot("char-1")
	fullDepot.Capacity = 1
	dummy, _ := item.NewInstance("dummy", 1)
	_ = fullDepot.AddItem(dummy)
	depotRepoFull := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return fullDepot, nil
		},
	}
	svc, _ = NewService(depotRepoFull, charRepoOK, invRepoOK)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got %v", err)
	}

	// 5. Item not in inventory
	svc, _ = NewService(depotRepoOK, charRepoOK, invRepoOK)
	if _, err := svc.DepositItem(ctx, "char-1", "nonexistent-id"); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}

	// 6. DepotRepo Find generic error
	depotRepoErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	}
	svc, _ = NewService(depotRepoErr, charRepoOK, invRepoOK)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 7. DepotRepo Find ErrNotFound -> creates depot and succeeds
	var savedDepotCount int
	depotRepoNotFound := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, ErrNotFound
		},
		saveFn: func(ctx context.Context, value Depot) error {
			savedDepotCount = len(value.Items)
			return nil
		},
	}
	svc, _ = NewService(depotRepoNotFound, charRepoOK, invRepoOK)
	res, err := svc.DepositItem(ctx, "char-1", potion.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 || savedDepotCount != 1 {
		t.Fatalf("expected 1 item in depot, got res=%d, saved=%d", len(res.Items), savedDepotCount)
	}

	// 8. DepotRepo Save error
	depotRepoSaveErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		saveFn: func(ctx context.Context, value Depot) error {
			return errTestBoom
		},
	}
	svc, _ = NewService(depotRepoSaveErr, charRepoOK, invRepoOK)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on depot save, got %v", err)
	}

	// 9. InvRepo Save error
	invRepoSaveErr := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			testInv, _ := coreinventory.New("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = testInv.Add(p)
			return testInv, nil
		},
		saveFn: func(ctx context.Context, inventory coreinventory.Inventory) error {
			return errTestBoom
		},
	}
	svc, _ = NewService(depotRepoOK, charRepoOK, invRepoSaveErr)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on inv save, got %v", err)
	}

	// 10. WithTransactionProvider error
	txProviderErr := &stubTxProvider{
		runInTxFn: func(ctx context.Context, fn func(ctx context.Context) error) error {
			return errTestBoom
		},
	}
	svc, _ = NewServiceWithTransaction(depotRepoOK, charRepoOK, invRepoOK, txProviderErr)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom with txProvider, got %v", err)
	}
}

func TestTransactionPaths_WithdrawItem(t *testing.T) {
	ctx := context.Background()

	potion, _ := item.NewInstance("potion", 2)
	depotWithItem, _ := NewDepot("char-1")
	_ = depotWithItem.AddItem(potion)

	char, _ := corecharacter.New("Hero")
	emptyInv, _ := coreinventory.New("char-1")

	charRepoOK := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
	}
	invRepoOK := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return emptyInv, nil
		},
	}
	depotRepoOK := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotWithItem, nil
		},
	}

	// 1. Transaction runner error
	runnerErr := &stubTransactionRunner{
		executeFn: func(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
			return nil, errTestBoom
		},
	}
	svc, _ := NewService(depotRepoOK, charRepoOK, invRepoOK, WithTransactionRunner(runnerErr))
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 2. Character not found error
	charRepoNotFound := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	svc, _ = NewService(depotRepoOK, charRepoNotFound, invRepoOK)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("expected corecharacter.ErrNotFound, got %v", err)
	}

	// 3. CharacterRepo Update error
	charRepoUpdateErr := &stubCharRepo{
		findFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return char, nil
		},
		updateFn: func(ctx context.Context, character corecharacter.Character) error {
			return errTestBoom
		},
	}
	svc, _ = NewService(depotRepoOK, charRepoUpdateErr, invRepoOK)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on char update, got %v", err)
	}

	// 4. DepotRepo Find generic error
	depotRepoErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	}
	svc, _ = NewService(depotRepoErr, charRepoOK, invRepoOK)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 5. DepotRepo Find ErrNotFound -> returns ErrNotFound
	depotRepoNotFound := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, ErrNotFound
		},
	}
	svc, _ = NewService(depotRepoNotFound, charRepoOK, invRepoOK)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 6. Item not found in depot
	svc, _ = NewService(depotRepoOK, charRepoOK, invRepoOK)
	if _, err := svc.WithdrawItem(ctx, "char-1", "nonexistent-item"); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}

	// 7. Inventory full (item ID already present in inventory)
	invWithSameItem, _ := coreinventory.New("char-1")
	_ = invWithSameItem.Add(potion)
	invRepoWithSameItem := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return invWithSameItem, nil
		},
	}
	svc, _ = NewService(depotRepoOK, charRepoOK, invRepoWithSameItem)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, ErrInventoryFull) {
		t.Fatalf("expected ErrInventoryFull, got %v", err)
	}

	// 8. DepotRepo Save error
	depotRepoSaveErr := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			d, _ := NewDepot("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = d.AddItem(p)
			return d, nil
		},
		saveFn: func(ctx context.Context, value Depot) error {
			return errTestBoom
		},
	}
	svc, _ = NewService(depotRepoSaveErr, charRepoOK, invRepoOK)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on depot save, got %v", err)
	}

	// 9. InvRepo Save error
	invRepoSaveErr := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return coreinventory.New("char-1")
		},
		saveFn: func(ctx context.Context, inventory coreinventory.Inventory) error {
			return errTestBoom
		},
	}
	svc, _ = NewService(depotRepoOK, charRepoOK, invRepoSaveErr)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on inv save, got %v", err)
	}

	// 10. WithTransactionProvider error
	txProviderErr := &stubTxProvider{
		runInTxFn: func(ctx context.Context, fn func(ctx context.Context) error) error {
			return errTestBoom
		},
	}
	svc, _ = NewServiceWithTransaction(depotRepoOK, charRepoOK, invRepoOK, txProviderErr)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom with txProvider, got %v", err)
	}

	// 11. Happy path
	freshDepot, _ := NewDepot("char-1")
	freshPotion, _ := item.NewInstance("potion", 2)
	freshPotion.ID = potion.ID
	_ = freshDepot.AddItem(freshPotion)

	depotRepoFresh := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return freshDepot, nil
		},
	}
	svc, _ = NewService(depotRepoFresh, charRepoOK, invRepoOK)
	res, err := svc.WithdrawItem(ctx, "char-1", potion.ID)
	if err != nil {
		t.Fatalf("unexpected error on happy path: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected 0 items in depot, got %d", len(res.Items))
	}
}

package depot

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

var errTestBoom = errors.New("database connection failed")

type stubDepotRepo struct {
	findFn func(ctx context.Context, characterID string) (Depot, error)
	saveFn func(ctx context.Context, value Depot) error
}

func (s *stubDepotRepo) FindByCharacterID(ctx context.Context, characterID string) (Depot, error) {
	if s.findFn != nil {
		return s.findFn(ctx, characterID)
	}
	return Depot{}, ErrNotFound
}

func (s *stubDepotRepo) Save(ctx context.Context, value Depot) error {
	if s.saveFn != nil {
		return s.saveFn(ctx, value)
	}
	return nil
}

type stubCharRepo struct {
	findFn   func(ctx context.Context, id string) (corecharacter.Character, error)
	updateFn func(ctx context.Context, character corecharacter.Character) error
}

func (s *stubCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if s.findFn != nil {
		return s.findFn(ctx, id)
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func (s *stubCharRepo) Update(ctx context.Context, character corecharacter.Character) error {
	if s.updateFn != nil {
		return s.updateFn(ctx, character)
	}
	return nil
}

type stubInvRepo struct {
	findFn func(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	saveFn func(ctx context.Context, inventory coreinventory.Inventory) error
}

func (s *stubInvRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if s.findFn != nil {
		return s.findFn(ctx, characterID)
	}
	return coreinventory.New(characterID)
}

func (s *stubInvRepo) Save(ctx context.Context, inventory coreinventory.Inventory) error {
	if s.saveFn != nil {
		return s.saveFn(ctx, inventory)
	}
	return nil
}

type stubTxRepo struct {
	executeFn func(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

func (s *stubTxRepo) Execute(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
	if s.executeFn != nil {
		return s.executeFn(ctx, fn)
	}
	return nil
}

type stubTx struct {
	getCharFn   func(ctx context.Context, characterID string) (corecharacter.Character, error)
	saveCharFn  func(ctx context.Context, character corecharacter.Character) error
	getInvFn    func(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	saveInvFn   func(ctx context.Context, inventory coreinventory.Inventory) error
	getDepotFn  func(ctx context.Context, characterID string) (Depot, error)
	saveDepotFn func(ctx context.Context, depot Depot) error
}

func (t *stubTx) GetCharacter(ctx context.Context, characterID string) (corecharacter.Character, error) {
	if t.getCharFn != nil {
		return t.getCharFn(ctx, characterID)
	}
	return corecharacter.Character{}, nil
}

func (t *stubTx) SaveCharacter(ctx context.Context, character corecharacter.Character) error {
	if t.saveCharFn != nil {
		return t.saveCharFn(ctx, character)
	}
	return nil
}

func (t *stubTx) GetInventory(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if t.getInvFn != nil {
		return t.getInvFn(ctx, characterID)
	}
	return coreinventory.New(characterID)
}

func (t *stubTx) SaveInventory(ctx context.Context, inventory coreinventory.Inventory) error {
	if t.saveInvFn != nil {
		return t.saveInvFn(ctx, inventory)
	}
	return nil
}

func (t *stubTx) GetDepot(ctx context.Context, characterID string) (Depot, error) {
	if t.getDepotFn != nil {
		return t.getDepotFn(ctx, characterID)
	}
	return Depot{}, nil
}

func (t *stubTx) SaveDepot(ctx context.Context, depot Depot) error {
	if t.saveDepotFn != nil {
		return t.saveDepotFn(ctx, depot)
	}
	return nil
}

func TestService_Constructors(t *testing.T) {
	depotRepo := &stubDepotRepo{}
	charRepo := &stubCharRepo{}
	invRepo := &stubInvRepo{}
	txRepo := &stubTxRepo{}

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
	if _, err := NewServiceWithTransaction(nil, charRepo, invRepo, txRepo); err == nil {
		t.Fatal("expected error when depotRepo is nil")
	}
	if _, err := NewServiceWithTransaction(depotRepo, nil, invRepo, txRepo); err == nil {
		t.Fatal("expected error when charRepo is nil")
	}
	if _, err := NewServiceWithTransaction(depotRepo, charRepo, nil, txRepo); err == nil {
		t.Fatal("expected error when invRepo is nil")
	}
	if _, err := NewServiceWithTransaction(depotRepo, charRepo, invRepo, nil); err == nil {
		t.Fatal("expected error when txRepo is nil")
	}
	svcWithTx, err := NewServiceWithTransaction(depotRepo, charRepo, invRepo, txRepo)
	if err != nil || svcWithTx == nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetDepot_Extended(t *testing.T) {
	ctx := context.Background()

	// Existing depot
	expectedDepot, _ := NewDepot("char-1")
	expectedDepot.Gold = 12345
	repo := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return expectedDepot, nil
		},
	}
	svc, _ := NewService(repo, &stubCharRepo{}, &stubInvRepo{})
	got, err := svc.GetDepot(ctx, "char-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Gold != 12345 {
		t.Fatalf("expected gold 12345, got %d", got.Gold)
	}

	// Repository error
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

func TestDepositGold_ValidationAndFallbackErrors(t *testing.T) {
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

func TestWithdrawGold_ValidationAndFallbackErrors(t *testing.T) {
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

func TestDepositItem_ValidationAndFallbackErrors(t *testing.T) {
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

	// InvRepo.FindByCharacterID error
	invRepoErr := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return coreinventory.Inventory{}, errTestBoom
		},
	}
	svc = baseService(&stubDepotRepo{}, invRepoErr)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
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

func TestWithdrawItem_ValidationAndFallbackErrors(t *testing.T) {
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

	// InvRepo.FindByCharacterID error
	depotRepoOK := &stubDepotRepo{
		findFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotWithItem, nil
		},
	}
	invRepoErr := &stubInvRepo{
		findFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return coreinventory.Inventory{}, errTestBoom
		},
	}
	svc = baseService(depotRepoOK, invRepoErr)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom on inv find, got %v", err)
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

	// 1. txRepo.Execute error
	txRepoExecErr := &stubTxRepo{
		executeFn: func(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
			return errTestBoom
		},
	}
	svc, _ := NewServiceWithTransaction(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, txRepoExecErr)
	if _, err := svc.DepositGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// Helper for tx tests
	runTx := func(tx *stubTx) error {
		repo := &stubTxRepo{
			executeFn: func(ctx context.Context, fn func(ctx context.Context, txArg Tx) error) error {
				return fn(ctx, tx)
			},
		}
		service, _ := NewServiceWithTransaction(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, repo)
		_, err := service.DepositGold(ctx, "char-1", 100)
		return err
	}

	// 2. tx.GetCharacter error
	err := runTx(&stubTx{
		getCharFn: func(ctx context.Context, characterID string) (corecharacter.Character, error) {
			return corecharacter.Character{}, errTestBoom
		},
	})
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 3. char.Money < amount -> ErrInsufficientFunds
	err = runTx(&stubTx{
		getCharFn: func(ctx context.Context, characterID string) (corecharacter.Character, error) {
			c := char
			c.Money = 50
			return c, nil
		},
	})
	if !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}

	// 4. tx.GetDepot generic error
	err = runTx(&stubTx{
		getCharFn: func(ctx context.Context, characterID string) (corecharacter.Character, error) {
			return char, nil
		},
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	})
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 5. tx.GetDepot ErrNotFound -> creates new depot and continues
	var savedDepotGold int
	err = runTx(&stubTx{
		getCharFn: func(ctx context.Context, characterID string) (corecharacter.Character, error) {
			return char, nil
		},
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, ErrNotFound
		},
		saveDepotFn: func(ctx context.Context, depot Depot) error {
			savedDepotGold = depot.Gold
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error when depot ErrNotFound: %v", err)
	}
	if savedDepotGold != 100 {
		t.Fatalf("expected saved depot gold 100, got %d", savedDepotGold)
	}

	// 6. tx.SaveCharacter error
	err = runTx(&stubTx{
		getCharFn: func(ctx context.Context, characterID string) (corecharacter.Character, error) {
			return char, nil
		},
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		saveCharFn: func(ctx context.Context, character corecharacter.Character) error {
			return errTestBoom
		},
	})
	if err == nil || !errors.Is(err, errTestBoom) {
		t.Fatalf("expected wrapped errTestBoom, got %v", err)
	}

	// 7. tx.SaveDepot error
	err = runTx(&stubTx{
		getCharFn: func(ctx context.Context, characterID string) (corecharacter.Character, error) {
			return char, nil
		},
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		saveDepotFn: func(ctx context.Context, depot Depot) error {
			return errTestBoom
		},
	})
	if err == nil || !errors.Is(err, errTestBoom) {
		t.Fatalf("expected wrapped errTestBoom, got %v", err)
	}
}

func TestTransactionPaths_WithdrawGold(t *testing.T) {
	ctx := context.Background()

	char, _ := corecharacter.New("Hero")
	char.Money = 500

	depotVal, _ := NewDepot("char-1")
	depotVal.Gold = 300

	// 1. txRepo.Execute error
	txRepoExecErr := &stubTxRepo{
		executeFn: func(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
			return errTestBoom
		},
	}
	svc, _ := NewServiceWithTransaction(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, txRepoExecErr)
	if _, err := svc.WithdrawGold(ctx, "char-1", 100); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	runTx := func(tx *stubTx) (Depot, error) {
		repo := &stubTxRepo{
			executeFn: func(ctx context.Context, fn func(ctx context.Context, txArg Tx) error) error {
				return fn(ctx, tx)
			},
		}
		service, _ := NewServiceWithTransaction(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, repo)
		return service.WithdrawGold(ctx, "char-1", 100)
	}

	// 2. tx.GetDepot error
	_, err := runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	})
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 3. dep.Gold < amount -> ErrInsufficientDepotGold
	_, err = runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			d := depotVal
			d.Gold = 50
			return d, nil
		},
	})
	if !errors.Is(err, ErrInsufficientDepotGold) {
		t.Fatalf("expected ErrInsufficientDepotGold, got %v", err)
	}

	// 4. tx.GetCharacter error
	_, err = runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		getCharFn: func(ctx context.Context, characterID string) (corecharacter.Character, error) {
			return corecharacter.Character{}, errTestBoom
		},
	})
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 5. tx.SaveDepot error
	_, err = runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		getCharFn: func(ctx context.Context, characterID string) (corecharacter.Character, error) {
			return char, nil
		},
		saveDepotFn: func(ctx context.Context, depot Depot) error {
			return errTestBoom
		},
	})
	if err == nil || !errors.Is(err, errTestBoom) {
		t.Fatalf("expected wrapped errTestBoom, got %v", err)
	}

	// 6. tx.SaveCharacter error
	_, err = runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		getCharFn: func(ctx context.Context, characterID string) (corecharacter.Character, error) {
			return char, nil
		},
		saveCharFn: func(ctx context.Context, character corecharacter.Character) error {
			return errTestBoom
		},
	})
	if err == nil || !errors.Is(err, errTestBoom) {
		t.Fatalf("expected wrapped errTestBoom, got %v", err)
	}

	// 7. Happy path
	res, err := runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		getCharFn: func(ctx context.Context, characterID string) (corecharacter.Character, error) {
			return char, nil
		},
	})
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

	depotVal, _ := NewDepot("char-1")

	// 1. txRepo.Execute error
	txRepoExecErr := &stubTxRepo{
		executeFn: func(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
			return errTestBoom
		},
	}
	svc, _ := NewServiceWithTransaction(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, txRepoExecErr)
	if _, err := svc.DepositItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	runTx := func(tx *stubTx) (Depot, error) {
		repo := &stubTxRepo{
			executeFn: func(ctx context.Context, fn func(ctx context.Context, txArg Tx) error) error {
				return fn(ctx, tx)
			},
		}
		service, _ := NewServiceWithTransaction(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, repo)
		return service.DepositItem(ctx, "char-1", potion.ID)
	}

	// 2. tx.GetInventory error
	_, err := runTx(&stubTx{
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return coreinventory.Inventory{}, errTestBoom
		},
	})
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 3. tx.GetDepot generic error
	_, err = runTx(&stubTx{
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return inv, nil
		},
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	})
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 4. Depot full
	fullDepot, _ := NewDepot("char-1")
	fullDepot.Capacity = 1
	dummy, _ := item.NewInstance("dummy", 1)
	_ = fullDepot.AddItem(dummy)
	_, err = runTx(&stubTx{
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return inv, nil
		},
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return fullDepot, nil
		},
	})
	if !errors.Is(err, ErrDepotFull) {
		t.Fatalf("expected ErrDepotFull, got %v", err)
	}

	// 5. Item not in inventory
	emptyInv, _ := coreinventory.New("char-1")
	_, err = runTx(&stubTx{
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return emptyInv, nil
		},
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
	})
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}

	// 6. tx.SaveInventory error
	_, err = runTx(&stubTx{
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			testInv, _ := coreinventory.New("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = testInv.Add(p)
			return testInv, nil
		},
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		saveInvFn: func(ctx context.Context, inventory coreinventory.Inventory) error {
			return errTestBoom
		},
	})
	if err == nil || !errors.Is(err, errTestBoom) {
		t.Fatalf("expected wrapped errTestBoom, got %v", err)
	}

	// 7. tx.SaveDepot error
	_, err = runTx(&stubTx{
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			testInv, _ := coreinventory.New("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = testInv.Add(p)
			return testInv, nil
		},
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotVal, nil
		},
		saveDepotFn: func(ctx context.Context, depot Depot) error {
			return errTestBoom
		},
	})
	if err == nil || !errors.Is(err, errTestBoom) {
		t.Fatalf("expected wrapped errTestBoom, got %v", err)
	}

	// 8. tx.GetDepot returns ErrNotFound -> creates depot and succeeds
	res, err := runTx(&stubTx{
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			testInv, _ := coreinventory.New("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = testInv.Add(p)
			return testInv, nil
		},
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, ErrNotFound
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item in depot, got %d", len(res.Items))
	}
}

func TestTransactionPaths_WithdrawItem(t *testing.T) {
	ctx := context.Background()

	potion, _ := item.NewInstance("potion", 2)
	depotWithItem, _ := NewDepot("char-1")
	_ = depotWithItem.AddItem(potion)

	// 1. txRepo.Execute error
	txRepoExecErr := &stubTxRepo{
		executeFn: func(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
			return errTestBoom
		},
	}
	svc, _ := NewServiceWithTransaction(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, txRepoExecErr)
	if _, err := svc.WithdrawItem(ctx, "char-1", potion.ID); !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	runTx := func(tx *stubTx) (Depot, error) {
		repo := &stubTxRepo{
			executeFn: func(ctx context.Context, fn func(ctx context.Context, txArg Tx) error) error {
				return fn(ctx, tx)
			},
		}
		service, _ := NewServiceWithTransaction(&stubDepotRepo{}, &stubCharRepo{}, &stubInvRepo{}, repo)
		return service.WithdrawItem(ctx, "char-1", potion.ID)
	}

	// 2. tx.GetDepot error
	_, err := runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return Depot{}, errTestBoom
		},
	})
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 3. tx.GetInventory error
	_, err = runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return depotWithItem, nil
		},
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return coreinventory.Inventory{}, errTestBoom
		},
	})
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("expected errTestBoom, got %v", err)
	}

	// 4. Item not found in depot
	emptyDepot, _ := NewDepot("char-1")
	_, err = runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			return emptyDepot, nil
		},
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return coreinventory.New("char-1")
		},
	})
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}

	// 5. Inventory full (inv.Add error)
	invWithSameItem, _ := coreinventory.New("char-1")
	_ = invWithSameItem.Add(potion)
	_, err = runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			d, _ := NewDepot("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = d.AddItem(p)
			return d, nil
		},
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return invWithSameItem, nil
		},
	})
	if !errors.Is(err, ErrInventoryFull) {
		t.Fatalf("expected ErrInventoryFull, got %v", err)
	}

	// 6. tx.SaveDepot error
	_, err = runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			d, _ := NewDepot("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = d.AddItem(p)
			return d, nil
		},
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return coreinventory.New("char-1")
		},
		saveDepotFn: func(ctx context.Context, depot Depot) error {
			return errTestBoom
		},
	})
	if err == nil || !errors.Is(err, errTestBoom) {
		t.Fatalf("expected wrapped errTestBoom, got %v", err)
	}

	// 7. tx.SaveInventory error
	_, err = runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			d, _ := NewDepot("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = d.AddItem(p)
			return d, nil
		},
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return coreinventory.New("char-1")
		},
		saveInvFn: func(ctx context.Context, inventory coreinventory.Inventory) error {
			return errTestBoom
		},
	})
	if err == nil || !errors.Is(err, errTestBoom) {
		t.Fatalf("expected wrapped errTestBoom, got %v", err)
	}

	// 8. Happy path
	res, err := runTx(&stubTx{
		getDepotFn: func(ctx context.Context, characterID string) (Depot, error) {
			d, _ := NewDepot("char-1")
			p, _ := item.NewInstance("potion", 2)
			p.ID = potion.ID
			_ = d.AddItem(p)
			return d, nil
		},
		getInvFn: func(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
			return coreinventory.New("char-1")
		},
	})
	if err != nil {
		t.Fatalf("unexpected error on happy path: %v", err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected 0 items in depot, got %d", len(res.Items))
	}
}

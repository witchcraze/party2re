package economy_test

import (
	"context"
	"errors"
	"math"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/economy"
)

type mockCharRepo struct {
	chars map[string]corecharacter.Character
	err   error
}

func newMockCharRepo() *mockCharRepo {
	return &mockCharRepo{chars: make(map[string]corecharacter.Character)}
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	if m.err != nil {
		return corecharacter.Character{}, m.err
	}
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(ctx context.Context, c corecharacter.Character) error {
	if m.err != nil {
		return m.err
	}
	m.chars[c.ID] = c
	return nil
}

type mockInvRepo struct {
	invs map[string]coreinventory.Inventory
	err  error
}

func newMockInvRepo() *mockInvRepo {
	return &mockInvRepo{invs: make(map[string]coreinventory.Inventory)}
}

func (m *mockInvRepo) FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	if m.err != nil {
		return coreinventory.Inventory{}, m.err
	}
	inv, ok := m.invs[characterID]
	if !ok {
		return coreinventory.New(characterID)
	}
	return inv, nil
}

func (m *mockInvRepo) FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error) {
	return m.FindByCharacterID(ctx, characterID)
}

func (m *mockInvRepo) Save(ctx context.Context, inv coreinventory.Inventory) error {
	if m.err != nil {
		return m.err
	}
	m.invs[inv.CharacterID] = inv
	return nil
}

type mockTxProvider struct {
	executed bool
}

func (m *mockTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	m.executed = true
	return fn(ctx)
}

func TestSafeMultiply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b    int
		want    int
		wantErr bool
	}{
		{a: 10, b: 20, want: 200, wantErr: false},
		{a: 0, b: 100, want: 0, wantErr: false},
		{a: 100, b: 0, want: 0, wantErr: false},
		{a: -5, b: 10, want: 0, wantErr: false},
		{a: math.MaxInt, b: 2, want: 0, wantErr: true},
		{a: math.MaxInt/2 + 1, b: 2, want: 0, wantErr: true},
	}

	for _, tt := range tests {
		got, err := economy.SafeMultiply(tt.a, tt.b)
		if (err != nil) != tt.wantErr {
			t.Errorf("SafeMultiply(%d, %d) err = %v, wantErr = %v", tt.a, tt.b, err, tt.wantErr)
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("SafeMultiply(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestNewService_NilDependencies(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()

	if _, err := economy.NewService(nil, invRepo); !errors.Is(err, economy.ErrNilDependency) {
		t.Fatalf("expected ErrNilDependency for nil charRepo, got %v", err)
	}
	if _, err := economy.NewService(charRepo, nil); !errors.Is(err, economy.ErrNilDependency) {
		t.Fatalf("expected ErrNilDependency for nil invRepo, got %v", err)
	}

	svc, err := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(&mockTxProvider{}))
	if err != nil {
		t.Fatalf("unexpected error creating service: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestDeductGold(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	txProv := &mockTxProvider{}
	svc, _ := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))

	char := corecharacter.Character{ID: "char-1", Money: 500}
	_ = charRepo.Update(context.Background(), char)

	// Success
	res, err := svc.DeductGold(context.Background(), "char-1", 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Money != 300 {
		t.Errorf("expected 300 gold remaining, got %d", res.Money)
	}
	if !txProv.executed {
		t.Error("expected transaction provider to be executed")
	}

	// Zero amount
	res, err = svc.DeductGold(context.Background(), "char-1", 0)
	if err != nil {
		t.Fatalf("unexpected error on 0 amount: %v", err)
	}
	if res.Money != 300 {
		t.Errorf("expected 300 gold, got %d", res.Money)
	}

	// Insufficient gold
	_, err = svc.DeductGold(context.Background(), "char-1", 400)
	if !errors.Is(err, economy.ErrInsufficientGold) {
		t.Errorf("expected ErrInsufficientGold, got %v", err)
	}

	// Negative amount
	_, err = svc.DeductGold(context.Background(), "char-1", -10)
	if !errors.Is(err, economy.ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}

	// Missing character ID
	_, err = svc.DeductGold(context.Background(), "", 10)
	if !errors.Is(err, economy.ErrInvalidCharacterID) {
		t.Errorf("expected ErrInvalidCharacterID, got %v", err)
	}
}

func TestAddGold(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	svc, _ := economy.NewService(charRepo, invRepo)

	char := corecharacter.Character{ID: "char-1", Money: 100}
	_ = charRepo.Update(context.Background(), char)

	res, err := svc.AddGold(context.Background(), "char-1", 250)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Money != 350 {
		t.Errorf("expected 350 gold, got %d", res.Money)
	}

	// Cap at MaxMoney
	res, err = svc.AddGold(context.Background(), "char-1", corecharacter.MaxMoney)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Money != corecharacter.MaxMoney {
		t.Errorf("expected MaxMoney (%d), got %d", corecharacter.MaxMoney, res.Money)
	}

	// Negative amount
	_, err = svc.AddGold(context.Background(), "char-1", -5)
	if !errors.Is(err, economy.ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}
}

func TestTransferGold(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	txProv := &mockTxProvider{}
	svc, _ := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))

	charA := corecharacter.Character{ID: "char-a", Money: 1000}
	charB := corecharacter.Character{ID: "char-b", Money: 200}
	_ = charRepo.Update(context.Background(), charA)
	_ = charRepo.Update(context.Background(), charB)

	// Success transfer A -> B
	from, to, err := svc.TransferGold(context.Background(), "char-a", "char-b", 300)
	if err != nil {
		t.Fatalf("unexpected error on transfer: %v", err)
	}
	if from.Money != 700 || to.Money != 500 {
		t.Errorf("expected A:700, B:500, got A:%d, B:%d", from.Money, to.Money)
	}

	// Success transfer B -> A (inverted order test)
	from, to, err = svc.TransferGold(context.Background(), "char-b", "char-a", 100)
	if err != nil {
		t.Fatalf("unexpected error on transfer B -> A: %v", err)
	}
	if from.Money != 400 || to.Money != 800 {
		t.Errorf("expected B:400, A:800, got B:%d, A:%d", from.Money, to.Money)
	}

	// Self transfer
	_, _, err = svc.TransferGold(context.Background(), "char-a", "char-a", 50)
	if !errors.Is(err, economy.ErrSelfTransferNotAllowed) {
		t.Errorf("expected ErrSelfTransferNotAllowed, got %v", err)
	}

	// Insufficient funds
	_, _, err = svc.TransferGold(context.Background(), "char-b", "char-a", 9999)
	if !errors.Is(err, economy.ErrInsufficientGold) {
		t.Errorf("expected ErrInsufficientGold, got %v", err)
	}

	// Invalid amount
	_, _, err = svc.TransferGold(context.Background(), "char-a", "char-b", 0)
	if !errors.Is(err, economy.ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}
}

func TestSmallMedals(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	svc, _ := economy.NewService(charRepo, invRepo)

	char := corecharacter.Character{ID: "char-1", SmallMedals: 10}
	_ = charRepo.Update(context.Background(), char)

	// Deduct medals
	res, err := svc.DeductSmallMedals(context.Background(), "char-1", 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SmallMedals != 6 {
		t.Errorf("expected 6 medals, got %d", res.SmallMedals)
	}

	// Insufficient medals
	_, err = svc.DeductSmallMedals(context.Background(), "char-1", 10)
	if !errors.Is(err, economy.ErrInsufficientMedals) {
		t.Errorf("expected ErrInsufficientMedals, got %v", err)
	}

	// Add medals
	res, err = svc.AddSmallMedals(context.Background(), "char-1", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SmallMedals != 11 {
		t.Errorf("expected 11 medals, got %d", res.SmallMedals)
	}
}

func TestGrantAndConsumeItem(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	svc, _ := economy.NewService(charRepo, invRepo)

	// Grant item
	inv, inst, err := svc.GrantItem(context.Background(), "char-1", "item-001", 3)
	if err != nil {
		t.Fatalf("unexpected grant item error: %v", err)
	}
	if len(inv.Items) != 1 || inv.Items[0].Quantity != 3 {
		t.Fatalf("expected 1 item with quantity 3, got %v", inv.Items)
	}
	if inst.DefinitionID != "item-001" || inst.Quantity != 3 {
		t.Fatalf("expected instance item-001 qty 3, got %v", inst)
	}

	// Consume item instance
	inv, err = svc.ConsumeItemInstance(context.Background(), "char-1", inst.ID, 1)
	if err != nil {
		t.Fatalf("unexpected consume item instance error: %v", err)
	}
	if inv.Items[0].Quantity != 2 {
		t.Errorf("expected 2 items remaining in instance, got %d", inv.Items[0].Quantity)
	}

	// Consume item definition
	inv, err = svc.ConsumeItemDefinition(context.Background(), "char-1", "item-001", 2)
	if err != nil {
		t.Fatalf("unexpected consume item definition error: %v", err)
	}
	if len(inv.Items) != 0 {
		t.Errorf("expected inventory to be empty, got %d items", len(inv.Items))
	}

	// Consume item definition when not present
	_, err = svc.ConsumeItemDefinition(context.Background(), "char-1", "item-001", 1)
	if !errors.Is(err, economy.ErrInsufficientItemQuantity) {
		t.Errorf("expected ErrInsufficientItemQuantity, got %v", err)
	}
}

func TestExchange(t *testing.T) {
	t.Parallel()

	charRepo := newMockCharRepo()
	invRepo := newMockInvRepo()
	txProv := &mockTxProvider{}
	svc, _ := economy.NewService(charRepo, invRepo, economy.WithTransactionProvider(txProv))

	char := corecharacter.Character{ID: "char-1", Money: 1000, SmallMedals: 5}
	_ = charRepo.Update(context.Background(), char)

	// Initial item
	_, inst, _ := svc.GrantItem(context.Background(), "char-1", "material-01", 10)

	// Compound Exchange: Deduct 300 gold, 2 medals, consume 4 materials, grant 1 equipment
	req := economy.ExchangeRequest{
		CharacterID:        "char-1",
		DeductGold:         300,
		DeductMedals:       2,
		ConsumeInstanceID:  inst.ID,
		ConsumeInstanceQty: 4,
		GrantDefinitionID:  "sword-01",
		GrantQuantity:      1,
	}

	res, err := svc.Exchange(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected exchange error: %v", err)
	}

	if res.Character.Money != 700 {
		t.Errorf("expected 700 gold, got %d", res.Character.Money)
	}
	if res.Character.SmallMedals != 3 {
		t.Errorf("expected 3 small medals, got %d", res.Character.SmallMedals)
	}
	if res.GrantedItem == nil || res.GrantedItem.DefinitionID != "sword-01" {
		t.Errorf("expected granted sword-01, got %v", res.GrantedItem)
	}
	if res.Inventory.Quantity("material-01") != 6 {
		t.Errorf("expected 6 material-01 remaining, got %d", res.Inventory.Quantity("material-01"))
	}
}

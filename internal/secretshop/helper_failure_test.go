package secretshop_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/secretshop"
)

type failedHelperFilter struct {
	err error
	ctx context.Context
}

func (f *failedHelperFilter) GetActiveHelperItemIDs(ctx context.Context) ([]string, error) {
	f.ctx = ctx
	return nil, f.err
}

type helperTransactionKey struct{}
type helperFailureTransaction struct{}

func (helperFailureTransaction) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, helperTransactionKey{}, true))
}

type unreadInventory struct{ secretshop.InventoryRepository }
type unreadDepot struct{ secretshop.DepotRepository }

func TestHelperReadFailurePreservesRequestAndTransaction(t *testing.T) {
	filter := &failedHelperFilter{err: errors.New("helper read failed")}
	chars := newMockCharacterRepo()
	chars.chars["hero"] = createTestCharacter("hero", "Hero", 1, 7, 50000)
	catalog, err := secretshop.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	// Any inventory/depot access panics: the failed filter must run before asset writes.
	svc, err := secretshop.NewService(chars, &unreadInventory{}, catalog,
		secretshop.WithHelperFilter(filter), secretshop.WithDepotRepository(&unreadDepot{}),
		secretshop.WithTransactionProvider(helperFailureTransaction{}))
	if err != nil {
		t.Fatal(err)
	}
	type requestKey struct{}
	ctx := context.WithValue(context.Background(), requestKey{}, "request")
	status, err := svc.GetShopStatus(ctx, "hero")
	if status != nil || !errors.Is(err, filter.err) || filter.ctx != ctx {
		t.Fatalf("status=%+v err=%v ctx=%v", status, err, filter.ctx)
	}
	result, err := svc.PurchaseItem(ctx, "hero", "secret_item_herbal_root", 1)
	if result != nil || !errors.Is(err, filter.err) {
		t.Fatalf("purchase=%+v err=%v", result, err)
	}
	if filter.ctx.Value(requestKey{}) != "request" || filter.ctx.Value(helperTransactionKey{}) != true {
		t.Fatal("filter lost request/transaction context")
	}
	if chars.chars["hero"].Money != 50000 {
		t.Fatal("failed filter changed wallet")
	}
}

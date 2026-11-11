package depot

import (
	"context"
	"errors"
	"reflect"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
)

func heldAmounts(values []Remittance) []int {
	out := []int{}
	for _, v := range values {
		out = append(out, v.Amount)
	}
	return out
}

// hikidasu collects held gold before items (depot.cgi:211–215). Gold that does
// not fit under the wallet cap stays held instead of being clamped away.
func TestWithdrawCollectsHeldGoldFirstUpToTheWalletCap(t *testing.T) {
	ctx := context.Background()
	herb := item.Instance{ID: "herb", DefinitionID: "item-001", Quantity: 1}
	f := newSwapFixture(t, nil, nil, []item.Instance{herb})
	f.char.Money = corecharacter.MaxMoney - 250
	f.chars.characters[f.char.ID] = f.char
	for _, r := range []Remittance{{ID: "r1", Amount: 100}, {ID: "r2", Amount: 300}} {
		r.CharacterID, r.Source, r.SenderName = f.char.ID, RemittanceDepotTransfer, "Ann"
		if err := f.depots.AddRemittance(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	state := func() (int, []int, int) {
		char, _ := f.chars.FindByID(ctx, f.char.ID)
		held, _ := f.service.PendingRemittances(ctx, f.char.ID)
		dep, _ := f.depots.FindByCharacterID(ctx, f.char.ID)
		return char.Money, heldAmounts(held), len(dep.Items)
	}

	_, got, err := f.service.WithdrawItem(ctx, f.char.ID, herb.ID)
	if err != nil || !reflect.DeepEqual(heldAmounts(got), []int{100, 150}) || got[1].SenderName != "Ann" {
		t.Fatalf("collect: %+v %v", got, err)
	}
	if money, held, rows := state(); money != corecharacter.MaxMoney || !reflect.DeepEqual(held, []int{150}) || rows != 1 {
		t.Fatalf("after collect: money=%d held=%v rows=%d", money, held, rows)
	}

	// A full wallet collects nothing, so the item withdrawal proceeds.
	if _, _, err := f.service.WithdrawItem(ctx, f.char.ID, ""); !errors.Is(err, ErrInvalidItemInstanceID) {
		t.Fatalf("empty target at cap: %v", err)
	}
	if _, got, err := f.service.WithdrawItem(ctx, f.char.ID, herb.ID); err != nil || len(got) != 0 {
		t.Fatalf("item at cap: %+v %v", got, err)
	}
	if money, held, rows := state(); money != corecharacter.MaxMoney || !reflect.DeepEqual(held, []int{150}) || rows != 0 {
		t.Fatalf("after item: money=%d held=%v rows=%d", money, held, rows)
	}

	// Once there is room, the remainder is collected exactly once.
	char, _ := f.chars.FindByID(ctx, f.char.ID)
	char.Money = 0
	f.chars.characters[f.char.ID] = char
	if _, got, err := f.service.WithdrawItem(ctx, f.char.ID, ""); err != nil || !reflect.DeepEqual(heldAmounts(got), []int{150}) {
		t.Fatalf("remainder: %+v %v", got, err)
	}
	if _, _, err := f.service.WithdrawItem(ctx, f.char.ID, ""); !errors.Is(err, ErrInvalidItemInstanceID) {
		t.Fatalf("retry after collection: %v", err)
	}
	if money, held, _ := state(); money != 150 || len(held) != 0 {
		t.Fatalf("final: money=%d held=%v", money, held)
	}
}

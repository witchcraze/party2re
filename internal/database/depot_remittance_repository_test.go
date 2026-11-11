package database

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/depot"
)

// failingRemittances fails the Nth remittance write to prove rollback.
type failingRemittances struct {
	*DepotRepository
	failAt, calls int
}

func (r *failingRemittances) AddRemittance(ctx context.Context, value depot.Remittance) error {
	if r.calls++; r.calls == r.failAt {
		return errors.New("injected add failure")
	}
	return r.DepotRepository.AddRemittance(ctx, value)
}

func (r *failingRemittances) SetRemittanceAmount(ctx context.Context, characterID, remittanceID string, amount int) error {
	if r.calls++; r.calls == r.failAt {
		return errors.New("injected settle failure")
	}
	return r.DepotRepository.SetRemittanceAmount(ctx, characterID, remittanceID, amount)
}

func TestDepotRemittancesHoldCollectAndRollBack_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	charRepo, _ := NewCharacterRepository(db)
	invRepo, _ := NewInventoryRepository(db)
	depotRepo, _ := NewDepotRepository(db)
	sender, err := CreateTestCharacterWithFunds(ctx, db, "Remit Sender", 1000)
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := CreateTestCharacterWithFunds(ctx, db, "Remit Recipient", corecharacter.MaxMoney-320)
	if err != nil {
		t.Fatal(err)
	}
	service := func(remittances depot.RemittanceRepository) *depot.Service {
		s, err := depot.NewService(depotRepo, charRepo, invRepo, depot.WithTransactionProvider(NewTransactionProvider(db)), depot.WithRemittanceRepository(remittances))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	svc := service(depotRepo)
	assertState := func(step string, senderMoney, recipientMoney int, held []int) {
		t.Helper()
		s, _ := charRepo.FindByID(ctx, sender.ID)
		r, _ := charRepo.FindByID(ctx, recipient.ID)
		values, err := depotRepo.ListRemittances(ctx, recipient.ID)
		got := []int{}
		for _, v := range values {
			got = append(got, v.Amount)
		}
		if err != nil || s.Money != senderMoney || r.Money != recipientMoney || !reflect.DeepEqual(got, held) {
			t.Fatalf("%s: sender=%d recipient=%d held=%v err=%v", step, s.Money, r.Money, got, err)
		}
	}
	walletStart := corecharacter.MaxMoney - 320

	if _, err := service(&failingRemittances{DepotRepository: depotRepo, failAt: 1}).SendMoney(ctx, sender.ID, recipient.ID, 300); err == nil {
		t.Fatal("injected send failure succeeded")
	}
	assertState("failed send", 1000, walletStart, []int{})

	for _, amount := range []int{300, 50} {
		if _, err := svc.SendMoney(ctx, sender.ID, recipient.ID, amount); err != nil {
			t.Fatal(err)
		}
	}
	held, _ := depotRepo.ListRemittances(ctx, recipient.ID)
	if held[0].Source != depot.RemittanceDepotTransfer || held[0].SenderName != sender.Name {
		t.Fatalf("held facts: %+v", held[0])
	}
	assertState("sent", 650, walletStart, []int{300, 50})

	if _, _, err := service(&failingRemittances{DepotRepository: depotRepo, failAt: 2}).WithdrawItem(ctx, recipient.ID, ""); err == nil {
		t.Fatal("injected collection failure succeeded")
	}
	assertState("failed collection", 650, walletStart, []int{300, 50})

	if err := depotRepo.SetRemittanceAmount(ctx, sender.ID, held[1].ID, 0); err == nil {
		t.Fatal("foreign remittance settled")
	}

	_, got, err := svc.WithdrawItem(ctx, recipient.ID, "")
	if err != nil || len(got) != 2 || got[0].Amount != 300 || got[1].Amount != 20 {
		t.Fatalf("collect: %+v %v", got, err)
	}
	assertState("collected to cap", 650, corecharacter.MaxMoney, []int{30})
	if _, _, err := svc.WithdrawItem(ctx, recipient.ID, ""); !errors.Is(err, depot.ErrInvalidItemInstanceID) {
		t.Fatalf("full wallet retry: %v", err)
	}
	assertState("full wallet retry", 650, corecharacter.MaxMoney, []int{30})
}

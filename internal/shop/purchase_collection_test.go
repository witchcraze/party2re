package shop_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
)

type errCollectionRecorder struct {
	err error
}

func (m *errCollectionRecorder) RecordItemDiscovered(_ context.Context, _, _, _, _ string) error {
	return m.err
}

func TestPurchase_PropagatesCollectionError(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _, _, _, _ := setupParityTest(t)

	colErr := errors.New("shop collection failure")
	recorder := &errCollectionRecorder{err: colErr}
	svc.SetCollectionRecorder(recorder)

	char := character.Character{
		ID:    "char-buyer",
		Name:  "Buyer",
		Money: 5000,
	}
	if err := charRepo.Update(ctx, char); err != nil {
		t.Fatal(err)
	}

	_, err := svc.Purchase(ctx, char.ID, "weapon-01", 1)
	if !errors.Is(err, colErr) {
		t.Fatalf("expected collection error %v, got: %v", colErr, err)
	}
}

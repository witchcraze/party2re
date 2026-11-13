package home_test

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
)

func (f bagFixture) godVisit(t *testing.T) string {
	t.Helper()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	repo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	c, err := repo.FindByID(context.Background(), f.charID)
	if err != nil {
		t.Fatal(err)
	}
	return c.GodVisit
}

func TestHomeGodReins_Integration_EntersRealmFromEitherSource(t *testing.T) {
	for _, tc := range []struct{ def, realm string }{
		{"item-059", corecharacter.GodVisitHeaven},
		{"item-262", corecharacter.GodVisitUnderworld},
	} {
		for _, source := range []string{"inventory", "depot"} {
			t.Run(tc.def+"/"+source, func(t *testing.T) {
				// A Depot over capacity must not block entry.
				f := newBagFixture(t, "Reins", tc.def, source, 1)
				svc := f.newSvc(nil, 0)
				res, err := svc.UseHomeItem(context.Background(), f.charID, f.bagID, source)
				if err != nil {
					t.Fatal(err)
				}
				if _, left := f.state(t); !res.Consumed || left != 0 || f.godVisit(t) != tc.realm {
					t.Fatalf("consumed=%v left=%d visit=%q, want consumed, 0, %q", res.Consumed, left, f.godVisit(t), tc.realm)
				}
			})
		}
	}
}

func TestHomeGodReins_Integration_AlreadyAdmittedKeepsReins(t *testing.T) {
	f := newBagFixture(t, "ReinsTwice", "item-262", "inventory", 0)
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	repo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c, err := repo.FindByID(ctx, f.charID)
	if err != nil {
		t.Fatal(err)
	}
	c.GodVisit = corecharacter.GodVisitHeaven
	if err := repo.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	res, err := f.newSvc(nil, 0).UseHomeItem(ctx, f.charID, f.bagID, "inventory")
	if err != nil {
		t.Fatal(err)
	}
	if _, left := f.state(t); res.Consumed || left != 1 || f.godVisit(t) != corecharacter.GodVisitHeaven {
		t.Fatalf("consumed=%v left=%d visit=%q, want kept reins and unchanged heaven visit", res.Consumed, left, f.godVisit(t))
	}
}

// failConsume rejects the Depot consumption so the entry must roll back.
type failConsume struct{ *testDepotManagerAdapter }

func (failConsume) ConsumeOne(context.Context, string, string) error {
	return errors.New("injected consume failure")
}

func TestHomeGodReins_Integration_SaveFailureKeepsReinsAndVisit(t *testing.T) {
	f := newBagFixture(t, "ReinsFail", "item-059", "depot", 0)
	if _, err := f.newSvc(failConsume{&testDepotManagerAdapter{repo: f.depotRepo}}, 0).
		UseHomeItem(context.Background(), f.charID, f.bagID, "depot"); err == nil {
		t.Fatal("expected injected failure")
	}
	if _, left := f.state(t); left != 1 || f.godVisit(t) != "" {
		t.Fatalf("left=%d visit=%q, want the reins kept and no visit", left, f.godVisit(t))
	}
}

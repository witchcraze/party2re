package home_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
)

func (f bagFixture) setOrb(t *testing.T, orb string) {
	t.Helper()
	repo := f.charRepo(t)
	c, err := repo.FindByID(context.Background(), f.charID)
	if err != nil {
		t.Fatal(err)
	}
	c.Orb = orb
	if err := repo.Update(context.Background(), c); err != nil {
		t.Fatal(err)
	}
}

func (f bagFixture) orb(t *testing.T) string {
	t.Helper()
	c, err := f.charRepo(t).FindByID(context.Background(), f.charID)
	if err != nil {
		t.Fatal(err)
	}
	return c.Orb
}

func (f bagFixture) charRepo(t *testing.T) *database.CharacterRepository {
	t.Helper()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestHomeOrb_Integration_OffersEachOrbFromEitherSource(t *testing.T) {
	for i, rune := range "srbgyp" {
		def := fmt.Sprintf("item-%03d", 60+i)
		for _, source := range []string{"inventory", "depot"} {
			t.Run(def+"/"+source, func(t *testing.T) {
				// A Depot over capacity must not block the offering.
				f := newBagFixture(t, "Orb", def, source, 1)
				res, err := f.newSvc(nil, 0).UseHomeItem(context.Background(), f.charID, f.bagID, source)
				if err != nil {
					t.Fatal(err)
				}
				if _, left := f.state(t); !res.Consumed || left != 0 || f.orb(t) != string(rune) {
					t.Fatalf("consumed=%v left=%d orb=%q, want consumed, 0, %q", res.Consumed, left, f.orb(t), string(rune))
				}
				if res.Message != res.ItemName+"を復活の祭壇にささげた！" {
					t.Fatalf("message = %q", res.Message)
				}
			})
		}
	}
}

func TestHomeOrb_Integration_DuplicateKeepsItem(t *testing.T) {
	f := newBagFixture(t, "OrbDup", "item-061", "depot", 0)
	f.setOrb(t, "sr")
	res, err := f.newSvc(nil, 0).UseHomeItem(context.Background(), f.charID, f.bagID, "depot")
	if err != nil {
		t.Fatal(err)
	}
	if _, left := f.state(t); res.Consumed || left != 1 || f.orb(t) != "sr" || res.Message != "すでにささげられています" {
		t.Fatalf("consumed=%v left=%d orb=%q msg=%q, want kept item and unchanged orbs", res.Consumed, left, f.orb(t), res.Message)
	}
}

// Legacy appends to any string without the rune, including Ramia's G state.
func TestHomeOrb_Integration_AddsAfterAwakening(t *testing.T) {
	f := newBagFixture(t, "OrbG", "item-065", "inventory", 0)
	f.setOrb(t, "G")
	if _, err := f.newSvc(nil, 0).UseHomeItem(context.Background(), f.charID, f.bagID, "inventory"); err != nil {
		t.Fatal(err)
	}
	if _, left := f.state(t); left != 0 || f.orb(t) != "Gp" {
		t.Fatalf("left=%d orb=%q, want consumed and Gp", left, f.orb(t))
	}
}

func TestHomeOrb_Integration_SaveFailureKeepsItemAndOrbs(t *testing.T) {
	f := newBagFixture(t, "OrbFail", "item-060", "depot", 0)
	if _, err := f.newSvc(failConsume{&testDepotManagerAdapter{repo: f.depotRepo}}, 0).
		UseHomeItem(context.Background(), f.charID, f.bagID, "depot"); err == nil {
		t.Fatal("expected injected failure")
	}
	if _, left := f.state(t); left != 1 || f.orb(t) != "" {
		t.Fatalf("left=%d orb=%q, want the orb kept and no offering", left, f.orb(t))
	}
}

func TestHomeOrb_Integration_ConcurrentUseOffersOnce(t *testing.T) {
	for _, source := range []string{"inventory", "depot"} {
		t.Run(source, func(t *testing.T) {
			f := newBagFixture(t, "OrbRace", "item-062", source, 0)
			svc := f.newSvc(nil, 0)
			var wg sync.WaitGroup
			consumed := make([]bool, 2)
			for i := range consumed {
				wg.Add(1)
				go func() {
					defer wg.Done()
					res, err := svc.UseHomeItem(context.Background(), f.charID, f.bagID, source)
					consumed[i] = err == nil && res.Consumed
				}()
			}
			wg.Wait()
			if _, left := f.state(t); left != 0 || f.orb(t) != "b" || consumed[0] == consumed[1] {
				t.Fatalf("left=%d orb=%q consumed=%v, want exactly one offering", left, f.orb(t), consumed)
			}
		})
	}
}

package home_test

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
)

// setStats stores stats on the fixture character and returns the saved value.
func (f bagFixture) setStats(t *testing.T, s corecharacter.Stats) corecharacter.Stats {
	t.Helper()
	repo, err := database.NewCharacterRepository(database.OpenTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c, err := repo.FindByID(ctx, f.charID)
	if err != nil {
		t.Fatal(err)
	}
	c.Stats = s
	if err := repo.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	if c, err = repo.FindByID(ctx, f.charID); err != nil {
		t.Fatal(err)
	}
	return c.Stats
}

func (f bagFixture) stats(t *testing.T) corecharacter.Stats {
	t.Helper()
	repo, err := database.NewCharacterRepository(database.OpenTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	c, err := repo.FindByID(context.Background(), f.charID)
	if err != nil {
		t.Fatal(err)
	}
	return c.Stats
}

// Legacy Home calls the 113/114 callbacks without stats and discards the
// returned hash: use consumes one and changes no stat, even with a full Depot.
func TestHomeStatReturnItems_Integration_ConsumeWithoutStatGain(t *testing.T) {
	statCases := map[string]corecharacter.Stats{
		"normal":   {HP: 300, MaxHP: 300, MP: 200, MaxMP: 200, Attack: 50, Defense: 40, Agility: 30},
		"near_max": {HP: 950, MaxHP: 950, MP: 999, MaxMP: 999, Attack: 50, Defense: 40, Agility: 30},
	}
	for _, def := range []string{"item-113", "item-114"} {
		for _, source := range []string{"inventory", "depot"} {
			for name, s := range statCases {
				t.Run(def+"/"+source+"/"+name, func(t *testing.T) {
					f := newBagFixture(t, "StatReturn", def, source, 0)
					want := f.setStats(t, s)
					res, err := f.newSvc(nil, 0).UseHomeItem(context.Background(), f.charID, f.bagID, source)
					if err != nil {
						t.Fatal(err)
					}
					if _, left := f.state(t); res.Action != "consumed" || !res.Consumed || left != 0 {
						t.Fatalf("action=%q consumed=%v left=%d, want consumed with none left", res.Action, res.Consumed, left)
					}
					if got := f.stats(t); got != want {
						t.Fatalf("stats changed: got %+v, want %+v", got, want)
					}
				})
			}
		}
	}
}

func TestHomeStatReturnItems_Integration_SaveFailureKeepsItem(t *testing.T) {
	f := newBagFixture(t, "StatReturnFail", "item-114", "depot", 0)
	if _, err := f.newSvc(failConsume{&testDepotManagerAdapter{repo: f.depotRepo}}, 0).
		UseHomeItem(context.Background(), f.charID, f.bagID, "depot"); err == nil {
		t.Fatal("expected injected failure")
	}
	if _, left := f.state(t); left != 1 {
		t.Fatalf("left=%d, want the ring kept", left)
	}
}

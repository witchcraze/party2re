package fleamarket_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/fleamarket"
)

type failingListingRepo struct {
	*mockFleaMarketRepo
	err error
}

func (r *failingListingRepo) GetListingByIDForUpdate(ctx context.Context, id string) (fleamarket.Listing, error) {
	if r.err != nil {
		return fleamarket.Listing{}, r.err
	}
	return r.mockFleaMarketRepo.GetListingByIDForUpdate(ctx, id)
}

type failingCharRepo struct {
	*mockCharRepo
	failID string
	err    error
}

func (r *failingCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	if id == r.failID {
		return corecharacter.Character{}, r.err
	}
	return r.mockCharRepo.FindByIDForUpdate(ctx, id)
}

type marketFixture struct {
	listings *failingListingRepo
	chars    *failingCharRepo
	depots   *mockDepotRepo
	svc      *fleamarket.Service
}

func newMarketFixture(t *testing.T) *marketFixture {
	t.Helper()
	f := &marketFixture{listings: &failingListingRepo{mockFleaMarketRepo: newMockFleaMarketRepo()}, chars: &failingCharRepo{mockCharRepo: newMockCharRepo()}, depots: newMockDepotRepo()}
	var err error
	f.svc, err = fleamarket.NewService(f.listings, f.chars, f.depots, fleamarket.WithItemDefinitionProvider(&mockItemDefs{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"seller", "buyer"} {
		f.chars.characters[id] = corecharacter.Character{ID: id, Name: id, Money: 1000}
		dep, err := depot.NewDepot(id)
		if err != nil {
			t.Fatal(err)
		}
		dep.Capacity = 10
		if err := f.depots.Save(context.Background(), dep); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *marketFixture) store(t *testing.T, enhancement int) string {
	t.Helper()
	ctx := context.Background()
	inst, err := coreitem.NewInstanceWithEnhancement("wea-sword", 1, enhancement)
	if err != nil {
		t.Fatal(err)
	}
	dep, err := f.depots.FindByCharacterID(ctx, "seller")
	if err != nil {
		t.Fatal(err)
	}
	if err := dep.AddItem(inst); err != nil {
		t.Fatal(err)
	}
	if err := f.depots.Save(ctx, dep); err != nil {
		t.Fatal(err)
	}
	return inst.ID
}

func TestListingRoundTripsPreserveEnhancement(t *testing.T) {
	ctx, now := context.Background(), time.Now().UTC()
	for _, level := range []int{0, 5} {
		for _, route := range []struct {
			name, owner string
			finish      func(*fleamarket.Service, string) error
		}{
			{"cancel", "seller", func(s *fleamarket.Service, id string) error { _, err := s.CancelListing(ctx, "seller", id); return err }},
			{"purchase", "buyer", func(s *fleamarket.Service, id string) error {
				_, err := s.PurchaseListing(ctx, "buyer", id, now)
				return err
			}},
		} {
			t.Run(fmt.Sprintf("%s/%d", route.name, level), func(t *testing.T) {
				f := newMarketFixture(t)
				listing, err := f.svc.CreateListing(ctx, "seller", f.store(t, level), 100, now)
				if err != nil || listing.EnhancementLevel != level {
					t.Fatalf("listing lost enhancement: %+v %v", listing, err)
				}
				if err := route.finish(f.svc, listing.ID); err != nil {
					t.Fatal(err)
				}
				dep, err := f.depots.FindByCharacterID(ctx, route.owner)
				if err != nil {
					t.Fatal(err)
				}
				if len(dep.Items) != 1 || dep.Items[0].EnhancementLevel != level || dep.Items[0].Quantity != 1 {
					t.Fatalf("delivered instance: %+v", dep.Items)
				}
			})
		}
	}
}

func TestMutationsDistinguishMissingRowsFromStorageFailures(t *testing.T) {
	ctx, now := context.Background(), time.Now().UTC()
	boom := errors.New("lock wait timeout")
	setup := func(t *testing.T) (*marketFixture, string) {
		f := newMarketFixture(t)
		listing, err := f.svc.CreateListing(ctx, "seller", f.store(t, 0), 100, now)
		if err != nil {
			t.Fatal(err)
		}
		return f, listing.ID
	}
	purchase := func(f *marketFixture, id string) error {
		_, err := f.svc.PurchaseListing(ctx, "buyer", id, now)
		return err
	}
	cancel := func(f *marketFixture, id string) error {
		_, err := f.svc.CancelListing(ctx, "seller", id)
		return err
	}
	create := func(f *marketFixture, _ string) error {
		_, err := f.svc.CreateListing(ctx, "seller", "wea-sword", 100, now)
		return err
	}
	for name, run := range map[string]func(*marketFixture, string) error{"purchase": purchase, "cancel": cancel} {
		t.Run(name+"/listing storage failure", func(t *testing.T) {
			f, id := setup(t)
			f.listings.err = boom
			if err := run(f, id); !errors.Is(err, boom) || errors.Is(err, fleamarket.ErrListingNotFound) {
				t.Fatalf("masked: %v", err)
			}
		})
		t.Run(name+"/missing listing", func(t *testing.T) {
			f, _ := setup(t)
			if err := run(f, "missing"); !errors.Is(err, fleamarket.ErrListingNotFound) {
				t.Fatalf("got %v", err)
			}
		})
	}
	for name, tc := range map[string]struct {
		run    func(*marketFixture, string) error
		failID string
	}{
		"create/seller":   {create, "seller"},
		"purchase/buyer":  {purchase, "buyer"},
		"purchase/seller": {purchase, "seller"},
		"cancel/seller":   {cancel, "seller"},
	} {
		t.Run(name+"/character storage failure", func(t *testing.T) {
			f, id := setup(t)
			f.chars.failID, f.chars.err = tc.failID, boom
			if err := tc.run(f, id); !errors.Is(err, boom) || errors.Is(err, fleamarket.ErrCharacterNotFound) {
				t.Fatalf("masked: %v", err)
			}
		})
		t.Run(name+"/missing character", func(t *testing.T) {
			f, id := setup(t)
			f.chars.failID, f.chars.err = tc.failID, corecharacter.ErrNotFound
			if err := tc.run(f, id); !errors.Is(err, fleamarket.ErrCharacterNotFound) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestActiveCharacterListingsExcludeFinishedListings(t *testing.T) {
	ctx, now := context.Background(), time.Now().UTC()
	f := newMarketFixture(t)
	var ids []string
	for range 3 {
		listing, err := f.svc.CreateListing(ctx, "seller", f.store(t, 1), 100, now)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, listing.ID)
	}
	if _, err := f.svc.PurchaseListing(ctx, "buyer", ids[0], now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CancelListing(ctx, "seller", ids[1]); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.GetActiveCharacterListings(ctx, "seller")
	if err != nil || len(got) != 1 || got[0].ID != ids[2] {
		t.Fatalf("active listings: %+v %v", got, err)
	}
}

package playercontext

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
)

type navigationStore struct {
	selection        Selection
	reads, writes    int
	loadErr, saveErr error
}

func (r *navigationStore) Load(ctx context.Context, id string) (Selection, error) {
	r.reads++
	if err := ctx.Err(); err != nil {
		return Selection{}, err
	}
	return r.selection, r.loadErr
}

func (r *navigationStore) Save(ctx context.Context, id string, s Selection) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.saveErr != nil {
		return r.saveErr
	}
	r.selection = s
	r.writes++
	return nil
}

func navigationFixture(t *testing.T) (*Service, *navigationStore, *queryReaders, *bool) {
	t.Helper()
	r := &queryReaders{character: character.Character{ID: "hero", PlayerID: "owner"}, ctx: context.Background()}
	store := &navigationStore{}
	available := true
	s := NewService(r, r, r, WithNavigation(store,
		SceneDefinition{ID: "town", Pageable: true},
		SceneDefinition{ID: "bank", Parent: "town"},
		SceneDefinition{ID: "shop_weapon", Parent: "town", SubjectKind: "item", Pageable: true,
			SubjectAvailable: func(ctx context.Context, actorID, targetID string) (bool, error) {
				if actorID != "hero" {
					t.Fatal("target replaced actor")
				}
				return available && (targetID == "weapon-01" || targetID == "weapon-02"), ctx.Err()
			}},
	))
	return s, store, r, &available
}

func TestNavigationTransitionsAndObservation(t *testing.T) {
	s, store, r, available := navigationFixture(t)
	ctx := r.ctx
	observe := func(want Selection, unavailable bool) {
		t.Helper()
		before := store.writes
		got, err := s.Query(ctx, "hero", "owner")
		if err != nil || got.Navigation == nil || got.Navigation.Selection != want || got.Navigation.Unavailable != unavailable {
			t.Fatalf("observation=%+v err=%v want=%+v unavailable=%v", got.Navigation, err, want, unavailable)
		}
		if store.writes != before || got.Snapshot.LocationID != "town" {
			t.Fatal("observation mutated selection or changed pre-composer scene")
		}
	}
	observe(Selection{Destination: "town"}, false)
	if store.selection != (Selection{}) {
		t.Fatal("GET created selection")
	}
	if _, err := s.Page(ctx, "owner", "hero", PageParams{Destination: "town", Offset: 20}); err != nil {
		t.Fatal(err)
	}
	observe(Selection{Destination: "town", Offset: 20, Limit: 20}, false)
	if _, err := s.Enter(ctx, "owner", "hero", "shop_weapon"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Page(ctx, "owner", "hero", PageParams{Destination: "shop_weapon", Offset: 20, Limit: 20}); err != nil {
		t.Fatal(err)
	}
	observe(Selection{Destination: "shop_weapon", Offset: 20, Limit: 20}, false)
	if _, err := s.Select(ctx, "owner", "hero", Subject{Kind: "item", ID: "weapon-01"}); err != nil {
		t.Fatal(err)
	}
	selected := Selection{Destination: "shop_weapon", Subject: Subject{Kind: "item", ID: "weapon-01"}}
	observe(selected, false)
	if _, err := s.Page(ctx, "owner", "hero", PageParams{Destination: "shop_weapon", Limit: 20}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("detail accepted list paging: %v", err)
	}
	if _, err := s.Select(ctx, "owner", "hero", Subject{Kind: "item", ID: "weapon-02"}); err != nil {
		t.Fatal(err)
	}
	observe(Selection{Destination: "shop_weapon", Subject: Subject{Kind: "item", ID: "weapon-02"}}, false)
	if _, err := s.Enter(ctx, "owner", "hero", "bank"); err != nil {
		t.Fatal(err)
	}
	observe(Selection{Destination: "bank"}, false)
	if _, err := s.Enter(ctx, "owner", "hero", "shop_weapon"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Select(ctx, "owner", "hero", selected.Subject); err != nil {
		t.Fatal(err)
	}
	*available = false
	observe(selected, true)
	if store.selection != selected {
		t.Fatal("unavailable target silently replaced")
	}
	if _, err := s.Back(ctx, "owner", "hero"); err != nil {
		t.Fatal(err)
	}
	observe(Selection{Destination: "shop_weapon"}, false)
	if _, err := s.Back(ctx, "owner", "hero"); err != nil {
		t.Fatal(err)
	}
	observe(Selection{Destination: "town"}, false)
	// Navigation does not recover or remove feature-owned state on selection loss.
	r.asleep = true
	store.selection = Selection{}
	got, err := s.Query(ctx, "hero", "owner")
	if err != nil || !got.Snapshot.Sleeping || !got.Snapshot.CanWake {
		t.Fatalf("lost sleep: %+v %v", got, err)
	}
}

func TestNavigationRejectsInvalidTargetsAndPreservesFailures(t *testing.T) {
	s, store, r, _ := navigationFixture(t)
	ctx := r.ctx
	for _, destination := range []string{"", "arbitrary", "../bank"} {
		if _, err := s.Enter(ctx, "owner", "hero", destination); !errors.Is(err, ErrInvalidSelection) {
			t.Fatalf("destination %q: %v", destination, err)
		}
	}
	reads := store.reads
	if _, err := s.Enter(ctx, "other", "hero", "bank"); !errors.Is(err, ErrNavigationForbidden) || store.writes != 0 || store.reads != reads {
		t.Fatalf("ownership: %v", err)
	}
	if _, err := s.Enter(ctx, "owner", "hero", "shop_weapon"); err != nil {
		t.Fatal(err)
	}
	before := store.selection
	for _, subject := range []Subject{{}, {Kind: "home", ID: "weapon-01"}, {Kind: "item", ID: "../bad"}, {Kind: "item", ID: "invented"}} {
		if _, err := s.Select(ctx, "owner", "hero", subject); err == nil {
			t.Fatalf("accepted %+v", subject)
		}
	}
	for _, p := range []PageParams{{Destination: "bank", Limit: 20}, {Destination: "shop_weapon", Offset: -1, Limit: 20}, {Destination: "shop_weapon", Limit: 101}, {Destination: "shop_weapon", Limit: -1}} {
		if _, err := s.Page(ctx, "owner", "hero", p); !errors.Is(err, ErrInvalidSelection) {
			t.Fatalf("page %+v: %v", p, err)
		}
	}
	if store.selection != before {
		t.Fatal("invalid request wrote selection")
	}
	wantErr := errors.New("storage unavailable")
	store.saveErr = wantErr
	if _, err := s.Enter(ctx, "owner", "hero", "bank"); !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
	store.loadErr = wantErr
	if got, err := s.Query(ctx, "hero", "owner"); !errors.Is(err, wantErr) || !reflect.DeepEqual(got, Result{}) {
		t.Fatalf("partial result: %+v %v", got, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	r.ctx = canceled
	if _, err := s.Enter(canceled, "owner", "hero", "bank"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

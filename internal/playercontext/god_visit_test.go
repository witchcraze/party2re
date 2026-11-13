package playercontext

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
)

func godVisitFixture(t *testing.T, visit string, forfeitErr error) (*Service, *navigationStore, *queryReaders, *[]string) {
	t.Helper()
	r := &queryReaders{character: character.Character{ID: "hero", PlayerID: "owner", GodVisit: visit, Stats: character.Stats{HP: 10}}, ctx: context.Background()}
	store := &navigationStore{selection: Selection{Destination: "home"}}
	var forfeits []string
	s := NewService(r, r, r, WithNavigation(store,
		SceneDefinition{ID: "town", Pageable: true}, SceneDefinition{ID: "home", Parent: "town"},
		SceneDefinition{ID: "god", Parent: "town"}, SceneDefinition{ID: "u_god", Parent: "town"}),
		WithGodVisitForfeit(func(_ context.Context, id string) error {
			forfeits = append(forfeits, id)
			if forfeitErr == nil {
				r.character.GodVisit = ""
			}
			return forfeitErr
		}))
	return s, store, r, &forfeits
}

func TestGodVisitDecidesTheObservedScene(t *testing.T) {
	for visit, scene := range map[string]string{character.GodVisitHeaven: "god", character.GodVisitUnderworld: "u_god", "": "home"} {
		s, store, r, _ := godVisitFixture(t, visit, nil)
		got, err := s.Query(r.ctx, "hero", "owner")
		if err != nil || got.Navigation.Selection.Destination != scene || got.Snapshot.LocationID != scene || store.writes != 0 {
			t.Fatalf("visit %q: %+v %v", visit, got.Navigation, err)
		}
		if visit != "" && (!slices.Contains(got.AvailableActions, "god_wish") || slices.Contains(got.AvailableActions, "home_sleep")) {
			t.Fatalf("visit %q offers %v", visit, got.AvailableActions)
		}
	}
}

func TestLeavingTheGodSceneForfeitsTheVisit(t *testing.T) {
	leave := map[string]func(*Service, context.Context) error{
		"enter": func(s *Service, ctx context.Context) error {
			_, err := s.Enter(ctx, "owner", "hero", "home")
			return err
		},
		"back": func(s *Service, ctx context.Context) error { _, err := s.Back(ctx, "owner", "hero"); return err },
	}
	for name, act := range leave {
		t.Run(name, func(t *testing.T) {
			s, store, r, forfeits := godVisitFixture(t, character.GodVisitUnderworld, nil)
			if err := act(s, r.ctx); err != nil {
				t.Fatal(err)
			}
			if store.writes != 1 || !slices.Equal(*forfeits, []string{"hero"}) {
				t.Fatalf("writes=%d forfeits=%v", store.writes, *forfeits)
			}
			if got, _ := s.Query(r.ctx, "hero", "owner"); got.Navigation.Selection.Destination == "u_god" {
				t.Fatal("still in the god scene after leaving")
			}
		})
	}
	// Re-entering the same scene and moving without a visit write nothing.
	s, _, r, forfeits := godVisitFixture(t, character.GodVisitHeaven, nil)
	if _, err := s.Enter(r.ctx, "owner", "hero", "god"); err != nil || len(*forfeits) != 0 {
		t.Fatalf("same scene: %v %v", err, *forfeits)
	}
	s, _, r, forfeits = godVisitFixture(t, "", nil)
	if _, err := s.Enter(r.ctx, "owner", "hero", "town"); err != nil || len(*forfeits) != 0 {
		t.Fatalf("no visit: %v %v", err, *forfeits)
	}
}

func TestFailedForfeitKeepsTheGodScene(t *testing.T) {
	failure := errors.New("private forfeit failure")
	s, _, r, _ := godVisitFixture(t, character.GodVisitHeaven, failure)
	if _, err := s.Enter(r.ctx, "owner", "hero", "town"); !errors.Is(err, failure) {
		t.Fatalf("err = %v", err)
	}
	if got, err := s.Query(r.ctx, "hero", "owner"); err != nil || got.Navigation.Selection.Destination != "god" {
		t.Fatalf("visit lost: %+v %v", got.Navigation, err)
	}
}

func TestReenteringTheGodSceneKeepsTheSavedSelection(t *testing.T) {
	s, store, r, forfeits := godVisitFixture(t, character.GodVisitHeaven, nil)
	if _, err := s.Enter(r.ctx, "owner", "hero", "god"); err != nil {
		t.Fatal(err)
	}
	if store.selection.Destination != "home" || store.writes != 0 || len(*forfeits) != 0 {
		t.Fatalf("re-entry overwrote the return selection: %+v writes=%d forfeits=%v", store.selection, store.writes, *forfeits)
	}
	r.character.GodVisit = "" // a granted wish clears the visit
	if got, err := s.Query(r.ctx, "hero", "owner"); err != nil || got.Navigation.Selection.Destination != "home" || got.Navigation.Unavailable {
		t.Fatalf("after the wish: %+v %v", got.Navigation, err)
	}
}

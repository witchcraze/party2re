package playercontext

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
)

func TestNavigationQualificationAndSafeBack(t *testing.T) {
	r := &queryReaders{character: character.Character{ID: "hero", PlayerID: "owner", JobLevel: 6}, ctx: context.Background()}
	store := &navigationStore{}
	reads := 0
	s := NewService(r, r, r, WithNavigation(store,
		SceneDefinition{ID: "town"},
		SceneDefinition{ID: "qualified", Parent: "town", Pageable: true, SubjectKind: "item",
			CanEnter:         func(c character.Character) bool { return c.JobLevel >= 7 },
			SubjectAvailable: func(context.Context, string, string) (bool, error) { reads++; return true, nil }},
	))
	if _, err := s.Enter(r.ctx, "owner", "hero", "qualified"); !errors.Is(err, ErrSceneAccessDenied) || store.writes != 0 {
		t.Fatalf("unqualified entry: %v writes=%d", err, store.writes)
	}
	r.character.JobLevel = 7
	if _, err := s.Enter(r.ctx, "owner", "hero", "qualified"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Select(r.ctx, "owner", "hero", Subject{Kind: "item", ID: "product"}); err != nil {
		t.Fatal(err)
	}
	r.character.JobLevel = 6
	before, beforeReads := store.selection, reads
	got, err := s.Query(r.ctx, "hero", "owner")
	if err != nil || !got.Navigation.Unavailable || store.selection != before || reads != beforeReads {
		t.Fatalf("qualification loss: %+v %v", got, err)
	}
	for _, transition := range []func() (Selection, error){
		func() (Selection, error) {
			return s.Select(r.ctx, "owner", "hero", Subject{Kind: "item", ID: "product"})
		},
		func() (Selection, error) {
			return s.Page(r.ctx, "owner", "hero", PageParams{Destination: "qualified", Limit: 1})
		},
	} {
		if _, err := transition(); !errors.Is(err, ErrSceneAccessDenied) || store.selection != before || reads != beforeReads {
			t.Fatalf("qualification bypass: %v", err)
		}
	}
	for range 2 {
		if _, err := s.Back(r.ctx, "owner", "hero"); err != nil {
			t.Fatal(err)
		}
	}
	if store.selection.Destination != "town" || store.writes != 4 {
		t.Fatalf("safe back: %+v writes=%d", store.selection, store.writes)
	}
}

func TestParentOnlySceneIsEnteredOnlyFromItsParent(t *testing.T) {
	r := &queryReaders{character: character.Character{ID: "hero", PlayerID: "owner", JobLevel: 15}, ctx: context.Background()}
	store := &navigationStore{}
	s := NewService(r, r, r, WithNavigation(store, SceneDefinition{ID: "town"}, SceneDefinition{ID: "oracle", Parent: "town"},
		SceneDefinition{ID: "hidden", Parent: "oracle", ParentOnly: true, CanEnter: func(c character.Character) bool { return c.JobLevel >= 15 }}))
	if _, err := s.Enter(r.ctx, "owner", "hero", "hidden"); !errors.Is(err, ErrSceneAccessDenied) || store.writes != 0 {
		t.Fatalf("entry from town: %v writes=%d", err, store.writes)
	}
	if _, err := s.Enter(r.ctx, "owner", "hero", "oracle"); err != nil {
		t.Fatal(err)
	}
	r.character.JobLevel = 14
	if _, err := s.Enter(r.ctx, "owner", "hero", "hidden"); !errors.Is(err, ErrSceneAccessDenied) || store.selection.Destination != "oracle" {
		t.Fatalf("unqualified entry from the parent: %v %+v", err, store.selection)
	}
	r.character.JobLevel = 15
	if _, err := s.Enter(r.ctx, "owner", "hero", "hidden"); err != nil || store.selection.Destination != "hidden" {
		t.Fatalf("entry from the parent: %v %+v", err, store.selection)
	}
	if _, err := s.Back(r.ctx, "owner", "hero"); err != nil || store.selection.Destination != "oracle" {
		t.Fatalf("back: %v %+v", err, store.selection)
	}
}

func TestOwnedSubjectAdmitsOwnerActionsAndPropagatesReadFailure(t *testing.T) {
	r := &queryReaders{character: character.Character{ID: "hero", PlayerID: "owner", Money: 1}, ctx: context.Background()}
	store := &navigationStore{selection: Selection{Destination: "district", Subject: Subject{Kind: "store", ID: "mine"}}}
	var ownerErr error
	s := NewService(r, r, r, WithNavigation(store, SceneDefinition{ID: "town"},
		SceneDefinition{ID: "district", Parent: "town", SubjectKind: "store",
			SubjectAvailable: func(context.Context, string, string) (bool, error) { return true, nil },
			SubjectOwned:     func(_ context.Context, _, target string) (bool, error) { return target == "mine", ownerErr }}))
	s.AdmitSceneActions("district", nil, []string{"store_buy"}, []string{"store_rename"})
	for subject, want := range map[string]string{"mine": "store_rename", "peer": "store_buy"} {
		store.selection.Subject.ID = subject
		got, err := s.Query(r.ctx, "hero", "owner")
		if err != nil || !slices.Contains(got.AvailableActions, want) || slices.Contains(got.AvailableActions, map[string]string{"store_rename": "store_buy", "store_buy": "store_rename"}[want]) {
			t.Fatalf("%s: %v %v", subject, got.AvailableActions, err)
		}
	}
	ownerErr = errors.New("store read failed")
	if _, err := s.Query(r.ctx, "hero", "owner"); !errors.Is(err, ownerErr) {
		t.Fatalf("owner read failure was hidden: %v", err)
	}
}

func TestInheritedSubjectSceneViewsTheParentSubject(t *testing.T) {
	r := &queryReaders{character: character.Character{ID: "hero", PlayerID: "owner"}, ctx: context.Background()}
	store := &navigationStore{}
	available := func(_ context.Context, _, target string) (bool, error) {
		return target == "other" || target == "hero", nil
	}
	owned := func(_ context.Context, actor, target string) (bool, error) { return actor == target, nil }
	s := NewService(r, r, r, WithNavigation(store, SceneDefinition{ID: "town"},
		SceneDefinition{ID: "home", Parent: "town", SubjectKind: "home", SubjectAvailable: available, SubjectOwned: owned},
		SceneDefinition{ID: "record", Parent: "home", InheritsSubject: true}))
	visited := Subject{Kind: "home", ID: "other"}
	for _, step := range []func() (Selection, error){
		func() (Selection, error) { return s.Enter(r.ctx, "owner", "hero", "home") },
		func() (Selection, error) { return s.Select(r.ctx, "owner", "hero", visited) },
		func() (Selection, error) { return s.Enter(r.ctx, "owner", "hero", "record") },
	} {
		if _, err := step(); err != nil {
			t.Fatal(err)
		}
	}
	if store.selection != (Selection{Destination: "record", Subject: visited}) {
		t.Fatalf("entry lost the visited subject: %+v", store.selection)
	}
	got, err := s.Query(r.ctx, "hero", "owner")
	if err != nil || got.Navigation.Unavailable || got.Navigation.SubjectOwned || slices.Contains(s.sceneActions(got.Navigation), "scene_select") {
		t.Fatalf("observation: %+v %v", got.Navigation, err)
	}
	if _, err := s.Select(r.ctx, "owner", "hero", Subject{Kind: "home", ID: "hero"}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("inherited scene accepted its own selection: %v", err)
	}
	if _, err := s.Back(r.ctx, "owner", "hero"); err != nil || store.selection != (Selection{Destination: "home", Subject: visited}) {
		t.Fatalf("back left the visited home: %v %+v", err, store.selection)
	}
	// Entry from the unselected parent views the actor's own.
	if _, err := s.Back(r.ctx, "owner", "hero"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Enter(r.ctx, "owner", "hero", "record"); err != nil || store.selection != (Selection{Destination: "record"}) {
		t.Fatalf("unselected entry: %v %+v", err, store.selection)
	}
	got, err = s.Query(r.ctx, "hero", "owner")
	if err != nil || slices.Contains(s.sceneActions(got.Navigation), "scene_select") {
		t.Fatalf("unselected observation offered selection: %+v %v", got.Navigation, err)
	}
}

func TestInheritedSubjectRequiresASubjectParent(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("accepted an inherited scene under a parent without subjects")
		}
	}()
	r := &queryReaders{ctx: context.Background()}
	NewService(r, r, r, WithNavigation(&navigationStore{}, SceneDefinition{ID: "town"}, SceneDefinition{ID: "record", Parent: "town", InheritsSubject: true}))
}

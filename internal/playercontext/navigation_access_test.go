package playercontext

import (
	"context"
	"errors"
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

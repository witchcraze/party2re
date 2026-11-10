package playercontext

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
)

func TestInspectLocation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selection Selection
		active    bool
		want      PublicLocation
	}{
		{"absent selection", Selection{}, false, PublicLocation{Kind: "scene", SceneID: "town"}},
		{"facility", Selection{Destination: "bank"}, false, PublicLocation{Kind: "scene", SceneID: "bank"}},
		{"product is private", Selection{Destination: "shop", Subject: Subject{Kind: "item", ID: "secret-item"}}, false, PublicLocation{Kind: "scene", SceneID: "shop"}},
		{"quest before facility", Selection{Destination: "bank"}, true, PublicLocation{Kind: "quest"}},
		{"own home", Selection{Destination: "home"}, false, PublicLocation{Kind: "home", HomeOwnerCharacterID: "peer", HomeOwnerName: "相手"}},
		{"visited home", Selection{Destination: "home", Subject: Subject{Kind: "home", ID: "host"}}, false, PublicLocation{Kind: "home", HomeOwnerCharacterID: "host", HomeOwnerName: "家主"}},
		{"inbox", Selection{Destination: "home_inbox"}, false, PublicLocation{Kind: "home", HomeOwnerCharacterID: "peer", HomeOwnerName: "相手"}},
		{"outbox", Selection{Destination: "home_outbox"}, false, PublicLocation{Kind: "home", HomeOwnerCharacterID: "peer", HomeOwnerName: "相手"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			owner := character.Character{ID: "peer", PlayerID: "another-owner", Name: "相手"}
			if tc.selection.Subject.Kind == "home" {
				owner.ID, owner.Name = "host", "家主"
			}
			chars := &queryReaders{character: owner, ctx: ctx}
			nav := &navigationStore{selection: tc.selection}
			s := NewService(chars, nil, nil, WithNavigation(nav,
				SceneDefinition{ID: "town"}, SceneDefinition{ID: "bank", Parent: "town"},
				SceneDefinition{ID: "home", Parent: "town", SubjectKind: "home", SubjectAvailable: func(context.Context, string, string) (bool, error) {
					t.Fatal("public inspection validated home admission")
					return false, nil
				}}, SceneDefinition{ID: "home_inbox", Parent: "home"}, SceneDefinition{ID: "home_outbox", Parent: "home"},
				SceneDefinition{ID: "shop", Parent: "town", SubjectKind: "item", SubjectAvailable: func(context.Context, string, string) (bool, error) {
					t.Fatal("public inspection read selected product")
					return false, nil
				}}), WithPublicActivities(func(_ context.Context, id string) (bool, error) {
				if id != "peer" {
					t.Fatal("wrong activity target")
				}
				return tc.active, nil
			}))
			got, err := s.InspectLocation(ctx, "peer")
			if err != nil || got != tc.want || nav.writes != 0 || nav.selection != tc.selection {
				t.Fatalf("got=%+v err=%v writes=%d", got, err, nav.writes)
			}
			if tc.active && nav.reads != 0 {
				t.Fatal("activity consulted selection")
			}
			if tc.want.Kind != "home" && len(chars.calls) != 0 {
				t.Fatal("public inspection queried owned character context")
			}
		})
	}
}

func TestInspectLocationFailsRequiredReads(t *testing.T) {
	want := errors.New("source unavailable")
	for _, stage := range []string{"activity", "navigation", "owner", "unknown scene", "invalid subject"} {
		t.Run(stage, func(t *testing.T) {
			s, nav, chars, _ := navigationFixture(t)
			s.publicActivities = func(context.Context, string) (bool, error) {
				if stage == "activity" {
					return false, want
				}
				return false, nil
			}
			s.scenes["home"] = SceneDefinition{ID: "home", Parent: "town"}
			switch stage {
			case "navigation":
				nav.loadErr = want
			case "owner":
				nav.selection = Selection{Destination: "home"}
				chars.errAt, chars.err = "character", want
			case "unknown scene":
				nav.selection = Selection{Destination: "removed"}
			case "invalid subject":
				nav.selection = Selection{Destination: "home", Subject: Subject{Kind: "item", ID: "private"}}
			}
			got, err := s.InspectLocation(chars.ctx, "hero")
			wantErr := want
			if stage == "unknown scene" || stage == "invalid subject" {
				wantErr = ErrInvalidSelection
			}
			if !errors.Is(err, wantErr) || !reflect.DeepEqual(got, PublicLocation{}) || nav.writes != 0 {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}

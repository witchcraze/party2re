package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/secretshop"
)

func TestPlayerContextRegistersQualifiedSecretShop(t *testing.T) {
	chars := &wireMockCharRepo{char: corecharacter.Character{ID: "hero", JobLevel: 7}}
	quests := &secretShopFailingQuests{}
	helper := helperquest.NewService(quests, chars, nil, nil)
	catalog, err := secretshop.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	service, err := secretshop.NewService(chars, &secretShopUntouchedInventory{}, catalog,
		secretshop.WithHelperFilter(secretShopHelperAdapter{helper: helper}))
	if err != nil {
		t.Fatal(err)
	}
	pc := newPlayerContext(&coreServices{}, &socServices{}, &econServices{}, &cmbtServices{}, &miscServices{secretshop: service})
	scenes := pc.SceneDefinitions()
	i := slices.IndexFunc(scenes, func(d playercontext.SceneDefinition) bool { return d.ID == "secretshop" })
	if i < 0 {
		t.Fatal("production SecretShop scene is missing")
	}
	d := scenes[i]
	if d.Parent != "town" || d.SubjectKind != "item" || !d.Pageable || d.CursorPageable || d.CanEnter == nil || d.SubjectAvailable == nil {
		t.Fatalf("registration: %+v", d)
	}
	for _, level := range []int{6, 7} {
		chars.char.JobLevel = level
		if d.CanEnter(chars.char) != (level == 7) {
			t.Fatalf("qualification: %d", level)
		}
	}
	ctx := context.WithValue(context.Background(), struct{}{}, "selected SecretShop product")
	for _, target := range []string{"secret_item_herbal_root", "item-010", "unknown"} {
		available, err := d.SubjectAvailable(ctx, "hero", target)
		if err != nil || available != (target == "secret_item_herbal_root") || quests.ctx != ctx {
			t.Fatalf("subject %s: %t %v", target, available, err)
		}
	}
	quests.err = errors.New("required HelperQuest read failed")
	if available, err := d.SubjectAvailable(ctx, "hero", "secret_item_herbal_root"); available || !errors.Is(err, quests.err) {
		t.Fatalf("required read: %t %v", available, err)
	}
	// Qualification may disappear between the owned snapshot and the feature read.
	chars.char.JobLevel = 6
	if available, err := d.SubjectAvailable(ctx, "hero", "secret_item_herbal_root"); available || err != nil {
		t.Fatalf("qualification loss: %t %v", available, err)
	}
}

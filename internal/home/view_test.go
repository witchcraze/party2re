package home

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type viewRepository struct {
	Repository
	err          error
	fail         string
	privateReads int
}

func (r *viewRepository) GetUnreadLetterCount(context.Context, string) (int, error) {
	r.privateReads++
	if r.fail == "" || r.fail == "unread" {
		return 1, r.err
	}
	return 1, nil
}
func (r *viewRepository) ListDeliveryNotices(context.Context, string, bool) ([]DeliveryNotice, error) {
	r.privateReads++
	if r.fail == "" || r.fail == "notices" {
		return nil, r.err
	}
	return []DeliveryNotice{{Message: "private"}}, nil
}
func (r *viewRepository) ListCompanionPhrases(context.Context, string) ([]CompanionPhrase, error) {
	r.privateReads++
	if r.fail == "" || r.fail == "phrases" {
		return nil, r.err
	}
	return []CompanionPhrase{{Phrase: "private"}}, nil
}

// viewInventory and viewDepot serve the owner's items, failing the named source.
type viewInventory struct {
	InventoryManager
	fail *string
	err  error
}

func (v viewInventory) FindByCharacterID(context.Context, string) (coreinventory.Inventory, error) {
	if *v.fail == "inventory" {
		return coreinventory.Inventory{}, v.err
	}
	return coreinventory.Inventory{Items: []item.Instance{{ID: "held", DefinitionID: "unknown", Quantity: 1}}}, nil
}

type viewDepot struct {
	DepotManager
	fail *string
	err  error
}

func (v viewDepot) FindByCharacterID(context.Context, string) (depot.Depot, error) {
	if *v.fail == "depot" {
		return depot.Depot{}, v.err
	}
	return depot.Depot{Items: []item.Instance{{ID: "stored", DefinitionID: "unknown", Quantity: 1}}}, nil
}

func TestHomeViewPrivacyAndRequiredReads(t *testing.T) {
	chars := &mockCharReader{chars: map[string]corecharacter.Character{
		"owner":   {ID: "owner", PlayerID: "player", Name: "Owner", Money: 123456789},
		"visitor": {ID: "visitor", PlayerID: "other"},
	}}
	repo := &viewRepository{Repository: newMockHomeRepo(chars.chars), err: errors.New("private read failure")}
	svc, err := NewService(repo, chars, WithInventoryManager(viewInventory{fail: &repo.fail, err: repo.err}),
		WithDepotManager(viewDepot{fail: &repo.fail, err: repo.err}), WithItemCatalog(&mockCatalog{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []string{"", "visitor"} {
		player := ""
		if viewer != "" {
			player = "other"
		}
		view, err := svc.GetHomeView(context.Background(), "owner", viewer, player)
		if err != nil || view.IsOwner || repo.privateReads != 0 {
			t.Fatalf("public: %+v %v reads=%d", view, err, repo.privateReads)
		}
		raw, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"player_id", "money", "unread_letter_count", "phrases", "notices", "items"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("leaked %s: %s", forbidden, raw)
			}
		}
	}
	if _, err := svc.GetHomeView(context.Background(), "owner", "owner", "other"); !errors.Is(err, ErrForbidden) || repo.privateReads != 0 {
		t.Fatalf("spoof: %v", err)
	}
	for _, failure := range []string{"unread", "phrases", "notices", "inventory", "depot"} {
		repo.fail = failure
		if _, err := svc.GetHomeView(context.Background(), "owner", "owner", "player"); !errors.Is(err, repo.err) {
			t.Fatalf("required %s enrichment: %v", failure, err)
		}
	}
	repo.err = nil
	view, err := svc.GetHomeView(context.Background(), "owner", "owner", "player")
	repo.fail = "none"
	view, err = svc.GetHomeView(context.Background(), "owner", "owner", "player")
	if err != nil || !view.IsOwner || view.Private == nil || view.Private.UnreadLetterCount != 1 || len(view.Private.Phrases) != 1 || len(view.Private.Notices) != 1 ||
		len(view.Private.Items) != 2 || view.Private.Items[0].Source != "inventory" || view.Private.Items[1].Source != "depot" {
		t.Fatalf("owned: %+v %v", view, err)
	}
	if _, err := svc.GetHomeView(context.Background(), "missing", "", ""); !errors.Is(err, ErrCharacterNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if len(repo.Repository.(*mockHomeRepo).homes) != 0 {
		t.Fatal("observation created home")
	}
}

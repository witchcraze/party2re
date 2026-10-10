package depot

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

type saleCatalog struct {
	price  int
	failID string
}

func (c saleCatalog) FindByID(id string) (item.Definition, error) {
	if id == c.failID {
		return item.Definition{}, errTestBoom
	}
	return item.Definition{ID: id, Price: c.price}, nil
}

func TestSaleValuationFailuresPreserveAssets(t *testing.T) {
	for _, single := range []bool{true, false} {
		mode := "batch"
		if single {
			mode = "single"
		}
		for _, tc := range []struct {
			name     string
			catalog  ItemDefinitionProvider
			quantity int
			ids      []string
			want     error
		}{
			{"missing provider", nil, 1, nil, nil},
			{"catalog failure", saleCatalog{100, "second"}, 1, nil, errTestBoom},
			{"negative price", saleCatalog{-1, ""}, 1, nil, ErrInvalidAmount},
			{"zero quantity", saleCatalog{100, ""}, 0, nil, ErrInvalidQuantity},
			{"negative quantity", saleCatalog{100, ""}, -1, nil, ErrInvalidQuantity},
			{"product overflow", saleCatalog{math.MaxInt, ""}, 3, nil, economy.ErrGoldOverflow},
			{"missing target", saleCatalog{100, ""}, 1, []string{"first", "missing"}, ErrItemNotFound},
			{"duplicate target", saleCatalog{100, ""}, 1, []string{"first", "first"}, ErrItemNotFound},
			{"sum overflow", saleCatalog{math.MaxInt, ""}, 2, nil, economy.ErrGoldOverflow},
		} {
			if single && (tc.name == "duplicate target" || tc.name == "sum overflow") {
				continue
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				items := []item.Instance{{ID: "first", DefinitionID: "first", Quantity: 1}, {ID: "second", DefinitionID: "second", Quantity: tc.quantity}, {ID: "survivor", DefinitionID: "third", Quantity: 4}}
				before := append([]item.Instance(nil), items...)
				saves, credits := 0, 0
				repo := &stubDepotRepo{findFn: func(context.Context, string) (Depot, error) {
					return Depot{CharacterID: "char", Items: items}, nil
				}, saveFn: func(context.Context, Depot) error { saves++; return nil }}
				chars := &stubCharRepo{updateFn: func(context.Context, corecharacter.Character) error { credits++; return nil }}
				svc, err := NewService(repo, chars, &stubInvRepo{}, WithItemDefinitionProvider(tc.catalog))
				if err != nil {
					t.Fatal(err)
				}
				ids := tc.ids
				if ids == nil {
					ids = []string{"first", "second"}
				}
				var earned int
				if single {
					_, earned, err = svc.SellItem(context.Background(), "char", ids[len(ids)-1])
				} else {
					_, earned, err = svc.SellItems(context.Background(), "char", ids)
				}
				if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
					t.Fatalf("error=%v, want %v", err, tc.want)
				}
				if earned != 0 || saves != 0 || credits != 0 || !reflect.DeepEqual(items, before) {
					t.Fatalf("failed sale mutated assets: earned=%d saves=%d credits=%d items=%+v", earned, saves, credits, items)
				}
			})
		}
	}
}

func TestSaleExactBasePrice(t *testing.T) {
	for _, single := range []bool{true, false} {
		for _, tc := range []struct{ price, quantity, want int }{{0, 3, 0}, {1, 3, 0}, {101, 3, 150}, {math.MaxInt, 2, math.MaxInt - 1}} {
			repo, chars := newMemoryDepotRepo(), newMemoryCharRepo()
			chars.characters["char"] = corecharacter.Character{ID: "char", Money: 100}
			stored := item.Instance{ID: "sold", DefinitionID: "weapon", Quantity: tc.quantity, EnhancementLevel: 6}
			survivor := item.Instance{ID: "survivor", DefinitionID: "other", Quantity: 1}
			repo.depots["char"] = Depot{CharacterID: "char", Items: []item.Instance{stored, survivor}}
			svc, err := NewService(repo, chars, newMemoryInvRepo(), WithItemDefinitionProvider(saleCatalog{price: tc.price}))
			if err != nil {
				t.Fatal(err)
			}
			var dep Depot
			var earned int
			if single {
				dep, earned, err = svc.SellItem(context.Background(), "char", stored.ID)
			} else {
				dep, earned, err = svc.SellItems(context.Background(), "char", []string{stored.ID})
			}
			if err != nil || earned != tc.want || !reflect.DeepEqual(dep.Items, []item.Instance{survivor}) {
				t.Fatalf("single=%v price=%d: earned=%d dep=%+v err=%v", single, tc.price, earned, dep, err)
			}
			wantMoney := corecharacter.MaxMoney
			if tc.want < corecharacter.MaxMoney-100 {
				wantMoney = 100 + tc.want
			}
			if chars.characters["char"].Money != wantMoney {
				t.Fatalf("money=%d want=%d", chars.characters["char"].Money, wantMoney)
			}
		}
	}
}

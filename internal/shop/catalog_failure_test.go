package shop_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/shop"
)

type failingCatalog struct {
	item.DefinitionProvider
	err      error
	overflow bool
}

func (c failingCatalog) FindByID(id string) (item.Definition, error) {
	if id == "item-007" && c.err != nil {
		return item.Definition{}, c.err
	}
	def, err := c.DefinitionProvider.FindByID(id)
	if id == "item-007" && c.overflow {
		def.Price = math.MaxInt
	}
	return def, err
}

func TestGetCatalogFailsWithoutPartialDefinitionOrPriceProjection(t *testing.T) {
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []error{errors.New("catalog storage failed"), item.ErrDefinitionNotFound, shop.ErrPriceOverflow} {
		t.Run(want.Error(), func(t *testing.T) {
			provider := failingCatalog{DefinitionProvider: catalog, err: want}
			if want == shop.ErrPriceOverflow {
				provider.err, provider.overflow = nil, true
			}
			chars := newCharacterRepoStub()
			char := createTestCharacter(t, chars, "Hero", 0)
			svc, err := shop.NewService(chars, newInventoryRepoStub(), provider)
			if err != nil {
				t.Fatal(err)
			}
			got, err := svc.GetCatalog(context.Background(), shop.ShopTypeItem, char.ID)
			if !errors.Is(err, want) || got.Items != nil {
				t.Fatalf("partial catalog: %+v %v", got, err)
			}
		})
	}
}

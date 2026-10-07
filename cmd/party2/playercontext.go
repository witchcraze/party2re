package main

import (
	"context"

	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/shop"
)

func newPlayerContext(core *coreServices, soc *socServices, econ *econServices) *playercontext.Service {
	scenes := []playercontext.SceneDefinition{
		{ID: "town"}, {ID: "bank", Parent: "town"}, {ID: "home", Parent: "town"},
	}
	for _, kind := range []shop.ShopType{shop.ShopTypeWeapon, shop.ShopTypeArmor, shop.ShopTypeItem, shop.ShopTypeAccessory} {
		scenes = append(scenes, playercontext.SceneDefinition{
			ID: "shop_" + string(kind), Parent: "town", SubjectKind: "item", Pageable: true,
			SubjectAvailable: func(ctx context.Context, actorID, targetID string) (bool, error) {
				catalog, err := econ.shop.GetCatalog(ctx, kind, actorID)
				if err != nil {
					return false, err
				}
				for _, item := range catalog.Items {
					if item.ID == targetID {
						return true, nil
					}
				}
				return false, nil
			},
		})
	}
	return playercontext.NewService(core.charRepo, soc.schedRepo, soc.timer, playercontext.WithNavigation(core.navigation, scenes...))
}

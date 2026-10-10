package main

import (
	"context"
	"errors"
	"slices"

	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/secretshop"
	"github.com/witchcraze/party2re/internal/shop"
)

func newPlayerContext(core *coreServices, soc *socServices, econ *econServices, cmbt *cmbtServices, misc *miscServices) *playercontext.Service {
	scenes := []playercontext.SceneDefinition{
		{ID: "town", Pageable: true}, {ID: "bank", Parent: "town"},
		{ID: "secretshop", Parent: "town", SubjectKind: "item", Pageable: true, CanEnter: secretshop.CheckEligibility,
			SubjectAvailable: func(ctx context.Context, actorID, targetID string) (bool, error) {
				status, err := misc.secretshop.GetShopStatus(ctx, actorID)
				if errors.Is(err, secretshop.ErrAccessDenied) {
					return false, nil
				}
				if err != nil {
					return false, err
				}
				return slices.ContainsFunc(status.Items, func(p secretshop.Item) bool { return p.ID == targetID }), nil
			}},
		{ID: "home", Parent: "town", SubjectKind: "home", SubjectAvailable: func(ctx context.Context, _, target string) (bool, error) {
			_, err := soc.home.GetHomeView(ctx, target, "", "")
			if errors.Is(err, home.ErrCharacterNotFound) {
				return false, nil
			}
			return err == nil, err
		}},
		{ID: "home_inbox", Parent: "home", Pageable: true, CursorPageable: true},
		{ID: "home_outbox", Parent: "home", Pageable: true, CursorPageable: true},
		{ID: "casino", Parent: "town", SubjectKind: "room", Pageable: true, SubjectAvailable: func(ctx context.Context, _, target string) (bool, error) {
			rooms, err := misc.casino.ListRooms(ctx)
			if err != nil {
				return false, err
			}
			for _, room := range rooms {
				if room.ID == target {
					return true, nil
				}
			}
			return false, nil
		}},
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
	readers := contextActivityReaders{party: cmbt.party, pvp: cmbt.pvp, gvg: cmbt.gvg, dungeon: cmbt.dungeon, challenge: cmbt.challenge, casino: misc.casino}
	return playercontext.NewService(core.charRepo, soc.schedRepo, soc.timer, playercontext.WithNavigation(core.navigation, scenes...), playercontext.WithActivities(readers.read))
}

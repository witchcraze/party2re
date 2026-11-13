package main

import (
	"context"
	"errors"
	"slices"

	"github.com/witchcraze/party2re/internal/blackmarket"
	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/secretshop"
	"github.com/witchcraze/party2re/internal/shop"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/town"
)

func newPlayerContext(core *coreServices, soc *socServices, econ *econServices, cmbt *cmbtServices, misc *miscServices, options ...func(*playercontext.Service)) *playercontext.Service {
	scenes := []playercontext.SceneDefinition{
		{ID: "town", Pageable: true}, {ID: "alchemy", Parent: "town", Pageable: true},
		{ID: "altar", Parent: "town"}, {ID: "auction", Parent: "town"}, {ID: "bank", Parent: "town"},
		{ID: "blackmarket", Parent: "town", Pageable: true, CanEnter: blackmarket.CheckEligibility},
		{ID: "blacksmith", Parent: "town"},
		{ID: "chapel", Parent: "town"},
		{ID: "depot", Parent: "town", Pageable: true},
		{ID: "exile", Parent: "town"},
		{ID: "fleamarket", Parent: "town", Pageable: true},
		{ID: "gemstore", Parent: "town"}, {ID: "wishingwell", Parent: "town"}, {ID: "tavern", Parent: "town"}, {ID: "lottery", Parent: "town"}, {ID: "takarakuji", Parent: "town"},
		// Reached only by Home reins; the recorded visit, not navigation, admits.
		{ID: "god", Parent: "town", CanEnter: func(c character.Character) bool { return c.GodVisit == character.GodVisitHeaven }},
		{ID: "u_god", Parent: "town", CanEnter: func(c character.Character) bool { return c.GodVisit == character.GodVisitUnderworld }},
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
	for _, t := range town.AllTowns() {
		scenes = append(scenes, playercontext.SceneDefinition{
			ID: t.ID, Parent: "town", SubjectKind: "store",
			SubjectAvailable: func(ctx context.Context, _, target string) (bool, error) {
				details, err := econ.store.GetStore(ctx, target)
				if errors.Is(err, store.ErrStoreNotFound) || errors.Is(err, store.ErrStoreExpired) {
					return false, nil
				}
				if err != nil {
					return false, err
				}
				return details.Store.TownID == t.ID, nil
			},
		})
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
	readers := newContextActivityReaders(cmbt, misc)
	return playercontext.NewService(core.charRepo, soc.schedRepo, contextSleepReader{TimerReader: soc.timer, home: soc.home}, append([]func(*playercontext.Service){playercontext.WithNavigation(core.navigation, scenes...), playercontext.WithGodVisitForfeit(misc.god.ForfeitVisit), playercontext.WithActivities(readers.read), playercontext.WithPublicActivities(readers.publicActivity)}, options...)...)
}

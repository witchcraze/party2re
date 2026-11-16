package http

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/town"
)

// TownDistrictSceneData is one legacy town (town1..town4): its active houses
// and stores, with the ＠たてる (Town) and ＠みせ construction terms. Selecting a
// store observes StoreSceneData; a house names its owner's Home subject.
type TownDistrictSceneData struct {
	Parent     string           `json:"parent"`
	Town       town.Town        `json:"town"`
	Houses     []home.TownHouse `json:"houses"`
	Stores     []store.Store    `json:"stores"`
	StoreTerms StoreBuildTerms  `json:"store_terms"`
}

type StoreBuildTerms struct {
	Price       int      `json:"price"`
	CycleDays   int      `json:"cycle_days"`
	MaxStores   int      `json:"max_stores"`
	HouseStyles []string `json:"house_styles"`
}

// StoreSceneData is a selected store. The scene registry admits the owner or
// the visitor command set, as legacy store.cgi does.
type StoreSceneData struct {
	Parent  string             `json:"parent"`
	Details store.StoreDetails `json:"details"`
	IsOwner bool               `json:"is_owner"`
}

var (
	storeOwnerCommands   = []string{"store_list_gold", "store_list_barter", "store_withdraw", "store_rename", "store_wallpaper", "store_interior_add", "store_interior_rename", "store_interior_clean"}
	storeVisitorCommands = []string{"store_buy", "store_trade"}
)

func (h *Handler) townDistrictSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.stores == nil || h.homes == nil {
		return nil, errSceneNotConfigured
	}
	n := r.Navigation.Selection
	if n.Subject != (playercontext.Subject{}) {
		details, err := h.stores.GetStore(ctx, n.Subject.ID)
		if errors.Is(err, store.ErrStoreNotFound) || errors.Is(err, store.ErrStoreExpired) || (err == nil && details.Store.TownID != n.Destination) {
			return nil, playercontext.ErrSelectionNotFound
		}
		if err != nil {
			return nil, err
		}
		return StoreSceneData{Parent: n.Destination, Details: *details, IsOwner: details.Store.CharacterID == r.Snapshot.Character.ID}, nil
	}
	t, ok := town.GetTown(n.Destination)
	if !ok {
		return nil, errSceneNotConfigured
	}
	houses, err := h.homes.ListTownHouses(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	stores, err := h.stores.GetTownStores(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	return TownDistrictSceneData{Parent: "town", Town: t, Houses: append([]home.TownHouse{}, houses...), Stores: append([]store.Store{}, stores...),
		StoreTerms: StoreBuildTerms{Price: store.StorePrice, CycleDays: store.StoreCycleDays, MaxStores: store.MaxTownStores, HouseStyles: t.HouseStyles}}, nil
}

func townDistrictFacility(name string) facility {
	return facility{title: name, commands: []string{"house_build", "store_build"}, subjectCommands: storeVisitorCommands, ownedSubjectCommands: storeOwnerCommands,
		bind: func(n playercontext.Selection, _ string) map[string]map[string]any {
			return map[string]map[string]any{"house_build": {"town_id": n.Destination}, "store_build": {"town_id": n.Destination}}
		},
		present: func(_ *Handler, _ playercontext.Result, data any) scenePresentation {
			if data, ok := data.(StoreSceneData); ok {
				return scenePresentation{title: &data.Details.Store.StoreName}
			}
			return scenePresentation{}
		},
		read: (*Handler).townDistrictSceneData, placeLog: true}
}

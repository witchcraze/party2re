package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/store"
)

// StoreService is the Player Store surface used by the town scenes and commands.
type StoreService interface {
	BuildStore(ctx context.Context, characterID, townID, houseStyle string) (*store.StoreCheckResult, error)
	GetStore(ctx context.Context, storeID string) (*store.StoreDetails, error)
	GetTownStores(ctx context.Context, townID string) ([]store.Store, error)
	ListGoldItem(ctx context.Context, characterID, depotItemInstanceID string, price int) (*store.Sale, error)
	ListBarterItem(ctx context.Context, characterID, depotItemInstanceID, wishItemName string) (*store.Sale, error)
	WithdrawListing(ctx context.Context, characterID, saleID string) error
	BuyItem(ctx context.Context, buyerCharacterID, saleID string) error
	TradeItem(ctx context.Context, buyerCharacterID, saleID, customerDepotItemInstanceID string) error
	ChangeStoreName(ctx context.Context, characterID, newName string) error
	ChangeWallpaper(ctx context.Context, characterID, wallpaper string) error
	AddInterior(ctx context.Context, characterID, furnitureID string) (*store.Interior, error)
	RenameInterior(ctx context.Context, characterID, interiorID, newName string) error
	CleanInteriors(ctx context.Context, characterID string) error
}

type storeBuildRequest struct {
	TownID     string `json:"town_id"`
	HouseStyle string `json:"house_style"`
}

type storeListGoldRequest struct {
	ItemID string `json:"item_id"`
	Price  int    `json:"price"`
}

type storeListBarterRequest struct {
	ItemID       string `json:"item_id"`
	WishItemName string `json:"wish_item_name"`
}

type storeSaleRequest struct {
	SaleID string `json:"sale_id"`
}

type storeTradeRequest struct {
	SaleID string `json:"sale_id"`
	ItemID string `json:"item_id"`
}

type storeRenameRequest struct {
	StoreName string `json:"store_name"`
}

type storeWallpaperRequest struct {
	Wallpaper string `json:"wallpaper"`
}

type storeInteriorAddRequest struct {
	FurnitureID string `json:"furniture_id"`
}

type storeInteriorRenameRequest struct {
	InteriorID string `json:"interior_id"`
	Name       string `json:"name"`
}

type storeSaleResult struct {
	SaleID string `json:"sale_id"`
}

// WithStore configures the town store observations and the eleven store commands.
func WithStore(service StoreService) Option {
	return func(h *Handler) {
		h.stores = service
		if service == nil {
			withActionCommand[storeBuildRequest]("store_build", nil, nil)(h)
			withActionCommand[storeListGoldRequest]("store_list_gold", nil, nil)(h)
			withActionCommand[storeListBarterRequest]("store_list_barter", nil, nil)(h)
			withActionCommand[storeSaleRequest]("store_withdraw", nil, nil)(h)
			withActionCommand[storeSaleRequest]("store_buy", nil, nil)(h)
			withActionCommand[storeTradeRequest]("store_trade", nil, nil)(h)
			withActionCommand[storeRenameRequest]("store_rename", nil, nil)(h)
			withActionCommand[storeWallpaperRequest]("store_wallpaper", nil, nil)(h)
			withActionCommand[storeInteriorAddRequest]("store_interior_add", nil, nil)(h)
			withActionCommand[storeInteriorRenameRequest]("store_interior_rename", nil, nil)(h)
			withActionCommand[struct{}]("store_interior_clean", nil, nil)(h)
			return
		}
		withActionCommand("store_build", func(ctx context.Context, actor string, p storeBuildRequest) (any, error) {
			return service.BuildStore(ctx, actor, p.TownID, p.HouseStyle)
		}, storeActionRejection)(h)
		withActionCommand("store_list_gold", func(ctx context.Context, actor string, p storeListGoldRequest) (any, error) {
			return service.ListGoldItem(ctx, actor, p.ItemID, p.Price)
		}, storeActionRejection)(h)
		withActionCommand("store_list_barter", func(ctx context.Context, actor string, p storeListBarterRequest) (any, error) {
			return service.ListBarterItem(ctx, actor, p.ItemID, p.WishItemName)
		}, storeActionRejection)(h)
		withActionCommand("store_withdraw", func(ctx context.Context, actor string, p storeSaleRequest) (any, error) {
			return storeSaleResult{p.SaleID}, service.WithdrawListing(ctx, actor, p.SaleID)
		}, storeActionRejection)(h)
		withActionCommand("store_buy", func(ctx context.Context, actor string, p storeSaleRequest) (any, error) {
			return storeSaleResult{p.SaleID}, service.BuyItem(ctx, actor, p.SaleID)
		}, storeActionRejection)(h)
		withActionCommand("store_trade", func(ctx context.Context, actor string, p storeTradeRequest) (any, error) {
			return storeSaleResult{p.SaleID}, service.TradeItem(ctx, actor, p.SaleID, p.ItemID)
		}, storeActionRejection)(h)
		withActionCommand("store_rename", func(ctx context.Context, actor string, p storeRenameRequest) (any, error) {
			return p, service.ChangeStoreName(ctx, actor, p.StoreName)
		}, storeActionRejection)(h)
		withActionCommand("store_wallpaper", func(ctx context.Context, actor string, p storeWallpaperRequest) (any, error) {
			return p, service.ChangeWallpaper(ctx, actor, p.Wallpaper)
		}, storeActionRejection)(h)
		withActionCommand("store_interior_add", func(ctx context.Context, actor string, p storeInteriorAddRequest) (any, error) {
			return service.AddInterior(ctx, actor, p.FurnitureID)
		}, storeActionRejection)(h)
		withActionCommand("store_interior_rename", func(ctx context.Context, actor string, p storeInteriorRenameRequest) (any, error) {
			return p, service.RenameInterior(ctx, actor, p.InteriorID, p.Name)
		}, storeActionRejection)(h)
		withActionCommand("store_interior_clean", func(ctx context.Context, actor string, _ struct{}) (any, error) {
			return struct{}{}, service.CleanInteriors(ctx, actor)
		}, storeActionRejection)(h)
	}
}

func storeActionRejection(err error) (int, ErrorDetail) {
	for _, r := range storeRejections {
		if errors.Is(err, r.err) {
			return r.status, ErrorDetail{Code: r.code, Message: r.message}
		}
	}
	if errors.Is(err, corecharacter.ErrNotFound) {
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	}
	return 0, ErrorDetail{}
}

var storeRejections = []struct {
	err           error
	status        int
	code, message string
}{
	{store.ErrCharacterNotFound, http.StatusNotFound, "CHARACTER_NOT_FOUND", "Character not found."},
	{store.ErrStoreNotFound, http.StatusNotFound, "STORE_NOT_FOUND", "You do not own a store."},
	{store.ErrStoreExpired, http.StatusConflict, "STORE_EXPIRED", "The store's ownership period has ended."},
	{store.ErrListingNotFound, http.StatusNotFound, "STORE_LISTING_NOT_FOUND", "Listing not found."},
	{store.ErrInteriorNotFound, http.StatusNotFound, "STORE_INTERIOR_NOT_FOUND", "Interior not found."},
	{store.ErrItemNotFound, http.StatusBadRequest, "STORE_ITEM_NOT_FOUND", "Item not found in your depot."},
	{store.ErrTradeItemMissing, http.StatusBadRequest, "STORE_TRADE_ITEM_MISSING", "You do not have the item to exchange."},
	{store.ErrInvalidTownID, http.StatusBadRequest, "STORE_INVALID_TOWN", "Invalid town."},
	{store.ErrInvalidHouseStyle, http.StatusBadRequest, "STORE_INVALID_HOUSE_STYLE", "This house style cannot be built in this town."},
	{store.ErrInvalidPrice, http.StatusBadRequest, "STORE_INVALID_PRICE", "Price must be between 1 and 999999 gold."},
	{store.ErrInvalidStoreName, http.StatusBadRequest, "STORE_INVALID_NAME", "Invalid store name."},
	{store.ErrInvalidInteriorName, http.StatusBadRequest, "STORE_INVALID_INTERIOR_NAME", "Invalid interior name."},
	{store.ErrInvalidWallpaper, http.StatusBadRequest, "STORE_INVALID_WALLPAPER", "Invalid wallpaper."},
	{store.ErrInvalidInterior, http.StatusBadRequest, "STORE_INVALID_INTERIOR", "Invalid interior."},
	{store.ErrInsufficientFunds, http.StatusBadRequest, "STORE_INSUFFICIENT_GOLD", "Insufficient gold."},
	{store.ErrCannotBuyOwnItem, http.StatusBadRequest, "STORE_OWN_LISTING", "You cannot buy from your own store."},
	{store.ErrCannotTradeOwnItem, http.StatusBadRequest, "STORE_OWN_LISTING", "You cannot exchange with your own store."},
	{store.ErrAlreadyOwnsStore, http.StatusConflict, "STORE_ALREADY_OWNED", "You already own a store."},
	{store.ErrTownMaxStoresReached, http.StatusConflict, "STORE_TOWN_FULL", "This town has reached its store limit."},
	{store.ErrStoreNameTaken, http.StatusConflict, "STORE_NAME_TAKEN", "A store with that name already exists."},
	{store.ErrMaxListingsReached, http.StatusConflict, "STORE_MAX_LISTINGS", "Your listing limit has been reached."},
	{store.ErrMaxInteriorsReached, http.StatusConflict, "STORE_MAX_INTERIORS", "Your interior limit has been reached."},
}

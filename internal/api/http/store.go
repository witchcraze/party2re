package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/store"
)

// StoreService defines player store operations exposed over HTTP.
type StoreService interface {
	BuildStore(ctx context.Context, characterID, townID, houseStyle, storeName string) (*store.StoreCheckResult, error)
	GetStore(ctx context.Context, storeID string) (*store.StoreDetails, error)
	GetTownStores(ctx context.Context, townID string) ([]store.Store, error)
	CheckStore(ctx context.Context, characterID string) (*store.StoreCheckResult, error)
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

func WithStore(stores StoreService) Option {
	return func(h *Handler) {
		h.stores = stores
	}
}

type buildStoreRequest struct {
	CharacterID string `json:"character_id"`
	HouseStyle  string `json:"house_style"`
	StoreName   string `json:"store_name"`
}

type listGoldItemRequest struct {
	DepotItemInstanceID string `json:"depot_item_instance_id"`
	Price               int    `json:"price"`
}

type listBarterItemRequest struct {
	DepotItemInstanceID string `json:"depot_item_instance_id"`
	WishItemName        string `json:"wish_item_name"`
}

type tradeItemRequest struct {
	DepotItemInstanceID string `json:"depot_item_instance_id"`
}

type changeStoreNameRequest struct {
	StoreName string `json:"store_name"`
}

type changeWallpaperRequest struct {
	Wallpaper string `json:"wallpaper"`
}

type addInteriorRequest struct {
	FurnitureID string `json:"furniture_id"`
}

type renameInteriorRequest struct {
	Name string `json:"name"`
}

func (h *Handler) handleBuildStore(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	townID := r.PathValue("town_id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *buildStoreRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req buildStoreRequest) {
		res, err := h.stores.BuildStore(r.Context(), char.ID, townID, req.HouseStyle, req.StoreName)
		if err != nil {
			if errors.Is(err, store.ErrInvalidTownID) || errors.Is(err, store.ErrInvalidHouseStyle) ||
				errors.Is(err, store.ErrInvalidStoreName) || errors.Is(err, store.ErrInsufficientFunds) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, store.ErrAlreadyOwnsStore) || errors.Is(err, store.ErrTownMaxStoresReached) ||
				errors.Is(err, store.ErrStoreNameTaken) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, res)
	})
}

func (h *Handler) handleListTownStores(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	townID := r.PathValue("town_id")
	stores, err := h.stores.GetTownStores(r.Context(), townID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, stores)
}

func (h *Handler) handleGetStore(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	storeID := r.PathValue("store_id")
	details, err := h.stores.GetStore(r.Context(), storeID)
	if err != nil {
		if errors.Is(err, store.ErrStoreNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, store.ErrStoreExpired) {
			writeError(w, http.StatusGone, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, details)
}

func (h *Handler) handleCheckStore(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		res, err := h.stores.CheckStore(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, store.ErrStoreNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, store.ErrStoreExpired) {
				writeError(w, http.StatusGone, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *Handler) handleListGoldItem(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(_ *listGoldItemRequest) string {
		return r.PathValue("id")
	}, func(_ coreplayer.Player, char corecharacter.Character, req listGoldItemRequest) {
		sale, err := h.stores.ListGoldItem(r.Context(), char.ID, req.DepotItemInstanceID, req.Price)
		if err != nil {
			if errors.Is(err, store.ErrInvalidPrice) || errors.Is(err, store.ErrItemNotFound) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, store.ErrStoreNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, store.ErrMaxListingsReached) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, sale)
	})
}

func (h *Handler) handleListBarterItem(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(_ *listBarterItemRequest) string {
		return r.PathValue("id")
	}, func(_ coreplayer.Player, char corecharacter.Character, req listBarterItemRequest) {
		sale, err := h.stores.ListBarterItem(r.Context(), char.ID, req.DepotItemInstanceID, req.WishItemName)
		if err != nil {
			if errors.Is(err, store.ErrItemNotFound) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, store.ErrStoreNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, store.ErrMaxListingsReached) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, sale)
	})
}

func (h *Handler) handleWithdrawListing(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	charID := r.PathValue("id")
	saleID := r.PathValue("sale_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		err := h.stores.WithdrawListing(r.Context(), char.ID, saleID)
		if err != nil {
			if errors.Is(err, store.ErrListingNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, store.ErrDepotFull) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (h *Handler) handleBuyStoreItem(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	charID := r.PathValue("id")
	saleID := r.PathValue("sale_id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		err := h.stores.BuyItem(r.Context(), char.ID, saleID)
		if err != nil {
			if errors.Is(err, store.ErrCannotBuyOwnItem) || errors.Is(err, store.ErrInsufficientFunds) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, store.ErrListingNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, store.ErrDepotFull) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "item purchased successfully"})
	})
}

func (h *Handler) handleTradeStoreItem(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	saleID := r.PathValue("sale_id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(_ *tradeItemRequest) string {
		return r.PathValue("id")
	}, func(_ coreplayer.Player, char corecharacter.Character, req tradeItemRequest) {
		err := h.stores.TradeItem(r.Context(), char.ID, saleID, req.DepotItemInstanceID)
		if err != nil {
			if errors.Is(err, store.ErrCannotTradeOwnItem) || errors.Is(err, store.ErrTradeItemMissing) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, store.ErrListingNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, store.ErrDepotFull) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "item traded successfully"})
	})
}

func (h *Handler) handleChangeStoreName(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(_ *changeStoreNameRequest) string {
		return r.PathValue("id")
	}, func(_ coreplayer.Player, char corecharacter.Character, req changeStoreNameRequest) {
		err := h.stores.ChangeStoreName(r.Context(), char.ID, req.StoreName)
		if err != nil {
			if errors.Is(err, store.ErrInvalidStoreName) || errors.Is(err, store.ErrInsufficientFunds) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, store.ErrStoreNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, store.ErrStoreNameTaken) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "store name updated successfully"})
	})
}

func (h *Handler) handleChangeWallpaper(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(_ *changeWallpaperRequest) string {
		return r.PathValue("id")
	}, func(_ coreplayer.Player, char corecharacter.Character, req changeWallpaperRequest) {
		err := h.stores.ChangeWallpaper(r.Context(), char.ID, req.Wallpaper)
		if err != nil {
			if errors.Is(err, store.ErrInvalidWallpaper) || errors.Is(err, store.ErrInsufficientFunds) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, store.ErrStoreNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "wallpaper updated successfully"})
	})
}

func (h *Handler) handleAddInterior(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(_ *addInteriorRequest) string {
		return r.PathValue("id")
	}, func(_ coreplayer.Player, char corecharacter.Character, req addInteriorRequest) {
		interior, err := h.stores.AddInterior(r.Context(), char.ID, req.FurnitureID)
		if err != nil {
			if errors.Is(err, store.ErrInvalidInterior) || errors.Is(err, store.ErrInsufficientFunds) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, store.ErrStoreNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			if errors.Is(err, store.ErrMaxInteriorsReached) {
				writeError(w, http.StatusConflict, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusCreated, interior)
	})
}

func (h *Handler) handleRenameInterior(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	interiorID := r.PathValue("interior_id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(_ *renameInteriorRequest) string {
		return r.PathValue("id")
	}, func(_ coreplayer.Player, char corecharacter.Character, req renameInteriorRequest) {
		err := h.stores.RenameInterior(r.Context(), char.ID, interiorID, req.Name)
		if err != nil {
			if errors.Is(err, store.ErrInvalidInteriorName) {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if errors.Is(err, store.ErrStoreNotFound) || errors.Is(err, store.ErrInteriorNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "interior renamed successfully"})
	})
}

func (h *Handler) handleCleanInteriors(w http.ResponseWriter, r *http.Request) {
	if h.stores == nil {
		writeError(w, http.StatusNotImplemented, errors.New("store service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		err := h.stores.CleanInteriors(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, store.ErrStoreNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

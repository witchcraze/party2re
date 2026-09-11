package store

import (
	"context"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type StoreBaseRepository interface {
	GetStoreByCharacterID(ctx context.Context, characterID string) (Store, error)
	GetStoreByID(ctx context.Context, storeID string) (Store, error)
	GetStoreByName(ctx context.Context, storeName string) (Store, error)
	CountActiveTownStores(ctx context.Context, townID string, now time.Time) (int, error)
	ListActiveStoresInTown(ctx context.Context, townID string, now time.Time) ([]Store, error)
	SaveStore(ctx context.Context, s Store) error
	DeleteStore(ctx context.Context, storeID string) error
}

type StoreSaleRepository interface {
	GetSalesByStoreID(ctx context.Context, storeID string) ([]Sale, error)
	GetSaleByIDForUpdate(ctx context.Context, saleID string) (Sale, error)
	SaveSale(ctx context.Context, sale Sale) error
	DeleteSale(ctx context.Context, saleID string) error
}

type StoreInteriorRepository interface {
	GetInteriorsByStoreID(ctx context.Context, storeID string) ([]Interior, error)
	SaveInterior(ctx context.Context, interior Interior) error
	DeleteInteriorsByStoreID(ctx context.Context, storeID string) error
	UpdateInteriorName(ctx context.Context, interiorID string, name string) error
}

type StoreRepository interface {
	StoreBaseRepository
	StoreSaleRepository
	StoreInteriorRepository
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Save(ctx context.Context, char corecharacter.Character) error
}

type DepotRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, dep depot.Depot) error
}

type ItemCatalog interface {
	FindByID(id string) (coreitem.Definition, error)
	FindByName(name string) (coreitem.Definition, error)
}

type GuildPointsRegistrar interface {
	AddGuildPoints(ctx context.Context, characterID string, points int) error
}

type TimerService interface {
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
}

type TxProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
